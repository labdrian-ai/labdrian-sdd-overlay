package repoevidence_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/repoevidence"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/staleness"
)

// realDir is a fresh directory named by its resolved path (a temporary directory can be a link, and the paths
// the adapter prints are the ones the test compares).
func realDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve the temporary directory: %v", err)
	}
	return dir
}

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

// requireGit skips a test that reads a repository on a host with no git.
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH: this test reads the history of a repository")
	}
}

// git runs git in dir with an identity and, when when is not empty, a date of the test's own.
func git(t *testing.T, dir, when string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	if when != "" {
		cmd.Env = append(cmd.Env, "GIT_AUTHOR_DATE="+when, "GIT_COMMITTER_DATE="+when)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// Every file of the tree is listed as a slash-separated path relative to the root, in the order the walk meets
// them; a directory is not a file, a link is (it is not followed), and the repository's own metadata is left
// out wherever it is: a .git directory at the root or below it. A .git that is a file (a worktree's pointer) is
// a file of the tree like any other.
func TestFiles_ListsTheFilesOfTheTreeAndNotTheRepositoriesOwnMetadata(t *testing.T) {
	root := realDir(t)
	write(t, root, "kept.go", "x")
	write(t, root, "module/promote/writer.go", "x")
	write(t, root, ".git/config", "x")
	write(t, root, ".git/hooks/pre-commit.sh", "x")
	write(t, root, "vendored/.git/config", "x")
	write(t, root, "worktree/.git", "gitdir: elsewhere\n")
	write(t, root, "has space.go", "x")
	if err := os.MkdirAll(filepath.Join(root, "empty", "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("module", filepath.Join(root, "link-to-a-directory")); err != nil {
		t.Skipf("cannot make a link here: %v", err)
	}
	if err := os.Symlink("kept.go", filepath.Join(root, "link-to-a-file.go")); err != nil {
		t.Skipf("cannot make a link here: %v", err)
	}

	got, err := repoevidence.At(root).Files()

	if err != nil {
		t.Fatalf("Files: %v", err)
	}
	want := []string{
		"has space.go",
		"kept.go",
		"link-to-a-directory",
		"link-to-a-file.go",
		"module/promote/writer.go",
		"worktree/.git",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Files = %v, want %v", got, want)
	}
}

// A tree with no file lists none, and it is not an error.
func TestFiles_AnEmptyTreeHasNoFiles(t *testing.T) {
	got, err := repoevidence.At(realDir(t)).Files()
	if err != nil || len(got) != 0 {
		t.Errorf("Files = %v, %v; want none and no error", got, err)
	}
}

// A root that is not there is said so, in the program's own words; any other failure of the walk names the root
// and carries the file system's own error. The words are the ones the report has always printed.
func TestFiles_SaysWhyTheTreeCannotBeListed(t *testing.T) {
	root := realDir(t)

	_, err := repoevidence.At(filepath.Join(root, "absent")).Files()
	if want := "staleness: " + filepath.Join(root, "absent") + " does not exist"; err == nil || err.Error() != want {
		t.Errorf("Files of a root that is not there = %v, want %q", err, want)
	}

	locked := filepath.Join(root, "locked")
	if err := os.Mkdir(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	if _, err := os.ReadDir(locked); err == nil {
		t.Skip("directory permissions are not enforced for this process (root or an equivalent capability): the tree cannot be made unreadable")
	}
	got, err := repoevidence.At(root).Files()
	if want := "staleness: walking " + root + ": open " + locked + ": permission denied"; err == nil || err.Error() != want || got != nil {
		t.Errorf("Files of a tree with a directory that cannot be read = %v, %v; want %q and no files", got, err, want)
	}
}

// A root that is a file is a tree of one file, which is named ".": what the walk does when it is started on a
// file. (The program only ever asks about a directory; this is what happens if it did not.)
func TestFiles_ARootThatIsAFileIsATreeOfOneFile(t *testing.T) {
	root := realDir(t)
	write(t, root, "a-file", "x")

	got, err := repoevidence.At(filepath.Join(root, "a-file")).Files()

	if err != nil || !reflect.DeepEqual(got, []string{"."}) {
		t.Errorf("Files = %v, %v; want the one file, named \".\"", got, err)
	}
}

// What the history says is told in the rule's own words: present, deleted by a commit (with its id and its
// committer date), renamed (with where it went), or never held. The paths come back under the text they were
// asked by.
func TestHistory_TellsWhatBecameOfEachPathInTheRulesOwnWords(t *testing.T) {
	requireGit(t)
	root := realDir(t)
	git(t, root, "", "init", "-q", "-b", "main")
	write(t, root, "kept.go", "package a\n")
	write(t, root, "gone.go", "package a\n// a distinctive body so that rename detection has nothing to match\n")
	write(t, root, "pkg/moved.go", "package b\n// another distinctive body, long enough to be recognised\n")
	git(t, root, "", "add", "-A")
	git(t, root, "", "commit", "-q", "-m", "first")
	git(t, root, "", "rm", "-q", "gone.go")
	git(t, root, "2027-06-01T00:00:00Z", "commit", "-q", "-m", "remove gone.go")
	git(t, root, "", "mv", "pkg/moved.go", "pkg/renamed.go")
	git(t, root, "2027-06-02T00:00:00Z", "commit", "-q", "-m", "rename moved.go")

	facts, err := repoevidence.At(root).History([]string{"kept.go", "gone.go", "pkg/moved.go", "never-was.go"})

	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(facts) != 4 {
		t.Fatalf("History = %+v, want one fact for each of the four paths asked about", facts)
	}
	if got := facts["kept.go"]; got.Path != "kept.go" || got.State != staleness.PathPresent || got.Commit != "" {
		t.Errorf("kept.go = %+v, want present, with no commit", got)
	}
	if got := facts["gone.go"]; got.Path != "gone.go" || got.State != staleness.PathDeleted || len(got.Commit) != 40 || !got.At.Equal(time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("gone.go = %+v, want deleted by a commit (its full id) on 2027-06-01", got)
	}
	if got := facts["pkg/moved.go"]; got.State != staleness.PathRenamed || got.NewPath != "pkg/renamed.go" || len(got.Commit) != 40 || !got.At.Equal(time.Date(2027, 6, 2, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("pkg/moved.go = %+v, want renamed to pkg/renamed.go by a commit on 2027-06-02", got)
	}
	if got := facts["never-was.go"]; got.State != staleness.PathUnknown || got.Commit != "" {
		t.Errorf("never-was.go = %+v, want unknown, with no commit", got)
	}
}

// A directory that is no repository cannot tell what became of a path; the answer is the history's error, not a
// guess, and no fact.
func TestHistory_ReportsADirectoryThatIsNoRepository(t *testing.T) {
	requireGit(t)
	facts, err := repoevidence.At(realDir(t)).History([]string{"lost.go"})

	if err == nil || !strings.Contains(err.Error(), "not a readable git repository") || facts != nil {
		t.Errorf("History = %v, %v; want the history's error and no facts", facts, err)
	}
}

// A refusal of one path is the refusal of all: git does not take a path outside the repository, and the answer
// is its error and no fact, not a guess for that path and not the facts for the others.
func TestHistory_AnErrorForOnePathIsTheAnswerForAll(t *testing.T) {
	requireGit(t)
	root := realDir(t)
	git(t, root, "", "init", "-q", "-b", "main")
	write(t, root, "kept.go", "package a\n")
	git(t, root, "", "add", "-A")
	git(t, root, "", "commit", "-q", "-m", "first")

	facts, err := repoevidence.At(root).History([]string{"kept.go", "a/../../outside.go"})

	if err == nil || facts != nil || !strings.Contains(err.Error(), "repohistory: reading the history of a/../../outside.go: ") {
		t.Errorf("History = %v, %v; want git's refusal of the path that climbs out, and no facts", facts, err)
	}
}
