package repohistory_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/repohistory"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

// newRepo builds a repository holding kept.go, gone.go and moved.go, then
// deletes gone.go and renames moved.go, in separate commits.
func newRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	git(t, root, "init", "-q", "-b", "main")
	write(t, root, "kept.go", "package a\n")
	write(t, root, "gone.go", "package a\n// a distinctive body so rename detection has something to match\n")
	write(t, root, "pkg/moved.go", "package b\n// another distinctive body, long enough to be recognisable\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "first")

	git(t, root, "rm", "-q", "gone.go")
	git(t, root, "commit", "-q", "-m", "remove gone.go")

	git(t, root, "mv", "pkg/moved.go", "pkg/renamed.go")
	git(t, root, "commit", "-q", "-m", "rename moved.go")
	return root
}

// A file still in the tree is present, and nothing about it is stale.
func TestInspect_PresentPath(t *testing.T) {
	got := inspect(t, newRepo(t), "kept.go")
	if got.State != repohistory.StatePresent {
		t.Fatalf("kept.go = %v, want present", got.State)
	}
}

// A deleted file is the finding this whole reader exists for, and the
// commit that removed it is the evidence -- without it there is only an
// assertion.
func TestInspect_DeletedPathCarriesItsEvidence(t *testing.T) {
	got := inspect(t, newRepo(t), "gone.go")
	if got.State != repohistory.StateDeleted {
		t.Fatalf("gone.go = %v, want deleted", got.State)
	}
	if got.Commit == "" {
		t.Error("a deletion with no commit behind it is an assertion, not evidence")
	}
	if got.At.IsZero() {
		t.Error("a deletion with no date cannot be compared against when a memory was written")
	}
}

// THE false positive that would make this feature harmful: a renamed file
// is absent from its old path and present in the tree under a new one.
// Reading that as a deletion would mark live memory as describing something
// removed, and invite deleting it.
func TestInspect_RenameIsNotADeletion(t *testing.T) {
	got := inspect(t, newRepo(t), "pkg/moved.go")
	if got.State == repohistory.StateDeleted {
		t.Fatal("a renamed file read as deleted: this marks live memory as dead")
	}
	if got.State != repohistory.StateRenamed {
		t.Fatalf("pkg/moved.go = %v, want renamed", got.State)
	}
	if got.NewPath != "pkg/renamed.go" {
		t.Fatalf("the rename must name where the content went: got %q", got.NewPath)
	}
}

// A path this repository never had is not evidence of anything. Observations
// routinely name another project's files, and treating those as deletions
// would manufacture findings out of somebody else's repository.
func TestInspect_PathNeverInHistoryIsNotOurs(t *testing.T) {
	got := inspect(t, newRepo(t), "internal/somebody/elses.go")
	if got.State != repohistory.StateUnknown {
		t.Fatalf("a path never in history = %v, want unknown", got.State)
	}
}

// Not a repository must fail loudly. A silent empty answer would read as
// "nothing was ever deleted", which is the answer that hides every finding.
func TestInspect_OutsideARepositoryIsAnError(t *testing.T) {
	if _, err := repohistory.Inspect(t.TempDir(), []string{"any.go"}); err == nil {
		t.Fatal("inspecting a non-repository must fail, not report nothing found")
	}
}

func inspect(t *testing.T, root, path string) repohistory.PathFact {
	t.Helper()
	facts, err := repohistory.Inspect(root, []string{path})
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	got, ok := facts[path]
	if !ok {
		t.Fatalf("Inspect returned no fact for %q", path)
	}
	return got
}
