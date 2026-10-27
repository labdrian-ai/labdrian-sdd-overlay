package review_test

// What the record that closes the exemption of a head is held to when it is appended (own-rdd T4, S5, L16): the same
// as the record that closes a review, over the stored exemption instead of the stored result. These are the pure
// rules; the use case that reads the files is review/app's CheckEvidence.

import (
	"errors"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/roles"
)

// Evidence that a record needs and does not carry is refused with the kind named, and says which record needs it:
// the words for a review-report are the ones they always were, and a review-exemption says its own kind.
func TestMissingEvidenceSaysWhichRecordNeedsIt(t *testing.T) {
	report := review.Handoff(candidate(t), storedRef, storedSHA)
	report.Evidence = nil
	exemption := review.ExemptionHandoff(candidate(t), exemptionRef, storedSHA)
	exemption.Evidence = nil

	_, errReport := review.RequiredEvidence(report, review.EvidenceDiff)
	_, errExemption := review.RequiredEvidence(exemption, review.EvidenceExemption)

	if want := "evidence review-diff: missing: a completed review-report record carries one"; errReport == nil || errReport.Error() != want {
		t.Errorf("the missing evidence of a review-report says %v, want %q", errReport, want)
	}
	if want := "evidence review-exemption: missing: a completed review-exemption record carries one"; errExemption == nil || errExemption.Error() != want {
		t.Errorf("the missing evidence of a review-exemption says %v, want %q", errExemption, want)
	}
	assertEvidenceError(t, errReport, review.ErrEvidenceMissing, review.EvidenceDiff, "")
	assertEvidenceError(t, errExemption, review.ErrEvidenceMissing, review.EvidenceExemption, "")
}

// The ref of the exemption is a path relative to the repository, and it is read as exactly one file of the results
// directory: odd/review-results/<feature>/<head12>-<n>.exempt.json. Nothing else is a ref the engine reads, so no
// ref leaves the directory, goes through a path it did not compose, or names a result, a verdict, a temporary file
// or a file of another kind. The refusal is by shape, before any file is looked at.
func TestExemptionRefNamesOnlyAnExemptionStoredInTheResultsDirectory(t *testing.T) {
	feature, name, err := review.ExemptionRef(roles.Evidence{Kind: review.EvidenceExemption, Ref: exemptionRef, SHA256: storedSHA})
	if err != nil || feature != "own-rdd" || name != "222222222222-1.exempt.json" {
		t.Fatalf("ExemptionRef(%q) = %q, %q, %v; want own-rdd, 222222222222-1.exempt.json and no error", exemptionRef, feature, name, err)
	}

	for _, ref := range []string{
		"",
		"   ",
		"/etc/passwd",
		"/work/repo/odd/review-results/own-rdd/222222222222-1.exempt.json",
		"./odd/review-results/own-rdd/222222222222-1.exempt.json",
		"../odd/review-results/own-rdd/222222222222-1.exempt.json",
		"odd/review-results/../../etc/passwd",
		"odd/review-results/own-rdd/../other/222222222222-1.exempt.json",
		"odd/review-results/own-rdd/222222222222-1.exempt.json/",
		"odd/review-results//222222222222-1.exempt.json",
		"odd/review-results/own-rdd/sub/222222222222-1.exempt.json",
		"odd/review-results/own-rdd",
		"odd/review-results/.hidden/222222222222-1.exempt.json",
		"odd/review-results/own..rdd/222222222222-1.exempt.json",
		`odd\review-results\own-rdd\222222222222-1.exempt.json`,
		storedRef,
		verdictRef,
		"odd/review-results/own-rdd/222222222222-0.exempt.json",
		"odd/review-results/own-rdd/222222222222-01.exempt.json",
		"odd/review-results/own-rdd/22222222222-1.exempt.json",
		"odd/review-results/own-rdd/notes.exempt.json",
		"odd/review-results/own-rdd/222222222222-1.exempt.json.tmp",
		"odd/review-results/own-rdd/222222222222-1.exempt.json ",
		"odd/review-results/own-rdd/222222222222-1.exempt.json\x00",
		"odd/other-results/own-rdd/222222222222-1.exempt.json",
		"docs/odd/review-results/own-rdd/222222222222-1.exempt.json",
	} {
		t.Run(ref, func(t *testing.T) {
			feature, name, err := review.ExemptionRef(roles.Evidence{Kind: review.EvidenceExemption, Ref: ref, SHA256: storedSHA})

			assertEvidenceError(t, err, review.ErrEvidenceRef, review.EvidenceExemption, ref)
			if feature != "" || name != "" {
				t.Errorf("ExemptionRef(%q) = %q, %q; want nothing for a ref that is refused", ref, feature, name)
			}
		})
	}

	// The reason is the rule of the name that was broken, and stays readable through the error.
	_, _, err = review.ExemptionRef(roles.Evidence{Kind: review.EvidenceExemption, Ref: "odd/review-results/.hidden/222222222222-1.exempt.json"})
	if !errors.Is(err, review.ErrInvalidFeature) {
		t.Errorf("a feature that is not one safe path segment: error = %v, want one that wraps ErrInvalidFeature too", err)
	}
	_, _, err = review.ExemptionRef(roles.Evidence{Kind: review.EvidenceExemption, Ref: storedRef})
	if !errors.Is(err, review.ErrInvalidExemptionName) {
		t.Errorf("the name of a result: error = %v, want one that wraps ErrInvalidExemptionName too", err)
	}
}
