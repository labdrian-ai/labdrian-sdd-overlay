package app_test

// Exempt is the use case behind `labdrian review exempt` (own-rdd T4, S5, L16): it holds the risk the agent declares
// and the report of the verifier to their rules, binds the head to the candidate git gives now, and stores the
// exemption beside the results of the feature. Nothing is stored unless every rule holds, and a refusal is made
// before git or the store is asked anything it does not need.

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewverdict"
)

const wantFirstExemption = "odd/review-results/own-rdd/222222222222-1.exempt.json"

// exemptReport is the report of a verifier that ran one check on the head and found it passing.
const exemptReport = `{"checks":[{"command":"go test ./review/...","result":"pass","output":"ok  review 0.009s"}]}`

// Exempt computes the candidate from git, holds the risk and the report to their rules, and stores the exemption
// with the binding in the next free file of the feature. What it returns is what the command prints: where the file
// is, its digest and the skeleton of the record of the chain.
func TestExemptStoresTheExemptionWithTheBindingTheRiskAndTheReport(t *testing.T) {
	g, store := newFakeGit(), newFakeResults()
	c := candidateOf(t, g)

	got, err := app.New(g, store).Exempt("/work/wt/feature", "own-rdd", "HEAD~1", "HEAD", "none", []byte(exemptReport))
	if err != nil {
		t.Fatalf("Exempt() error = %v, want nil", err)
	}

	wantFile, err := review.EncodeExemption(c, "none", []byte(exemptReport))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if stored := store.files["own-rdd"]["222222222222-1.exempt.json"]; string(stored) != string(wantFile) {
		t.Errorf("stored file = %q, want the report with its binding and the risk: %q", stored, wantFile)
	}
	if got.Path != wantFirstExemption || got.Number != 1 {
		t.Errorf("Path = %q, Number = %d; want %q and 1", got.Path, got.Number, wantFirstExemption)
	}
	if want := sha256Of(wantFile); got.SHA256 != want {
		t.Errorf("SHA256 = %q, want the digest of the stored bytes %q", got.SHA256, want)
	}
	if want := review.ExemptionHandoff(c, wantFirstExemption, sha256Of(wantFile)); !reflect.DeepEqual(got.Handoff, want) {
		t.Errorf("Handoff = %+v, want %+v", got.Handoff, want)
	}
	if !reflect.DeepEqual(got.Candidate, c) {
		t.Errorf("Candidate = %+v, want %+v", got.Candidate, c)
	}
	if got.NotFlushed != nil {
		t.Errorf("NotFlushed = %v, want nil", got.NotFlushed)
	}
	// It stores in the root of the working tree the command ran in, not in the main checkout.
	if want := "Names " + fakeRepo.Root + " own-rdd"; len(store.calls) == 0 || store.calls[0] != want {
		t.Errorf("store calls = %q, want the first to be %q", store.calls, want)
	}
	if want := "Create " + fakeRepo.Root + " own-rdd 222222222222-1.exempt.json"; len(store.calls) != 2 || store.calls[1] != want {
		t.Errorf("store calls = %q, want one listing and then %q", store.calls, want)
	}
}

// The binding is computed from git exactly as `review record` computes it: the same calls, in the same order, and
// no other way to get a diff. The one question more is the first parent of the head, because an exemption is for one
// commit (own-rdd T4, L21).
func TestExemptComputesTheBindingFromGitAsRecordDoes(t *testing.T) {
	gExempt, storeExempt := newFakeGit(), newFakeResults()
	if _, err := app.New(gExempt, storeExempt).Exempt("/work/wt/feature", "own-rdd", "HEAD~1", "HEAD", "none", []byte(exemptReport)); err != nil {
		t.Fatalf("Exempt() error = %v, want nil", err)
	}
	gRecord, storeRecord := newFakeGit(), newFakeResults()
	if _, err := app.New(gRecord, storeRecord).Record("/work/wt/feature", "own-rdd", "HEAD~1", "HEAD", "default", replyFor(t, candidateOf(t, gRecord))); err != nil {
		t.Fatalf("setup: Record() error = %v", err)
	}

	if want := append(append([]string(nil), gRecord.calls...), "Parent "+headSHA); !reflect.DeepEqual(gExempt.calls, want) {
		t.Errorf("Exempt asked git %q, Record asked %q; want the same questions in the same order and then the parent of the head", gExempt.calls, gRecord.calls)
	}
}

