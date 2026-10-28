package gitexec_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/gitexec"
)

// The copy a sweep runs the analyzers on is the exact bytes of the tree of a commit (own-rdd T6b4b, S6 b): built from
// the objects with `git ls-tree` and `git cat-file --batch`, never from an export, so no attribute, filter or
// end-of-line setting changes a byte of it.

// The attributes `git archive` honors, in the tree itself and in the repository, and the setting that converts the
// lines of a working tree, change nothing in the copy: each file is the blob git prints.
func TestArchiveIsTheTreeByteForByteWhateverTheAttributesAndTheLineEndingsSay(t *testing.T) {
	f := newFixture(t, "sha1")
	f.git("config", "core.autocrlf", "true")
	f.write(".gitattributes", "a.go export-ignore\nb.txt export-subst\nc.txt text eol=crlf\n")
	f.write("a.go", "package a\n")
	f.write("b.txt", "$Format:%H$\n")
	f.write("c.txt", "one\ntwo\r\nthree\n")
	f.write("run.sh", "#!/bin/sh\n")
	if err := os.Chmod(filepath.Join(f.root, "run.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("a.go", filepath.Join(f.root, "link.go")); err != nil {
		t.Fatal(err)
	}
	head := f.snapshotAll("feat: attributes")
	f.write(".git/info/attributes", "a.go export-ignore\n")
	dir := t.TempDir()

	err := f.adapter().Archive(f.repo(), head, dir)

	if err != nil {
		t.Fatalf("Archive error = %v", err)
	}
	for _, rel := range []string{".gitattributes", "a.go", "b.txt", "c.txt", "run.sh"} {
		want := f.git("cat-file", "blob", head+":"+rel)
		got, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil || string(got) != want {
			t.Errorf("%s = %q, %v, want the blob %q", rel, got, err, want)
		}
	}
	if info, err := os.Stat(filepath.Join(dir, "run.sh")); err != nil || info.Mode().Perm()&0o111 == 0 {
		t.Errorf("run.sh mode = %v, %v, want it executable", info, err)
	}
	if info, err := os.Lstat(filepath.Join(dir, "a.go")); err != nil || info.Mode().Perm()&0o111 != 0 {
		t.Errorf("a.go mode = %v, %v, want it not executable", info, err)
	}
	if target, err := os.Readlink(filepath.Join(dir, "link.go")); err != nil || target != "a.go" {
		t.Errorf("link.go = %q, %v, want a link to a.go", target, err)
	}
}

// listingRunner answers `git ls-tree` with a listing and `git cat-file --batch` with an answer, and records the
// standard input of the second.
type listingRunner struct {
	listing, answer []byte
	stdin           []byte
	batches         int
}

func (r *listingRunner) Output(ctx context.Context, env []string, bin string, args ...string) ([]byte, []byte, error) {
	if !slices.Contains(args, "ls-tree") {
		return nil, []byte("unexpected: " + strings.Join(args, " ")), fmt.Errorf("unexpected start of git")
	}
	return r.listing, nil, nil
}

func (r *listingRunner) OutputStdin(ctx context.Context, env []string, stdin []byte, bin string, args ...string) ([]byte, []byte, error) {
	if !slices.Contains(args, "cat-file") || !slices.Contains(args, "--batch") {
		return nil, []byte("unexpected: " + strings.Join(args, " ")), fmt.Errorf("unexpected start of git")
	}
	r.batches++
	r.stdin = stdin
	return r.answer, nil, nil
}

// object is the name of the n-th object of a test.
func object(n int) string { return fmt.Sprintf("%040x", n) }

// entry is one entry of the NUL-terminated listing `git ls-tree -r -z -l` prints.
func entry(mode, name string, size int, path string) string {
	kind, shown := "blob", fmt.Sprintf("%7d", size)
	if mode == "160000" {
		kind, shown = "commit", "      -"
	}
	return fmt.Sprintf("%s %s %s %s\t%s\x00", mode, kind, name, shown, path)
}

// blob is the answer `git cat-file --batch` gives for an object it has.
func blob(name, content string) string {
	return fmt.Sprintf("%s blob %d\n%s\n", name, len(content), content)
}

// archiveWorld is a directory to copy into, with a sibling the copy must never reach.
type archiveWorld struct {
	outer, dir, outside string
}

func newArchiveWorld(t *testing.T) archiveWorld {
	t.Helper()
	w := archiveWorld{outer: resolved(t, t.TempDir())}
	w.dir, w.outside = filepath.Join(w.outer, "copy"), filepath.Join(w.outer, "outside")
	for _, d := range []string{w.dir, w.outside} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return w
}

// untouched fails the test when anything but the two directories is in the outer directory or anything is in
// the one the copy must never reach.
func (w archiveWorld) untouched(t *testing.T) {
	t.Helper()
	for dir, want := range map[string]int{w.outer: 2, w.outside: 0} {
		if entries, err := os.ReadDir(dir); err != nil || len(entries) != want {
			t.Errorf("%s holds %v, %v, want %d entries: something was written outside the copy", dir, entries, err, want)
		}
	}
}

func TestArchiveRefusesAnEntryThatLeadsOutOfTheDirectoryAndWritesNothingOutside(t *testing.T) {
	for _, evil := range []string{"../evil.go", "/abs.go", "sub/../../x.go"} {
		t.Run(evil, func(t *testing.T) {
			w := newArchiveWorld(t)
			runner := &listingRunner{
				listing: []byte(entry("100644", object(1), 5, evil)),
				answer:  []byte(blob(object(1), "evil\n")),
			}

			err := gitexec.New(runner, gitexec.Options{}).Archive(repo, headSHA, w.dir)

			if err == nil || !strings.Contains(err.Error(), evil) {
				t.Errorf("Archive error = %v, want a refusal naming %q", err, evil)
			}
			w.untouched(t)
		})
	}
}

func TestArchiveRefusesAFileWhoseDirectoryIsALinkAndWritesNothingThroughIt(t *testing.T) {
	w := newArchiveWorld(t)
	runner := &listingRunner{
		listing: []byte(entry("120000", object(1), len(w.outside), "d") + entry("100644", object(2), 5, "d/f.go")),
		answer:  []byte(blob(object(1), w.outside) + blob(object(2), "evil\n")),
	}

	err := gitexec.New(runner, gitexec.Options{}).Archive(repo, headSHA, w.dir)

	if err == nil || !strings.Contains(err.Error(), "d/f.go") {
		t.Errorf("Archive error = %v, want a refusal naming d/f.go", err)
	}
	if entries, _ := os.ReadDir(w.outside); len(entries) != 0 {
		t.Errorf("the directory the link leads to holds %v, want nothing written through the link", entries)
	}
}

// The listing says what each object is; the answer of git must say the same, in the order asked.
func TestArchiveRefusesAnAnswerThatIsNotTheObjectSizeAndOrderTheListingGives(t *testing.T) {
	for _, tc := range []struct{ name, answer, says string }{
		{"an object git does not have", object(1) + " missing\n", object(1)},
		{"another object", blob(object(9), "hello\n"), object(9)},
		{"another size", fmt.Sprintf("%s blob 3\nabc\n", object(1)), "size"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newArchiveWorld(t)
			runner := &listingRunner{listing: []byte(entry("100644", object(1), 6, "a.go")), answer: []byte(tc.answer)}

			err := gitexec.New(runner, gitexec.Options{}).Archive(repo, headSHA, w.dir)

			if err == nil || !strings.Contains(err.Error(), tc.says) {
				t.Errorf("Archive error = %v, want a refusal that says %q", err, tc.says)
			}
			if entries, _ := os.ReadDir(w.dir); len(entries) != 0 {
				t.Errorf("the copy holds %v after a refusal, want nothing", entries)
			}
		})
	}
}

