package app_test

import (
	"encoding/json"
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

const (
	fourSHA     = "4444444444444444444444444444444444444444" // the base of a candidate other than the fake git's
	wantVerdict = "odd/review-results/own-rdd/222222222222-1.verifier.json"
)

// finding is a finding of a reviewer's reply that the diff introduced, with the proof a severe one needs.
func finding(id, severity string) map[string]any {
	return map[string]any{
		"id": id, "location": "engine/guard.go:10", "severity": severity, "claim": "the guard is skipped",
		"evidence_class": "deterministic", "causal_disposition": "introduced", "proof_refs": []string{"engine/guard.go:10"},
	}
}

// reviewed stores the result number n of the candidate from base to head with the findings, as Record does, and
// returns its name.
func reviewed(t *testing.T, store *fakeResults, base, head string, n int, findings ...map[string]any) string {
	t.Helper()
	c, err := review.NewCandidate(base, head, []byte(diffText), changedPaths)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	reply, err := json.Marshal(map[string]any{
		"candidate":  map[string]string{"base": c.Base, "head": c.Head, "diff_sha256": c.DiffSHA256},
		"inspection": map[string]any{"status": "completed", "paths": c.Paths, "reason": ""},
		"findings":   append([]map[string]any{}, findings...),
		"evidence":   []string{"read every changed path"},
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	file, err := review.EncodeResult(c, reply)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	store.put("own-rdd", review.ResultName(c, n), file)
	return review.ResultName(c, n)
}

// verdict is one verdict of a verifier's reply; a check that did not run names no command.
func verdict(id, result string) string {
	command := "go test ./engine/"
	if result == "not-run" {
		command = ""
	}
	return fmt.Sprintf(`{"id":%q,"result":%q,"command":%q,"output":"the output","base_result":"not-run"}`, id, result, command)
}

func verdictReply(entries ...string) []byte {
	return []byte(`{"verdicts":[` + strings.Join(entries, ",") + `]}`)
}

// blocking is the reply of a verifier that reproduces F1, the one BLOCKER of the results below.
var blocking = verdictReply(verdict("F1", "reproduced"))

// Verify reads the stored result, holds the verdicts to it, decides, and stores the reply of the verifier
// beside the result. What it returns is what the command prints: where the verdict is, its digest, the
// decision and the skeleton of the record of the chain, which binds the result and the verdict.
func TestVerifyStoresTheVerdictBesideTheResultAndDecides(t *testing.T) {
	g, store := newFakeGit(), newFakeResults()
	name := reviewed(t, store, baseSHA, headSHA, 1, finding("F1", "BLOCKER"), finding("F2", "WARNING"))
	resultFile := store.files["own-rdd"][name]
	reply := verdictReply(verdict("F1", "reproduced"), verdict("F2", "not-run"))

	got, err := app.New(g, store).Verify("/work/wt/feature", "own-rdd", name, reply)

	wantFile, _ := review.EncodeVerdict(reply)
	wantDecision := reviewverdict.Decision{Outcome: reviewverdict.Blocks, Blocking: []string{"F1"}, Fix: []string{"F2"}}
	wantHandoff := review.VerifiedHandoff(candidateOf(t, g).Binding(), wantFirstPath, sha256Of(resultFile), wantVerdict, sha256Of(wantFile), reviewverdict.Blocks)
	if err != nil || got.Path != wantVerdict || got.SHA256 != sha256Of(wantFile) {
		t.Fatalf("Verify() = %q, %q, %v; want the verdict stored at %s with the digest of its bytes %s", got.Path, got.SHA256, err, wantVerdict, sha256Of(wantFile))
	}
	if !reflect.DeepEqual(got.Decision, wantDecision) || !reflect.DeepEqual(got.Handoff, wantHandoff) {
		t.Errorf("Decision = %+v, Handoff = %+v; want %+v and %+v", got.Decision, got.Handoff, wantDecision, wantHandoff)
	}
	if stored := store.files["own-rdd"]["222222222222-1.verifier.json"]; string(stored) != string(wantFile) {
		t.Errorf("stored verdict = %q, want the reply of the verifier: %q", stored, wantFile)
	}
	if string(store.files["own-rdd"][name]) != string(resultFile) {
		t.Errorf("the stored result was changed")
	}
	// The candidate is the one the result was stored with: git is asked where the repository is, and no more.
	if want := []string{"Locate /work/wt/feature"}; !reflect.DeepEqual(g.calls, want) {
		t.Errorf("git calls = %q, want %q", g.calls, want)
	}
}

// The outcome decides who is next: the builder when a finding blocks, the delivery when the review is clear,
// and nobody yet when a check did not run.
func TestVerifyEndsTheReviewAsTheVerdictsDecide(t *testing.T) {
	for result, want := range map[string]struct {
		outcome reviewverdict.Outcome
		next    roles.Role
	}{
		"reproduced": {reviewverdict.Blocks, roles.RoleBuilder}, "not-run": {reviewverdict.Unverified, ""}, "not-reproduced": {reviewverdict.Clear, roles.RoleDelivery},
	} {
		t.Run(result, func(t *testing.T) {
			g, store := newFakeGit(), newFakeResults()
			name := reviewed(t, store, baseSHA, headSHA, 1, finding("F1", "BLOCKER"))

			got, err := app.New(g, store).Verify("/work/wt/feature", "own-rdd", name, verdictReply(verdict("F1", result)))

			if err != nil || got.Decision.Outcome != want.outcome || got.Handoff.ToRole != want.next || store.created() != 1 {
				t.Errorf("Verify() = outcome %q, to_role %q, %v with %d creates; want %q, %q and one create", got.Decision.Outcome, got.Handoff.ToRole, err, store.created(), want.outcome, want.next)
			}
		})
	}
}

// A reply that breaks a rule of the contract is a verification that did not run: it is refused with the named
// rule, and nothing is stored.
func TestVerifyRefusesAReplyThatBreaksTheContractAndStoresNothing(t *testing.T) {
	for _, tc := range []struct {
		name  string
		reply []byte
		want  error
	}{
		{"it is prose", []byte("Both findings reproduce."), reviewverdict.ErrUnparseable},
		{"it answers an id no finding has", verdictReply(verdict("F1", "reproduced"), verdict("F9", "not-run")), reviewverdict.ErrUnknownID},
		{"it answers a finding twice", verdictReply(verdict("F1", "reproduced"), verdict("F1", "reproduced")), reviewverdict.ErrDuplicateID},
		{"it holds a value outside the enumeration", verdictReply(verdict("F1", "maybe")), reviewverdict.ErrOutsideEnum},
		{"it is above the limit", []byte(strings.Repeat(" ", review.MaxReplyBytes+1)), review.ErrReplyTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, store := newFakeGit(), newFakeResults()
			name := reviewed(t, store, baseSHA, headSHA, 1, finding("F1", "BLOCKER"))

			_, err := app.New(g, store).Verify("/work/wt/feature", "own-rdd", name, tc.reply)

			var refused *app.ReplyRefusedError
			if !errors.As(err, &refused) || !errors.Is(err, tc.want) || !strings.Contains(err.Error(), tc.want.Error()) || store.created() != 0 {
				t.Errorf("Verify() error = %v with %d creates, want a *app.ReplyRefusedError that is %v, with its message, and no create", err, store.created(), tc.want)
			}
		})
	}
}

// A verdict is about a stored result: one that is not stored is not found, and one that is not a result the
// store could have written, or whose reply no longer holds to its binding, is refused. A check that cannot run
// never passes.
func TestVerifyRefusesWhatIsNotAStoredResult(t *testing.T) {
	elsewhere, err := review.EncodeResult(candidateOf(t, newFakeGit()), replyWith(t, baseSHA, otherSHA, sha256Of([]byte(diffText)), "completed", changedPaths))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	for _, tc := range []struct {
		name string
		file []byte
		want error
	}{
		{"it is not stored", nil, app.ErrNotFound},
		{"it is not a stored file", []byte("not a result"), review.ErrInvalidStored},
		{"its reply echoes another head than its binding", elsewhere, review.ErrInvalidStored},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeResults()
			if tc.file != nil {
				store.put("own-rdd", "222222222222-1.json", tc.file)
			}

			_, err := app.New(newFakeGit(), store).Verify("/work/wt/feature", "own-rdd", "222222222222-1.json", blocking)

			if !errors.Is(err, tc.want) || !strings.Contains(err.Error(), "222222222222-1.json") || store.created() != 0 {
				t.Errorf("Verify() error = %v with %d creates, want one that is %v, names the result and creates nothing", err, store.created(), tc.want)
			}
		})
	}
}

