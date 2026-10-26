package gitexec_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/execrunner"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/gitexec"
)

// The tests of this file run the git of the machine on repositories they make in temporary directories.
// Two environments are kept apart: the one of the git the test runs to make a repository (own), which reads no
// configuration of whoever runs the tests, and the one the adapter under test is given, whose HOME is a
// directory of the test where a test writes the configuration of a hostile user. A premise is a check that a
// hostile setting really changes what a git with nothing pinned prints; it is reported and does not fail the
// test, because the setting being ignored by another version of git would not make the adapter wrong.

// fixture is a repository made with the real git.
type fixture struct {
	t    *testing.T
	root string   // the main working tree, symbolic links resolved
	home string   // the HOME the adapter's git is given
	own  []string // the environment of the git the test runs to make the repository
}

// newFixture makes an empty repository with the given object format (sha1 or sha256). A git that cannot make
// a SHA-256 repository skips the test.
func newFixture(t *testing.T, objectFormat string) *fixture {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	f := &fixture{t: t, root: resolved(t, t.TempDir()), home: t.TempDir()}
	f.own = append([]string{
		"PATH=" + os.Getenv("PATH"), "HOME=" + f.home, "XDG_CONFIG_HOME=" + t.TempDir(), "LC_ALL=C", "GIT_ATTR_NOSYSTEM=1",
	}, overlaytest.GitSettings()...)
	if out, err := f.run(f.root, f.own, "init", "-q", "-b", "main", "--object-format="+objectFormat); err != nil {
		if objectFormat != "sha1" {
			t.Skipf("this git cannot make a %s repository: %v", objectFormat, err)
		}
		t.Fatalf("setup: git init: %v\n%s", err, out)
	}
	return f
}

// at is the fixture bound to the test t of a subtest, so that a failure of the setup stops that subtest.
func (f *fixture) at(t *testing.T) *fixture {
	c := *f
	c.t = t
	return &c
}

func resolved(t *testing.T, dir string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("setup: resolve %s: %v", dir, err)
	}
	return r
}

// run runs git in dir under env and returns what it printed on standard output; the error holds what it
// printed on standard error.
func (f *fixture) run(dir string, env []string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = env
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return string(out), fmt.Errorf("git %q: %w\n%s", args, err, stderr.Bytes())
	}
	return string(out), nil
}

// git runs git in the repository as the test and fails the test when it fails.
func (f *fixture) git(args ...string) string {
	f.t.Helper()
	out, err := f.run(f.root, f.own, args...)
	if err != nil {
		f.t.Fatalf("setup: %v", err)
	}
	return out
}

func (f *fixture) name(rev string) string { return strings.TrimSpace(f.git("rev-parse", rev)) }

func (f *fixture) write(rel, content string) { overlaytest.WriteFile(f.t, f.root, rel, content) }

func (f *fixture) writeBytes(rel string, content []byte) {
	f.t.Helper()
	p := filepath.Join(f.root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		f.t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(p, content, 0o644); err != nil {
		f.t.Fatalf("setup: %v", err)
	}
}

func (f *fixture) stage() { f.t.Helper(); f.git("add", "-A") }

// snapshot commits what is staged and returns the full name of the commit.
func (f *fixture) snapshot(message string) string {
	f.t.Helper()
	f.git("commit", "-q", "-m", message)
	return f.name("HEAD")
}

// lines is n numbered lines, the one numbered changed (when it is not 0) reworded: a file with enough
// context around its change for the size of the context to show.
func lines(n, changed int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		if i == changed {
			fmt.Fprintf(&b, "line %02d changed\n", i)
			continue
		}
		fmt.Fprintf(&b, "line %02d\n", i)
	}
	return b.String()
}

