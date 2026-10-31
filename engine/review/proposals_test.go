package review_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
)

func TestProposeNeverProposesTheMainWorktreeNorTheOneOfCwd(t *testing.T) {
	worktrees := []review.Worktree{
		{Path: "/r/main", Head: "aaa"},
		{Path: "/r/here", Head: "bbb"},
		{Path: "/r/other", Head: "ccc"},
	}
	merged := map[string]bool{"aaa": true, "bbb": true, "ccc": true}
	got := review.Propose("main", "feat/x", "/r/here", worktrees, merged, nil)
	want := review.Proposals{
		Default: "main",
		Worktrees: []review.WorktreeProposal{
			{Path: "/r/other", Head: "ccc", Reason: "merged", Command: []string{"git", "worktree", "remove", "/r/other"}},
		},
		Branches: []review.BranchProposal{},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Propose() = %+v, want %+v", got, want)
	}
}

func TestProposeNeverProposesALockedWorktree(t *testing.T) {
	worktrees := []review.Worktree{
		{Path: "/r/main", Head: "aaa"},
		{Path: "/r/locked", Head: "bbb", Locked: true},
		{Path: "/r/lockedgone", Head: "ccc", Locked: true, Prunable: true},
	}
	got := review.Propose("main", "main", "/r/main", worktrees, map[string]bool{"bbb": true}, nil)
	if len(got.Worktrees) != 0 {
		t.Errorf("Propose().Worktrees = %+v, want none", got.Worktrees)
	}
}

func TestProposePutsAPrunableWorktreeToPruneAndAMergedOneToRemoveInTheOrderOfTheirPaths(t *testing.T) {
	worktrees := []review.Worktree{
		{Path: "/r/main", Head: "aaa"},
		{Path: "/r/z-merged", Head: "bbb"},
		{Path: "/r/m-unmerged", Head: "ccc"},
		{Path: "/r/c-gone", Head: "bbb", Prunable: true},
		{Path: "/r/a-merged", Head: "ddd"},
	}
	merged := map[string]bool{"bbb": true, "ddd": true}
	got := review.Propose("main", "main", "/r/main", worktrees, merged, nil).Worktrees
	want := []review.WorktreeProposal{
		{Path: "/r/a-merged", Head: "ddd", Reason: "merged", Command: []string{"git", "worktree", "remove", "/r/a-merged"}},
		{Path: "/r/c-gone", Head: "bbb", Reason: "prunable", Command: []string{"git", "worktree", "prune"}},
		{Path: "/r/z-merged", Head: "bbb", Reason: "merged", Command: []string{"git", "worktree", "remove", "/r/z-merged"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Propose().Worktrees = %+v, want %+v", got, want)
	}
}

func TestProposeLeavesTheDefaultAndTheCurrentBranchOutAndSortsTheRest(t *testing.T) {
	got := review.Propose("main", "feat/now", "/r/main", nil, nil, []string{"main", "z", "feat/now", "a", "a", "m"}).Branches
	want := []review.BranchProposal{
		{Name: "a", Command: []string{"git", "branch", "-d", "a"}},
		{Name: "m", Command: []string{"git", "branch", "-d", "m"}},
		{Name: "z", Command: []string{"git", "branch", "-d", "z"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Propose().Branches = %+v, want %+v", got, want)
	}
}

func TestProposeNeverProposesWithForce(t *testing.T) {
	worktrees := []review.Worktree{{Path: "/r/main", Head: "a"}, {Path: "/r/w", Head: "b"}}
	got := review.Propose("main", "main", "/r/main", worktrees, map[string]bool{"b": true}, []string{"x"})
	for _, command := range [][]string{got.Worktrees[0].Command, got.Branches[0].Command} {
		for _, word := range command {
			if word == "--force" || word == "-f" || word == "-D" {
				t.Errorf("command %v holds %q, want git's own refusing form", command, word)
			}
		}
	}
}

func TestProposeEncodesEmptyListsAsArraysAndNeverNull(t *testing.T) {
	got, err := json.Marshal(review.Propose("main", "main", "/r/main", nil, nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"default":"main","worktrees":[],"branches":[]}`; string(got) != want {
		t.Errorf("json = %s, want %s", got, want)
	}
}

func TestProposeEncodesTheMembersAndNamesOfTheVerb(t *testing.T) {
	worktrees := []review.Worktree{{Path: "/r/main", Head: "a"}, {Path: "/r/w", Head: "b"}}
	got, err := json.Marshal(review.Propose("main", "main", "/r/main", worktrees, map[string]bool{"b": true}, []string{"x"}))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"default":"main","worktrees":[{"path":"/r/w","head":"b","reason":"merged","command":["git","worktree","remove","/r/w"]}],` +
		`"branches":[{"name":"x","command":["git","branch","-d","x"]}]}`
	if string(got) != want {
		t.Errorf("json = %s\nwant   %s", got, want)
	}
}

func TestRefuseNoDefaultBranchSaysTheSentenceOfTheVerbAndWrapsItsSentinel(t *testing.T) {
	err := review.RefuseNoDefaultBranch()
	want := "the repository has no default branch (origin/HEAD, main or master), so nothing can be told merged"
	if err.Error() != want || !errors.Is(err, review.ErrNoDefaultBranch) {
		t.Errorf("RefuseNoDefaultBranch() = %q, want %q wrapping ErrNoDefaultBranch", err, want)
	}
}
