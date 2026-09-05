package staleness_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/staleness"
)

// Measured against a real database, a naive path scrape pulls in issue
// references, diff stats and URLs and calls them files. Every one of those
// would later be reported as a missing file, which is a finding invented
// out of punctuation.
func TestPaths_TakesFilesAndNothingElse(t *testing.T) {
	content := "**What**: a thing\n" +
		"**Where**: internal/promote/writer.go, .github/workflows/ci.yml (#139/#138), +111/-1, " +
		"https://example.com/a/b.go, README.md\n" +
		"**Learned**: nothing\n"

	got := staleness.Paths(content)
	want := map[string]bool{
		"internal/promote/writer.go": true,
		".github/workflows/ci.yml":   true,
		"README.md":                  true,
	}
	if len(got) != len(want) {
		t.Fatalf("extracted %v, want exactly %d file paths", got, len(want))
	}
	for _, p := range got {
		if !want[p] {
			t.Errorf("%q is not a file path this repository could hold", p)
		}
	}
}

// The finding the whole feature exists for: a memory describing something
// that was removed from the project AFTER the memory was written. Left
// unreported, an agent reads it and reintroduces what was extirpated.
func TestDetect_APathRemovedAfterTheMemoryWasWritten(t *testing.T) {
	repo := fixture(t)

	got := detect(t, repo, engram.Observation{
		ID: 1, Title: "how gone.go works",
		Content:   "**Where**: gone.go\n",
		UpdatedAt: "2026-01-01 00:00:00",
	})
	if len(got) != 1 {
		t.Fatalf("a memory describing a removed file must be reported: got %+v", got)
	}
	if len(got[0].Removed) != 1 || got[0].Removed[0].Commit == "" {
		t.Fatalf("the finding must carry the commit that removed it: %+v", got[0])
	}
}

// The inverse, and it matters just as much: a memory written AFTER the
// removal is very likely the record OF the removal. Reporting it would
// invite deleting the one memory that says the thing is gone -- the exact
// knowledge that stops it being reintroduced.
func TestDetect_APathRemovedBeforeTheMemoryIsNotStale(t *testing.T) {
	repo := fixture(t)

	got := detect(t, repo, engram.Observation{
		ID: 2, Title: "we removed gone.go on purpose",
		Content:   "**Where**: gone.go\n",
		UpdatedAt: "2030-01-01 00:00:00",
	})
	if len(got) != 0 {
		t.Fatalf("a memory written after the removal records it; reporting it invites deleting the very thing that prevents the revival: %+v", got)
	}
}

// The false positive measured on the real database: a path recorded
// relative to a different root. internal/promote/reconcile.go looked absent
// and is alive at longterm-mem/internal/promote/reconcile.go.
func TestDetect_APathRecordedUnderAnotherRootIsAlive(t *testing.T) {
	repo := fixture(t)

	// The fixture once held moved-root.go at the top level, deleted it, and
	// now holds module/moved-root.go. History therefore says "deleted" for
	// the recorded path, and only resolving it against the tree by suffix
	// shows the file is alive -- without that step this is a false report
	// of a removal, which is the finding that gets a live memory deleted.
	got := detect(t, repo, engram.Observation{
		ID: 3, Title: "about a file recorded without its module prefix",
		Content:   "**Where**: moved-root.go\n",
		UpdatedAt: "2026-01-01 00:00:00",
	})
	if len(got) != 0 {
		t.Fatalf("a file that exists under a different root is alive, not removed: %+v", got)
	}
}

// A rename leaves the old path absent and the content alive. The memory is
// out of date about WHERE, never wrong about WHAT.
func TestDetect_ARenameIsReportedAsMovedNotRemoved(t *testing.T) {
	repo := fixture(t)

	got := detect(t, repo, engram.Observation{
		ID: 4, Title: "about a moved file",
		Content:   "**Where**: pkg/moved.go\n",
		UpdatedAt: "2026-01-01 00:00:00",
	})
	if len(got) != 1 {
		t.Fatalf("a moved file is worth saying out loud: %+v", got)
	}
	if len(got[0].Removed) != 0 {
		t.Errorf("a rename must never be reported as a removal: %+v", got[0])
	}
	if len(got[0].Moved) != 1 || got[0].Moved[0].NewPath != "pkg/renamed.go" {
		t.Errorf("the move must say where it went: %+v", got[0].Moved)
	}
}

// The other false positive from the real database: observations naming
// another repository's files. This repository has nothing to say about
// them, and saying something anyway is inventing evidence.
func TestDetect_APathThisRepositoryNeverHadIsIgnored(t *testing.T) {
	repo := fixture(t)

	got := detect(t, repo, engram.Observation{
		ID: 5, Title: "about gentle-ai's code",
		Content:   "**Where**: internal/cli/review_facade.go\n",
		UpdatedAt: "2026-01-01 00:00:00",
	})
	if len(got) != 0 {
		t.Fatalf("another project's file is not evidence about this one: %+v", got)
	}
}

func detect(t *testing.T, repo string, obs ...engram.Observation) []staleness.Finding {
	t.Helper()
	got, err := staleness.Detect(repo, obs)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	return got
}

// fixture builds a repository where gone.go was deleted and pkg/moved.go
// renamed, both in 2027 -- between the "written before" and "written after"
// timestamps the tests use.
func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	git(t, root, "init", "-q", "-b", "main")
	write(t, root, "kept.go", "package a\n")
	write(t, root, "gone.go", "package a\n// distinctive body for rename detection to chew on\n")
	write(t, root, "pkg/moved.go", "package b\n// another distinctive body, long enough to match\n")
	write(t, root, "module/promote/writer.go", "package promote\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "first")

	write(t, root, "moved-root.go", "package a\n// moved to another root by delete-and-add, not a rename\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "add moved-root.go")

	git(t, root, "rm", "-q", "gone.go")
	commitAt(t, root, "2027-06-01T00:00:00Z", "remove gone.go")

	// Re-rooted: deleted where it was, added where it now lives, in
	// separate commits so rename detection cannot pair them.
	git(t, root, "rm", "-q", "moved-root.go")
	commitAt(t, root, "2027-06-03T00:00:00Z", "drop moved-root.go from the top level")
	write(t, root, "module/moved-root.go", "package a\n// moved to another root by delete-and-add, not a rename\n")
	git(t, root, "add", "-A")
	commitAt(t, root, "2027-06-04T00:00:00Z", "re-add moved-root.go under module/")
	git(t, root, "mv", "pkg/moved.go", "pkg/renamed.go")
	commitAt(t, root, "2027-06-02T00:00:00Z", "rename moved.go")
	return root
}

func commitAt(t *testing.T, dir, when, msg string) {
	t.Helper()
	cmd := exec.Command("git", "commit", "-q", "-m", msg)
	cmd.Dir = dir
	cmd.Env = append(gitEnv(), "GIT_AUTHOR_DATE="+when, "GIT_COMMITTER_DATE="+when)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}
}

func gitEnv() []string {
	return append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir, cmd.Env = dir, gitEnv()
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
		t.Fatalf("write: %v", err)
	}
}