// Whatever keeps git from saying the first parent of the head keeps the head from being exempted, for the exemption
// that is made and for the one that is read back: a rule that cannot be checked refuses, and nothing is stored.
func TestExemptAndCheckRefuseWhenGitCannotSayTheFirstParentOfTheHead(t *testing.T) {
	broken := errors.New("git: not a repository")
	t.Run("Exempt", func(t *testing.T) {
		g, store := newFakeGit(), newFakeResults()
		g.fail["Parent"] = broken

		_, err := app.New(g, store).Exempt("/work/wt/feature", "own-rdd", "HEAD~1", "HEAD", "none", []byte(exemptReport))

		if !errors.Is(err, broken) || store.created() != 0 {
			t.Errorf("Exempt() error = %v with %d files created; want one that wraps %v and nothing stored", err, store.created(), broken)
		}
	})
	t.Run("Check", func(t *testing.T) {
		g, store := newFakeGit(), newFakeResults()
		exempted(t, store, baseSHA, headSHA, 1)
		g.fail["Parent"] = broken

		got, err := app.New(g, store).Check(evidenceDir, "own-rdd", "HEAD")

		if !errors.Is(err, broken) || len(got.Exemptions) != 0 {
			t.Errorf("Check() = %+v, %v; want one that wraps %v and no exemption that cleared the head", got, err, broken)
		}
	})
}

// The three kinds of file of a head share one numbering: an exemption takes the next number after the results and
// the verdicts the head already has, and nothing that is stored is changed.
func TestExemptTakesTheNextFreeNumberAmongTheResultsTheVerdictsAndTheExemptionsOfTheHead(t *testing.T) {
	g, store := newFakeGit(), newFakeResults()
	store.put("own-rdd", "222222222222-1.json", []byte("first launch"))
	store.put("own-rdd", "222222222222-1.verifier.json", []byte("its verdict"))
	store.put("own-rdd", "222222222222-2.exempt.json", []byte("an exemption"))
	store.put("own-rdd", "333333333333-9.json", []byte("another head"))

	got, err := app.New(g, store).Exempt("/work/wt/feature", "own-rdd", "HEAD~1", "HEAD", "none", []byte(exemptReport))
	if err != nil {
		t.Fatalf("Exempt() error = %v, want nil", err)
	}

	if want := "odd/review-results/own-rdd/222222222222-3.exempt.json"; got.Path != want || got.Number != 3 {
		t.Errorf("Path = %q, Number = %d; want %q and 3", got.Path, got.Number, want)
	}
	for name, content := range map[string]string{
		"222222222222-1.json": "first launch", "222222222222-1.verifier.json": "its verdict",
		"222222222222-2.exempt.json": "an exemption", "333333333333-9.json": "another head",
	} {
		if string(store.files["own-rdd"][name]) != content {
			t.Errorf("%s was changed: %q, want %q", name, store.files["own-rdd"][name], content)
		}
	}
}

// Two writers racing for one number: the one that lost lists the directory again and takes the next, and the file
// of the other writer is left as it is.
func TestExemptTakesTheNextNumberWhenAnotherWriterTookItFirst(t *testing.T) {
	g, store := newFakeGit(), newFakeResults()
	store.races = 1

	got, err := app.New(g, store).Exempt("/work/wt/feature", "own-rdd", "HEAD~1", "HEAD", "none", []byte(exemptReport))
	if err != nil {
		t.Fatalf("Exempt() error = %v, want nil", err)
	}

	if want := "odd/review-results/own-rdd/222222222222-2.exempt.json"; got.Path != want || got.Number != 2 {
		t.Errorf("Path = %q, Number = %d; want %q and 2", got.Path, got.Number, want)
	}
	if string(store.files["own-rdd"]["222222222222-1.exempt.json"]) != otherWriter {
		t.Errorf("the file of the other writer was changed: %q", store.files["own-rdd"]["222222222222-1.exempt.json"])
	}
}