// history makes two commits that differ in every way a diff can: text changed with a lot and little context,
// a blank line, a non-ASCII name and names with a tab, a newline and a space, a binary file, a rename, a
// deletion, an addition, an empty file, a symbolic link, a change of mode, a file with no newline at its end
// and a gitlink.
func (f *fixture) history() (base, head string) {
	f.t.Helper()
	f.write("a.txt", "one\ntwo\nthree\n")
	f.write("ctx.txt", lines(30, 0))
	f.write("café.txt", "x\n")
	f.write("blank.txt", "a\n\nb\n")
	f.write("old.txt", "old first line\nsame 2\nsame 3\nsame 4\nsame 5\nsame 6\nsame 7\nsame 8\n")
	f.write("gone.txt", "gone\n")
	f.write("run.sh", "#!/bin/sh\n")
	f.write("nonl.txt", "no newline")
	f.writeBytes("bin.dat", append([]byte{0}, bytes.Repeat([]byte("Z"), 30000)...))
	f.write("tab\tname.txt", "t\n")
	f.write("new\nline.txt", "n\n")
	f.write("with space.txt", "s\n")
	f.write("dir/x.txt", "x\n")
	f.stage()
	f.git("update-index", "--add", "--cacheinfo", "160000,"+strings.Repeat("1", 40)+",subm")
	base = f.snapshot("base")

	f.write("a.txt", "one\ntwo changed\nthree\nfour\n")
	f.write("ctx.txt", lines(30, 15))
	f.write("café.txt", "xx\n")
	f.write("blank.txt", "a\n\nb\nc\n")
	f.git("mv", "old.txt", "new.txt")
	f.write("new.txt", "new first line\nsame 2\nsame 3\nsame 4\nsame 5\nsame 6\nsame 7\nsame 8\n")
	if err := os.Remove(filepath.Join(f.root, "gone.txt")); err != nil {
		f.t.Fatalf("setup: %v", err)
	}
	if err := os.Chmod(filepath.Join(f.root, "run.sh"), 0o755); err != nil {
		f.t.Fatalf("setup: %v", err)
	}
	f.write("nonl.txt", "no newline either")
	f.writeBytes("bin.dat", append(append([]byte{0}, bytes.Repeat([]byte("Y"), 30000)...), "tail"...))
	f.write("tab\tname.txt", "t2\n")
	f.write("new\nline.txt", "n2\n")
	f.write("with space.txt", "s2\n")
	f.write("dir/x.txt", "x2\n")
	f.write("empty-added.txt", "")
	if err := os.Symlink("a.txt", filepath.Join(f.root, "link")); err != nil {
		f.t.Fatalf("setup: %v", err)
	}
	f.stage()
	f.git("update-index", "--add", "--cacheinfo", "160000,"+strings.Repeat("2", 40)+",subm")
	head = f.snapshot("head")
	return base, head
}

// user makes the HOME of the adapter's git the home of a user: a global configuration (when config is not
// empty) and the files, named relative to the home.
func (f *fixture) user(config string, files map[string]string) {
	f.t.Helper()
	if err := os.RemoveAll(f.home); err != nil {
		f.t.Fatalf("setup: %v", err)
	}
	if err := os.MkdirAll(f.home, 0o755); err != nil {
		f.t.Fatalf("setup: %v", err)
	}
	if config != "" {
		files = mergeFile(files, ".gitconfig", config)
	}
	for rel, content := range files {
		overlaytest.WriteFile(f.t, f.home, rel, content)
	}
}

func mergeFile(files map[string]string, rel, content string) map[string]string {
	out := map[string]string{rel: content}
	for k, v := range files {
		out[k] = v
	}
	return out
}

// environ is the environment of the process the adapter is given, with extra entries.
func (f *fixture) environ(extra ...string) []string {
	return append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + f.home}, extra...)
}

func (f *fixture) adapter(extra ...string) gitexec.Git {
	return gitexec.New(execrunner.New().WithMaxOutput(64<<20), gitexec.Options{Environ: f.environ(extra...), Timeout: time.Minute})
}

func (f *fixture) repo() app.Repository {
	return app.Repository{Root: f.root, Main: f.root, CommonDir: filepath.Join(f.root, ".git")}
}

// plain runs git in the repository as a person's git would, under the adapter's HOME, with none of the pins of
// the adapter: the plumbing diff of two commits, and the arguments given before the command.
func (f *fixture) plain(global []string, env []string, base, head string) []byte {
	f.t.Helper()
	args := append(append([]string{}, global...), "diff-tree", "-p", "-r", "--full-index", "--binary", base, head)
	out, err := f.run(f.root, append(f.environ("LC_ALL=C", "GIT_ATTR_NOSYSTEM=1"), env...), args...)
	if err != nil {
		f.t.Fatalf("setup: %v", err)
	}
	return []byte(out)
}

// premise reports, without failing, that a setting changes the diff of a git with nothing pinned.
func premise(t *testing.T, name string, clean, hostile []byte) {
	t.Helper()
	if bytes.Equal(clean, hostile) {
		t.Logf("premise: %s does not change the diff of this git when nothing is pinned, so this case does not show the pin that keeps it out", name)
	}
}

