package app

import (
	"fmt"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/sha256hex"
)

// Classified is the port that tells whether stored results have been classified into the evidence page of a working
// tree (S6): the learning loop counts a result as classified when the page holds an Evidence row, or a row of its table
// of Classified without evidence, whose Review is the result's identifier.
type Classified interface {
	// Classified says, for each of reviews, whether the evidence page of the working tree at root holds an Evidence row,
	// or a row of its table of Classified without evidence, whose Review is that identifier, <feature>/<head12>-<n>. With a nil error the answers are exactly len(reviews), the
	// i-th about reviews[i], all read from one state of the page. An empty reviews gives an empty answer and reads
	// nothing. An error is a page or a script that cannot say, never "not classified".
	Classified(root string, reviews []string) ([]bool, error)
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
// first 12 characters of its name said it was about this one), a result with no kept finding, and a result held until the
// check of its severe finding has run (a BLOCKER or CRITICAL that reviewverdict.Decide lists as unverified: a stored
// not-run holds it for good, and review check refuses its head, see Findings.Unverified) are skipped. A result
// that cannot be read, that does not hold, or that does not bind its commit is a refusal, the second value, as 'review
// check' refuses the same file (its text is the error's, its review the identifier of the file, whatever the error says);
// it is left out and the pass goes on, so one bad file hides no other result. What the
// pass as a whole cannot tell (the features, where HEAD stands, the names of a feature's results) is the third value,
// returned with no result.
//
// There is nothing in a tree that keeps no features, on a detached HEAD, on a repository that does not tell its
// default branch, and on a branch with no commit of its own, and that is no error. Nothing is stored.
func (s Service) keptResultsOf(repo Repository) ([]KeptResult, []refusal, error) {
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
		refused []refusal
	)
	for _, commit := range standing.Commits {
		for _, listing := range listings {
			for _, name := range review.ResultsOfHead(listing.names, commit) {
				h, binds, err := s.heldResult(repo, listing.feature, name, commit)
				if err != nil {
					refused = append(refused, refusal{review: reviewID(listing.feature, name), err: err})
					continue
				}
				if !binds {
					continue
				}
				kept, _ := keptFindings(h.result, h.verdicts)
				if len(kept) == 0 || len(unverifiedOf(h.result, h.verdicts)) > 0 {
					continue
				}
				out = append(out, KeptResult{Review: reviewID(listing.feature, name), Head: commit, Kept: len(kept)})
			}
		}
	}
	return out, refused, nil
}

// refusal is a stored result the pass refused: review is its identifier, <feature>/<head12>-<n>, and err why. Its text and
// its causes are err's, so errors.Is reaches them; the identifier is what a session names it by (refusedKey).
type refusal struct {
	review string
	err    error
}

func (r refusal) Error() string { return r.err.Error() }

func (r refusal) Unwrap() error { return r.err }

// incomplete is the problems of a classification pass that did not stop it: the results it refused, and the records of a
// named refusal that failed. Its text joins the problems by "; ", and the hook prints it on one line (oneLine); the type
// does not promise one line itself. Unwrap gives each problem, so errors.Is reaches every cause.
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

// refusedKey names, in the questions of a session, that the refusal of the stored file whose identifier is review
// (<feature>/<head12>-<n>) was named: refused- and the first 16 hex digits of the SHA-256 of the identifier, 24 bytes of
// lowercase letters, digits and dashes, a key of the same store as classifyKey's. The text of the refusal is not part of
// it: a file is named once for each session, whatever its error says.
func refusedKey(review string) string {
	return "refused-" + sha256hex.Sum([]byte(review))[:16]
}

// errorsOf is refusals as errors, in order.
func errorsOf(refusals []refusal) []error {
	if len(refusals) == 0 {
		return nil
	}
	out := make([]error, len(refusals))
	for i, r := range refusals {
		out[i] = r
	}
	return out
}

