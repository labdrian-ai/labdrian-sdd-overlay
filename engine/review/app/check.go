package app

import (
	"errors"
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewresult"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewverdict"
)

// Checked is what Check found out about a head that is cleared for delivery: its full name, every stored result
// that binds it, by launch, with the files used and what each makes of the review, and every stored exemption that
// binds it, by number. A head with no exemption has none.
type Checked struct {
	Head       string
	Results    []review.Reviewed
	Exemptions []review.Exempted
}

// Check decides whether the head being delivered was reviewed, or exempted from review, and nothing the engine
// stored keeps it from delivery, in the repository that holds dir (absolute). head is a revision as a person types
// it. Every result of the feature that is about that commit is read back, and so is every exemption, and the head is
// cleared (review.Cleared) when it has an exemption and no result, or at least one result and every one is clear
// (review.Clearance): a result that blocks wins over any exemption. Each result and each exemption is held to the
// files and to git again, and none of it is taken from what an earlier command said:
//
//   - the stored file is read as Record wrote it and its reply is held to its binding again, as Verify does, so
//     a file that was changed, or is not a result, is refused;
//   - the candidate is recomputed from git for the base the result names and the head being delivered, and the
//     stored binding must be equal to it (review.CheckBinding): the digest of the diff is the one of the commits
//     as they are now, which is what the reviewer read;
//   - the verdict stored beside the result, when there is one, is read and held to the result it answers, and the
//     decision is made from the findings and the verdicts (reviewverdict.Decide). With no verdict the verifier did
//     not run and a finding counts as a check that did not run, so a result with a severe finding the diff may have
//     caused is not clear. A verdict that is stored and cannot be read is refused, never read as no verdict.
//
// An exemption is held in the same way, to the rules `review exempt` made it under (review.CheckExemption): the
// stored file is read as Exempt wrote it, its binding names a candidate before its base is handed to git, its
// declared risk still names no high-risk item and its report is still clear, so a file that was edited to say
// something else is refused; and the candidate is recomputed from git for the base it names and the head being
// delivered, and the stored binding must be equal to it. An exemption that does not hold refuses the head even
// when another file clears it, as a result that does not hold does.
//
// A result or an exemption is about a commit by its binding and not by its name. The feature and the head are checked
// before git or the store is asked anything. Whatever cannot be told, because git or the store fails or a file does
// not hold, is a refusal: a head is cleared only on what was checked. Nothing is stored.
func (s Service) Check(dir, feature, head string) (Checked, error) {
	if err := review.CheckFeature(feature); err != nil {
		return Checked{}, err
	}
	if err := review.CheckRev(head); err != nil {
		return Checked{}, err
	}
	repo, err := s.git.Locate(dir)
	if err != nil {
		return Checked{}, fmt.Errorf("locate the repository of %s: %w", dir, err)
	}
	headName, err := s.git.Commit(repo, head)
	if err != nil {
		return Checked{}, fmt.Errorf("resolve head %q: %w", head, err)
	}
	names, err := s.results.Names(repo.Root, feature)
	if err != nil {
		return Checked{}, fmt.Errorf("list %s/%s: %w", review.ResultsDir, feature, err)
	}
	var reviewed []review.Reviewed
	for _, name := range review.ResultsOfHead(names, headName) {
		r, binds, err := s.reviewedResult(repo, feature, name, headName)
		if err != nil {
			return Checked{}, err
		}
		if binds {
			reviewed = append(reviewed, r)
		}
	}
	var exempted []review.Exempted
	for _, name := range review.ExemptionsOfHead(names, headName) {
		e, binds, err := s.exemptedHead(repo, feature, name, headName)
		if err != nil {
			return Checked{}, err
		}
		if binds {
			exempted = append(exempted, e)
		}
	}
	if err := review.Cleared(reviewed, exempted); err != nil {
		return Checked{}, fmt.Errorf("head %s: %w", headName, err)
	}
	return Checked{Head: headName, Results: reviewed, Exemptions: exempted}, nil
}

// exemptedHead reads the stored exemption called name and holds it as Check says. It reports false for an exemption
// that is about another commit than head, which only the first 12 characters of the name say it is not.
func (s Service) exemptedHead(repo Repository, feature, name, head string) (review.Exempted, bool, error) {
	path := review.ResultPath(feature, name)
	file, err := s.results.Read(repo.Root, feature, name)
	if err != nil {
		return review.Exempted{}, false, fmt.Errorf("read %s: %w", path, err)
	}
	stored, err := review.DecodeExemption(file)
	if err == nil {
		err = review.CheckExemption(stored)
	}
	if err != nil {
		return review.Exempted{}, false, fmt.Errorf("%s: %w", path, err)
	}
	if stored.Binding.Head != head {
		return review.Exempted{}, false, nil
	}
	candidate, err := s.bind(repo, stored.Binding.Base, head)
	if err != nil {
		return review.Exempted{}, false, fmt.Errorf("%s: %w", path, err)
	}
	if err := review.CheckBinding(stored.Binding, candidate); err != nil {
		return review.Exempted{}, false, fmt.Errorf("%s: %w", path, err)
	}
	return review.Exempted{Candidate: candidate, Exemption: path}, true, nil
}

// reviewedResult reads the stored result called name and holds it as Check says. It reports false for a result
// that is about another commit than head, which only the first 12 characters of the name say it is not.
func (s Service) reviewedResult(repo Repository, feature, name, head string) (review.Reviewed, bool, error) {
	path := review.ResultPath(feature, name)
	file, err := s.results.Read(repo.Root, feature, name)
	if err != nil {
		return review.Reviewed{}, false, fmt.Errorf("read %s: %w", path, err)
	}
	stored, result, err := judged(file)
	if err != nil {
		return review.Reviewed{}, false, fmt.Errorf("%s: %w", path, err)
	}
	if stored.Head != head {
		return review.Reviewed{}, false, nil
	}
	candidate, err := s.bind(repo, stored.Base, head)
	if err != nil {
		return review.Reviewed{}, false, fmt.Errorf("%s: %w", path, err)
	}
	if err := review.CheckBinding(stored, candidate); err != nil {
		return review.Reviewed{}, false, fmt.Errorf("%s: %w", path, err)
	}
	verdicts, verdictPath, err := s.verdictsOf(repo, feature, name, result)
	if err != nil {
		return review.Reviewed{}, false, err
	}
	return review.Reviewed{Candidate: candidate, Result: path, Verdict: verdictPath, Decision: reviewverdict.Decide(result, verdicts)}, true, nil
}

// verdictsOf reads the verdict stored beside the result called name and holds it to the result it answers. It
// returns no verdicts and no path when none is stored, which is the verifier not having run. Anything else that
// stops the verdict from being read, or from holding, is an error.
func (s Service) verdictsOf(repo Repository, feature, name string, result reviewresult.Result) ([]reviewverdict.Verdict, string, error) {
	verdictName, err := review.VerdictName(name)
	if err != nil {
		return nil, "", err
	}
	path := review.ResultPath(feature, verdictName)
	file, err := s.results.Read(repo.Root, feature, verdictName)
	switch {
	case errors.Is(err, ErrNotFound):
		return nil, "", nil
	case err != nil:
		return nil, "", fmt.Errorf("read %s: %w", path, err)
	}
	verdicts, err := reviewverdict.Parse(file, result)
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", path, err)
	}
	return verdicts, path, nil
}