func diffOf(t *testing.T, g gitexec.Git, repo app.Repository, base, head string) []byte {
	t.Helper()
	got, err := g.Diff(repo, base, head)
	if err != nil {
		t.Fatalf("Diff() error = %v, want nil", err)
	}
	return got
}

// The bytes of a diff, to the byte, for a pair of commits that no configuration touches: the definition
// written out for one file.
func TestDiffIsExactlyThePatchOfTheTwoCommits(t *testing.T) {
	f := newFixture(t, "sha1")
	f.write("a.txt", "one\ntwo\nthree\n")
	f.stage()
	base := f.snapshot("base")
	f.write("a.txt", "one\ntwo changed\nthree\nfour\n")
	f.stage()
	head := f.snapshot("head")

	got := diffOf(t, f.adapter(), f.repo(), base, head)

	want := "diff --git a/a.txt b/a.txt\n" +
		"index 4cb29ea38f70d7c61b2a3a25b02e3bdf44905402..661170007c97cd4069435a1b1067594a0538887b 100644\n" +
		"--- a/a.txt\n" +
		"+++ b/a.txt\n" +
		"@@ -1,3 +1,4 @@\n" +
		" one\n" +
		"-two\n" +
		"+two changed\n" +
		" three\n" +
		"+four\n"
	if string(got) != want {
		t.Errorf("Diff() =\n%s\nwant\n%s", got, want)
	}
}

// Each kind of change is in the bytes as the change it is, which is what makes the digest bind it: a rename
// is a removal and an addition, a binary file is a patch, an object is named in full, and nothing is colored.
func TestDiffShowsEveryKindOfChangeAsTheChangeItIs(t *testing.T) {
	f := newFixture(t, "sha1")
	base, head := f.history()

	text := string(diffOf(t, f.adapter(), f.repo(), base, head))

	for _, want := range []string{
		`diff --git "a/caf\303\251.txt" "b/caf\303\251.txt"`,                                                // a non-ASCII name is quoted
		"diff --git a/old.txt b/old.txt\ndeleted file mode 100644",                                          // a rename is a removal...
		"diff --git a/new.txt b/new.txt\nnew file mode 100644",                                              // ... and an addition
		"diff --git a/gone.txt b/gone.txt\ndeleted file mode 100644",                                        // a deletion
		"diff --git a/empty-added.txt b/empty-added.txt\nnew file mode 100644",                              // an empty file
		"diff --git a/link b/link\nnew file mode 120000",                                                    // a symbolic link
		"old mode 100644\nnew mode 100755",                                                                  // a change of mode
		"GIT binary patch\nliteral ",                                                                        // a binary file is a patch
		"\\ No newline at end of file",                                                                      // a file with no newline at its end
		"-Subproject commit " + strings.Repeat("1", 40) + "\n+Subproject commit " + strings.Repeat("2", 40), // a gitlink
		"@@ -12,7 +12,7 @@", // three lines of context around line 15
	} {
		if !strings.Contains(text, want) {
			t.Errorf("Diff() lacks %q", want)
		}
	}
	for _, unwanted := range []string{"rename from", "similarity index", "\x1b["} {
		if strings.Contains(text, unwanted) {
			t.Errorf("Diff() holds %q, want a diff with no rename detection and no color", unwanted)
		}
	}
	indexes := regexp.MustCompile(`(?m)^index ([0-9a-f]+)\.\.([0-9a-f]+)`).FindAllStringSubmatch(text, -1)
	if len(indexes) == 0 {
		t.Fatal("Diff() has no index line")
	}
	for _, m := range indexes {
		if len(m[1]) != 40 || len(m[2]) != 40 {
			t.Errorf("index line %q names objects by %d and %d characters, want the full 40", m[0], len(m[1]), len(m[2]))
		}
	}
}

