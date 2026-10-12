package vaultfs

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/promote"
)

// Vault is the adapter behind the ports promote owns for the pages: the one a promotion reads and writes
// through, the read half the page check and reconcile read through, and the question the wikilink rule asks.
var (
	_ promote.PageRepository = (*Vault)(nil)
	_ promote.PageReader     = (*Vault)(nil)
	_ promote.PageExistence  = (*Vault)(nil)
)

// pagesDir is where the pages of the vault at root live: written out as the literal a vault written by an
// earlier build holds, not derived from the layout under test.
func pagesDir(root string) string { return filepath.Join(root, "wiki", "memory") }

// writePageFile leaves content, at mode, as the file name in the pages directory.
func writePageFile(t *testing.T, root, name, content string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(pagesDir(root), name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
	return path
}

// symlinkOrSkip makes newname a symbolic link to oldname, and skips the test on a file system that cannot make
// one: a scenario about links is about a state the file system must be able to hold.
func symlinkOrSkip(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Skipf("this file system cannot make a symbolic link: %v", err)
	}
}

// A vault nothing was promoted in has no pages directory and so no pages: listing it answers nothing, not
// an error, and creates nothing.
func TestListPagesOfAFreshVaultIsEmptyAndCreatesNothing(t *testing.T) {
	root := t.TempDir()

	addresses, err := New(root).ListPages()
	if err != nil {
		t.Fatalf("ListPages: %v", err)
	}
	if len(addresses) != 0 {
		t.Fatalf("ListPages = %v, want no pages", addresses)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("listing created %d entries in the vault, want none", len(entries))
	}
}

// The pages are the files of the pages directory whose name ends in .md and that are not directories,
// named by their address, in the order of the directory. Anything else beside them is not a page: another
// extension, capitals in the extension, a directory with a page's name.
func TestListPagesReturnsTheAddressOfEveryMarkdownFileInDirectoryOrder(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"c-000003.md", "c-000001.md", "c-000002.md", "notes.txt", "C-000004.MD", "README"} {
		writePageFile(t, root, name, "x", 0o644)
	}
	if err := os.Mkdir(filepath.Join(pagesDir(root), "folder.md"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := New(root).ListPages()
	if err != nil {
		t.Fatalf("ListPages: %v", err)
	}
	want := []string{"c-000001", "c-000002", "c-000003"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListPages = %v, want %v", got, want)
	}
}

// A page that is a link is a page whatever it points at, as the directory lists it: a link to a directory
// is listed (reading it is what fails, and its caller says so for that page), and a link to nothing is
// listed too.
func TestListPagesListsLinksWhateverTheyPointAt(t *testing.T) {
	root := t.TempDir()
	writePageFile(t, root, "c-000001.md", "x", 0o644)
	symlinkOrSkip(t, "c-000001.md", filepath.Join(pagesDir(root), "c-000002.md"))
	symlinkOrSkip(t, "../folder", filepath.Join(pagesDir(root), "c-000003.md"))
	symlinkOrSkip(t, "../absent.md", filepath.Join(pagesDir(root), "c-000004.md"))

	got, err := New(root).ListPages()
	if err != nil {
		t.Fatalf("ListPages: %v", err)
	}
	want := []string{"c-000001", "c-000002", "c-000003", "c-000004"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListPages = %v, want %v", got, want)
	}
}

// The pages directory itself may be a link, which is followed, as a vault kept behind one is an ordinary
// setup; one that points at nothing is a vault without pages. A pages directory that is a file cannot be
// listed, and that is an error, the file system's own, which names the directory and is not the error of a
// missing one.
func TestListPagesOfAPagesDirectoryThatIsALinkAFileOrNothing(t *testing.T) {
	t.Run("a link to a directory is followed", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "notes", "pages"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "notes", "pages", "c-000001.md"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(pagesDir(root)), 0o755); err != nil {
			t.Fatal(err)
		}
		symlinkOrSkip(t, "../notes/pages", pagesDir(root))

		got, err := New(root).ListPages()
		if err != nil {
			t.Fatalf("ListPages: %v", err)
		}
		if want := []string{"c-000001"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("ListPages = %v, want %v", got, want)
		}
	})

	t.Run("a link to nothing is no pages", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Dir(pagesDir(root)), 0o755); err != nil {
			t.Fatal(err)
		}
		symlinkOrSkip(t, "../notes/absent", pagesDir(root))

		got, err := New(root).ListPages()
		if err != nil || len(got) != 0 {
			t.Fatalf("ListPages = %v, %v, want no pages and no error", got, err)
		}
	})

	t.Run("a file cannot be listed", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Dir(pagesDir(root)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(pagesDir(root), []byte("not a directory"), 0o644); err != nil {
			t.Fatal(err)
		}

		_, err := New(root).ListPages()
		if err == nil {
			t.Fatal("ListPages = nil error, want the listing to fail")
		}
		if errors.Is(err, fs.ErrNotExist) {
			t.Errorf("ListPages: %v is the error of a missing directory", err)
		}
		if !strings.Contains(err.Error(), pagesDir(root)) {
			t.Errorf("ListPages: %q does not name %s", err, pagesDir(root))
		}
	})
}

