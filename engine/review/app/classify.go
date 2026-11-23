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

// keptResultsOf lists the stored results of the commits of the branch HEAD is on, in a repository already located,
// that have one kept finding or more (S6). The commits are those NextUnreviewed reads (Git.Standing), oldest first; in
// each commit every feature of the tree in order, and in each feature the results of the commit by number. The findings
// are those of keptFindings, with the verdict stored beside the result, both read once (heldResult).
//
// Only a result that is about another commit than the one whose name it carries (its binding says so, and only the
// first 12 characters of its name said it was about this one) and a result with no kept finding are skipped. A result
// that cannot be read, that does not hold, or that does not bind its commit is an error with no result, as 'review
// check' refuses the same file; so is whatever git or the store cannot tell.
//
// There is nothing in a tree that keeps no features, on a detached HEAD, on a repository that does not tell its
// default branch, and on a branch with no commit of its own, and that is no error. Nothing is stored.
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
				h, binds, err := s.heldResult(repo, listing.feature, name, commit)
				if err != nil {
					return nil, err
				}
				if !binds {
					continue
				}
				kept, _ := keptFindings(h.result, h.verdicts)
				if len(kept) == 0 {
					continue
				}
				out = append(out, KeptResult{Review: reviewID(listing.feature, name), Head: commit, Kept: len(kept)})
			}
		}
	}
	return out, nil
}

// classifyKey names, in the questions of a session, the question that asks for the classification of the result
// called review: classify- and the first 16 hex digits of the SHA-256 of the identifier, 25 bytes of lowercase letters
// and digits, a key of the store of the questions whatever the feature is called.
func classifyKey(review string) string {
	return "classify-" + sha256hex.Sum([]byte(review))[:16]
}

// classifiedKey names, in the questions of a session, the finding that the page holds the classification of the result
// called review: classified- and the first 16 hex digits of the SHA-256 of the identifier, 27 bytes of lowercase
// letters, digits and dashes, a key of the same store as classifyKey's.
func classifiedKey(review string) string {
	return "classified-" + sha256hex.Sum([]byte(review))[:16]
}

// classifyRuling is the block that asks for the classification of the results of dir that the page lacks, or no block.
// Each result of keptResultsOf is checked in order. One the session found classified (classifiedKey) is not asked of
// the port again; the port is asked for the rest, and a result it says is classified is remembered for the session.
// Every result still lacking goes to once with its classifyKey: the stop is blocked when one of those questions is new
// to the session, and the block names every result still lacking, the ones asked before included, as a review
// question names every need. An error of keptResultsOf (nothing a stored result could not give is skipped), of the
// port or of the store of the questions is returned with no block.
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
		known, err := s.asked.Has(session, classifiedKey(result.Review))
		if err != nil {
			return Ruling{}, fmt.Errorf("read the questions asked in the session: %w", err)
		}
		if known {
			continue
		}
		done, err := s.classified.Classified(repo.Root, result.Review)
		if err != nil {
			return Ruling{}, fmt.Errorf("tell whether %s is classified: %w", result.Review, err)
		}
		if done {
			if err := s.asked.Add(session, classifiedKey(result.Review)); err != nil {
				return Ruling{}, fmt.Errorf("record that %s is classified: %w", result.Review, err)
			}
			continue
		}
		lacking, keys = append(lacking, result), append(keys, classifyKey(result.Review))
	}
	if len(lacking) == 0 {
		return Ruling{}, nil
	}
	return s.once(session, keys, Ruling{Block: true, Classify: lacking})
}