// The user's configuration is not part of the definition. Each hostile case is a setting that changes the
// diff of a git with nothing pinned (the premise) and does not change the bytes the adapter returns, for the
// diff nor for the list of paths.
func TestDiffIsTheSameWhateverTheConfigurationOfTheUserSays(t *testing.T) {
	f := newFixture(t, "sha1")
	base, head := f.history()
	f.user("", nil)
	cleanDiff := diffOf(t, f.adapter(), f.repo(), base, head)
	cleanPaths, err := f.adapter().Paths(f.repo(), base, head)
	if err != nil {
		t.Fatalf("Paths() error = %v", err)
	}
	attributes := filepath.Join(f.home, "attributes-of-the-user")

	for _, tc := range []struct {
		name   string
		config string
		files  map[string]string
		leaks  bool // a git with nothing pinned prints another diff
	}{
		{"core.quotePath=false", "[core]\n\tquotePath = false\n", nil, true},
		{"diff.suppressBlankEmpty=true", "[diff]\n\tsuppressBlankEmpty = true\n", nil, true},
		{"core.attributesFile with * -diff", "[core]\n\tattributesFile = " + attributes + "\n", map[string]string{"attributes-of-the-user": "* -diff\n"}, true},
		{"the XDG attributes file with * -diff", "", map[string]string{".config/git/attributes": "* -diff\n"}, true},
		{"core.compression=9", "[core]\n\tcompression = 9\n", nil, true},
		{"core.looseCompression=9", "[core]\n\tlooseCompression = 9\n", nil, true},
		{"the settings of a diff that the plumbing leaves to the porcelain", "[diff]\n\talgorithm = patience\n\tindentHeuristic = false\n" +
			"\tcontext = 9\n\tinterHunkContext = 5\n\tnoprefix = true\n\tmnemonicPrefix = true\n\trenames = copies\n" +
			"\tignoreSubmodules = all\n\texternal = /bin/false\n\tsrcPrefix = X/\n\tdstPrefix = Y/\n\trelative = true\n" +
			"[color]\n\tui = always\n[core]\n\tabbrev = 12\n\twhitespace = trailing-space\n", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := f.at(t)
			f.user("", nil)
			plainClean := f.plain(nil, nil, base, head)
			f.user(tc.config, tc.files)
			if tc.leaks {
				premise(t, tc.name, plainClean, f.plain(nil, nil, base, head))
			}

			got := diffOf(t, f.adapter(), f.repo(), base, head)
			paths, err := f.adapter().Paths(f.repo(), base, head)
			if err != nil {
				t.Fatalf("Paths() error = %v, want nil", err)
			}

			if !bytes.Equal(got, cleanDiff) {
				t.Errorf("Diff() under %s is %d bytes, not the %d bytes it is under no configuration", tc.name, len(got), len(cleanDiff))
			}
			if strings.Join(paths, "\x00") != strings.Join(cleanPaths, "\x00") {
				t.Errorf("Paths() under %s = %q, want %q", tc.name, paths, cleanPaths)
			}
		})
	}
}

// The working tree and the index are not part of the definition either: the attributes of a diff come from
// the empty tree, so a .gitattributes there, staged or not, and the commit that is checked out change
// nothing.
func TestDiffIsTheSameWhateverTheWorkingTreeAndTheIndexHold(t *testing.T) {
	f := newFixture(t, "sha1")
	base, head := f.history()
	f.user("", nil)
	clean := diffOf(t, f.adapter(), f.repo(), base, head)
	plainClean := f.plain(nil, nil, base, head)

	for _, tc := range []struct {
		name    string
		prepare func(f *fixture)
		restore func(f *fixture)
		leaks   bool
	}{
		{"a .gitattributes in the working tree that makes the text files binary",
			func(f *fixture) { f.write(".gitattributes", "*.txt -diff\n") },
			func(f *fixture) { _ = os.Remove(filepath.Join(f.root, ".gitattributes")) }, true},
		{"a .gitattributes in the index that makes the text files binary",
			func(f *fixture) { f.write(".gitattributes", "*.txt -diff\n"); f.git("add", ".gitattributes") },
			func(f *fixture) {
				f.git("rm", "-q", "--cached", ".gitattributes")
				_ = os.Remove(filepath.Join(f.root, ".gitattributes"))
			}, true},
		{"the base checked out instead of the head",
			func(f *fixture) { f.git("checkout", "-q", "--detach", base) },
			func(f *fixture) { f.git("checkout", "-q", "main") }, false},
		{"a modified file in the working tree",
			func(f *fixture) { f.write("a.txt", "modified and not committed\n") },
			func(f *fixture) { f.git("checkout", "-q", "--", "a.txt") }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := f.at(t)
			tc.prepare(f)
			defer tc.restore(f)
			if tc.leaks {
				premise(t, tc.name, plainClean, f.plain(nil, nil, base, head))
			}

			got := diffOf(t, f.adapter(), f.repo(), base, head)

			if !bytes.Equal(got, clean) {
				t.Errorf("Diff() with %s is %d bytes, not the %d bytes it is without", tc.name, len(got), len(clean))
			}
		})
	}
}

