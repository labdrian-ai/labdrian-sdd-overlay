package app_test

// What Proposals does (own-rdd T12e, S15 g): it tells what the owner may remove, the worktrees and the local branches
// merged into the default branch, and asks git nothing but reads. The repository of the fakes is the linked worktree
// /work/wt/feature of the main checkout /work/main.

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
)

const (
	mergedSHA   = "4444444444444444444444444444444444444444"
	unmergedSHA = "5555555555555555555555555555555555555555"
	goneSHA     = "6666666666666666666666666666666666666666"
)

// proposalsWorld is a repository whose default branch main is at headSHA and whose checked-out branch is feat/x.
func proposalsWorld() *fakeGit {
	g := newFakeGit()
	g.standing = app.Standing{Branch: "feat/x", Default: "main"}
	g.commitOf = map[string]string{"main": headSHA}
	g.worktrees = []review.Worktree{
		{Path: "/work/main", Head: headSHA},
		{Path: "/work/wt/feature", Head: mergedSHA},
		{Path: "/work/wt/review", Head: mergedSHA},
		{Path: "/work/wt/spike", Head: unmergedSHA},
		{Path: "/work/wt/locked", Head: mergedSHA, Locked: true},
		{Path: "/work/wt/gone", Head: goneSHA, Prunable: true},
	}
	g.ancestors = map[string]bool{mergedSHA: true}
	g.merged = []string{"main", "feat/x", "old-b", "old-a"}
	return g
}

func TestProposalsTellsTheMergedWorktreesTheStaleOnesAndTheMergedBranches(t *testing.T) {
	g := proposalsWorld()
	service := app.New(g, newFakeResults())

	got, err := service.Proposals(evidenceDir)

	want := review.Proposals{
		Default: "main",
		Worktrees: []review.WorktreeProposal{
			{Path: "/work/wt/gone", Head: goneSHA, Reason: "prunable", Command: []string{"git", "worktree", "prune"}},
			{Path: "/work/wt/review", Head: mergedSHA, Reason: "merged", Command: []string{"git", "worktree", "remove", "/work/wt/review"}},
		},
		Branches: []review.BranchProposal{
			{Name: "old-a", Command: []string{"git", "branch", "-d", "old-a"}},
			{Name: "old-b", Command: []string{"git", "branch", "-d", "old-b"}},
		},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("Proposals() = %+v, %v; want %+v, nil", got, err, want)
	}
}

// Nothing but reads: the only questions put to git are the ones that read, and none of them is about a worktree that
// could not be proposed.
func TestProposalsAsksGitNothingButReads(t *testing.T) {
	g := proposalsWorld()

	_, err := app.New(g, newFakeResults()).Proposals(evidenceDir)

	want := []string{
		"Locate " + evidenceDir, "Standing", "Commit main", "Worktrees",
		"IsAncestor " + mergedSHA + " " + headSHA, "IsAncestor " + unmergedSHA + " " + headSHA,
		"MergedBranches " + headSHA,
	}
	if err != nil || !reflect.DeepEqual(g.calls, want) {
		t.Errorf("calls = %q, %v; want %q", g.calls, err, want)
	}
}

func TestProposalsRefusesARepositoryWithNoDefaultBranchAndAsksNoMore(t *testing.T) {
	g := proposalsWorld()
	g.standing = app.Standing{Branch: "feat/x"}

	got, err := app.New(g, newFakeResults()).Proposals(evidenceDir)

	want := "the repository has no default branch (origin/HEAD, main or master), so nothing can be told merged"
	if !errors.Is(err, review.ErrNoDefaultBranch) || err.Error() != want || got.Default != "" || got.Worktrees != nil || got.Branches != nil {
		t.Errorf("Proposals() = %+v, %v; want nothing and %q wrapping ErrNoDefaultBranch", got, err, want)
	}
	if wantCalls := []string{"Locate " + evidenceDir, "Standing"}; !reflect.DeepEqual(g.calls, wantCalls) {
		t.Errorf("calls = %q, want %q", g.calls, wantCalls)
	}
}

// Standing tells nothing of the default branch for a detached HEAD, so the refusal says that and not that the
// repository has none.
func TestProposalsRefusesADetachedHeadWithoutSayingTheRepositoryHasNoDefaultBranch(t *testing.T) {
	g := proposalsWorld()
	g.standing = app.Standing{}

	_, err := app.New(g, newFakeResults()).Proposals(evidenceDir)

	if err == nil || errors.Is(err, review.ErrNoDefaultBranch) || !strings.Contains(err.Error(), "detached") {
		t.Errorf("Proposals() error = %v; want a refusal that names the detached HEAD and is not ErrNoDefaultBranch", err)
	}
}

func TestProposalsReportsWhatGitFailsToAnswer(t *testing.T) {
	boom := errors.New("boom")
	for _, call := range []string{"Locate", "Standing", "Commit main", "Worktrees", "IsAncestor", "MergedBranches"} {
		t.Run(call, func(t *testing.T) {
			g := proposalsWorld()
			g.fail[call] = boom

			got, err := app.New(g, newFakeResults()).Proposals(evidenceDir)

			if !errors.Is(err, boom) || got.Default != "" || got.Worktrees != nil || got.Branches != nil {
				t.Errorf("Proposals() = %+v, %v; want nothing and an error that wraps boom", got, err)
			}
		})
	}
}
