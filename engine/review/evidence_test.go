package review_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewresult"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewverdict"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/roles"
)

// assertEvidenceError requires err to wrap sentinel and to be an *EvidenceError that names the kind and the
// ref of the evidence it is about, in its fields and in its message, so that a person reads which piece of
// evidence was refused and what it pointed at.
func assertEvidenceError(t *testing.T, err, sentinel error, kind, ref string) {
	t.Helper()
	var ee *review.EvidenceError
	if !errors.Is(err, sentinel) || !errors.As(err, &ee) || ee.Kind != kind || ee.Ref != ref {
		t.Errorf("error = %v, want an *EvidenceError for the %s evidence with ref %q that wraps %q", err, kind, ref, sentinel)
		return
	}
	if msg := err.Error(); !strings.Contains(msg, kind) || (ref != "" && !strings.Contains(msg, fmt.Sprintf("%q", ref))) {
		t.Errorf("message %q must name the kind %q and the ref %q", msg, kind, ref)
	}
}

// Only a record that closes a review that ran needs evidence of the stored files: a review-report whose status
// is completed. An interrupted one says the review did not run, and a review that did not run stores nothing,
// so its evidence has no file to name; a record of any other kind is not about a review.
func TestNeedsEvidenceIsACompletedReviewReport(t *testing.T) {
	interrupted := func(h *roles.RoleHandoff) {
		reason := "review not run: the inspection was incomplete"
		h.Status, h.ResumeReason = roles.StatusInterrupted, &reason
	}
	for _, tc := range []struct {
		name string
		edit func(*roles.RoleHandoff)
		want bool
	}{
		{"a completed review-report", func(*roles.RoleHandoff) {}, true},
		{"an interrupted review-report", interrupted, false},
		{"a completed progress record", func(h *roles.RoleHandoff) { h.PayloadKind = "progress" }, false},
		{"a completed verify-report", func(h *roles.RoleHandoff) { h.PayloadKind = "verify-report" }, false},
		{"a kind that differs in case", func(h *roles.RoleHandoff) { h.PayloadKind = "Review-Report" }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := review.Handoff(candidate(t), storedRef, storedSHA)
			tc.edit(&h)

			if got := review.NeedsEvidence(h); got != tc.want {
				t.Errorf("NeedsEvidence() = %v, want %v", got, tc.want)
			}
		})
	}
}

// The evidence of a kind is the one piece of that kind the record carries. A kind that is needed and absent,
// or given twice (so that neither is the evidence of its kind), is refused with an error that names the kind;
// the verdict of the verifier is optional, because the verifier may not have run.
func TestEvidenceOfAKindIsTheOnePieceTheRecordCarries(t *testing.T) {
	h := review.Handoff(candidate(t), storedRef, storedSHA)

	got, err := review.RequiredEvidence(h, review.EvidenceResult)
	if err != nil || got != h.Evidence[1] {
		t.Errorf("RequiredEvidence(review-result) = %+v, %v; want %+v and no error", got, err, h.Evidence[1])
	}
	_, err = review.RequiredEvidence(h, review.EvidenceVerifier)
	assertEvidenceError(t, err, review.ErrEvidenceMissing, review.EvidenceVerifier, "")
	h.Evidence = nil
	_, err = review.RequiredEvidence(h, review.EvidenceDiff)
	assertEvidenceError(t, err, review.ErrEvidenceMissing, review.EvidenceDiff, "")

	twice := review.Handoff(candidate(t), storedRef, storedSHA)
	twice.Evidence = append(twice.Evidence, twice.Evidence[0])
	_, err = review.RequiredEvidence(twice, review.EvidenceDiff)
	assertEvidenceError(t, err, review.ErrEvidenceRepeated, review.EvidenceDiff, "")

	one, found, err := review.OptionalEvidence(review.VerifiedHandoff(candidate(t).Binding(),
		review.FileRef{Ref: storedRef, SHA256: storedSHA}, review.FileRef{Ref: verdictRef, SHA256: storedSHA}, reviewverdict.Clear), review.EvidenceVerifier)
	if !found || err != nil || one.Ref != verdictRef {
		t.Errorf("OptionalEvidence(verifier-report) = %+v, %v, %v; want the verdict and no error", one, found, err)
	}
	none, found, err := review.OptionalEvidence(review.Handoff(candidate(t), storedRef, storedSHA), review.EvidenceVerifier)
	if found || err != nil || none != (roles.Evidence{}) {
		t.Errorf("OptionalEvidence(verifier-report) of a record without one = %+v, %v, %v; want nothing, not found and no error", none, found, err)
	}
	both := review.Handoff(candidate(t), storedRef, storedSHA)
	both.Evidence = append(both.Evidence, roles.Evidence{Kind: review.EvidenceVerifier, Ref: verdictRef, SHA256: storedSHA}, roles.Evidence{Kind: review.EvidenceVerifier, Ref: verdictRef, SHA256: storedSHA})
	_, _, err = review.OptionalEvidence(both, review.EvidenceVerifier)
	assertEvidenceError(t, err, review.ErrEvidenceRepeated, review.EvidenceVerifier, "")
}