func TestArchiveRefusesAModeThatIsNotAFileALinkOrASubmodule(t *testing.T) {
	w := newArchiveWorld(t)
	runner := &listingRunner{
		listing: []byte(entry("100664", object(1), 6, "odd.go")),
		answer:  []byte(blob(object(1), "hello\n")),
	}

	err := gitexec.New(runner, gitexec.Options{}).Archive(repo, headSHA, w.dir)

	if err == nil || !strings.Contains(err.Error(), "100664") || !strings.Contains(err.Error(), "odd.go") {
		t.Errorf("Archive error = %v, want a refusal naming the mode 100664 and odd.go", err)
	}
	w.untouched(t)
}

// A submodule is a commit of another repository: it has no blob here, so it is not asked for and not copied. The
// names asked for are those of the blobs, one per line, in the order of the listing.
func TestArchiveSkipsASubmoduleAndAsksGitForTheBlobsInTheOrderOfTheListing(t *testing.T) {
	w := newArchiveWorld(t)
	runner := &listingRunner{
		listing: []byte(entry("100644", object(2), 2, "b.go") + entry("160000", object(7), 0, "mod") + entry("100755", object(1), 2, "a.sh")),
		answer:  []byte(blob(object(2), "b\n") + blob(object(1), "a\n")),
	}

	err := gitexec.New(runner, gitexec.Options{}).Archive(repo, headSHA, w.dir)

	if err != nil {
		t.Fatalf("Archive error = %v", err)
	}
	if want := object(2) + "\n" + object(1) + "\n"; string(runner.stdin) != want {
		t.Errorf("git was asked for %q, want %q", runner.stdin, want)
	}
	if _, err := os.Lstat(filepath.Join(w.dir, "mod")); err == nil {
		t.Error("the submodule mod is in the copy, want it skipped")
	}
	if info, err := os.Stat(filepath.Join(w.dir, "a.sh")); err != nil || info.Mode().Perm() != 0o755 {
		t.Errorf("a.sh = %v, %v, want a file of mode 0755", info, err)
	}
	if got, err := os.ReadFile(filepath.Join(w.dir, "b.go")); err != nil || string(got) != "b\n" {
		t.Errorf("b.go = %q, %v, want %q", got, err, "b\n")
	}
}
