package main

// What `review proposals` prints (own-rdd T12e, S15 g). The tests run the verb over a repository they make in a
// temporary directory, with worktrees and branches of every kind the verb tells apart. The rules of the decision are
// in review/proposals_test.go and review/app/proposals_test.go; these tests are the command line, the output, and the
// two things the verb promises: nothing is deleted and nothing is stored.

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// proposalsWorld makes the leftovers of a feature in the world: a branch merged into main, a branch that is not, a
// worktree at a merged commit, one at a commit that is not merged, a locked one at a merged commit and one whose
// directory is gone. It returns the paths of the worktrees by name.
func (w *reviewWorld) proposalsWorld() map[string]string {
	w.t.Helper()
	dirs := map[string]string{}
	add := func(name, rev string) {
		dirs[name] = filepath.Join(w.parent, name)
		w.git("worktree", "add", "-q", "--detach", dirs[name], rev)
	}
	w.git("branch", "merged-b", w.base)
	w.git("checkout", "-q", "-b", "unmerged-b")
	unmerged := w.commitMore("u.txt")
	w.git("checkout", "-q", "main")
	add("wt-merged", w.base)
	add("wt-unmerged", unmerged)
	add("wt-locked", w.base)
	w.git("worktree", "lock", dirs["wt-locked"])
	add("wt-gone", w.base)
	if err := os.RemoveAll(dirs["wt-gone"]); err != nil {
		w.t.Fatalf("setup: %v", err)
	}
	return dirs
}

// untouched is everything the repository and its worktrees hold that a verb could change: its refs, its worktree list
// and the SHA-256 of every file under the temporary directory, the git directory included.
func (w *reviewWorld) untouched() string {
	w.t.Helper()
	var b strings.Builder
	b.WriteString(w.git("for-each-ref"))
	b.WriteString(w.git("worktree", "list", "--porcelain"))
	err := filepath.WalkDir(w.parent, func(p string, e fs.DirEntry, err error) error {
		if err != nil || !e.Type().IsRegular() {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		fmt.Fprintf(&b, "%s %x\n", p, sha256.Sum256(data))
		return nil
	})
	if err != nil {
		w.t.Fatalf("setup: %v", err)
	}
	return b.String()
}

func TestReviewProposalsPrintsTheMergedAndStaleWorktreesAndTheMergedBranchesAndChangesNothing(t *testing.T) {
	w := newCheckedWorld(t)
	dirs := w.proposalsWorld()
	before := w.untouched()

	stdout, stderr, exits := w.run("", "proposals")

	// The locked worktree and the one at a commit that is not merged are left out; the unmerged branch too.
	want := fmt.Sprintf(`{
  "default": "main",
  "worktrees": [
    {
      "path": %q,
      "head": %q,
      "reason": "prunable",
      "command": [
        "git",
        "worktree",
        "prune"
      ]
    },
    {
      "path": %q,
      "head": %q,
      "reason": "merged",
      "command": [
        "git",
        "worktree",
        "remove",
        %q
      ]
    }
  ],
  "branches": [
    {
      "name": "merged-b",
      "command": [
        "git",
        "branch",
        "-d",
        "merged-b"
      ]
    }
  ]
}
`, dirs["wt-gone"], w.base, dirs["wt-merged"], w.base, dirs["wt-merged"])
	if stdout != want || stderr != "" || !equalInts(exits, []int{0}) {
		t.Errorf("stdout = %q, stderr = %q, exits = %v; want %q, nothing, [0]", stdout, stderr, exits, want)
	}
	if after := w.untouched(); after != before {
		t.Errorf("the repository changed:\nbefore: %s\nafter:  %s", before, after)
	}
}

// Nothing to propose is two empty lists, never null.
func TestReviewProposalsPrintsEmptyListsForARepositoryWithNothingToClean(t *testing.T) {
	w := newCheckedWorld(t)

	stdout, stderr, exits := w.run("", "proposals")

	want := "{\n  \"default\": \"main\",\n  \"worktrees\": [],\n  \"branches\": []\n}\n"
	if stdout != want || stderr != "" || !equalInts(exits, []int{0}) {
		t.Errorf("stdout = %q, stderr = %q, exits = %v; want %q, nothing, [0]", stdout, stderr, exits, want)
	}
}

// The worktree of --cwd is never proposed, nor is the branch checked out in it, even when both are merged.
func TestReviewProposalsLeavesOutTheWorktreeAndTheBranchOfCwd(t *testing.T) {
	w := newCheckedWorld(t)
	dirs := w.proposalsWorld()
	here := filepath.Join(w.parent, "wt-here")
	w.git("worktree", "add", "-q", "-b", "here-b", here, w.base)

	stdout, stderr, exits := w.run("", "proposals", "--cwd", here)

	if stderr != "" || !equalInts(exits, []int{0}) || strings.Contains(stdout, here) || strings.Contains(stdout, `"here-b"`) ||
		!strings.Contains(stdout, dirs["wt-merged"]) || !strings.Contains(stdout, `"merged-b"`) {
		t.Errorf("stdout = %q, stderr = %q, exits = %v; want the others but neither the worktree nor the branch of --cwd, nothing on stderr, [0]", stdout, stderr, exits)
	}
}

func TestReviewProposalsRefusesARepositoryWithNoDefaultBranch(t *testing.T) {
	w := newCheckedWorld(t)
	w.git("branch", "-m", "main", "trunk")

	stdout, stderr, exits := w.run("", "proposals")

	want := "error: review proposals: the repository has no default branch (origin/HEAD, main or master), so nothing can be told merged\n"
	if stdout != "" || stderr != want || !equalInts(exits, []int{1}) {
		t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, %q, [1]", stdout, stderr, exits, want)
	}
}

func TestReviewProposalsRefusesACommandLineItDoesNotUnderstand(t *testing.T) {
	for name, args := range map[string][]string{
		"an unknown flag":        {"proposals", "--bogus"},
		"a flag of another verb": {"proposals", "--feature", "f"},
		"a positional argument":  {"proposals", "main"},
		"a flag with no value":   {"proposals", "--cwd"},
	} {
		t.Run(name, func(t *testing.T) {
			w := newCheckedWorld(t)

			stdout, stderr, exits := w.run("", args...)

			if stdout != "" || !strings.HasPrefix(stderr, "error: review proposals: ") || !strings.HasSuffix(stderr, "\n") || !equalInts(exits, []int{1}) {
				t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, an error starting %q, [1]", stdout, stderr, exits, "error: review proposals: ")
			}
		})
	}
}

// A directory that is no repository is exit 1 with the reason, as the other verbs word it.
func TestReviewProposalsRefusesADirectoryThatIsNoRepository(t *testing.T) {
	w := newCheckedWorld(t)
	dir := t.TempDir()

	stdout, stderr, exits := w.run("", "proposals", "--cwd", dir)

	if stdout != "" || !strings.HasPrefix(stderr, "error: review proposals: ") || !equalInts(exits, []int{1}) {
		t.Errorf("stdout = %q, stderr = %q, exits = %v; want nothing, an error starting %q, [1]", stdout, stderr, exits, "error: review proposals: ")
	}
}
