package app_test

// The evidence of the record that closes the exemption of a head (own-rdd T4, S5, L16). CheckEvidence holds a
// completed review-exemption to the exemption `review exempt` stored, as it holds a completed review-report to the
// stored result: the range with the digest of its diff and the stored exemption with its digest, each given once,
// and the exemption must still be one that holds to the rules it was made under.

import (
	"bytes"
	"errors"
	"reflect"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewresult"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewverdict"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/roles"
)

// exemptionName is the file the exemption of keptExemption is stored in.
const exemptionName = "222222222222-1.exempt.json"

// exemptionKept is a results directory with the exemption that Exempt stored, and the record Exempt printed for it:
// the range and the exemption.
type exemptionKept struct {
	store  *fakeResults
	record roles.RoleHandoff
}

func keptExemption(t *testing.T) exemptionKept {
	t.Helper()
	g, store := newFakeGit(), newFakeResults()
	exempted, err := app.New(g, store).Exempt(evidenceDir, "own-rdd", "HEAD~1", "HEAD", "none", []byte(exemptReport))
	if err != nil {
		t.Fatalf("setup: Exempt() error = %v", err)
	}
	return exemptionKept{store: store, record: exempted.Handoff}
}

// stored puts data in the file of the exemption, and returns the record with the digest of those bytes, so that
// the digest is not what refuses the evidence.
func (k exemptionKept) stored(data []byte) roles.RoleHandoff {
	k.store.put("own-rdd", exemptionName, data)
	return edited(k.record, review.EvidenceExemption, func(e *roles.Evidence) { e.SHA256 = sha256Of(data) })
}

// The record that Exempt prints is the record the check accepts: it reads the exemption it names and nothing else,
// asks git where the repository is and no more, and stores nothing.
func TestCheckEvidenceAcceptsTheRecordThatExemptPrints(t *testing.T) {
	kept := keptExemption(t)
	created := kept.store.created()

	gitCalls, storeCalls, err := check(kept.store, kept.record)

	if err != nil {
		t.Fatalf("CheckEvidence() error = %v, want nil", err)
	}
	if want := []string{"Locate " + evidenceDir}; !reflect.DeepEqual(gitCalls, want) {
		t.Errorf("git calls = %q, want %q: the evidence is the stored file, not the repository", gitCalls, want)
	}
	if want := []string{"Read /work/wt/feature own-rdd " + exemptionName}; !reflect.DeepEqual(storeCalls, want) {
		t.Errorf("store calls = %q, want %q", storeCalls, want)
	}
	if kept.store.created() != created {
		t.Errorf("CheckEvidence() stored a file; it must only read")
	}
}

// A record that closes an exemption carries the range with the digest of its diff and the stored exemption. One
// that lacks a piece, or gives a kind twice so that neither piece is the evidence of its kind, is refused with the
// kind named, and before any file is read.
func TestCheckEvidenceRefusesAnExemptionRecordThatLacksOrRepeatsAPieceBeforeReadingAnything(t *testing.T) {
	kept := keptExemption(t)
	noEvidence := kept.record
	noEvidence.Evidence = nil
	for _, tc := range []struct {
		name     string
		record   roles.RoleHandoff
		sentinel error
		kind     string
	}{
		{"it has no review-diff", without(kept.record, review.EvidenceDiff), review.ErrEvidenceMissing, review.EvidenceDiff},
		{"it has no review-exemption", without(kept.record, review.EvidenceExemption), review.ErrEvidenceMissing, review.EvidenceExemption},
		{"it has no evidence at all", noEvidence, review.ErrEvidenceMissing, review.EvidenceDiff},
		{"it has the review-diff twice", doubled(kept.record, review.EvidenceDiff), review.ErrEvidenceRepeated, review.EvidenceDiff},
		{"it has the review-exemption twice", doubled(kept.record, review.EvidenceExemption), review.ErrEvidenceRepeated, review.EvidenceExemption},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gitCalls, storeCalls, err := check(kept.store, tc.record)

			assertEvidence(t, err, tc.sentinel, tc.kind, "")
			if len(gitCalls) != 0 || len(storeCalls) != 0 {
				t.Errorf("git calls = %q, store calls = %q; want none, the record is refused by its shape", gitCalls, storeCalls)
			}
		})
	}
}

