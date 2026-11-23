package app

import (
	"fmt"
	"strings"

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
	// Stamp is the state of everything Classified(root, review) reads, for any review: equal stamps give equal answers,
	// so Stop asks Classified again only when the stamp changed. An error is a state that cannot be read.
	Stamp(root string) (string, error)
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
// that cannot be read, that does not hold, or that does not bind its commit is a refusal, the second value, as 'review
// check' refuses the same file; it is left out and the pass goes on, so one bad file hides no other result. What the
// pass as a whole cannot tell (the features, where HEAD stands, the names of a feature's results) is the third value,
// returned with no result.
//
// There is nothing in a tree that keeps no features, on a detached HEAD, on a repository that does not tell its
// default branch, and on a branch with no commit of its own, and that is no error. Nothing is stored.
func (s Service) keptResultsOf(repo Repository) ([]KeptResult, []error, error) {
	features, err := s.results.Features(repo.Root)
	if err != nil {
		return nil, nil, fmt.Errorf("read the features of %s: %w", repo.Root, err)
	}
	if !features.Tracked {
		return nil, nil, nil
	}
	standing, err := s.git.Standing(repo)
	if err != nil {
		return nil, nil, fmt.Errorf("find where HEAD stands: %w", err)
	}
	if standing.Branch == "" || standing.Default == "" || len(standing.Commits) == 0 {
		return nil, nil, nil
	}
	listings := make([]featureListing, 0, len(features.Names))
	for _, feature := range features.Names {
		names, err := s.results.Names(repo.Root, feature)
		if err != nil {
			return nil, nil, fmt.Errorf("list %s/%s: %w", review.ResultsDir, feature, err)
		}
		listings = append(listings, featureListing{feature: feature, names: names})
	}
	var (
		out     []KeptResult
		refused []error
	)
	for _, commit := range standing.Commits {
		for _, listing := range listings {
			for _, name := range review.ResultsOfHead(listing.names, commit) {
				h, binds, err := s.heldResult(repo, listing.feature, name, commit)
				if err != nil {
					refused = append(refused, err)
					continue
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
	return out, refused, nil
}

// incomplete is the problems of a classification pass that did not stop it: the results it refused, and the records of
// its memory that failed. It is one line, the problems joined by "; ", since the hook prints an error as one line of
// stderr, and errors.Join would break it with a line break. Unwrap gives each problem, so errors.Is reaches every cause.
type incomplete []error

func (e incomplete) Error() string {
	texts := make([]string, len(e))
	for i, problem := range e {
		texts[i] = problem.Error()
	}
	return "classify the results with kept findings: " + strings.Join(texts, "; ")
}

func (e incomplete) Unwrap() []error { return e }

// withIncomplete is err with the problems that did not stop the pass: no error when there is neither, the problems alone
// when err is nil, and err and then the problems otherwise.
func withIncomplete(err error, problems []error) error {
	switch {
	case len(problems) == 0:
		return err
	case err == nil:
		return incomplete(problems)
	}
	return fmt.Errorf("%w; %w", err, incomplete(problems))
}

// classifyKey names, in the questions of a session, the question that asks for the classification of the result
// called review: classify- and the first 16 hex digits of the SHA-256 of the identifier, 25 bytes of lowercase letters
// and digits, a key of the store of the questions whatever the feature is called.
func classifyKey(review string) string {
	return "classify-" + sha256hex.Sum([]byte(review))[:16]
}

// The prefixes of the memory of a session: what the port said about a result, at a page state.
const (
	classifiedPrefix = "classified-"
	lackingPrefix    = "lacking-"
)

// memoKey names, in the questions of a session, what the port said of the result called review when the page was in
// the state stamp: the prefix (classifiedPrefix or lackingPrefix) and the first 16 hex digits of the SHA-256 of the
// identifier, a NUL and the stamp, 27 or 24 bytes of lowercase letters, digits and dashes, a key of the same store as
// classifyKey's. A page that changed has another stamp, so what was remembered of the old one is not read.
func memoKey(prefix, review, stamp string) string {
	return prefix + sha256hex.Sum([]byte(review + "\x00" + stamp))[:16]
}

// classifyRuling is the block that asks for the classification of the results of dir that the page lacks, or no block.
// The results are those that hold (keptResultsOf); each one that cannot be read, held or bound is a refusal, named in
// the error returned beside the ruling (incomplete). For each result in order, the session's memory is read first: what
// the port said at the page state Stamp gives now (memoKey, classified or lacking) is not asked of the port again, and
// what the port says now is remembered. A memory that cannot be recorded is named in the same error, and the answer in
// hand is used. Every result still lacking goes to once with its classifyKey: the stop is blocked when one of those
// questions is new to the session, and the block names every result still lacking, the ones asked before included, as
// a review question names every need. What the pass as a whole cannot tell (the repository, the features, the state of
// the page, the port, the questions of the session) is an error with no block, joined with the refusals.
func (s Service) classifyRuling(dir, session string) (Ruling, error) {
	repo, err := s.git.Locate(dir)
	if err != nil {
		return Ruling{}, fmt.Errorf("locate the repository of %s: %w", dir, err)
	}
	results, problems, err := s.keptResultsOf(repo)
	if err != nil {
		return Ruling{}, withIncomplete(fmt.Errorf("find the results with kept findings: %w", err), problems)
	}
	if len(results) == 0 {
		return Ruling{}, withIncomplete(nil, problems)
	}
	stamp, err := s.classified.Stamp(repo.Root)
	if err != nil {
		return Ruling{}, withIncomplete(fmt.Errorf("read the state of the evidence page of %s: %w", repo.Root, err), problems)
	}
	var (
		lacking []KeptResult
		keys    []string
	)
	for _, result := range results {
		done, remembered, err := s.remembered(session, result.Review, stamp)
		if err != nil {
			return Ruling{}, withIncomplete(err, problems)
		}
		if !remembered {
			done, err = s.classified.Classified(repo.Root, result.Review)
			if err != nil {
				return Ruling{}, withIncomplete(fmt.Errorf("tell whether %s is classified: %w", result.Review, err), problems)
			}
			prefix, what := lackingPrefix, "lacking"
			if done {
				prefix, what = classifiedPrefix, "classified"
			}
			if err := s.asked.Add(session, memoKey(prefix, result.Review, stamp)); err != nil {
				problems = append(problems, fmt.Errorf("record that %s is %s: %w", result.Review, what, err))
			}
		}
		if !done {
			lacking, keys = append(lacking, result), append(keys, classifyKey(result.Review))
		}
	}
	if len(lacking) == 0 {
		return Ruling{}, withIncomplete(nil, problems)
	}
	ruling, err := s.once(session, keys, Ruling{Block: true, Classify: lacking})
	if err != nil {
		return Ruling{}, withIncomplete(err, problems)
	}
	return ruling, withIncomplete(nil, problems)
}

// remembered reads what the session remembers of the port's answer about review at the page state stamp: whether the
// result is classified, and whether anything was remembered at all.
func (s Service) remembered(session, review, stamp string) (classified, known bool, err error) {
	for _, prefix := range []string{classifiedPrefix, lackingPrefix} {
		known, err := s.asked.Has(session, memoKey(prefix, review, stamp))
		if err != nil {
			return false, false, fmt.Errorf("read the questions asked in the session: %w", err)
		}
		if known {
			return prefix == classifiedPrefix, true, nil
		}
	}
	return false, false, nil
}