// The commit itself is not part of the definition: a .gitattributes in the candidate names a diff driver, and
// the configuration of that driver is the user's. The attributes come from the empty tree, so the driver is
// never named.
func TestDiffIgnoresTheAttributesTheCandidateCommitsAndTheDriversTheyName(t *testing.T) {
	f := newFixture(t, "sha1")
	base, _ := f.history()
	f.write(".gitattributes", "*.txt diff=custom\n")
	f.stage()
	head := f.snapshot("a candidate that names a diff driver")
	f.user("", nil)
	clean := diffOf(t, f.adapter(), f.repo(), base, head)
	plainClean := f.plain([]string{"--attr-source=" + head}, nil, base, head)
	f.user("[diff \"custom\"]\n\tbinary = true\n", nil)
	premise(t, "a driver named by the commit", plainClean, f.plain([]string{"--attr-source=" + head}, nil, base, head))

	got := diffOf(t, f.adapter(), f.repo(), base, head)

	if !bytes.Equal(got, clean) {
		t.Errorf("Diff() with the driver configured is %d bytes, not the %d bytes it is without", len(got), len(clean))
	}
	if !strings.Contains(string(got), "+++ b/a.txt") {
		t.Errorf("Diff() shows a.txt as something else than text, want the text diff the empty tree of attributes gives")
	}
}

// The environment of the process is not part of the definition: every variable of git is removed before git
// starts, so a variable that sets the context, an external diff, the repository or the language changes nothing.
func TestResultsAreTheSameWhateverTheEnvironmentOfTheProcessHolds(t *testing.T) {
	f := newFixture(t, "sha1")
	base, head := f.history()
	f.user("", nil)
	hostile := f.adapter(
		"GIT_DIFF_OPTS=--unified=9", "GIT_EXTERNAL_DIFF=/bin/false", "GIT_DIR=/nonexistent/.git", "GIT_WORK_TREE=/nonexistent",
		"GIT_INDEX_FILE=/nonexistent/index", "GIT_OBJECT_DIRECTORY=/nonexistent/objects", "GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=core.quotePath", "GIT_CONFIG_VALUE_0=false", "GIT_ATTR_NOSYSTEM=0", "LC_ALL=de_DE.UTF-8", "LANGUAGE=de",
	)
	plain := f.adapter()
	premise(t, "GIT_DIFF_OPTS=--unified=9",
		runCanonical(t, f, nil, base, head), runCanonical(t, f, []string{"GIT_DIFF_OPTS=--unified=9"}, base, head))

	if got, want := diffOf(t, hostile, f.repo(), base, head), diffOf(t, plain, f.repo(), base, head); !bytes.Equal(got, want) {
		t.Errorf("Diff() under a hostile environment is %d bytes, not the %d bytes it is under a clean one", len(got), len(want))
	}
	gotPaths, err := hostile.Paths(f.repo(), base, head)
	if err != nil {
		t.Fatalf("Paths() error = %v, want nil under a hostile environment", err)
	}
	wantPaths, _ := plain.Paths(f.repo(), base, head)
	if strings.Join(gotPaths, "\x00") != strings.Join(wantPaths, "\x00") {
		t.Errorf("Paths() under a hostile environment = %q, want %q", gotPaths, wantPaths)
	}
	gotHead, err := hostile.Commit(f.repo(), "HEAD")
	if err != nil || gotHead != head {
		t.Errorf("Commit(HEAD) under a hostile environment = %q, %v; want %q", gotHead, err, head)
	}
}

// runCanonical runs the arguments of the canonical diff under the given extra environment, without the
// scrubbing of the adapter.
func runCanonical(t *testing.T, f *fixture, env []string, base, head string) []byte {
	t.Helper()
	out, err := f.run(f.root, append(f.environ("LC_ALL=C", "GIT_ATTR_NOSYSTEM=1"), env...), gitexec.DiffArgs(base, head)...)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	return []byte(out)
}

