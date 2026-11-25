package app_test

// What Findings says of the severe findings whose check did not run (T4d-a, L98 choice 2): Unverified, the ids that
// reviewverdict.Decide lists as unverified, and the hold the stop hook keeps a result under until its check has run.

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewresult"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewverdict"
)

// U1: Unverified is what Decide lists as unverified, over every disposition, result and base_result and with no verdict,
// and is [] and never nil when it lists none.
func TestFindingsListsTheSevereFindingsDecideReadsAsUnverified(t *testing.T) {
	dispositions := []reviewresult.Disposition{
		reviewresult.DispositionIntroduced, reviewresult.DispositionBehaviorActivated, reviewresult.DispositionWorsened,
		reviewresult.DispositionPreExisting, reviewresult.DispositionBaseOnly, reviewresult.DispositionUnknown,
	}
	reproductions := []reviewverdict.Reproduction{reviewverdict.Reproduced, reviewverdict.NotReproduced, reviewverdict.NotRun}
	for _, severity := range []string{"BLOCKER", "CRITICAL"} {
		for _, d := range dispositions {
			for _, result := range append([]reviewverdict.Reproduction{""}, reproductions...) {
				for _, base := range reproductions {
					if result == "" && base != reviewverdict.NotRun {
						continue // no verdict has no base_result to vary
					}
					name := fmt.Sprintf("%s %s result %q base %q", severity, d, result, base)
					store := newFakeResults()
					stored := reviewed(t, store, baseSHA, headSHA, 1, disposed("F1", severity, string(d)))
					var verdicts []reviewverdict.Verdict
					if result != "" {
						withVerdict(t, store, verdictReply(verdictOnBase("F1", string(result), string(base))))
						verdicts = []reviewverdict.Verdict{{ID: "F1", Result: result, Base: base}}
					}
					want := reviewverdict.Decide(reviewresult.Result{Findings: []reviewresult.Finding{{ID: "F1", Severity: reviewresult.Severity(severity), Disposition: d}}}, verdicts).Unverified
					if want == nil {
						want = []string{}
					}

					got, err := app.New(newFakeGit(), store).Findings("/work/wt/feature", "own-rdd", stored)

					if err != nil || !reflect.DeepEqual(got.Unverified, want) {
						t.Errorf("%s: Unverified = %#v (%v), want %#v as Decide lists it", name, got.Unverified, err, want)
					}
				}
			}
		}
	}

	t.Run("a result with a WARNING only", func(t *testing.T) {
		store := newFakeResults()
		stored := reviewed(t, store, baseSHA, headSHA, 1, finding("F1", "WARNING"))

		got, err := app.New(newFakeGit(), store).Findings("/work/wt/feature", "own-rdd", stored)

		if err != nil || !reflect.DeepEqual(got.Unverified, []string{}) {
			t.Errorf("Unverified = %#v (%v), want [] and no error", got.Unverified, err)
		}
	})
	t.Run("several findings keep the order of the reply", func(t *testing.T) {
		store := newFakeResults()
		stored := reviewed(t, store, baseSHA, headSHA, 1, finding("F3", "CRITICAL"), finding("F1", "WARNING"), finding("F2", "BLOCKER"), finding("F4", "BLOCKER"))
		withVerdict(t, store, verdictReply(verdict("F3", "not-run"), verdict("F1", "not-run"), verdict("F2", "not-run"), verdict("F4", "reproduced")))

		got, err := app.New(newFakeGit(), store).Findings("/work/wt/feature", "own-rdd", stored)

		if err != nil || !reflect.DeepEqual(got.Unverified, []string{"F3", "F2"}) {
			t.Errorf("Unverified = %#v (%v), want F3 then F2: the severe findings whose check did not run, F4 being verified", got.Unverified, err)
		}
	})
}

// held is a world whose one stored result has a WARNING (F1) and a BLOCKER the diff introduced (F2), with the verdict of
// F2 (and the "not-run" of F1, which a verdict must cover) stored beside it when given.
func held(t *testing.T, verdictOfF2 string) (*fakeGit, *fakeResults) {
	t.Helper()
	g, store := sweptWorld(t)
	name := reviewed(t, store, baseSHA, otherSHA, 1, warning("F1"), finding("F2", "BLOCKER"))
	if verdictOfF2 != "" {
		verdictName, err := review.VerdictName(name)
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		store.put("own-rdd", verdictName, verdictReply(verdict("F1", "not-run"), verdict("F2", verdictOfF2)))
	}
	return g, store
}

// U2: the stop hook does not ask for a result with an unverified severe finding until its check has run, however many
// other findings the result keeps; a stored not-run keeps it held; the same result is asked for once the verdict
// reproduced the severe finding, and a result whose severe finding the verdict did not reproduce is asked for by its WARNING.
func TestStopHoldsAResultWithAnUnverifiedSevereFindingUntilItsCheckRan(t *testing.T) {
	for _, tc := range []struct {
		name    string
		verdict string // the verdict of F2, or "" for no verdict stored
		want    app.Ruling
	}{
		{"a WARNING and a severe finding with no verdict", "", app.Ruling{}},
		{"the severe finding not run", "not-run", app.Ruling{}},
		{"the severe finding reproduced", "reproduced", app.Ruling{Block: true, Classify: []app.KeptResult{{Review: otherReview, Head: otherSHA, Kept: 2}}}},
		{"the severe finding not reproduced, and a WARNING", "not-reproduced", app.Ruling{Block: true, Classify: []app.KeptResult{{Review: otherReview, Head: otherSHA, Kept: 1}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, store := held(t, tc.verdict)
			port := newFakeClassified()

			got, err := app.New(g, store).WithAsked(newFakeAsked()).WithClassified(port).Stop(evidenceDir, "session-1", false)

			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Stop() = %+v, %v; want %+v and no error", got, err, tc.want)
			}
			if !tc.want.Block && len(port.calls) != 0 {
				t.Errorf("the port was asked %q for a held result, want no call", port.calls)
			}
		})
	}
}