// A feature or a result name that is not safe is refused before git or the store is asked for anything, and
// the name of a verdict is not the name of a result.
func TestVerifyRefusesAnUnsafeFeatureOrNameBeforeAskingAnyone(t *testing.T) {
	for _, tc := range []struct {
		feature, stored string
		want            error
	}{
		{"../x", "222222222222-1.json", review.ErrInvalidFeature},
		{"", "222222222222-1.json", review.ErrInvalidFeature},
		{"own-rdd", "../222222222222-1.json", review.ErrInvalidResultName},
		{"own-rdd", "", review.ErrInvalidResultName},
		{"own-rdd", "222222222222-1.verifier.json", review.ErrInvalidResultName},
	} {
		g, store := newFakeGit(), newFakeResults()

		_, err := app.New(g, store).Verify("/work/wt/feature", tc.feature, tc.stored, blocking)

		if !errors.Is(err, tc.want) || len(g.calls) != 0 || len(store.calls) != 0 {
			t.Errorf("Verify(%q, %q) error = %v with git calls %q and store calls %q, want one that is %v and no call", tc.feature, tc.stored, err, g.calls, store.calls, tc.want)
		}
	}
}

// A result has one verdict, the file that is there stays as it is, and a store that published a verdict but
// could not flush its directory stored it all the same.
func TestVerifyNeverReplacesAStoredVerdictAndReportsAnUnflushedOneAsStored(t *testing.T) {
	g, store := newFakeGit(), newFakeResults()
	name := reviewed(t, store, baseSHA, headSHA, 1, finding("F1", "BLOCKER"))
	store.put("own-rdd", "222222222222-1.verifier.json", []byte("the first verdict"))

	_, err := app.New(g, store).Verify("/work/wt/feature", "own-rdd", name, blocking)

	if !errors.Is(err, app.ErrAlreadyVerified) || !strings.Contains(err.Error(), wantVerdict) || string(store.files["own-rdd"]["222222222222-1.verifier.json"]) != "the first verdict" {
		t.Errorf("Verify() error = %v, want one that is %v, names %s and leaves the verdict as it was", err, app.ErrAlreadyVerified, wantVerdict)
	}

	fresh := newFakeResults()
	fresh.put("own-rdd", name, store.files["own-rdd"][name])
	fresh.unflushed = fmt.Errorf("%w: sync: input/output error", app.ErrNotFlushed)
	got, err := app.New(g, fresh).Verify("/work/wt/feature", "own-rdd", name, blocking)
	if err != nil || got.Path != wantVerdict || !errors.Is(got.NotFlushed, app.ErrNotFlushed) || fresh.created() != 1 {
		t.Errorf("Verify() = %q, not flushed %v, %v with %d creates; want it stored, NotFlushed set and one create", got.Path, got.NotFlushed, err, fresh.created())
	}
}