// The paths are the names as git holds them, once each, whatever the name.
func TestPathsListsEveryChangedPathOnceWhateverItsName(t *testing.T) {
	f := newFixture(t, "sha1")
	base, head := f.history()

	got, err := f.adapter().Paths(f.repo(), base, head)
	if err != nil {
		t.Fatalf("Paths() error = %v, want nil", err)
	}

	want := []string{
		"a.txt", "bin.dat", "blank.txt", "café.txt", "ctx.txt", "dir/x.txt", "empty-added.txt", "gone.txt", "link", "new\nline.txt",
		"new.txt", "nonl.txt", "old.txt", "run.sh", "subm", "tab\tname.txt", "with space.txt",
	}
	sorted := append([]string{}, got...)
	sort.Strings(sorted)
	if strings.Join(sorted, "\x00") != strings.Join(want, "\x00") {
		t.Errorf("Paths() = %q, want the paths %q (a rename counts as the removal of the old path and the creation of the new one)", got, want)
	}
}

func TestPathsOfTwoCommitsWithNoDifferenceIsEmptyAndTheDiffToo(t *testing.T) {
	f := newFixture(t, "sha1")
	_, head := f.history()

	paths, err := f.adapter().Paths(f.repo(), head, head)
	diff, diffErr := f.adapter().Diff(f.repo(), head, head)

	if err != nil || diffErr != nil {
		t.Fatalf("Paths() error = %v, Diff() error = %v; want nil, nil", err, diffErr)
	}
	if paths == nil || len(paths) != 0 || len(diff) != 0 {
		t.Errorf("Paths() = %#v, Diff() = %q; want an empty list and no bytes", paths, diff)
	}
}

func TestCommitResolvesEveryKindOfRevisionToTheCommitItNames(t *testing.T) {
	f := newFixture(t, "sha1")
	base, head := f.history()
	f.git("branch", "topic", base)
	f.git("tag", "light", base)
	f.git("tag", "-a", "-m", "release", "v1", base)
	if f.name("v1") == base {
		t.Fatal("setup: the annotated tag is the commit, want a tag object")
	}
	g := f.adapter()

	for _, tc := range []struct{ rev, want string }{
		{"HEAD", head}, {"HEAD~1", base}, {"main", head}, {"topic", base}, {"light", base}, {"v1", base},
		{"refs/heads/topic", base}, {head, head}, {head[:8], head}, {base + "~0", base},
	} {
		t.Run(tc.rev, func(t *testing.T) {
			got, err := g.Commit(f.repo(), tc.rev)
			if err != nil || got != tc.want {
				t.Errorf("Commit(%q) = %q, %v; want %q", tc.rev, got, err, tc.want)
			}
		})
	}
}

func TestCommitRefusesWhatIsNotACommit(t *testing.T) {
	f := newFixture(t, "sha1")
	f.history()
	g := f.adapter()

	for _, tc := range []struct{ name, rev string }{
		{"a name that is nothing", "nonesuch"},
		{"a tree", f.name("HEAD^{tree}")},
		{"a blob", f.name("HEAD:a.txt")},
		{"an option", "--all"},
		{"a short option", "-x"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := g.Commit(f.repo(), tc.rev)
			if err == nil {
				t.Fatalf("Commit(%q) = %q, nil; want an error", tc.rev, got)
			}
			if !strings.Contains(err.Error(), "rev-parse") || !strings.Contains(err.Error(), "fatal:") {
				t.Errorf("Commit(%q) error = %q, want it to name the command and say what git said", tc.rev, err)
			}
		})
	}
}

func TestAFailureOfARealGitSaysWhatGitSaid(t *testing.T) {
	f := newFixture(t, "sha1")
	_, head := f.history()

	_, err := f.adapter().Diff(f.repo(), strings.Repeat("0", 40), head)

	if err == nil || !strings.Contains(err.Error(), "diff-tree") || !strings.Contains(err.Error(), "fatal:") {
		t.Errorf("Diff() of an object that is not there: error = %v, want it to name the command and say what git said", err)
	}
}