// The page is its whole file, byte for byte: the adapter reads it and adds nothing, not a newline, not a
// trimmed byte order mark.
func TestReadPageReturnsTheWholeFileByteForByte(t *testing.T) {
	root := t.TempDir()
	content := string(rune(0xFEFF)) + "---\naddress: c-000001\n---\nbody without a final newline"
	writePageFile(t, root, "c-000001.md", content, 0o644)

	got, err := New(root).ReadPage("c-000001")
	if err != nil {
		t.Fatalf("ReadPage: %v", err)
	}
	if got != content {
		t.Fatalf("ReadPage = %q, want %q", got, content)
	}
}

// A link to a page is read through, as the vault's tools do.
func TestReadPageFollowsALink(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "c-000001.md"), []byte("the real page"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(pagesDir(root), 0o755); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, "../../notes/c-000001.md", filepath.Join(pagesDir(root), "c-000001.md"))

	got, err := New(root).ReadPage("c-000001")
	if err != nil || got != "the real page" {
		t.Fatalf("ReadPage = %q, %v, want the page the link names", got, err)
	}
}

// A page that is not there is the file system's error for it, which the caller recognises with
// errors.Is(err, fs.ErrNotExist); one that is there and cannot be read (a directory, a link to one) is not.
// The failures are the file system's own: they name the file, and no consumer's name is put in front of
// them, because each caller words its failure around the cause.
func TestReadPageFailuresAreTheFileSystemsOwn(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(pagesDir(root), "c-000002.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, "../folder", filepath.Join(pagesDir(root), "c-000003.md"))
	vault := New(root)

	_, missing := vault.ReadPage("c-000001")
	if !errors.Is(missing, fs.ErrNotExist) {
		t.Errorf("ReadPage of a page that is not there = %v, want an error for which errors.Is(err, fs.ErrNotExist) holds", missing)
	}
	for _, address := range []string{"c-000002", "c-000003"} {
		_, err := vault.ReadPage(address)
		if err == nil {
			t.Fatalf("ReadPage(%s) = nil error, want the read to fail", address)
		}
		if errors.Is(err, fs.ErrNotExist) && address == "c-000002" {
			t.Errorf("ReadPage(%s): %v is the error of a missing page", address, err)
		}
		if strings.HasPrefix(err.Error(), "promote:") {
			t.Errorf("ReadPage(%s): %q carries a consumer's name, want the file system's error untouched", address, err)
		}
		if !strings.Contains(err.Error(), filepath.Join(pagesDir(root), address+".md")) {
			t.Errorf("ReadPage(%s): %q does not name the page", address, err)
		}
	}
}

// PageExists tells three things apart: there (a file, a directory, a link to either), not there (no file, a
// link to nothing), and not to be known (a parent that is not a directory).
func TestPageExists(t *testing.T) {
	root := t.TempDir()
	writePageFile(t, root, "c-000001.md", "x", 0o644)
	if err := os.Mkdir(filepath.Join(pagesDir(root), "c-000002.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, "c-000001.md", filepath.Join(pagesDir(root), "c-000003.md"))
	symlinkOrSkip(t, "../absent.md", filepath.Join(pagesDir(root), "c-000004.md"))
	vault := New(root)

	for _, c := range []struct {
		address string
		want    bool
	}{
		{"c-000001", true},
		{"c-000002", true},
		{"c-000003", true},
		{"c-000004", false},
		{"c-000005", false},
	} {
		got, err := vault.PageExists(c.address)
		if err != nil || got != c.want {
			t.Errorf("PageExists(%s) = %v, %v, want %v and no error", c.address, got, err, c.want)
		}
	}

	t.Run("a pages directory that is a file cannot say", func(t *testing.T) {
		other := t.TempDir()
		if err := os.MkdirAll(filepath.Join(other, "wiki"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(pagesDir(other), []byte("not a directory"), 0o644); err != nil {
			t.Fatal(err)
		}

		exists, err := New(other).PageExists("c-000001")
		if err == nil || exists {
			t.Fatalf("PageExists = %v, %v, want no answer and the error", exists, err)
		}
		if errors.Is(err, fs.ErrNotExist) || strings.HasPrefix(err.Error(), "promote:") {
			t.Errorf("PageExists: %q, want the file system's error for a parent that is not a directory, untouched", err)
		}
	})
}

// A page the module creates is owner-only, in a pages directory it makes when the vault has none yet, and
// holds exactly the content it was given.
func TestWritePageCreatesThePageOwnerOnlyAndTheDirectoryItNeeds(t *testing.T) {
	root := t.TempDir()
	content := "---\naddress: c-000001\n---\nbody"

	if err := New(root).WritePage("c-000001", content); err != nil {
		t.Fatalf("WritePage: %v", err)
	}

	path := filepath.Join(pagesDir(root), "c-000001.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != content {
		t.Errorf("the page holds %q, want %q", data, content)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("the page was created %v, want owner-only 0600", got)
	}
	dir, err := os.Stat(pagesDir(root))
	if err != nil {
		t.Fatal(err)
	}
	if got := dir.Mode().Perm(); got != 0o755 {
		t.Errorf("the pages directory was created %v, want 0755", got)
	}
}

// A page that is there is replaced whole and keeps the mode its owner gave it; nothing is left beside it.
func TestWritePageReplacesAPageKeepingItsModeAndLeavesNothingBeside(t *testing.T) {
	root := t.TempDir()
	path := writePageFile(t, root, "c-000001.md", "the old page, which is longer than the new one", 0o644)

	if err := New(root).WritePage("c-000001", "new"); err != nil {
		t.Fatalf("WritePage: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Errorf("the page holds %q, want it replaced whole by \"new\"", data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Errorf("the page is %v after the replacement, want the 0644 it had", got)
	}
	entries, err := os.ReadDir(pagesDir(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("the pages directory holds %d entries after the write, want only the page", len(entries))
	}
}

// A page that is a link is written through: the file it names changes, and the link stays a link.
func TestWritePageWritesThroughALink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "notes", "c-000001.md")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("old"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(pagesDir(root), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(pagesDir(root), "c-000001.md")
	symlinkOrSkip(t, "../../notes/c-000001.md", link)

	if err := New(root).WritePage("c-000001", "new"); err != nil {
		t.Fatalf("WritePage: %v", err)
	}

	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("the page is no longer a link: the write replaced the link and left the file it names")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Errorf("the file the link names holds %q, want \"new\"", data)
	}
	if mode := mustStat(t, target).Mode().Perm(); mode != 0o640 {
		t.Errorf("the file the link names is %v, want the 0640 it had", mode)
	}
}

func mustStat(t *testing.T, path string) os.FileInfo {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info
}

// A write that cannot land says what failed and leaves the pages
// as they were. A dangling link where the pages directory belongs cannot be made a directory, whoever runs
// the test.
func TestWritePageFailureNamesWhatFailedAndWritesNothing(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(pagesDir(root)), 0o755); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, "../notes/absent", pagesDir(root))

	err := New(root).WritePage("c-000001", "x")
	if err == nil {
		t.Fatal("WritePage = nil error, want the write to fail")
	}
	want := "create directory for " + filepath.Join(pagesDir(root), "c-000001.md") + ": "
	if !strings.HasPrefix(err.Error(), want) {
		t.Errorf("WritePage: %q, want it to begin %q", err, want)
	}
	if _, statErr := os.Stat(filepath.Join(root, "notes")); statErr == nil {
		t.Error("the write created the directory the dangling link names")
	}
	if strings.HasPrefix(err.Error(), "promote:") {
		t.Errorf("WritePage: %q begins with a consumer's name", err)
	}
}

// The locations are the paths an operator would open, in the platform's spelling, and touch no file.
func TestLocationsNameTheFilesAnOperatorWouldOpen(t *testing.T) {
	root := t.TempDir()
	vault := New(root)

	if got, want := vault.PageLocation("c-000042"), filepath.Join(root, "wiki", "memory", "c-000042.md"); got != want {
		t.Errorf("PageLocation = %q, want %q", got, want)
	}
	if got, want := vault.PagesLocation(), filepath.Join(root, "wiki", "memory"); got != want {
		t.Errorf("PagesLocation = %q, want %q", got, want)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("naming the locations created %d entries, want none", len(entries))
	}
}