// A candidate is corrected once. The builder is sent back when a verdict blocks, and the fix is reviewed
// against the same base; when that blocks again the round is spent, and it is for the owner to decide: the
// verdict is refused and nothing is stored. Only another head of the same base spends the round, and only a
// verdict that blocked.
func TestVerifyAllowsOneCorrectionRoundPerCandidate(t *testing.T) {
	const spentBy = "odd/review-results/own-rdd/333333333333-1.verifier.json"
	for _, tc := range []struct {
		name                                string
		earlierBase, earlierHead, earlierOn string
		currentN                            int
		currentOn                           string
		spent                               bool
	}{
		{"another head of the same base blocked", baseSHA, otherSHA, "reproduced", 1, "reproduced", true},
		{"another head of the same base was unverified", baseSHA, otherSHA, "not-run", 1, "reproduced", false},
		{"another head of the same base was clear", baseSHA, otherSHA, "not-reproduced", 1, "reproduced", false},
		{"the same head blocked: a second launch is no correction", baseSHA, headSHA, "reproduced", 2, "reproduced", false},
		{"another base blocked", fourSHA, otherSHA, "reproduced", 1, "reproduced", false},
		{"the round is spent and this review is clear", baseSHA, otherSHA, "reproduced", 1, "not-reproduced", false},
		{"the round is spent and this review is unverified", baseSHA, otherSHA, "reproduced", 1, "not-run", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, store := newFakeGit(), newFakeResults()
			earlier := reviewed(t, store, tc.earlierBase, tc.earlierHead, 1, finding("F1", "BLOCKER"))
			earlierVerdict, _ := review.VerdictName(earlier)
			store.put("own-rdd", earlierVerdict, verdictReply(verdict("F1", tc.earlierOn)))
			current := reviewed(t, store, baseSHA, headSHA, tc.currentN, finding("F1", "BLOCKER"))

			_, err := app.New(g, store).Verify("/work/wt/feature", "own-rdd", current, verdictReply(verdict("F1", tc.currentOn)))

			switch {
			case tc.spent && (!errors.Is(err, app.ErrCorrectionSpent) || !strings.Contains(err.Error(), spentBy) || store.created() != 0):
				t.Errorf("Verify() error = %v with %d creates, want one that is %v, names %s and stores nothing", err, store.created(), app.ErrCorrectionSpent, spentBy)
			case !tc.spent && (err != nil || store.created() != 1):
				t.Errorf("Verify() error = %v with %d creates, want the verdict stored", err, store.created())
			}
		})
	}
}

