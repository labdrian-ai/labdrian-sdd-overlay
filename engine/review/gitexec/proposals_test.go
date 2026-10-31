package gitexec_test

// The three answers `review proposals` asks of git (own-rdd T12e, S15 g), on real repositories: the worktrees of the
// repository, the local branches merged into a commit, and whether one commit is an ancestor of another.

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
)

// proposalsRepo is a repository with main two commits ahead of a merged branch, an unmerged branch, and worktrees of every
// kind the verb tells apart.
type proposalsRepo struct {
	*fixture
	first, second, unmerged string
	dir                     map[string]string // the path of each worktree, by the name the test gave it
}

func newProposalsRepo(t *testing.T) *proposalsRepo {
	t.Helper()
	f := newFixture(t, "sha1")
	r := &proposalsRepo{fixture: f, dir: map[string]string{}}
	f.write("a.txt", "one\n")
	f.stage()
	r.first = f.snapshot("first")
	f.git("branch", "merged-branch")
	f.git("checkout", "-q", "-b", "unmerged-branch")
	f.write("b.txt", "side\n")
	f.stage()
	r.unmerged = f.snapshot("side")
	f.git("checkout", "-q", "main")
	f.write("a.txt", "two\n")
	f.stage()
	r.second = f.snapshot("second")

	add := func(name string, args ...string) {
		p := filepath.Join(resolved(t, t.TempDir()), name)
		r.dir[name] = p
		f.git(append([]string{"worktree", "add", "-q", "--detach", p}, args...)...)
	}
	add("merged-wt", r.first)
	add("unmerged-wt", r.unmerged)
	add("locked-wt", r.first)
	f.git("worktree", "lock", r.dir["locked-wt"])
	add("gone-wt", r.first)
	if err := os.RemoveAll(r.dir["gone-wt"]); err != nil {
		t.Fatalf("setup: %v", err)
	}
	return r
}

func TestWorktreesListsTheMainFirstAndTellsLockedAndPrunableOnes(t *testing.T) {
	r := newProposalsRepo(t)

	got, err := r.adapter().Worktrees(r.repo())

	if err != nil {
		t.Fatalf("Worktrees() error = %v; want nil", err)
	}
	if len(got) != 5 || got[0] != (review.Worktree{Path: r.root, Head: r.second}) {
		t.Fatalf("Worktrees() = %+v; want five, the main one first as {%s %s}", got, r.root, r.second)
	}
	byPath := map[string]review.Worktree{}
	for _, w := range got {
		byPath[w.Path] = w
	}
	want := map[string]review.Worktree{
		"merged-wt":   {Path: r.dir["merged-wt"], Head: r.first},
		"unmerged-wt": {Path: r.dir["unmerged-wt"], Head: r.unmerged},
		"locked-wt":   {Path: r.dir["locked-wt"], Head: r.first, Locked: true},
		"gone-wt":     {Path: r.dir["gone-wt"], Head: r.first, Prunable: true},
	}
	for name, w := range want {
		if byPath[w.Path] != w {
			t.Errorf("Worktrees() has %+v for %s; want %+v", byPath[w.Path], name, w)
		}
	}
}

func TestWorktreesTakesAPathWithALineBreakAndASpaceWhole(t *testing.T) {
	r := newProposalsRepo(t)
	odd := filepath.Join(resolved(t, t.TempDir()), "a b\nc")
	r.git("worktree", "add", "-q", "--detach", odd, r.first)

	got, err := r.adapter().Worktrees(r.repo())

	found := false
	for _, w := range got {
		found = found || w.Path == odd
	}
	if err != nil || !found {
		t.Errorf("Worktrees() = %+v, %v; want a worktree at %q", got, err, odd)
	}
}

func TestWorktreesIsAnErrorForARecordOfAnotherShape(t *testing.T) {
	for name, out := range map[string]string{
		"no worktree line": "HEAD " + strings.Repeat("a", 40) + "\x00\x00",
		"no HEAD line":     "worktree /r\x00branch refs/heads/main\x00\x00",
		"an unknown field": "worktree /r\x00HEAD " + strings.Repeat("a", 40) + "\x00surprise\x00\x00",
		"a bad HEAD":       "worktree /r\x00HEAD nonsense\x00\x00",
		"no list at all":   "",
	} {
		t.Run(name, func(t *testing.T) {
			r := &fakeRunner{stdout: []byte(out)}

			got, err := newGit(r).Worktrees(app.Repository{Root: "/r"})

			if err == nil {
				t.Errorf("Worktrees() = %+v, nil; want an error", got)
			}
		})
	}
}

func TestMergedBranchesListsTheBranchesMergedIntoACommitAndNotTheOthers(t *testing.T) {
	r := newProposalsRepo(t)

	got, err := r.adapter().MergedBranches(r.repo(), r.second)

	want := []string{"main", "merged-branch"}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("MergedBranches() = %q, %v; want %q, nil", got, err, want)
	}
}

func TestIsAncestorIsTrueForAMergedCommitFalseForAnUnmergedOneAndAnErrorForAnObjectGitDoesNotHave(t *testing.T) {
	r := newProposalsRepo(t)
	g := r.adapter()

	for _, tc := range []struct {
		name, a, b string
		want       bool
	}{
		{"an ancestor", r.first, r.second, true},
		{"the commit itself", r.second, r.second, true},
		{"a commit that is not", r.unmerged, r.second, false},
		{"a descendant", r.second, r.first, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := g.IsAncestor(r.repo(), tc.a, tc.b)

			if err != nil || got != tc.want {
				t.Errorf("IsAncestor(%s, %s) = %v, %v; want %v, nil", tc.a, tc.b, got, err, tc.want)
			}
		})
	}
	if got, err := g.IsAncestor(r.repo(), strings.Repeat("0", 40), r.second); err == nil {
		t.Errorf("IsAncestor() of an object that is not there = %v, nil; want an error", got)
	}
	if got, err := g.IsAncestor(r.repo(), "main", r.second); err == nil {
		t.Errorf("IsAncestor() of a name that is not a full object name = %v, nil; want an error", got)
	}
}