// unnamed is, in order, the refusals whose key the session does not hold: the ones a stop that returns its ruling names.
// It only reads. A key that cannot be read is an error and no refusal is returned: the pass cannot tell.
func (s Service) unnamed(session string, refusals []refusal) ([]refusal, error) {
	var fresh []refusal
	for _, r := range refusals {
		named, err := s.asked.Has(session, refusedKey(r.review))
		if err != nil {
			return nil, fmt.Errorf("read the questions asked in the session: %w", err)
		}
		if !named {
			fresh = append(fresh, r)
		}
	}
	return fresh, nil
}

// named records the keys of fresh, the refusals a stop returns with its ruling, and gives them as errors, in order, each
// followed by the failed record of its naming when the record fails (then it is named again at the next stop). Only a
// stop that returns its ruling with no error of the pass calls it: a stop that ends in an error names the refusals
// beside that error and records nothing, so the next stop names them again.
func (s Service) named(session string, fresh []refusal) []error {
	var problems []error
	for _, r := range fresh {
		problems = append(problems, r)
		if err := s.asked.Add(session, refusedKey(r.review)); err != nil {
			problems = append(problems, fmt.Errorf("record that a refusal was named: %w", err))
		}
	}
	return problems
}

// classifyRuling is the block that asks for the classification of the results of dir that the page lacks, or no block.
// In order:
//  1. the repository is located;
//  2. the results are those that hold (keptResultsOf); a pass error is returned with no block;
//  3. each refusal (a result that cannot be read, held or bound) is named in the error returned beside the ruling
//     (incomplete) once for each session and stored file, recorded when the stop returns its ruling with no error of the
//     pass (unnamed reads, named records);
//  4. with no kept result there is no block, and the refusals are recorded (named);
//  5. the port is asked once, for every kept result in order, and nothing it says is remembered: a stop asks again. An
//     error of the port, or a number of answers that is not the number of results, is an error with no block;
//  6. every result still lacking goes to once with its classifyKey: the stop is blocked when one of those questions is
//     new to the session, and the block names every result still lacking, the ones asked before included, as a review
//     question names every need;
//  7. the ruling, joined with the problems of the refusals, which are recorded (named) only now.
//
// What the pass as a whole cannot tell (the repository, the features, the port, the questions of the session) is an error
// with no block, joined with the refusals not named yet; nothing is recorded for them.
func (s Service) classifyRuling(dir, session string) (Ruling, error) {
	repo, err := s.git.Locate(dir)
	if err != nil {
		return Ruling{}, fmt.Errorf("locate the repository of %s: %w", dir, err)
	}
	results, refusals, err := s.keptResultsOf(repo)
	if err != nil {
		return Ruling{}, withIncomplete(fmt.Errorf("find the results with kept findings: %w", err), errorsOf(refusals))
	}
	fresh, err := s.unnamed(session, refusals)
	if err != nil {
		return Ruling{}, withIncomplete(err, errorsOf(refusals))
	}
	if len(results) == 0 {
		return Ruling{}, withIncomplete(nil, s.named(session, fresh))
	}
	reviews := make([]string, len(results))
	for i, result := range results {
		reviews[i] = result.Review
	}
	answers, err := s.classified.Classified(repo.Root, reviews)
	if err != nil {
		return Ruling{}, withIncomplete(fmt.Errorf("tell whether the results with kept findings are classified: %w", err), errorsOf(fresh))
	}
	if len(answers) != len(reviews) {
		return Ruling{}, withIncomplete(fmt.Errorf("the evidence page of %s gave %d answers for %d results", repo.Root, len(answers), len(reviews)), errorsOf(fresh))
	}
	var (
		lacking []KeptResult
		keys    []string
	)
	for i, result := range results {
		if !answers[i] {
			lacking, keys = append(lacking, result), append(keys, classifyKey(result.Review))
		}
	}
	if len(lacking) == 0 {
		return Ruling{}, withIncomplete(nil, s.named(session, fresh))
	}
	ruling, err := s.once(session, keys, Ruling{Block: true, Classify: lacking})
	if err != nil {
		return Ruling{}, withIncomplete(err, errorsOf(fresh))
	}
	return ruling, withIncomplete(nil, s.named(session, fresh))
}
