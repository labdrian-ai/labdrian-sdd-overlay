package app

import (
	"errors"
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
)

// ErrNoDefaultBranch is what NextUnreviewed returns, wrapped, for a repository that does not tell its default branch:
// without it there is no telling which commits of the branch are its own.
var ErrNoDefaultBranch = errors.New("review: the repository does not tell its default branch")

// NoDefaultBranchError is the refusal NextUnreviewed makes for a repository that does not tell its default branch: it
// names the branch HEAD is on, and errors.Is reads it as ErrNoDefaultBranch.
type NoDefaultBranchError struct{ Branch string }

func (e *NoDefaultBranchError) Error() string {
	return fmt.Sprintf("%v: neither origin/HEAD, main nor master tells which commits of branch %s are its own", ErrNoDefaultBranch, e.Branch)
}

func (e *NoDefaultBranchError) Unwrap() error { return ErrNoDefaultBranch }

// Question is the next commit of a branch that no review covers: what the stop hook asks for (own-rdd T6a, S6, L18).
type Question struct {
	// Branch is the short name of the branch HEAD is on.
	Branch string
	// Head is the commit to review and Base the commit before it in the chain of the branch: the last commit a
	// review reached, or the commit the branch was made on when none is. Both are full object names, and they are the
	// arguments of `review bind` for the review to ask for, so that only the next commit is asked for and never the
	// whole branch.
	Base, Head string
}

// NextUnreviewed finds the next commit of the branch HEAD is on that no review covers, in the repository that holds dir
// (absolute), and says whether there is one (own-rdd T6a, S6, L18). The commits are those of the first-parent line of
// HEAD that the default branch does not have (Git.Standing), oldest first, and a commit is covered when the chain of
// stored reviews reaches it from the commit the branch was made on (review.FirstUnreviewed).
//
// A review is a stored result or exemption of any feature of the tree (Results.Features) that is read back and held to
// git again, as Check holds it, whatever it made of the review: a commit whose review blocks, or is not clear, is not
// asked for again, because the question is whether it was reviewed and 'review check' is the judge of how it turned
// out. A record that cannot be read or does not hold is no review of the commit; one that is read back and does not
// bind the commit is none either.
//
// There is nothing to ask in a tree that keeps no features, on a detached HEAD, on the default branch, behind it, and
// when the chain reaches the last commit. A repository that does not tell its default branch is an error that wraps
// ErrNoDefaultBranch and names the branch; so is whatever git or the store cannot tell, and a first commit with no
// parent, which leaves no base to name. Nothing is stored.
func (s Service) NextUnreviewed(dir string) (Question, bool, error) {
	repo, err := s.git.Locate(dir)
	if err != nil {
		return Question{}, false, fmt.Errorf("locate the repository of %s: %w", dir, err)
	}
	features, err := s.results.Features(repo.Root)
	if err != nil {
		return Question{}, false, fmt.Errorf("read the features of %s: %w", repo.Root, err)
	}
	if !features.Tracked {
		return Question{}, false, nil
	}
	standing, err := s.git.Standing(repo)
	if err != nil {
		return Question{}, false, fmt.Errorf("find where HEAD stands: %w", err)
	}
	switch {
	case standing.Branch == "":
		return Question{}, false, nil
	case standing.Default == "":
		return Question{}, false, &NoDefaultBranchError{Branch: standing.Branch}
	case len(standing.Commits) == 0:
		return Question{}, false, nil
	}
	first := standing.Commits[0]
	from, err := s.git.Parent(repo, first)
	if err != nil {
		return Question{}, false, fmt.Errorf("find the parent of %s: %w", first, err)
	}
	if from == "" {
		return Question{}, false, fmt.Errorf("the first commit of branch %s, %s, has no parent, so no base can be named for it", standing.Branch, first)
	}
	links, err := s.links(repo, features.Names, standing.Commits)
	if err != nil {
		return Question{}, false, err
	}
	base, head, found := review.FirstUnreviewed(from, standing.Commits, links)
	if !found {
		return Question{}, false, nil
	}
	return Question{Branch: standing.Branch, Base: base, Head: head}, true, nil
}

// links are the ranges the stored reviews of the features cover for the commits: for each stored result or exemption
// that is read back and binds a commit, the base it was made against and the commit.
func (s Service) links(repo Repository, features, commits []string) ([]review.Link, error) {
	var links []review.Link
	for _, feature := range features {
		names, err := s.results.Names(repo.Root, feature)
		if err != nil {
			return nil, fmt.Errorf("list %s/%s: %w", review.ResultsDir, feature, err)
		}
		for _, commit := range commits {
			for _, name := range review.ResultsOfHead(names, commit) {
				if r, binds, err := s.reviewedResult(repo, feature, name, commit); err == nil && binds {
					links = append(links, review.Link{Base: r.Candidate.Base, Head: commit})
				}
			}
			for _, name := range review.ExemptionsOfHead(names, commit) {
				if e, binds, err := s.exemptedHead(repo, feature, name, commit); err == nil && binds {
					links = append(links, review.Link{Base: e.Candidate.Base, Head: commit})
				}
			}
		}
	}
	return links, nil
}