// A directory that never has a free number is an error, not a loop, and a file is never written over another:
// sixteen tries, each one listing the directory again, and nothing stored.
func TestExemptGivesUpWhenNoNumberStaysFreeAndWritesOverNothing(t *testing.T) {
	g, store := newFakeGit(), newFakeResults()
	store.put("own-rdd", "222222222222-1.json", []byte("a result"))
	store.fail["Create"] = app.ErrExists

	_, err := app.New(g, store).Exempt("/work/wt/feature", "own-rdd", "HEAD~1", "HEAD", "none", []byte(exemptReport))

	if !errors.Is(err, app.ErrNoFreeNumber) {
		t.Fatalf("Exempt() error = %v, want one that is %v", err, app.ErrNoFreeNumber)
	}
	if n := store.created(); n != 16 {
		t.Errorf("Create was called %d times, want exactly 16 tries", n)
	}
	if len(store.files["own-rdd"]) != 1 || string(store.files["own-rdd"]["222222222222-1.json"]) != "a result" {
		t.Errorf("the store holds %v, want only the result that was there, untouched", store.files["own-rdd"])
	}
}

// A declared risk that names an item of the high-risk list is never exempt, whichever it is and however many are
// named, and a declaration the engine cannot read is not taken for the absence of risk. The refusal is made before
// git or the store is asked anything, and nothing is stored.
func TestExemptRefusesADeclaredRiskThatIsNotNoneAndAsksNoOne(t *testing.T) {
	for _, tc := range []struct {
		risk string
		want error
	}{
		{"1", review.ErrHighRisk}, {"2", review.ErrHighRisk}, {"3", review.ErrHighRisk},
		{"4", review.ErrHighRisk}, {"5", review.ErrHighRisk}, {"6", review.ErrHighRisk},
		{"2,4", review.ErrHighRisk}, {"6,1,3", review.ErrHighRisk},
		{"", review.ErrInvalidRisk}, {"None", review.ErrInvalidRisk}, {"no", review.ErrInvalidRisk},
		{"7", review.ErrInvalidRisk}, {"1,1", review.ErrInvalidRisk}, {"1, 2", review.ErrInvalidRisk},
	} {
		t.Run(tc.risk, func(t *testing.T) {
			g, store := newFakeGit(), newFakeResults()

			_, err := app.New(g, store).Exempt("/work/wt/feature", "own-rdd", "HEAD~1", "HEAD", tc.risk, []byte(exemptReport))

			if !errors.Is(err, tc.want) {
				t.Fatalf("Exempt() error = %v, want one that is %v", err, tc.want)
			}
			var refused *app.ReplyRefusedError
			if errors.As(err, &refused) {
				t.Errorf("Exempt() error = %v: a declared risk is not a reply, so it is not a *app.ReplyRefusedError", err)
			}
			if len(g.calls) != 0 || len(store.calls) != 0 {
				t.Errorf("git calls %q and store calls %q, want none for a refused risk", g.calls, store.calls)
			}
		})
	}
}

