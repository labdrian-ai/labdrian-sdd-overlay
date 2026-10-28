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

// Question is the first commit of a branch that lacks anything S6 asks of a commit: what the stop hook asks for (own-rdd
// T6a, S6, L18; T6b6b, L66). A passive commit is never asked.
type Question struct {
	// Branch is the short name of the branch HEAD is on.
	Branch string
	// Head is the commit that lacks something and Base its first parent. Both are full object names, and they are the
	// arguments of `review bind` for the review to ask for, so that only that commit is asked for and never the whole
	// branch.
	Base, Head string
	// Needs is what review.Needs lists for Head: everything the commit lacks, in the order of S6 (d).
	Needs []review.Need
	// Risk is the value of the trailer of Head (review.RiskValue), and "" when the trailer fails.
	Risk string
	// Sweep is the repository-relative path of the latest sweep record of Head, and "" when it has none.
	Sweep string
}

// NextUnreviewed finds the first commit of the branch HEAD is on that lacks anything review.Needs lists, in the repository
// that holds dir (absolute), and says whether there is one (own-rdd T6a, T6b6b, S6, L18, L66). The commits are those of the
// first-parent line of HEAD that the default branch does not have (Git.Standing), oldest first. The facts of each come from
// every feature of the tree (factsOfAnyFeature); a commit is covered when the chain of stored reviews reaches it from the
// commit the branch was made on (review.FirstUnreviewed), and only the first commit the chain does not reach is asked for
// its review, the commits before it being asked for everything but that. A passive commit is a link of the chain from its
// first parent to itself and is never asked, so a review that starts at one is reached.
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
	listings := make([]featureListing, 0, len(features.Names))
	for _, feature := range features.Names {
		names, err := s.results.Names(repo.Root, feature)
		if err != nil {
			return Question{}, false, fmt.Errorf("list %s/%s: %w", review.ResultsDir, feature, err)
		}
		listings = append(listings, featureListing{feature: feature, names: names})
	}
	facts := make([]review.Facts, len(standing.Commits))
	parents := make([]string, len(standing.Commits))
	links := s.links(repo, listings, standing.Commits)
	for i, commit := range standing.Commits {
		facts[i], parents[i], err = s.factsOfAnyFeature(repo, listings, commit)
		if err != nil {
			return Question{}, false, fmt.Errorf("find what %s lacks: %w", commit, err)
		}
		if facts[i].Passive {
			links = append(links, review.Link{Base: parents[i], Head: commit})
		}
	}
	_, unreviewed, found := review.FirstUnreviewed(from, standing.Commits, links)
	for i, commit := range standing.Commits {
		if found && commit == unreviewed {
			facts[i].AskCoverage, facts[i].Coverage = true, review.ErrNoResult
		}
		needs := review.Needs(facts[i])
		if len(needs) == 0 {
			if found && commit == unreviewed {
				break
			}
			continue
		}
		q := Question{Branch: standing.Branch, Base: parents[i], Head: commit, Needs: needs, Sweep: facts[i].Sweep}
		if facts[i].RiskErr == nil {
			q.Risk = review.RiskValue(facts[i].Risk)
		}
		return q, true, nil
	}
	return Question{}, false, nil
}

// links are the ranges the stored reviews of the features cover for the commits: for each stored result or exemption
// that is read back and binds a commit, the base it was made against and the commit. A record that cannot be read or
// does not hold is no link.
func (s Service) links(repo Repository, listings []featureListing, commits []string) []review.Link {
	var links []review.Link
	for _, listing := range listings {
		for _, commit := range commits {
			for _, name := range review.ResultsOfHead(listing.names, commit) {
				if r, binds, err := s.reviewedResult(repo, listing.feature, name, commit); err == nil && binds {
					links = append(links, review.Link{Base: r.Candidate.Base, Head: commit})
				}
			}
			for _, name := range review.ExemptionsOfHead(listing.names, commit) {
				if e, binds, err := s.exemptedHead(repo, listing.feature, name, commit); err == nil && binds {
					links = append(links, review.Link{Base: e.Candidate.Base, Head: commit})
				}
			}
		}
	}
	return links
}
