package sourcefs

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/ingest"
)

// sameDirectory reports whether a and b name one directory (the temporary directory of a test may be reached
// through a link, so the names alone are not compared).
func sameDirectory(t *testing.T, a, b string) bool {
	t.Helper()
	infoA, err := os.Stat(a)
	if err != nil {
		t.Fatal(err)
	}
	infoB, err := os.Stat(b)
	if err != nil {
		t.Fatal(err)
	}
	return os.SameFile(infoA, infoB)
}

func TestReaderAnswersTheAbsoluteDirectoryTheProcessWorksIn(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	got, err := Reader{}.WorkingDirectory()
	if err != nil {
		t.Fatalf("WorkingDirectory: %v", err)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("WorkingDirectory = %q, want an absolute path", got)
	}
	if !sameDirectory(t, got, dir) {
		t.Errorf("WorkingDirectory = %q, want the directory the process changed to, %q", got, dir)
	}
}

// The directory is asked at every call, not remembered: a process that changes directory is in another one.
func TestReaderAnswersTheDirectoryOfTheMomentItIsAsked(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()

	t.Chdir(first)
	gotFirst, err := Reader{}.WorkingDirectory()
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(second)
	gotSecond, err := Reader{}.WorkingDirectory()
	if err != nil {
		t.Fatal(err)
	}

	if !sameDirectory(t, gotFirst, first) || !sameDirectory(t, gotSecond, second) {
		t.Errorf("answers %q and %q, want the directories %q and %q", gotFirst, gotSecond, first, second)
	}
}

func TestReaderFailsWhenTheDirectoryTheProcessWorksInIsGone(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}

	if got, err := (Reader{}).WorkingDirectory(); err == nil {
		t.Errorf("WorkingDirectory = %q with no error, want the failure of the process to tell where it is", got)
	}
}

// These are the cases the ingest package pinned while it asked the process itself (the id of a relative path is
// the id of the absolute path it makes from the working directory, which the process can lose), now held through
// the adapter on the real machine: the rule is ingest's and is tested there over a reader that answers a
// directory; this proves that the adapter answers it with the directory the cases are in.
func TestAResolvedRelativeOriginIsIdentifiedByItsPathFromTheWorkingDirectory(t *testing.T) {
	t.Chdir(t.TempDir())
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"notes/Guide v2.md", "./a.md", "../sibling/x.md", "repo://openspec/specs/x.md"} {
		for _, kind := range []ingest.SourceKind{ingest.KindFile, ingest.KindDirectory} {
			resolved, err := ingest.ResolveOrigin(Reader{}, ingest.Origin{Kind: kind, URI: rel})
			if err != nil {
				t.Fatalf("ResolveOrigin(%s %q): %v", kind, rel, err)
			}
			got, err := ingest.SourceID(resolved)
			if err != nil {
				t.Fatalf("SourceID(%s %q): %v", kind, resolved.URI, err)
			}
			want, err := ingest.SourceID(ingest.Origin{Kind: kind, URI: filepath.Join(cwd, rel)})
			if err != nil {
				t.Fatalf("SourceID(%s %q): %v", kind, filepath.Join(cwd, rel), err)
			}
			if got != want {
				t.Errorf("id of %s %q = %q, want the id of %q, %q", kind, rel, got, filepath.Join(cwd, rel), want)
			}
		}
	}
}

func TestAResolvedRelativeOriginChangesWithTheDirectoryTheProcessIsIn(t *testing.T) {
	origin := ingest.Origin{Kind: ingest.KindFile, URI: "notes/a.md"}
	idIn := func(dir string) string {
		t.Helper()
		t.Chdir(dir)
		resolved, err := ingest.ResolveOrigin(Reader{}, origin)
		if err != nil {
			t.Fatal(err)
		}
		id, err := ingest.SourceID(resolved)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}

	first, second := idIn(t.TempDir()), idIn(t.TempDir())

	if first == second {
		t.Errorf("the same relative path got the same id %q in two different directories", first)
	}
}

