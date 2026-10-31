package app

import (
	"errors"
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
)

// Proposals tells what the owner may remove from the repository that holds dir: the worktrees whose head the default
// branch has, or that git marks prunable, and the local branches merged into the default branch (own-rdd T12e, S15 g).
// It asks git nothing but reads and stores nothing; running what it proposes is the owner's.
//
// A repository that does not tell its default branch is refused with review.ErrNoDefaultBranch, since nothing can be told
// merged. Standing tells nothing of the default branch for a detached HEAD, so that is refused apart, saying so.
func (s Service) Proposals(dir string) (review.Proposals, error) {
	repo, err := s.git.Locate(dir)
	if err != nil {
		return review.Proposals{}, err
	}
	standing, err := s.git.Standing(repo)
	if err != nil {
		return review.Proposals{}, err
	}
	if standing.Default == "" {
		if standing.Branch == "" {
			return review.Proposals{}, errors.New("HEAD is detached, so the default branch cannot be told; check out a branch and ask again")
		}
		return review.Proposals{}, review.RefuseNoDefaultBranch()
	}
	tip, err := s.git.Commit(repo, standing.Default)
	if err != nil {
		return review.Proposals{}, err
	}
	worktrees, err := s.git.Worktrees(repo)
	if err != nil {
		return review.Proposals{}, err
	}
	mergedHeads := map[string]bool{}
	for i, w := range worktrees {
		if i == 0 || w.Path == repo.Root || w.Locked || w.Prunable || w.Head == "" {
			continue
		}
		if _, asked := mergedHeads[w.Head]; asked {
			continue
		}
		merged, err := s.git.IsAncestor(repo, w.Head, tip)
		if err != nil {
			return review.Proposals{}, fmt.Errorf("tell whether %s, the head of the worktree %s, is merged: %w", w.Head, w.Path, err)
		}
		mergedHeads[w.Head] = merged
	}
	branches, err := s.git.MergedBranches(repo, tip)
	if err != nil {
		return review.Proposals{}, err
	}
	return review.Propose(standing.Default, standing.Branch, repo.Root, worktrees, mergedHeads, branches), nil
}
