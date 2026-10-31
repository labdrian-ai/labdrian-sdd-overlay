package review

import (
	"errors"
	"slices"
	"sort"
)

// The decision of `review proposals` (own-rdd T12e, S15 g): which worktrees and which local branches the owner may
// remove. It is pure; the facts come from git through the use case. It proposes, as commands for the owner to confirm,
// and nothing here deletes or stores anything.

// ErrNoDefaultBranch is a repository that does not tell its default branch (origin/HEAD, main or master), so nothing
// can be told merged.
var ErrNoDefaultBranch = errors.New("review: the repository has no default branch")

// RefuseNoDefaultBranch is the refusal of `review proposals` for a repository with no default branch: the sentence S15 (g)
// gives, wrapping ErrNoDefaultBranch.
func RefuseNoDefaultBranch() error {
	return &landingRefusal{
		text:     "the repository has no default branch (origin/HEAD, main or master), so nothing can be told merged",
		sentinel: ErrNoDefaultBranch,
	}
}

// Worktree is a working tree of the repository as `git worktree list` tells it. Head is the full object name checked
// out in it, and "" for a bare repository.
type Worktree struct {
	Path, Head       string
	Locked, Prunable bool
}

// The reasons a worktree is proposed.
const (
	ReasonMerged   = "merged"
	ReasonPrunable = "prunable"
)

// WorktreeProposal is a worktree the owner may remove, with the command that removes it.
type WorktreeProposal struct {
	Path    string   `json:"path"`
	Head    string   `json:"head"`
	Reason  string   `json:"reason"`
	Command []string `json:"command"`
}

// BranchProposal is a local branch merged into the default branch, with the command that deletes it.
type BranchProposal struct {
	Name    string   `json:"name"`
	Command []string `json:"command"`
}

// Proposals is what `review proposals` prints. Both lists are always there, empty when there is nothing to propose.
type Proposals struct {
	Default   string             `json:"default"`
	Worktrees []WorktreeProposal `json:"worktrees"`
	Branches  []BranchProposal   `json:"branches"`
}

// Propose decides what is proposed. def is the default branch and current the branch checked out at --cwd ("" when
// none); here is the root of --cwd. worktrees are in the order git lists them, the first being the main one. A worktree is
// proposed unless it is the main one, the one of here or a locked one; one git marks prunable is proposed with
// `git worktree prune`, and one whose head mergedHeads holds with `git worktree remove <path>`. merged are the local
// branches merged into the default branch; the default and current are left out. The commands are git's own refusing
// forms, never --force or -D. Worktrees are sorted by path and branches by name.
func Propose(def, current, here string, worktrees []Worktree, mergedHeads map[string]bool, merged []string) Proposals {
	out := Proposals{Default: def, Worktrees: []WorktreeProposal{}, Branches: []BranchProposal{}}
	for i, w := range worktrees {
		if i == 0 || w.Path == here || w.Locked {
			continue
		}
		switch {
		case w.Prunable:
			out.Worktrees = append(out.Worktrees, WorktreeProposal{Path: w.Path, Head: w.Head, Reason: ReasonPrunable, Command: []string{"git", "worktree", "prune"}})
		case w.Head != "" && mergedHeads[w.Head]:
			out.Worktrees = append(out.Worktrees, WorktreeProposal{Path: w.Path, Head: w.Head, Reason: ReasonMerged, Command: []string{"git", "worktree", "remove", w.Path}})
		}
	}
	sort.SliceStable(out.Worktrees, func(i, j int) bool { return out.Worktrees[i].Path < out.Worktrees[j].Path })
	names := slices.Clone(merged)
	sort.Strings(names)
	for i, name := range names {
		if name == def || name == current || (i > 0 && names[i-1] == name) {
			continue
		}
		out.Branches = append(out.Branches, BranchProposal{Name: name, Command: []string{"git", "branch", "-d", name}})
	}
	return out
}