// An override is the identity the caller chose, so the same override has one id in every directory the process
// works in, while the relative path beside it resolves to a different file in each.
func TestAnOverrideHasOneIDInEveryWorkingDirectory(t *testing.T) {
	origin := ingest.Origin{Kind: ingest.KindFile, URI: "notes/x.md", Override: "repo://openspec/specs/x.md"}
	idIn := func(dir string) string {
		t.Helper()
		t.Chdir(dir)
		resolved, err := ingest.ResolveOrigin(Reader{}, origin)
		if err != nil {
			t.Fatal(err)
		}
		id, err := ingest.SourceID(resolved)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}

	first, second := idIn(t.TempDir()), idIn(t.TempDir())

	if first != second || first != "x-8bfa69fe" {
		t.Errorf("the override's id is %q and %q in two directories, want %q in both", first, second, "x-8bfa69fe")
	}
}

func TestAnAbsoluteOriginNeedsNoWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	origin := ingest.Origin{Kind: ingest.KindFile, URI: "/srv/notes/Guide v2.md"}

	if _, err := ingest.ResolveOrigin(Reader{}, ingest.Origin{Kind: ingest.KindFile, URI: "notes/a.md"}); err == nil {
		t.Error("a relative path was resolved although the working directory is gone")
	}
	resolved, err := ingest.ResolveOrigin(Reader{}, origin)
	if err != nil {
		t.Fatalf("an absolute path needs no working directory, got %v", err)
	}
	if id, err := ingest.SourceID(resolved); err != nil || id != "guide-v2-191bacc1" {
		t.Errorf("SourceID = %q, %v, want %q", id, err, "guide-v2-191bacc1")
	}
}

// Lstat tells a file, a directory, a link and anything else apart, and reports the size of a file only. A link is
// a link, whatever it points at: one to a directory is not a directory, and one whose target is gone (so following
// it would fail) still answers.
func TestLstatTellsKindsApartWithoutFollowingALink(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	write(t, filepath.Join(dir, "a.md"), "hello")
	write(t, filepath.Join(outside, "secret.md"), "outside the directory")
	mkdir(t, filepath.Join(dir, "notes"))
	symlink(t, filepath.Join(outside, "secret.md"), filepath.Join(dir, "to-a-file"))
	symlink(t, filepath.Join(dir, "notes"), filepath.Join(dir, "to-a-directory"))
	symlink(t, filepath.Join(outside, "gone.md"), filepath.Join(dir, "dangling"))

	cases := []struct {
		name string
		want ingest.Entry
	}{
		{"a.md", ingest.Entry{Kind: ingest.EntryFile, Size: 5}},
		{"notes", ingest.Entry{Kind: ingest.EntryDirectory}},
		{"to-a-file", ingest.Entry{Kind: ingest.EntrySymlink}},
		{"to-a-directory", ingest.Entry{Kind: ingest.EntrySymlink}},
		{"dangling", ingest.Entry{Kind: ingest.EntrySymlink}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Reader{}.Lstat(filepath.Join(dir, c.name))
			if err != nil {
				t.Fatalf("Lstat: %v", err)
			}
			if got != c.want {
				t.Errorf("Lstat = %+v, want %+v", got, c.want)
			}
		})
	}
}

// What is not there fails, for every question, with an error that satisfies errors.Is(err, fs.ErrNotExist), so
// the use case tells a missing entry apart without reading the operating system's words.
func TestAnEntryThatIsNotThereFailsAsNotExisting(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	_, lstat := Reader{}.Lstat(missing)
	_, read := Reader{}.ReadFile(missing)
	_, list := Reader{}.ReadDir(missing)
	for name, err := range map[string]error{"Lstat": lstat, "ReadFile": read, "ReadDir": list} {
		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s of a missing entry: error %v does not satisfy errors.Is(err, fs.ErrNotExist)", name, err)
		}
	}
}

func TestReadFileReturnsTheBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.md")
	write(t, path, "line one\nline two\n")

	got, err := Reader{}.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(got, []byte("line one\nline two\n")) {
		t.Errorf("ReadFile = %q, want the file's bytes", got)
	}
}