func TestLocateFindsTheWorkingTreeTheMainCheckoutAndTheGitDirectory(t *testing.T) {
	f := newFixture(t, "sha1")
	f.write("sub/b.txt", "y\n")
	f.stage()
	head := f.snapshot("one")
	linked := filepath.Join(resolved(t, t.TempDir()), "linked")
	f.git("worktree", "add", "-q", "--detach", linked, head)
	common := filepath.Join(f.root, ".git")
	g := f.adapter()

	for _, tc := range []struct {
		name string
		dir  string
		want app.Repository
	}{
		{"the main working tree", f.root, app.Repository{Root: f.root, Main: f.root, CommonDir: common}},
		{"a directory of the main working tree", filepath.Join(f.root, "sub"), app.Repository{Root: f.root, Main: f.root, CommonDir: common}},
		{"a linked working tree", linked, app.Repository{Root: linked, Main: f.root, CommonDir: common}},
		{"a directory of a linked working tree", filepath.Join(linked, "sub"), app.Repository{Root: linked, Main: f.root, CommonDir: common}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := g.Locate(tc.dir)
			if err != nil || got != tc.want {
				t.Errorf("Locate(%s) = %+v, %v; want %+v", tc.dir, got, err, tc.want)
			}
		})
	}
}

func TestLocateRefusesWhatIsNotOneWorkingTreeOfOneRepository(t *testing.T) {
	f := newFixture(t, "sha1")
	f.history()

	for _, tc := range []struct {
		name    string
		dir     string
		adapter gitexec.Git
	}{
		{"a directory that is in no repository", resolved(t, t.TempDir()), f.adapter()},
		{"a relative path", ".", f.adapter()},
		{"an environment that redirects git to another repository", f.root, f.adapter("GIT_DIR=/elsewhere/.git")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.adapter.Locate(tc.dir)
			if err == nil {
				t.Errorf("Locate(%q) = %+v, nil; want an error", tc.dir, got)
			}
		})
	}
}

func TestAddWorktreeChecksTheHeadOutDetachedAndMakesTheMissingDirectories(t *testing.T) {
	f := newFixture(t, "sha1")
	base, head := f.history()
	path := filepath.Join(resolved(t, t.TempDir()), "repo-worktrees", "review-"+head[:12])

	if err := f.adapter().AddWorktree(f.repo(), path, head); err != nil {
		t.Fatalf("AddWorktree() error = %v, want nil", err)
	}

	content, err := os.ReadFile(filepath.Join(path, "a.txt"))
	if err != nil || string(content) != "one\ntwo changed\nthree\nfour\n" {
		t.Errorf("a.txt in the worktree = %q, %v; want the file of the head", content, err)
	}
	if got, _ := f.run(path, f.own, "rev-parse", "HEAD"); strings.TrimSpace(got) != head {
		t.Errorf("HEAD of the worktree = %q, want %q (not %q)", strings.TrimSpace(got), head, base)
	}
	if got, _ := f.run(path, f.own, "rev-parse", "--abbrev-ref", "HEAD"); strings.TrimSpace(got) != "HEAD" {
		t.Errorf("the worktree is on %q, want a detached HEAD", strings.TrimSpace(got))
	}
}