// Whether the round was spent is read from the verdicts that are stored, and what cannot be read is not
// taken to mean it was not spent: a verdict that is not a reply of the verifier, a verdict without its result
// and a directory that cannot be listed stop a verdict that blocks. A verdict that does not block never asks.
func TestVerifyCannotTellWhetherTheRoundWasSpentFromWhatItCannotRead(t *testing.T) {
	boom := errors.New("boom")
	for _, tc := range []struct {
		name  string
		setup func(store *fakeResults)
		want  error
	}{
		{"a verdict that is not a reply of the verifier", func(store *fakeResults) {
			reviewed(t, store, baseSHA, otherSHA, 1, finding("F1", "BLOCKER"))
			store.put("own-rdd", "333333333333-1.verifier.json", []byte("not a reply"))
		}, reviewverdict.ErrUnparseable},
		{"a verdict whose result is not stored", func(store *fakeResults) {
			store.put("own-rdd", "333333333333-1.verifier.json", blocking)
		}, app.ErrNotFound},
		{"a directory that cannot be listed", func(store *fakeResults) { store.fail["Names"] = boom }, boom},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, store := newFakeGit(), newFakeResults()
			tc.setup(store)
			current := reviewed(t, store, baseSHA, headSHA, 1, finding("F1", "BLOCKER"))

			_, err := app.New(g, store).Verify("/work/wt/feature", "own-rdd", current, blocking)

			if !errors.Is(err, tc.want) || errors.Is(err, app.ErrCorrectionSpent) || store.created() != 0 {
				t.Errorf("Verify() error = %v with %d creates, want one that wraps %v, is not %v and stores nothing", err, store.created(), tc.want, app.ErrCorrectionSpent)
			}
			if _, err := app.New(g, store).Verify("/work/wt/feature", "own-rdd", current, verdictReply(verdict("F1", "not-reproduced"))); err != nil {
				t.Errorf("Verify() of a verdict that does not block error = %v, want nil: the stored verdicts are not asked", err)
			}
		})
	}
}

// What the store could not do is the error of the command, with the cause, and nothing is created after it.
func TestVerifyReturnsWhatTheStoreCouldNotDo(t *testing.T) {
	boom := errors.New("boom")
	for method, creates := range map[string]int{"Read": 0, "Create": 1} {
		g, store := newFakeGit(), newFakeResults()
		name := reviewed(t, store, baseSHA, headSHA, 1, finding("F1", "BLOCKER"))
		store.fail[method] = boom

		_, err := app.New(g, store).Verify("/work/wt/feature", "own-rdd", name, blocking)

		if !errors.Is(err, boom) || store.created() != creates {
			t.Errorf("Verify() with %s failing error = %v and %d creates, want one that wraps %v and %d", method, err, store.created(), boom, creates)
		}
	}
}