// The ref of the result is a path relative to the repository, and it is read as exactly one file of the results
// directory: odd/review-results/<feature>/<head12>-<n>.json. Nothing else is a ref the engine reads, so no ref
// leaves the directory, goes through a link of a path it composed, or names a verdict, a temporary file or a
// file of another kind. The refusal is by shape, before any file is looked at.
func TestResultRefNamesOnlyAResultStoredInTheResultsDirectory(t *testing.T) {
	feature, name, err := review.ResultRef(roles.Evidence{Kind: review.EvidenceResult, Ref: storedRef, SHA256: storedSHA})
	if err != nil || feature != "own-rdd" || name != "222222222222-1.json" {
		t.Fatalf("ResultRef(%q) = %q, %q, %v; want own-rdd, 222222222222-1.json and no error", storedRef, feature, name, err)
	}

	for _, ref := range []string{
		"",
		"   ",
		"/etc/passwd",
		"/work/repo/odd/review-results/own-rdd/222222222222-1.json",
		"./odd/review-results/own-rdd/222222222222-1.json",
		"../odd/review-results/own-rdd/222222222222-1.json",
		"odd/review-results/../../etc/passwd",
		"odd/review-results/own-rdd/../other/222222222222-1.json",
		"odd/review-results/own-rdd/222222222222-1.json/",
		"odd/review-results//222222222222-1.json",
		"odd/review-results/own-rdd/sub/222222222222-1.json",
		"odd/review-results/own-rdd",
		"odd/review-results/",
		"odd/review-results/.hidden/222222222222-1.json",
		"odd/review-results/own..rdd/222222222222-1.json",
		`odd\review-results\own-rdd\222222222222-1.json`,
		"odd/review-results/own-rdd/222222222222-1.verifier.json",
		"odd/review-results/own-rdd/222222222222-1.exempt.json",
		"odd/review-results/own-rdd/222222222222-0.json",
		"odd/review-results/own-rdd/222222222222-01.json",
		"odd/review-results/own-rdd/notes.json",
		"odd/review-results/own-rdd/.review-1.tmp",
		"odd/other-results/own-rdd/222222222222-1.json",
		"docs/odd/review-results/own-rdd/222222222222-1.json",
		"odd/review-results/own-rdd/222222222222-1.json ",
		"odd/review-results/own-rdd/222222222222-1.json\x00",
	} {
		t.Run(ref, func(t *testing.T) {
			feature, name, err := review.ResultRef(roles.Evidence{Kind: review.EvidenceResult, Ref: ref, SHA256: storedSHA})

			assertEvidenceError(t, err, review.ErrEvidenceRef, review.EvidenceResult, ref)
			if feature != "" || name != "" {
				t.Errorf("ResultRef(%q) = %q, %q; want nothing for a ref that is refused", ref, feature, name)
			}
		})
	}

	// The reason is the rule of the name that was broken, and stays readable through the error.
	_, _, err = review.ResultRef(roles.Evidence{Kind: review.EvidenceResult, Ref: "odd/review-results/.hidden/222222222222-1.json"})
	if !errors.Is(err, review.ErrInvalidFeature) {
		t.Errorf("a feature that is not one safe path segment: error = %v, want one that wraps ErrInvalidFeature too", err)
	}
	_, _, err = review.ResultRef(roles.Evidence{Kind: review.EvidenceResult, Ref: "odd/review-results/own-rdd/notes.json"})
	if !errors.Is(err, review.ErrInvalidResultName) {
		t.Errorf("a name that is not <head12>-<n>.json: error = %v, want one that wraps ErrInvalidResultName too", err)
	}
}