// A hook of the repository does not run in the worktree of a review: what a checkout of someone else's branch
// brings, or what the user keeps in .git/hooks, is not run by a command that only reads.
func TestAddWorktreeRunsNoHookOfTheRepository(t *testing.T) {
	f := newFixture(t, "sha1")
	_, head := f.history()
	marker := filepath.Join(resolved(t, t.TempDir()), "hook-ran")
	hook := filepath.Join(f.root, ".git", "hooks", "post-checkout")
	if err := os.MkdirAll(filepath.Dir(hook), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hook, []byte("#!/bin/sh\ntouch '"+marker+"'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	parent := resolved(t, t.TempDir())
	f.git("worktree", "add", "-q", "--detach", filepath.Join(parent, "plain"), head)
	if _, err := os.Stat(marker); err != nil {
		t.Skipf("premise: the hook does not run in a plain 'git worktree add' on this machine (%v), so the test shows nothing", err)
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}

	if err := f.adapter().AddWorktree(f.repo(), filepath.Join(parent, "review"), head); err != nil {
		t.Fatalf("AddWorktree() error = %v, want nil", err)
	}

	if _, err := os.Stat(marker); err == nil {
		t.Error("the post-checkout hook ran, want no hook in the worktree of a review")
	}
}

func TestAddWorktreeRefusesAPathThatIsTaken(t *testing.T) {
	f := newFixture(t, "sha1")
	_, head := f.history()
	path := filepath.Join(resolved(t, t.TempDir()), "taken")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "keep"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := f.adapter().AddWorktree(f.repo(), path, head)

	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("AddWorktree() onto a directory that is there: error = %v, want git's 'already exists'", err)
	}
	if content, readErr := os.ReadFile(filepath.Join(path, "keep")); readErr != nil || string(content) != "mine" {
		t.Errorf("the file that was there = %q, %v; want it untouched", content, readErr)
	}
}

func TestWorktreeSaysWhatIsAtThePath(t *testing.T) {
	f := newFixture(t, "sha1")
	base, head := f.history()
	other := newFixture(t, "sha1")
	other.write("o.txt", "o\n")
	other.stage()
	otherHead := other.snapshot("other")
	parent := resolved(t, t.TempDir())
	at := func(name string) string { return filepath.Join(parent, name) }
	g := f.adapter()

	f.git("worktree", "add", "-q", "--detach", at("at-head"), head)
	f.git("worktree", "add", "-q", "--detach", at("at-base"), head)
	if _, err := f.run(at("at-base"), f.own, "checkout", "-q", "--detach", base); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.Mkdir(at("plain"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(at("file"), []byte("f"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(at("at-head"), at("link")); err != nil {
		t.Fatal(err)
	}
	other.git("worktree", "add", "-q", "--detach", at("of-another"), otherHead)

	for _, tc := range []struct {
		name string
		path string
		want app.WorktreeState
	}{
		{"nothing", at("nothing"), app.WorktreeState{}},
		{"a worktree of the repository at the head", at("at-head"), app.WorktreeState{Present: true, OfRepository: true, Head: head}},
		{"a worktree of the repository at another commit", at("at-base"), app.WorktreeState{Present: true, OfRepository: true, Head: base}},
		{"a plain directory", at("plain"), app.WorktreeState{Present: true}},
		{"a file", at("file"), app.WorktreeState{Present: true}},
		{"a symbolic link to a worktree of the repository", at("link"), app.WorktreeState{Present: true}},
		{"a worktree of another repository", at("of-another"), app.WorktreeState{Present: true}},
		{"the main working tree of another repository", other.root, app.WorktreeState{Present: true}},
		{"a directory inside the working tree of the repository", filepath.Join(f.root, "dir"), app.WorktreeState{Present: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := g.Worktree(f.repo(), tc.path)
			if err != nil || got != tc.want {
				t.Errorf("Worktree(%s) = %+v, %v; want %+v", tc.path, got, err, tc.want)
			}
		})
	}
}

// A repository that names its objects with SHA-256 is served the same way, with names of 64 characters and
// the empty tree of that hash function.
func TestASHA256RepositoryIsServedLikeASHA1One(t *testing.T) {
	f := newFixture(t, "sha256")
	f.write("a.txt", "one\n")
	f.stage()
	base := f.snapshot("one")
	f.write("a.txt", "two\n")
	f.stage()
	head := f.snapshot("two")
	g := f.adapter()

	gotHead, err := g.Commit(f.repo(), "HEAD")
	if err != nil || gotHead != head || len(gotHead) != 64 {
		t.Fatalf("Commit(HEAD) = %q, %v; want the 64 characters of %q", gotHead, err, head)
	}
	diff := string(diffOf(t, g, f.repo(), base, head))
	index := regexp.MustCompile(`(?m)^index [0-9a-f]{64}\.\.[0-9a-f]{64} 100644$`)
	if !index.MatchString(diff) || !strings.Contains(diff, "-one\n+two\n") {
		t.Errorf("Diff() =\n%s\nwant the patch with the full SHA-256 names", diff)
	}
	if paths, err := g.Paths(f.repo(), base, head); err != nil || strings.Join(paths, ",") != "a.txt" {
		t.Errorf("Paths() = %q, %v; want [a.txt]", paths, err)
	}
	if empty := strings.TrimSpace(f.git("hash-object", "-t", "tree", os.DevNull)); empty != gitexec.EmptyTreeSHA256 {
		t.Errorf("the empty tree of a SHA-256 repository is %s, EmptyTreeSHA256 is %s", empty, gitexec.EmptyTreeSHA256)
	}
}

func TestTheEmptyTreeOfASHA1RepositoryIsEmptyTreeSHA1(t *testing.T) {
	f := newFixture(t, "sha1")

	if empty := strings.TrimSpace(f.git("hash-object", "-t", "tree", os.DevNull)); empty != gitexec.EmptyTreeSHA1 {
		t.Errorf("the empty tree of a SHA-1 repository is %s, EmptyTreeSHA1 is %s", empty, gitexec.EmptyTreeSHA1)
	}
}
