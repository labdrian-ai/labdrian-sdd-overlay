package app_test

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewverdict"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/roles"
)

// evidenceDir is the directory the commands run in, inside the working tree the fakes locate.
const evidenceDir = "/work/wt/feature"

// The files the second review of keptReviews leaves, as a record refers to them.
const (
	wantSecondPath    = "odd/review-results/own-rdd/222222222222-2.json"
	wantSecondVerdict = "odd/review-results/own-rdd/222222222222-2.verifier.json"
)

// assertEvidence requires err to wrap sentinel and to be an *EvidenceError that names the kind and the ref of the
// piece of evidence it refuses, in its fields and in its message, so that a person reads which piece was refused
// and what it pointed at. A piece the record does not have has no ref to name.
func assertEvidence(t *testing.T, err, sentinel error, kind, ref string) {
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

// reviewsKept is a results directory as the two commands leave it, and the records they print for it: a clean
// result that Record stored (number 1), and a second result with one blocker that Verify answered with a verdict
// that clears it (number 2).
type reviewsKept struct {
	store *fakeResults
	// recordRecord is the record Record printed for the first result: the range and the result.
	recordRecord roles.RoleHandoff
	// verifyRecord is the record Verify printed for the second: the range, the result and the verdict.
	verifyRecord roles.RoleHandoff
}

func keptReviews(t *testing.T) reviewsKept {
	t.Helper()
	g, store := newFakeGit(), newFakeResults()
	recorded, err := app.New(g, store).Record(evidenceDir, "own-rdd", "HEAD~1", "HEAD", replyFor(t, candidateOf(t, g)))
	if err != nil {
		t.Fatalf("setup: Record() error = %v", err)
	}
	second := reviewed(t, store, baseSHA, headSHA, 2, finding("F1", "BLOCKER"))
	verified, err := app.New(g, store).Verify(evidenceDir, "own-rdd", second, verdictReply(verdict("F1", "not-reproduced")))
	if err != nil {
		t.Fatalf("setup: Verify() error = %v", err)
	}
	return reviewsKept{store: store, recordRecord: recorded.Handoff, verifyRecord: verified.Handoff}
}

// check runs CheckEvidence of a fake git of its own over the store, and returns the calls git saw and the calls
// the store saw from then on, so that a test says which files were read and which were not.
func check(store *fakeResults, h roles.RoleHandoff) (gitCalls, storeCalls []string, err error) {
	g := newFakeGit()
	from := len(store.calls)
	err = app.New(g, store).CheckEvidence(evidenceDir, h)
	return g.calls, store.calls[from:], err
}

// edited is h with edit applied to every piece of evidence of the kind, on a copy of the evidence.
func edited(h roles.RoleHandoff, kind string, edit func(*roles.Evidence)) roles.RoleHandoff {
	h.Evidence = append([]roles.Evidence(nil), h.Evidence...)
	for i := range h.Evidence {
		if h.Evidence[i].Kind == kind {
			edit(&h.Evidence[i])
		}
	}
	return h
}

// without is h without the evidence of the kind.
func without(h roles.RoleHandoff, kind string) roles.RoleHandoff {
	var kept []roles.Evidence
	for _, e := range h.Evidence {
		if e.Kind != kind {
			kept = append(kept, e)
		}
	}
	h.Evidence = kept
	return h
}

// doubled is h with one more copy of its evidence of the kind.
func doubled(h roles.RoleHandoff, kind string) roles.RoleHandoff {
	h.Evidence = append([]roles.Evidence(nil), h.Evidence...)
	for _, e := range h.Evidence {
		if e.Kind == kind {
			h.Evidence = append(h.Evidence, e)
			break
		}
	}
	return h
}

// changeStored changes the bytes of the file stored under name, as another process could between the review and
// the append.
func changeStored(store *fakeResults, name string) {
	store.files["own-rdd"][name] = append(append([]byte(nil), store.files["own-rdd"][name]...), '\n')
}

// The records the two commands print are the records the check accepts: it reads the files they name and nothing
// else, asks git where the repository is and no more, and stores nothing.
func TestCheckEvidenceAcceptsTheRecordsThatRecordAndVerifyPrint(t *testing.T) {
	kept := keptReviews(t)
	for _, tc := range []struct {
		name      string
		record    roles.RoleHandoff
		wantReads []string
	}{
		{"the record of Record", kept.recordRecord, []string{"Read /work/wt/feature own-rdd 222222222222-1.json"}},
		{"the record of Verify", kept.verifyRecord, []string{
			"Read /work/wt/feature own-rdd 222222222222-2.json", "Read /work/wt/feature own-rdd 222222222222-2.verifier.json"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			created := kept.store.created()

			gitCalls, storeCalls, err := check(kept.store, tc.record)

			if err != nil {
				t.Fatalf("CheckEvidence() error = %v, want nil", err)
			}
			if want := []string{"Locate " + evidenceDir}; !reflect.DeepEqual(gitCalls, want) {
				t.Errorf("git calls = %q, want %q: the evidence is the stored files, not the repository", gitCalls, want)
			}
			if !reflect.DeepEqual(storeCalls, tc.wantReads) {
				t.Errorf("store calls = %q, want %q", storeCalls, tc.wantReads)
			}
			if kept.store.created() != created {
				t.Errorf("CheckEvidence() stored a file; it must only read")
			}
		})
	}
}

// The stored verdict is parsed against its own result, the one the record names: a result of two findings whose
// verdict answers both is accepted, so the parse is held to that result's findings and not to another reply's.
func TestCheckEvidenceAcceptsAVerdictOfEveryFindingOfItsResult(t *testing.T) {
	g, store := newFakeGit(), newFakeResults()
	stored := reviewed(t, store, baseSHA, headSHA, 1, finding("F1", "BLOCKER"), finding("F2", "WARNING"))
	verified, err := app.New(g, store).Verify(evidenceDir, "own-rdd", stored,
		verdictReply(verdict("F1", "not-reproduced"), verdict("F2", "not-reproduced")))
	if err != nil {
		t.Fatalf("setup: Verify() error = %v", err)
	}

	if _, _, err := check(store, verified.Handoff); err != nil {
		t.Errorf("CheckEvidence() of a verdict that answers both findings of its result error = %v, want nil", err)
	}
}

// A record that closes a review that ran carries the range with the digest of its diff and the stored result;
// the verdict is optional. One that lacks a needed piece, or gives a kind twice so that neither piece is the
// evidence of its kind, is refused with the kind named, and before any file is read.
func TestCheckEvidenceRefusesARecordThatLacksOrRepeatsAPieceBeforeReadingAnything(t *testing.T) {
	kept := keptReviews(t)
	noEvidence := kept.verifyRecord
	noEvidence.Evidence = nil
	for _, tc := range []struct {
		name     string
		record   roles.RoleHandoff
		sentinel error
		kind     string
	}{
		{"it has no review-diff", without(kept.verifyRecord, review.EvidenceDiff), review.ErrEvidenceMissing, review.EvidenceDiff},
		{"it has no review-result", without(kept.verifyRecord, review.EvidenceResult), review.ErrEvidenceMissing, review.EvidenceResult},
		{"it has no evidence at all", noEvidence, review.ErrEvidenceMissing, review.EvidenceDiff},
		{"it has the review-diff twice", doubled(kept.verifyRecord, review.EvidenceDiff), review.ErrEvidenceRepeated, review.EvidenceDiff},
		{"it has the review-result twice", doubled(kept.verifyRecord, review.EvidenceResult), review.ErrEvidenceRepeated, review.EvidenceResult},
		{"it has the verifier-report twice", doubled(kept.verifyRecord, review.EvidenceVerifier), review.ErrEvidenceRepeated, review.EvidenceVerifier},
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
// with the result: a digest of another file, a file changed after the record was made, a digest of another diff
// and another range are all refused, with the kind and the ref of the piece named.
func TestCheckEvidenceRefusesEvidenceWhoseDigestIsNotThatOfTheStoredFile(t *testing.T) {
	for _, tc := range []struct {
		name string
		act  func(k reviewsKept) roles.RoleHandoff
		kind string
		ref  string
	}{
		{"the review-result has the digest of another file", func(k reviewsKept) roles.RoleHandoff {
			return edited(k.verifyRecord, review.EvidenceResult, func(e *roles.Evidence) { e.SHA256 = sha256Of([]byte("another file")) })
		}, review.EvidenceResult, wantSecondPath},
		{"the stored result was changed after the record was made", func(k reviewsKept) roles.RoleHandoff {
			changeStored(k.store, "222222222222-2.json")
			return k.verifyRecord
		}, review.EvidenceResult, wantSecondPath},
		{"the review-diff has the digest of another diff", func(k reviewsKept) roles.RoleHandoff {
			return edited(k.verifyRecord, review.EvidenceDiff, func(e *roles.Evidence) { e.SHA256 = sha256Of([]byte("another diff")) })
		}, review.EvidenceDiff, baseSHA + ".." + headSHA},
		{"the review-diff is the range of other commits", func(k reviewsKept) roles.RoleHandoff {
			return edited(k.verifyRecord, review.EvidenceDiff, func(e *roles.Evidence) { e.Ref = baseSHA + ".." + otherSHA })
		}, review.EvidenceDiff, baseSHA + ".." + otherSHA},
		{"the verifier-report has the digest of another file", func(k reviewsKept) roles.RoleHandoff {
			return edited(k.verifyRecord, review.EvidenceVerifier, func(e *roles.Evidence) { e.SHA256 = sha256Of([]byte("another verdict")) })
		}, review.EvidenceVerifier, wantSecondVerdict},
		{"the stored verdict was changed after the record was made", func(k reviewsKept) roles.RoleHandoff {
			changeStored(k.store, "222222222222-2.verifier.json")
			return k.verifyRecord
		}, review.EvidenceVerifier, wantSecondVerdict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kept := keptReviews(t)
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

// A ref to a file that is not stored, and a store that cannot read the file, are evidence that cannot be held to
// anything: the error names the kind and the ref, and says what the store said.
func TestCheckEvidenceRefusesAReferenceToAFileThatCannotBeRead(t *testing.T) {
	cause := errors.New("the disk is gone")
	for _, tc := range []struct {
		name     string
		act      func(k reviewsKept) roles.RoleHandoff
		sentinel error
		kind     string
		ref      string
	}{
		{"the result is not stored", func(k reviewsKept) roles.RoleHandoff {
			return edited(k.recordRecord, review.EvidenceResult, func(e *roles.Evidence) { e.Ref = "odd/review-results/own-rdd/222222222222-9.json" })
		}, app.ErrNotFound, review.EvidenceResult, "odd/review-results/own-rdd/222222222222-9.json"},
		{"the verdict is not stored", func(k reviewsKept) roles.RoleHandoff {
			delete(k.store.files["own-rdd"], "222222222222-2.verifier.json")
			return k.verifyRecord
		}, app.ErrNotFound, review.EvidenceVerifier, wantSecondVerdict},
		{"the store cannot read the result", func(k reviewsKept) roles.RoleHandoff {
			k.store.fail["Read"] = cause
			return k.recordRecord
		}, cause, review.EvidenceResult, wantFirstPath},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kept := keptReviews(t)
			record := tc.act(kept)

			_, _, err := check(kept.store, record)

			assertEvidence(t, err, tc.sentinel, tc.kind, tc.ref)
		})
	}
}

// The refs are read as the files the engine stores and nothing else: a ref that leaves the results directory, or
// is not a stored result, and a verdict ref that is not the verdict beside the result, are refused by their shape
// before git or the store is asked anything.
func TestCheckEvidenceReadsOnlyTheFilesOfTheResultsDirectory(t *testing.T) {
	kept := keptReviews(t)
	for _, tc := range []struct {
		name   string
		record roles.RoleHandoff
		kind   string
		ref    string
	}{
		{"a result outside the repository", edited(kept.recordRecord, review.EvidenceResult, func(e *roles.Evidence) { e.Ref = "/etc/passwd" }), review.EvidenceResult, "/etc/passwd"},
		{"a result that climbs out of the directory", edited(kept.recordRecord, review.EvidenceResult, func(e *roles.Evidence) {
			e.Ref = "odd/review-results/own-rdd/../../../etc/passwd"
		}), review.EvidenceResult, "odd/review-results/own-rdd/../../../etc/passwd"},
		{"a result that is a verdict", edited(kept.recordRecord, review.EvidenceResult, func(e *roles.Evidence) { e.Ref = wantSecondVerdict }), review.EvidenceResult, wantSecondVerdict},
		{"a verdict that is the result itself", edited(kept.verifyRecord, review.EvidenceVerifier, func(e *roles.Evidence) { e.Ref = wantSecondPath }), review.EvidenceVerifier, wantSecondPath},
		{"a verdict of another result", edited(kept.verifyRecord, review.EvidenceVerifier, func(e *roles.Evidence) { e.Ref = "odd/review-results/own-rdd/222222222222-1.verifier.json" }),
			review.EvidenceVerifier, "odd/review-results/own-rdd/222222222222-1.verifier.json"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gitCalls, storeCalls, err := check(kept.store, tc.record)

			assertEvidence(t, err, review.ErrEvidenceRef, tc.kind, tc.ref)
			if len(gitCalls) != 0 || len(storeCalls) != 0 {
				t.Errorf("git calls = %q, store calls = %q; want none, the ref is refused by its shape", gitCalls, storeCalls)
			}
		})
	}
}

// A file with the digest the record gives is evidence only when it is a result: bytes that are not one are refused,
// as the use case of the verifier refuses them, with the kind and the ref named.
func TestCheckEvidenceRefusesAStoredFileThatIsNotAResult(t *testing.T) {
	kept := keptReviews(t)
	notAResult := []byte("this is not a stored result")
	kept.store.put("own-rdd", "222222222222-1.json", notAResult)
	record := edited(kept.recordRecord, review.EvidenceResult, func(e *roles.Evidence) { e.SHA256 = sha256Of(notAResult) })

	_, _, err := check(kept.store, record)

	assertEvidence(t, err, review.ErrInvalidStored, review.EvidenceResult, wantFirstPath)
}

// A file with the digest the record gives is evidence of a verdict only when it is a verdict of the result it sits
// beside, as a result is evidence only when it is a result: bytes that are not a verdict, and a verdict of a finding
// the result does not have, are refused with the kind and the ref named. What the verdict makes of the review is
// Check's.
func TestCheckEvidenceRefusesAStoredFileThatIsNotAVerdictOfItsResult(t *testing.T) {
	for _, tc := range []struct {
		name     string
		stored   []byte
		sentinel error
	}{
		{"bytes that are not a verdict", []byte("this is not a verdict"), reviewverdict.ErrUnparseable},
		{"a verdict of a finding the result does not have", verdictReply(verdict("F9", "not-reproduced")), reviewverdict.ErrUnknownID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kept := keptReviews(t)
			kept.store.put("own-rdd", "222222222222-2.verifier.json", tc.stored)
			record := edited(kept.verifyRecord, review.EvidenceVerifier, func(e *roles.Evidence) { e.SHA256 = sha256Of(tc.stored) })

			_, _, err := check(kept.store, record)

			assertEvidence(t, err, tc.sentinel, review.EvidenceVerifier, wantSecondVerdict)
		})
	}
}

// A record of any other kind, and a review-report that says the review did not run, is not held to evidence: it
// is accepted as it is without git or the store being asked anything, so `roles append` of such a record behaves
// as it did before.
func TestCheckEvidenceLeavesTheOtherRecordsAlone(t *testing.T) {
	kept := keptReviews(t)
	somewhere := []roles.Evidence{{Kind: review.EvidenceResult, Ref: "../../elsewhere", SHA256: "not a digest"}}
	reason := "review not run: the inspection was incomplete"
	interrupted := kept.verifyRecord
	interrupted.Status, interrupted.ResumeReason, interrupted.Evidence = roles.StatusInterrupted, &reason, somewhere
	progress := kept.verifyRecord
	progress.PayloadKind, progress.Evidence = "progress", somewhere
	for name, record := range map[string]roles.RoleHandoff{"an interrupted review-report": interrupted, "a progress record": progress} {
		t.Run(name, func(t *testing.T) {
			gitCalls, storeCalls, err := check(kept.store, record)

			if err != nil || len(gitCalls) != 0 || len(storeCalls) != 0 {
				t.Errorf("CheckEvidence() = %v with git calls %q and store calls %q; want nil and no call", err, gitCalls, storeCalls)
			}
		})
	}
}

// A repository that cannot be located is not a piece of evidence that is wrong: the error says so, wraps what
// git said, and is not an *EvidenceError.
func TestCheckEvidenceSaysWhenTheRepositoryCannotBeLocated(t *testing.T) {
	kept := keptReviews(t)
	cause := errors.New("not a git repository")
	g := newFakeGit()
	g.fail["Locate"] = cause

	err := app.New(g, kept.store).CheckEvidence(evidenceDir, kept.recordRecord)

	var ee *review.EvidenceError
	if !errors.Is(err, cause) || errors.As(err, &ee) || !strings.Contains(err.Error(), evidenceDir) {
		t.Errorf("CheckEvidence() error = %v, want one that wraps %q, names %s and is not an *EvidenceError", err, cause, evidenceDir)
	}
}