// A report that is missing, is not the object of the contract, or is not clear is refused as a reply is, naming the
// rule it breaks, and nothing is stored. A check that did not run never passes.
func TestExemptRefusesAReportThatIsMissingOrNotClearAndStoresNothing(t *testing.T) {
	for _, tc := range []struct {
		name   string
		report []byte
		want   error
	}{
		{"no report", nil, reviewverdict.ErrUnparseable},
		{"an empty report", []byte("  \n"), reviewverdict.ErrUnparseable},
		{"prose", []byte("All checks passed."), reviewverdict.ErrUnparseable},
		{"a member the contract does not name", []byte(`{"checks":[],"outcome":"clear"}`), reviewverdict.ErrUnknownMember},
		{"a result outside its enumeration", []byte(`{"checks":[{"command":"c","result":"ok","output":"o"}]}`), reviewverdict.ErrOutsideEnum},
		{"a check without a command", []byte(`{"checks":[{"command":"","result":"pass","output":"o"}]}`), reviewverdict.ErrMissingEvidence},
		{"a check without output", []byte(`{"checks":[{"command":"c","result":"pass","output":""}]}`), reviewverdict.ErrMissingEvidence},
		{"no check", []byte(`{"checks":[]}`), reviewverdict.ErrNotClear},
		{"a check that failed", []byte(`{"checks":[{"command":"c","result":"pass","output":"o"},{"command":"d","result":"fail","output":"FAIL"}]}`), reviewverdict.ErrNotClear},
		{"a check that did not run", []byte(`{"checks":[{"command":"c","result":"not-run","output":"no toolchain"}]}`), reviewverdict.ErrNotClear},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, store := newFakeGit(), newFakeResults()

			_, err := app.New(g, store).Exempt("/work/wt/feature", "own-rdd", "HEAD~1", "HEAD", "none", tc.report)

			var refused *app.ReplyRefusedError
			if !errors.As(err, &refused) {
				t.Fatalf("Exempt() error = %v, want a *app.ReplyRefusedError", err)
			}
			if !errors.Is(err, tc.want) {
				t.Errorf("Exempt() error = %v, want one that is %v", err, tc.want)
			}
			if !strings.Contains(err.Error(), tc.want.Error()) {
				t.Errorf("Exempt() error = %q, want the message to carry the named error %q", err, tc.want)
			}
			if len(g.calls) != 0 || len(store.calls) != 0 {
				t.Errorf("git calls %q and store calls %q, want none for a refused report", g.calls, store.calls)
			}
		})
	}
}

// The size of a report is bounded as the size of a reply is: one byte over is refused, and exactly the limit is read
// like any other (here a clear report with white space after it).
func TestExemptBoundsTheReportLikeAReply(t *testing.T) {
	over := []byte(strings.Repeat(" ", review.MaxReplyBytes+1))
	g, store := newFakeGit(), newFakeResults()
	_, err := app.New(g, store).Exempt("/work/wt/feature", "own-rdd", "HEAD~1", "HEAD", "none", over)
	var refused *app.ReplyRefusedError
	if !errors.As(err, &refused) || !errors.Is(err, review.ErrReplyTooLarge) {
		t.Fatalf("Exempt() error = %v, want a refusal that is %v", err, review.ErrReplyTooLarge)
	}
	if len(g.calls) != 0 || len(store.calls) != 0 {
		t.Errorf("git calls %q and store calls %q, want none", g.calls, store.calls)
	}

	exact := append([]byte(exemptReport), strings.Repeat(" ", review.MaxReplyBytes-len(exemptReport))...)
	if len(exact) != review.MaxReplyBytes {
		t.Fatalf("setup: the report has %d bytes, want %d", len(exact), review.MaxReplyBytes)
	}
	g, store = newFakeGit(), newFakeResults()
	got, err := app.New(g, store).Exempt("/work/wt/feature", "own-rdd", "HEAD~1", "HEAD", "none", exact)
	if err != nil {
		t.Fatalf("Exempt() error = %v, want nil for a report of exactly %d bytes", err, review.MaxReplyBytes)
	}
	if got.Path != wantFirstExemption || len(store.files["own-rdd"]["222222222222-1.exempt.json"]) == 0 {
		t.Errorf("Path = %q, want %q stored", got.Path, wantFirstExemption)
	}
}

// A feature or a revision that is not safe is refused before git or the store is asked for anything.
func TestExemptRefusesAnUnsafeFeatureOrRevisionBeforeAskingAnyone(t *testing.T) {
	for _, tc := range []struct {
		name                string
		feature, base, head string
		want                error
	}{
		{"a way up", "../x", "HEAD~1", "HEAD", review.ErrInvalidFeature},
		{"a separator", "a/b", "HEAD~1", "HEAD", review.ErrInvalidFeature},
		{"no feature", "", "HEAD~1", "HEAD", review.ErrInvalidFeature},
		{"an option as the base", "own-rdd", "--output=x", "HEAD", review.ErrInvalidRev},
		{"an option as the head", "own-rdd", "HEAD~1", "--output=x", review.ErrInvalidRev},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, store := newFakeGit(), newFakeResults()

			_, err := app.New(g, store).Exempt("/work/wt/feature", tc.feature, tc.base, tc.head, "none", []byte(exemptReport))

			if !errors.Is(err, tc.want) {
				t.Errorf("Exempt() error = %v, want one that is %v", err, tc.want)
			}
			if len(g.calls) != 0 || len(store.calls) != 0 {
				t.Errorf("git calls %q and store calls %q, want none", g.calls, store.calls)
			}
		})
	}
}

