package repoevidence_test

import (
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/repoevidence"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/staleness"
)

// The cases below run the staleness rule over the real evidence, a directory and a git repository: the rule's own
// cases (internal/staleness) run it over a table, and these show that the adapter and the rule agree on what a
// repository looks like.

// fixture builds a repository where gone.go was deleted and pkg/moved.go renamed, both in 2027 -- between the
// "written before" and "written after" timestamps the tests use.
func fixture(t *testing.T) string {
	t.Helper()
	requireGit(t)
	root := realDir(t)
	git(t, root, "", "init", "-q", "-b", "main")
	write(t, root, "kept.go", "package a\n")
	write(t, root, "gone.go", "package a\n// distinctive body for rename detection to chew on\n")
	write(t, root, "pkg/moved.go", "package b\n// another distinctive body, long enough to match\n")
	write(t, root, "module/promote/writer.go", "package promote\n")
	git(t, root, "", "add", "-A")
	git(t, root, "", "commit", "-q", "-m", "first")

	write(t, root, "moved-root.go", "package a\n// moved to another root by delete-and-add, not a rename\n")
	git(t, root, "", "add", "-A")
	git(t, root, "", "commit", "-q", "-m", "add moved-root.go")

	git(t, root, "", "rm", "-q", "gone.go")
	git(t, root, "2027-06-01T00:00:00Z", "commit", "-q", "-m", "remove gone.go")

	// Re-rooted: deleted where it was, added where it now lives, in separate commits so rename detection
	// cannot pair them.
	git(t, root, "", "rm", "-q", "moved-root.go")
	git(t, root, "2027-06-03T00:00:00Z", "commit", "-q", "-m", "drop moved-root.go from the top level")
	write(t, root, "module/moved-root.go", "package a\n// moved to another root by delete-and-add, not a rename\n")
	git(t, root, "", "add", "-A")
	git(t, root, "2027-06-04T00:00:00Z", "commit", "-q", "-m", "re-add moved-root.go under module/")
	git(t, root, "", "mv", "pkg/moved.go", "pkg/renamed.go")
	git(t, root, "2027-06-02T00:00:00Z", "commit", "-q", "-m", "rename moved.go")
	return root
}

func detect(t *testing.T, repo string, obs ...memory.Observation) []staleness.Finding {
	t.Helper()
	got, err := staleness.Detect(repoevidence.At(repo), obs)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	return got
}

func TestClassifyPaths_UsesTreeAndHistory(t *testing.T) {
	repo := fixture(t)

	got, err := staleness.ClassifyPaths(repoevidence.At(repo), []string{"kept.go", "gone.go", "pkg/moved.go", "never-was.go"})

	if err != nil {
		t.Fatalf("ClassifyPaths: %v", err)
	}
	cases := map[string]staleness.PathState{
		"kept.go":      staleness.PathPresent,
		"gone.go":      staleness.PathDeleted,
		"pkg/moved.go": staleness.PathRenamed,
		"never-was.go": staleness.PathUnknown,
	}
	for path, want := range cases {
		fact, ok := got[path]
		if !ok {
			t.Errorf("ClassifyPaths omitted %q", path)
			continue
		}
		if fact.State != want {
			t.Errorf("ClassifyPaths[%q].State = %q, want %q (fact=%+v)", path, fact.State, want, fact)
		}
	}
	if got["pkg/moved.go"].NewPath != "pkg/renamed.go" {
		t.Errorf("ClassifyPaths[pkg/moved.go].NewPath = %q, want pkg/renamed.go", got["pkg/moved.go"].NewPath)
	}
}

func TestDetect_APathRemovedAfterTheMemoryWasWritten(t *testing.T) {
	repo := fixture(t)

	got := detect(t, repo, memory.Observation{
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

func TestDetect_APathRemovedBeforeTheMemoryIsNotStale(t *testing.T) {
	repo := fixture(t)

	got := detect(t, repo, memory.Observation{
		ID: 2, Title: "we removed gone.go on purpose",
		Content:   "**Where**: gone.go\n",
		UpdatedAt: "2030-01-01 00:00:00",
	})
	if len(got) != 0 {
		t.Fatalf("a memory written after the removal records it; reporting it invites deleting the very thing that prevents the revival: %+v", got)
	}
}

// The false positive measured on the real database: a path recorded relative to a different root. The fixture
// once held moved-root.go at the top level, deleted it, and now holds module/moved-root.go. History therefore says
// "deleted" for the recorded path, and only resolving it against the tree by suffix shows the file is alive.
func TestDetect_APathRecordedUnderAnotherRootIsAlive(t *testing.T) {
	repo := fixture(t)

	got := detect(t, repo, memory.Observation{
		ID: 3, Title: "about a file recorded without its module prefix",
		Content:   "**Where**: moved-root.go\n",
		UpdatedAt: "2026-01-01 00:00:00",
	})
	if len(got) != 0 {
		t.Fatalf("a file that exists under a different root is alive, not removed: %+v", got)
	}
}

func TestDetect_ARenameIsReportedAsMovedNotRemoved(t *testing.T) {
	repo := fixture(t)

	got := detect(t, repo, memory.Observation{
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

func TestDetect_APathThisRepositoryNeverHadIsIgnored(t *testing.T) {
	repo := fixture(t)

	got := detect(t, repo, memory.Observation{
		ID: 5, Title: "about gentle-ai's code",
		Content:   "**Where**: internal/cli/review_facade.go\n",
		UpdatedAt: "2026-01-01 00:00:00",
	})
	if len(got) != 0 {
		t.Fatalf("another project's file is not evidence about this one: %+v", got)
	}
}

// A path that climbs out of the project would be refused by git, and the refusal would be the answer for the
// whole report; it is not asked (the owner's decision of 2026-10-13: such a path is unknown, and the rest of
// the report prints). Over the real repository: the report is what the other paths of the memory say.
func TestDetect_APathThatClimbsOutOfTheProjectIsSkippedNotRefused(t *testing.T) {
	repo := fixture(t)

	got, err := staleness.Detect(repoevidence.At(repo), []memory.Observation{{
		ID: 6, Title: "names a path above the project",
		Content:   "**Where**: kept.go, a/../../outside.go, gone.go\n",
		UpdatedAt: "2026-01-01 00:00:00",
	}})

	if err != nil {
		t.Fatalf("Detect: %v; want the path that climbs out skipped", err)
	}
	if len(got) != 1 || len(got[0].Removed) != 1 || got[0].Removed[0].Path != "gone.go" {
		t.Fatalf("Detect = %+v, want the one finding the other paths give (gone.go removed)", got)
	}
}

// A tree that cannot be listed is the answer, in the words the report has always used.
func TestDetect_ATreeThatIsNotThereFailsTheReport(t *testing.T) {
	root := realDir(t)
	absent := root + "/absent"

	_, err := staleness.Detect(repoevidence.At(absent), nil)

	if want := "staleness: " + absent + " does not exist"; err == nil || err.Error() != want {
		t.Errorf("Detect = %v, want %q", err, want)
	}
}
