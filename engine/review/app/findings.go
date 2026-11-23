package app

import (
	"fmt"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewresult"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewverdict"
)

// The reasons Findings gives for a finding it leaves out. A finding the diff did not cause, or that the verifier
// could not reproduce, is not evidence of a kind of mistake (the classify rule "a finding the review marks as not
// real, dismissed, or already fixed before the review is not evidence").
const (
	ReasonPreExisting   = "causal_disposition pre-existing"
	ReasonBaseOnly      = "causal_disposition base-only"
	ReasonNotReproduced = "verdict not-reproduced"
	// ReasonBaseReproduced is a severe finding of unknown disposition that the verifier found on base too: the diff
	// did not cause it, and reviewverdict.Decide carries it as a follow-up.
	ReasonBaseReproduced = "verdict base-reproduced"
)

// Findings is the listing of one stored result for the learning loop: the findings that count as evidence and the
// ones that do not, each left out with its reason.
type Findings struct {
	// Review is the identifier of the result, <feature>/<head12>-<n>: the same on every run, for it is the name of
	// the stored file without its ending.
	Review string
	// Verdict is the stored verdict that was read, relative to the root of the working tree, or "" when the
	// verifier did not run.
	Verdict string
	// Kept are the findings that count as evidence, in the order the reviewer wrote them.
	Kept []reviewresult.Finding
	// LeftOut are the others, in the same order, each with the one reason that left it out.
	LeftOut []LeftOut
}

// LeftOut is a finding that is not evidence and why.
type LeftOut struct {
	ID       string
	Severity reviewresult.Severity
	Reason   string
}

// Findings lists the findings of the stored result called stored, in the directory of the feature of the repository
// that holds dir (absolute). The result and the verdict stored beside it are read as verify and check read them
// (judged, verdictsOf), so a file they would refuse is refused here: a result that is not stored or is not one, and
// a verdict that is stored and does not hold to its result, are errors and never an empty listing. It stores
// nothing and asks git only where the repository is.
//
// A finding is left out when its causal disposition is pre-existing or base-only, whatever its severity (read
// first), or when it is BLOCKER or CRITICAL and the verdict found it not-reproduced on head, or when it is BLOCKER
// or CRITICAL of unknown disposition and the verdict found it on base (reviewverdict.Decide carries it as a fate of
// the base). Every other finding is kept: a verdict that reproduced it or did not run, and a WARNING or SUGGESTION,
// which a verdict does not judge.
func (s Service) Findings(dir, feature, stored string) (Findings, error) {
	if err := review.CheckFeature(feature); err != nil {
		return Findings{}, err
	}
	if err := review.CheckResultName(stored); err != nil {
		return Findings{}, err
	}
	repo, err := s.git.Locate(dir)
	if err != nil {
		return Findings{}, fmt.Errorf("locate the repository of %s: %w", dir, err)
	}
	path := review.ResultPath(feature, stored)
	file, err := s.results.Read(repo.Root, feature, stored)
	if err != nil {
		return Findings{}, fmt.Errorf("read %s: %w", path, err)
	}
	_, result, err := judged(file)
	if err != nil {
		return Findings{}, fmt.Errorf("%s: %w", path, err)
	}
	verdicts, verdictPath, err := s.verdictsOf(repo, feature, stored, result)
	if err != nil {
		return Findings{}, err
	}
	kept, leftOut := keptFindings(result, verdicts)
	return Findings{Review: reviewID(feature, stored), Verdict: verdictPath, Kept: kept, LeftOut: leftOut}, nil
}

// reviewID is the identifier of the stored result called stored in feature, <feature>/<head12>-<n>: the name of the
// stored file without its ending.
func reviewID(feature, stored string) string {
	return feature + "/" + strings.TrimSuffix(stored, ".json")
}

// keptFindings splits the findings of result in the ones that count as evidence and the ones that do not, each of those
// with its reason, both in the order the reviewer wrote them and never nil. It is the one rule of what is evidence: the
// listing of Findings and the question of the stop hook (KeptResults) both read it, so they cannot disagree.
func keptFindings(result reviewresult.Result, verdicts []reviewverdict.Verdict) (kept []reviewresult.Finding, leftOut []LeftOut) {
	kept, leftOut = []reviewresult.Finding{}, []LeftOut{}
	for _, f := range result.Findings {
		if reason := leftOutReason(f, verdicts); reason != "" {
			leftOut = append(leftOut, LeftOut{ID: f.ID, Severity: f.Severity, Reason: reason})
			continue
		}
		kept = append(kept, f)
	}
	return kept, leftOut
}

// leftOutReason is why f is not evidence, or "" when it is.
func leftOutReason(f reviewresult.Finding, verdicts []reviewverdict.Verdict) string {
	switch {
	case f.Disposition == reviewresult.DispositionPreExisting:
		return ReasonPreExisting
	case f.Disposition == reviewresult.DispositionBaseOnly:
		return ReasonBaseOnly
	case f.Severity.Severe():
		for _, v := range verdicts {
			if v.ID != f.ID {
				continue
			}
			if v.Result == reviewverdict.NotReproduced {
				return ReasonNotReproduced
			}
			if f.Disposition == reviewresult.DispositionUnknown && v.Base == reviewverdict.Reproduced {
				return ReasonBaseReproduced
			}
		}
	}
	return ""
}