// What git or the store could not do is the error of the command, with the cause, and nothing is created after it.
func TestExemptReturnsWhatGitOrTheStoreCouldNotDo(t *testing.T) {
	boom := errors.New("boom")
	for _, tc := range []struct {
		name        string
		failGit     string
		failStore   string
		wantCreates int
	}{
		{"git cannot locate the repository", "Locate", "", 0},
		{"git cannot resolve the head", "Commit HEAD", "", 0},
		{"git cannot read the diff", "Diff", "", 0},
		{"git cannot list the paths", "Paths", "", 0},
		{"the store cannot list the directory", "", "Names", 0},
		{"the store cannot create the file", "", "Create", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, store := newFakeGit(), newFakeResults()
			if tc.failGit != "" {
				g.fail[tc.failGit] = boom
			}
			if tc.failStore != "" {
				store.fail[tc.failStore] = boom
			}

			_, err := app.New(g, store).Exempt("/work/wt/feature", "own-rdd", "HEAD~1", "HEAD", "none", []byte(exemptReport))

			if !errors.Is(err, boom) {
				t.Errorf("Exempt() error = %v, want one that wraps %v", err, boom)
			}
			if n := store.created(); n != tc.wantCreates {
				t.Errorf("Create was called %d times, want %d (an error that is not a taken name is not retried)", n, tc.wantCreates)
			}
		})
	}
}

// A store that published the exemption but could not flush its directory stored it all the same: Exempt answers
// with what it stored and says the flush failed, instead of a failure a retry would answer with a second copy.
func TestExemptReportsAnExemptionWhoseDirectoryWasNotFlushedAsStored(t *testing.T) {
	g, store := newFakeGit(), newFakeResults()
	store.unflushed = fmt.Errorf("%w: sync: input/output error", app.ErrNotFlushed)

	got, err := app.New(g, store).Exempt("/work/wt/feature", "own-rdd", "HEAD~1", "HEAD", "none", []byte(exemptReport))

	if err != nil {
		t.Fatalf("Exempt() error = %v, want nil: the exemption is stored", err)
	}
	if got.Path != wantFirstExemption || got.Number != 1 {
		t.Errorf("Path = %q, Number = %d; want %q and 1", got.Path, got.Number, wantFirstExemption)
	}
	if !errors.Is(got.NotFlushed, app.ErrNotFlushed) {
		t.Errorf("NotFlushed = %v, want an error wrapping ErrNotFlushed", got.NotFlushed)
	}
	if n := store.created(); n != 1 {
		t.Errorf("Create called %d times, want 1: a stored exemption is not stored again", n)
	}
}

// A result is still stored under its own name and in the next number of the head when the head already has an
// exemption: the numbering is shared, and `review record` keeps writing <head12>-<n>.json.
func TestRecordStillStoresAResultUnderItsOwnNameBesideAnExemption(t *testing.T) {
	g, store := newFakeGit(), newFakeResults()
	store.put("own-rdd", "222222222222-1.exempt.json", []byte("an exemption"))

	got, err := app.New(g, store).Record("/work/wt/feature", "own-rdd", "HEAD~1", "HEAD", "default", replyFor(t, candidateOf(t, g)))
	if err != nil {
		t.Fatalf("Record() error = %v, want nil", err)
	}

	if want := "odd/review-results/own-rdd/222222222222-2.json"; got.Path != want || got.Number != 2 {
		t.Errorf("Path = %q, Number = %d; want %q and 2", got.Path, got.Number, want)
	}
	if string(store.files["own-rdd"]["222222222222-1.exempt.json"]) != "an exemption" {
		t.Errorf("the exemption was changed: %q", store.files["own-rdd"]["222222222222-1.exempt.json"])
	}
}