// A link whose target the process may not reach (its directory has no permission bits) is still a link to Lstat:
// an adapter that looked at the target, with os.Stat or by opening it, would fail here. (One that looked and threw
// the answer away cannot be told apart from outside.)
func TestLstatDoesNotReachALinksTarget(t *testing.T) {
	dir, closed := t.TempDir(), t.TempDir()
	write(t, filepath.Join(closed, "target.md"), "out of reach")
	symlink(t, filepath.Join(closed, "target.md"), filepath.Join(dir, "link"))
	if err := os.Chmod(closed, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(closed, 0o700) })
	if _, err := os.Stat(filepath.Join(dir, "link")); err == nil {
		t.Skip("this process reaches a directory with no permission bits (root): the case cannot be shown")
	}

	got, err := Reader{}.Lstat(filepath.Join(dir, "link"))
	if err != nil || got != (ingest.Entry{Kind: ingest.EntrySymlink}) {
		t.Errorf("Lstat = %+v, %v, want a link and no error", got, err)
	}
}

// ReadFile reads a regular file and nothing else: a link (to a file or to a directory) is refused rather than
// followed, and so is a directory, so a caller that measured an entry with Lstat never reads something else.
func TestReadFileRefusesAnythingButAFile(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	write(t, filepath.Join(outside, "secret.md"), "outside the directory")
	mkdir(t, filepath.Join(dir, "notes"))
	symlink(t, filepath.Join(outside, "secret.md"), filepath.Join(dir, "to-a-file"))
	symlink(t, filepath.Join(dir, "notes"), filepath.Join(dir, "to-a-directory"))

	for _, name := range []string{"to-a-file", "to-a-directory", "notes"} {
		t.Run(name, func(t *testing.T) {
			got, err := Reader{}.ReadFile(filepath.Join(dir, name))
			if err == nil {
				t.Errorf("ReadFile = %q with no error, want a refusal", got)
			}
			if got != nil {
				t.Errorf("ReadFile returned %q with its refusal, want no bytes", got)
			}
		})
	}
}

func TestReadFileFailsOnAFileTheProcessMayNotRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "locked.md")
	write(t, path, "private")
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	if f, err := os.Open(path); err == nil {
		_ = f.Close()
		t.Skip("this process reads a file with no permission bits (root): the failure cannot be shown")
	}

	if got, err := (Reader{}).ReadFile(path); err == nil {
		t.Errorf("ReadFile = %q with no error, want the failure to read it", got)
	}
}

// ReadDir lists the names in a directory, not paths, without . and .., links included as names; it promises no
// order, so the case sorts.
func TestReadDirListsTheNames(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "b.md"), "b")
	write(t, filepath.Join(dir, "a.txt"), "a")
	mkdir(t, filepath.Join(dir, "sub"))
	write(t, filepath.Join(dir, "sub", "deeper.md"), "not listed")
	symlink(t, filepath.Join(dir, "a.txt"), filepath.Join(dir, "link"))

	got, err := Reader{}.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	slices.Sort(got)
	if want := []string{"a.txt", "b.md", "link", "sub"}; !slices.Equal(got, want) {
		t.Errorf("ReadDir = %q, want %q", got, want)
	}
}

// The operating system follows a link that a path names with a trailing slash or "/.", so a Path spelled that way
// must still be refused as a link (Phase 9 C22, D1), never scanned through to its target.
func TestExtractRefusesALinkNamedWithATrailingSlash(t *testing.T) {
	dir := t.TempDir()
	mkdir(t, filepath.Join(dir, "real"))
	write(t, filepath.Join(dir, "real", "secret.md"), "# Secret\n\nbehind the link\n")
	link := filepath.Join(dir, "link")
	symlink(t, filepath.Join(dir, "real"), link)

	for _, path := range []string{link + "/", link + "/."} {
		req := ingest.Request{
			Path:       path,
			Origin:     ingest.Origin{Kind: ingest.KindDirectory},
			Now:        func() time.Time { return time.Unix(0, 0).UTC() },
			IngestedBy: "tester",
		}
		got, err := ingest.Extractor{Reader: Reader{}}.Extract(req)
		if err == nil || !strings.Contains(err.Error(), "neither file nor directory") {
			t.Errorf("Extract(%q) = %d sources, %v, want the link refused as neither file nor directory", path, len(got.Sources), err)
		}
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
}

func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}