// The digest of each piece of evidence is held to the stored file it names, and the range to the binding stored
// with the exemption: a digest of another file, a file changed after the record was made, a digest of another diff
// and another range are all refused, with the kind and the ref of the piece named.
func TestCheckEvidenceRefusesAnExemptionRecordWhoseDigestIsNotThatOfTheStoredFile(t *testing.T) {
	for _, tc := range []struct {
		name string
		act  func(k exemptionKept) roles.RoleHandoff
		kind string
		ref  string
	}{
		{"the review-exemption has the digest of another file", func(k exemptionKept) roles.RoleHandoff {
			return edited(k.record, review.EvidenceExemption, func(e *roles.Evidence) { e.SHA256 = sha256Of([]byte("another file")) })
		}, review.EvidenceExemption, wantFirstExemption},
		{"the stored exemption was changed after the record was made", func(k exemptionKept) roles.RoleHandoff {
			changeStored(k.store, exemptionName)
			return k.record
		}, review.EvidenceExemption, wantFirstExemption},
		{"the review-diff has the digest of another diff", func(k exemptionKept) roles.RoleHandoff {
			return edited(k.record, review.EvidenceDiff, func(e *roles.Evidence) { e.SHA256 = sha256Of([]byte("another diff")) })
		}, review.EvidenceDiff, baseSHA + ".." + headSHA},
		{"the review-diff is the range of other commits", func(k exemptionKept) roles.RoleHandoff {
			return edited(k.record, review.EvidenceDiff, func(e *roles.Evidence) { e.Ref = baseSHA + ".." + otherSHA })
		}, review.EvidenceDiff, baseSHA + ".." + otherSHA},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kept := keptExemption(t)
			record := tc.act(kept)
			created := kept.store.created()

			_, _, err := check(kept.store, record)

			assertEvidence(t, err, review.ErrEvidenceDiffers, tc.kind, tc.ref)
			if kept.store.created() != created {
				t.Errorf("CheckEvidence() stored a file; it must only read")
			}
		})
	}
}

// A ref to an exemption that is not stored, and a store that cannot read the file, are evidence that cannot be held
// to anything: the error names the kind and the ref, and says what the store said.
func TestCheckEvidenceRefusesAReferenceToAnExemptionThatCannotBeRead(t *testing.T) {
	cause := errors.New("the disk is gone")
	for _, tc := range []struct {
		name     string
		act      func(k exemptionKept) roles.RoleHandoff
		sentinel error
		ref      string
	}{
		{"the exemption is not stored", func(k exemptionKept) roles.RoleHandoff {
			return edited(k.record, review.EvidenceExemption, func(e *roles.Evidence) { e.Ref = "odd/review-results/own-rdd/222222222222-9.exempt.json" })
		}, app.ErrNotFound, "odd/review-results/own-rdd/222222222222-9.exempt.json"},
		{"the store cannot read the exemption", func(k exemptionKept) roles.RoleHandoff {
			k.store.fail["Read"] = cause
			return k.record
		}, cause, wantFirstExemption},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kept := keptExemption(t)
			record := tc.act(kept)

			_, _, err := check(kept.store, record)

			assertEvidence(t, err, tc.sentinel, review.EvidenceExemption, tc.ref)
		})
	}
}

// The ref is read as the stored exemption and nothing else: a ref that leaves the results directory, or is a result
// or a verdict, is refused by its shape before git or the store is asked anything.
func TestCheckEvidenceReadsOnlyTheExemptionsOfTheResultsDirectory(t *testing.T) {
	kept := keptExemption(t)
	for _, ref := range []string{
		"/etc/passwd",
		"odd/review-results/own-rdd/../../../etc/passwd",
		"odd/review-results/own-rdd/222222222222-1.json",
		"odd/review-results/own-rdd/222222222222-1.verifier.json",
		"odd/review-results/own-rdd/222222222222-1.exempt.json.tmp",
	} {
		t.Run(ref, func(t *testing.T) {
			record := edited(kept.record, review.EvidenceExemption, func(e *roles.Evidence) { e.Ref = ref })

			gitCalls, storeCalls, err := check(kept.store, record)

			assertEvidence(t, err, review.ErrEvidenceRef, review.EvidenceExemption, ref)
			if len(gitCalls) != 0 || len(storeCalls) != 0 {
				t.Errorf("git calls = %q, store calls = %q; want none, the ref is refused by its shape", gitCalls, storeCalls)
			}
		})
	}
}