// The verdict of the verifier is evidence only as the file stored beside the result it answers: the same
// feature, the same <head12>-<n>, and the ending .verifier.json. Any other ref, the result itself included, is
// refused by comparing it with the one name the result gives.
func TestVerdictRefIsTheVerdictStoredBesideTheResult(t *testing.T) {
	good := roles.Evidence{Kind: review.EvidenceVerifier, Ref: verdictRef, SHA256: storedSHA}
	name, err := review.VerdictRef(good, "own-rdd", "222222222222-1.json")
	if err != nil || name != "222222222222-1.verifier.json" {
		t.Fatalf("VerdictRef() = %q, %v; want 222222222222-1.verifier.json and no error", name, err)
	}

	for _, ref := range []string{
		"",
		"odd/review-results/own-rdd/222222222222-2.verifier.json",
		"odd/review-results/other/222222222222-1.verifier.json",
		"odd/review-results/own-rdd/333333333333-1.verifier.json",
		storedRef,
		"odd/review-results/own-rdd/../own-rdd/222222222222-1.verifier.json",
		"/" + verdictRef,
		"./" + verdictRef,
	} {
		t.Run(ref, func(t *testing.T) {
			name, err := review.VerdictRef(roles.Evidence{Kind: review.EvidenceVerifier, Ref: ref, SHA256: storedSHA}, "own-rdd", "222222222222-1.json")

			assertEvidenceError(t, err, review.ErrEvidenceRef, review.EvidenceVerifier, ref)
			if name != "" {
				t.Errorf("VerdictRef(%q) = %q, want nothing for a ref that is refused", ref, name)
			}
		})
	}
	// A result whose name is not one is no result to take a verdict from.
	if _, err := review.VerdictRef(good, "own-rdd", "notes.json"); !errors.Is(err, review.ErrInvalidResultName) {
		t.Errorf("VerdictRef() for a result that is not named <head12>-<n>.json: error = %v, want one that wraps ErrInvalidResultName", err)
	}
}

// The digest of the evidence over a stored file is the digest of the bytes that are stored now. One byte of
// difference is another digest, and the error says both, with the kind and the ref of the evidence.
func TestCheckFileHoldsTheEvidenceToTheBytesOfTheStoredFile(t *testing.T) {
	// diffSHA is the SHA-256 of the three bytes "abc" (fixtures_test.go).
	e := roles.Evidence{Kind: review.EvidenceResult, Ref: storedRef, SHA256: diffSHA}

	if err := review.CheckFile(e, []byte("abc")); err != nil {
		t.Errorf("CheckFile() of the bytes the digest is of: error = %v, want nil", err)
	}

	err := review.CheckFile(e, []byte("abd"))

	assertEvidenceError(t, err, review.ErrEvidenceDiffers, review.EvidenceResult, storedRef)
	stored := sha256.Sum256([]byte("abd"))
	for _, want := range []string{diffSHA, hex.EncodeToString(stored[:])} {
		if err != nil && !strings.Contains(err.Error(), want) {
			t.Errorf("message %q must say the digest %s", err, want)
		}
	}
}

// The evidence over a range holds the range and the digest of its diff, which are in the binding stored with
// the result: the ref is <base>..<head> of that binding, and the digest its diff_sha256. A ref of another range,
// the same commits the other way round, and a digest of another diff are all refused.
func TestCheckRangeHoldsTheEvidenceToTheBindingStoredWithTheResult(t *testing.T) {
	binding := candidate(t).Binding()
	good := roles.Evidence{Kind: review.EvidenceDiff, Ref: baseSHA + ".." + headSHA, SHA256: diffSHA}

	if err := review.CheckRange(good, binding); err != nil {
		t.Errorf("CheckRange() of the range and the digest the binding holds: error = %v, want nil", err)
	}

	for _, tc := range []struct {
		name string
		edit func(*roles.Evidence)
	}{
		{"another head", func(e *roles.Evidence) { e.Ref = baseSHA + ".." + baseSHA }},
		{"another base", func(e *roles.Evidence) { e.Ref = headSHA + ".." + headSHA }},
		{"the commits the other way round", func(e *roles.Evidence) { e.Ref = headSHA + ".." + baseSHA }},
		{"three dots", func(e *roles.Evidence) { e.Ref = baseSHA + "..." + headSHA }},
		{"the head alone", func(e *roles.Evidence) { e.Ref = headSHA }},
		{"an empty ref", func(e *roles.Evidence) { e.Ref = "" }},
		{"the digest of another diff", func(e *roles.Evidence) { e.SHA256 = strings.Repeat("f", 64) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := good
			tc.edit(&e)

			err := review.CheckRange(e, binding)

			assertEvidenceError(t, err, review.ErrEvidenceDiffers, review.EvidenceDiff, e.Ref)
		})
	}
	// Nothing is taken from the record to complete a binding that names nothing.
	if err := review.CheckRange(good, reviewresult.Binding{}); !errors.Is(err, review.ErrEvidenceDiffers) {
		t.Errorf("CheckRange() against a binding that names no commit: error = %v, want one that wraps ErrEvidenceDiffers", err)
	}
}
