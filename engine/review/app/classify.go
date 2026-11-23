package app

import (
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/sha256hex"
)

// Classified is the port that tells whether a stored result has been classified into the evidence page of a working
// tree (S6): the learning loop counts a result as classified when the page holds an Evidence row whose Review is the
// result's identifier.
type Classified interface {
	// Classified says whether the evidence page of the working tree at root holds an Evidence row whose Review is
	// review, <feature>/<head12>-<n>. An error is a page or a script that cannot say, never "not classified".
	Classified(root, review string) (bool, error)
}

// KeptResult is a stored result of a commit of the branch that has findings which count as evidence (the rules of
// Findings, shared in keptFindings).
type KeptResult struct {
	// Review is the identifier of the result, <feature>/<head12>-<n>.
	Review string
	// Head is the full object name of the commit the result reviews.
	Head string
	// Kept is how many of its findings count as evidence; never less than one.
	Kept int
}

// WithClassified is the Service that asks Classified whether a result with kept findings is classified, which Stop does
// once it has no review to ask for. Without one Stop asks for no classification.
func (s Service) WithClassified(c Classified) Service {
	s.classified = c
	return s
}

// KeptResults lists the stored results of the commits of the branch HEAD is on, in the repository that holds dir
// (absolute), that have one kept finding or more (S6). The commits are those NextUnreviewed reads (Git.Standing),
// oldest first; in each commit every feature of the tree in order, and in each feature the results of the commit by
// number. A result counts when it is read back and binds its commit, as the links of NextUnreviewed count it; one that
// cannot be read or does not bind is skipped, not an error, since the question is which results the page lacks and
// 'review check' judges the rest. The findings are those of keptFindings, with the verdict stored beside the result.
//
// There is nothing in a tree that keeps no features, on a detached HEAD, on a repository that does not tell its
// default branch, and on a branch with no commit of its own, and that is no error. What git or the store cannot tell is
// an error. Nothing is stored.
func (s Service) KeptResults(dir string) ([]KeptResult, error) {
	repo, err := s.git.Locate(dir)
	if err != nil {
		return nil, fmt.Errorf("locate the repository of %s: %w", dir, err)
	}
	return s.keptResultsOf(repo)
}

// keptResultsOf is KeptResults for a repository already located.
func (s Service) keptResultsOf(repo Repository) ([]KeptResult, error) {
	features, err := s.results.Features(repo.Root)
	if err != nil {
		return nil, fmt.Errorf("read the features of %s: %w", repo.Root, err)
	}
	if !features.Tracked {
		return nil, nil
	}
	standing, err := s.git.Standing(repo)
	if err != nil {
		return nil, fmt.Errorf("find where HEAD stands: %w", err)
	}
	if standing.Branch == "" || standing.Default == "" || len(standing.Commits) == 0 {
		return nil, nil
	}
	listings := make([]featureListing, 0, len(features.Names))
	for _, feature := range features.Names {
		names, err := s.results.Names(repo.Root, feature)
		if err != nil {
			return nil, fmt.Errorf("list %s/%s: %w", review.ResultsDir, feature, err)
		}
		listings = append(listings, featureListing{feature: feature, names: names})
	}
	var out []KeptResult
	for _, commit := range standing.Commits {
		for _, listing := range listings {
			for _, name := range review.ResultsOfHead(listing.names, commit) {
				if _, binds, err := s.reviewedResult(repo, listing.feature, name, commit); err != nil || !binds {
					continue
				}
				kept, err := s.keptOf(repo, listing.feature, name)
				if err != nil || kept == 0 {
					continue
				}
				out = append(out, KeptResult{Review: reviewID(listing.feature, name), Head: commit, Kept: kept})
			}
		}
	}
	return out, nil
}

// keptOf is how many findings of the stored result called name count as evidence, with the verdict stored beside it.
func (s Service) keptOf(repo Repository, feature, name string) (int, error) {
	file, err := s.results.Read(repo.Root, feature, name)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", review.ResultPath(feature, name), err)
	}
	_, result, err := judged(file)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", review.ResultPath(feature, name), err)
	}
	verdicts, _, err := s.verdictsOf(repo, feature, name, result)
	if err != nil {
		return 0, err
	}
	kept, _ := keptFindings(result, verdicts)
	return len(kept), nil
}

// classifyKey names, in the questions of a session, the question that asks for the classification of the result
// called review: classify- and the first 16 hex digits of the SHA-256 of the identifier, 25 bytes of lowercase letters
// and digits, a key of the store of the questions whatever the feature is called.
func classifyKey(review string) string {
	return "classify-" + sha256hex.Sum([]byte(review))[:16]
}

// classifyRuling is the block that asks for the classification of the results of dir the session has not been asked
// about and the page lacks, or no block. A result whose question the session holds is dropped before the port runs, so
// no script runs for it; the port is asked for the rest, and what it says is not classified is asked once (once). An
// error of KeptResults, of the port or of the store of the questions is returned with no block.
func (s Service) classifyRuling(dir, session string) (Ruling, error) {
	repo, err := s.git.Locate(dir)
	if err != nil {
		return Ruling{}, fmt.Errorf("locate the repository of %s: %w", dir, err)
	}
	results, err := s.keptResultsOf(repo)
	if err != nil {
		return Ruling{}, fmt.Errorf("find the results with kept findings: %w", err)
	}
	var (
		lacking []KeptResult
		keys    []string
	)
	for _, result := range results {
		key := classifyKey(result.Review)
		before, err := s.asked.Has(session, key)
		if err != nil {
			return Ruling{}, fmt.Errorf("read the questions asked in the session: %w", err)
		}
		if before {
			continue
		}
		done, err := s.classified.Classified(repo.Root, result.Review)
		if err != nil {
			return Ruling{}, fmt.Errorf("tell whether %s is classified: %w", result.Review, err)
		}
		if !done {
			lacking, keys = append(lacking, result), append(keys, key)
		}
	}
	if len(lacking) == 0 {
		return Ruling{}, nil
	}
	return s.once(session, keys, Ruling{Block: true, Classify: lacking})
}