// A file with the digest the record gives is evidence only when it is an exemption: bytes that are not one, and the
// stored file of a result, are refused with the kind and the ref named.
func TestCheckEvidenceRefusesAStoredFileThatIsNotAnExemption(t *testing.T) {
	result := keptReviews(t).store.files["own-rdd"]["222222222222-1.json"]
	for name, data := range map[string][]byte{
		"text":                 []byte("this is not a stored exemption"),
		"the file of a result": result,
		"an empty object":      []byte("{}"),
	} {
		t.Run(name, func(t *testing.T) {
			kept := keptExemption(t)
			record := kept.stored(data)

			_, _, err := check(kept.store, record)

			assertEvidence(t, err, review.ErrInvalidStored, review.EvidenceExemption, wantFirstExemption)
		})
	}
}

// An exemption is evidence only while it holds to the rules it was made under. The file was edited after the record
// was made, and the record names the digest of the edited bytes, so the digest does not refuse it: a declared risk
// that names an item, a report that is not clear and a binding that names no candidate do.
func TestCheckEvidenceRefusesAnExemptionThatNoLongerHoldsItsRules(t *testing.T) {
	candidate := candidateOf(t, newFakeGit())
	encode := func(t *testing.T, risk, report string) []byte {
		t.Helper()
		data, err := review.EncodeExemption(candidate, risk, []byte(report))
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		return data
	}
	for _, tc := range []struct {
		name     string
		data     func(t *testing.T) []byte
		sentinel error
	}{
		{"the risk names an item", func(t *testing.T) []byte { return encode(t, "2", exemptReport) }, review.ErrHighRisk},
		{"the risk is a list", func(t *testing.T) []byte { return encode(t, "1,5", exemptReport) }, review.ErrHighRisk},
		{"the risk is not a declaration", func(t *testing.T) []byte { return encode(t, "None", exemptReport) }, review.ErrInvalidRisk},
		{"a check of the report failed", func(t *testing.T) []byte {
			return encode(t, "none", `{"checks":[{"command":"go test ./...","result":"fail","output":"FAIL"}]}`)
		}, reviewverdict.ErrNotClear},
		{"the report names no check", func(t *testing.T) []byte { return encode(t, "none", `{"checks":[]}`) }, reviewverdict.ErrNotClear},
		{"the report is not the object of the contract", func(t *testing.T) []byte { return encode(t, "none", `{"outcome":"clear"}`) }, reviewverdict.ErrUnknownMember},
		{"the binding names a base that is an option", func(t *testing.T) []byte {
			return bytes.Replace(encode(t, "none", exemptReport), []byte(baseSHA), []byte("--output=x"), 1)
		}, reviewresult.ErrInvalidBinding},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kept := keptExemption(t)
			record := kept.stored(tc.data(t))

			_, _, err := check(kept.store, record)

			assertEvidence(t, err, tc.sentinel, review.EvidenceExemption, wantFirstExemption)
		})
	}
}

// A completed review-exemption is held to the exemption, and an interrupted one, which says the exemption was not
// made, is not: it is accepted as it is without git or the store being asked anything, as an interrupted
// review-report is.
func TestCheckEvidenceLeavesAnInterruptedExemptionRecordAlone(t *testing.T) {
	kept := keptExemption(t)
	reason := "exemption not made: the verifier did not run"
	interrupted := kept.record
	interrupted.Status, interrupted.ResumeReason = roles.StatusInterrupted, &reason
	interrupted.Evidence = []roles.Evidence{{Kind: review.EvidenceExemption, Ref: "../../elsewhere", SHA256: "not a digest"}}

	gitCalls, storeCalls, err := check(kept.store, interrupted)

	if err != nil || len(gitCalls) != 0 || len(storeCalls) != 0 {
		t.Errorf("CheckEvidence() = %v with git calls %q and store calls %q; want nil and no call", err, gitCalls, storeCalls)
	}
}
