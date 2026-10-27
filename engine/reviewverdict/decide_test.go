package reviewverdict_test

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewresult"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewverdict"
)

const (
	reproduced    = reviewverdict.Reproduced
	notReproduced = reviewverdict.NotReproduced
	notRun        = reviewverdict.NotRun
)

var (
	severe    = []reviewresult.Severity{reviewresult.SeverityBlocker, reviewresult.SeverityCritical}
	ofTheDiff = []reviewresult.Disposition{reviewresult.DispositionIntroduced, reviewresult.DispositionBehaviorActivated, reviewresult.DispositionWorsened}
	ofTheBase = []reviewresult.Disposition{reviewresult.DispositionPreExisting, reviewresult.DispositionBaseOnly}
	checks    = []reviewverdict.Reproduction{reproduced, notReproduced, notRun}
)

// standing is the list of the decision that holds a finding, or nowhere.
type standing string

const (
	blocking   standing = "blocking"
	unverified standing = "unverified"
	fix        standing = "fix"
	carry      standing = "carry"
	nowhere    standing = "nowhere"
)

// where says which list of the decision holds id. A finding that two lists hold is a defect of its own.
func where(d reviewverdict.Decision, id string) standing {
	var held []string
	for s, ids := range map[standing][]string{blocking: d.Blocking, unverified: d.Unverified, fix: d.Fix, carry: d.Carry} {
		if slices.Contains(ids, id) {
			held = append(held, string(s))
		}
	}
	slices.Sort(held)
	switch len(held) {
	case 0:
		return nowhere
	case 1:
		return standing(held[0])
	}
	return standing("held by " + strings.Join(held, " and "))
}

// decideOne decides a result of the one finding F1, which the verifier answered with result on head and base.
func decideOne(severity reviewresult.Severity, disposition reviewresult.Disposition, result, base reviewverdict.Reproduction) reviewverdict.Decision {
	return reviewverdict.Decide(
		reviewresult.Result{Findings: []reviewresult.Finding{{ID: "F1", Severity: severity, Disposition: disposition}}},
		[]reviewverdict.Verdict{{ID: "F1", Result: result, Base: base}})
}

// expect requires F1 to stand where want says, and the review to end as that standing makes it end.
func expect(t *testing.T, d reviewverdict.Decision, want standing) {
	t.Helper()
	outcome := map[standing]reviewverdict.Outcome{blocking: reviewverdict.Blocks, unverified: reviewverdict.Unverified}[want]
	if outcome == "" {
		outcome = reviewverdict.Clear
	}
	if got := where(d, "F1"); got != want || d.Outcome != outcome {
		t.Errorf("F1 is %s and the review %q; want %s and %q (%+v)", got, d.Outcome, want, outcome, d)
	}
}

func TestDecideASevereFindingTheDiffCausedBlocksOnlyWhenItReproducesOnHead(t *testing.T) {
	for _, severity := range severe {
		for _, disposition := range ofTheDiff {
			for _, base := range checks {
				for result, want := range map[reviewverdict.Reproduction]standing{reproduced: blocking, notReproduced: nowhere, notRun: unverified} {
					t.Run(fmt.Sprintf("%s %s head %s base %s", severity, disposition, result, base), func(t *testing.T) {
						expect(t, decideOne(severity, disposition, result, base), want)
					})
				}
			}
		}
	}
}

func TestDecideASevereFindingOfTheBaseNeverBlocks(t *testing.T) {
	for _, severity := range severe {
		for _, disposition := range ofTheBase {
			for _, result := range checks {
				t.Run(fmt.Sprintf("%s %s head %s", severity, disposition, result), func(t *testing.T) {
					expect(t, decideOne(severity, disposition, result, notRun), carry)
				})
			}
		}
	}
}

func TestDecideUnknownCausalityBlocksOnlyWhenReproducedOnHeadAndNotOnBase(t *testing.T) {
	for _, tc := range []struct {
		result, base reviewverdict.Reproduction
		want         standing
	}{
		{reproduced, notReproduced, blocking},
		{reproduced, reproduced, carry},
		{reproduced, notRun, unverified},
		{notReproduced, notReproduced, nowhere},
		{notReproduced, reproduced, nowhere},
		{notReproduced, notRun, nowhere},
		{notRun, notReproduced, unverified},
		{notRun, notRun, unverified},
		{notRun, reproduced, carry},
	} {
		for _, severity := range severe {
			t.Run(fmt.Sprintf("%s head %s base %s", severity, tc.result, tc.base), func(t *testing.T) {
				expect(t, decideOne(severity, reviewresult.DispositionUnknown, tc.result, tc.base), tc.want)
			})
		}
	}
}

func TestDecideAWarningIsFixedAndASuggestionCarriedWhateverTheVerdict(t *testing.T) {
	dispositions := append(append([]reviewresult.Disposition{reviewresult.DispositionUnknown}, ofTheDiff...), ofTheBase...)
	for severity, want := range map[reviewresult.Severity]standing{reviewresult.SeverityWarning: fix, reviewresult.SeveritySuggestion: carry} {
		for _, disposition := range dispositions {
			for _, result := range checks {
				t.Run(fmt.Sprintf("%s %s head %s", severity, disposition, result), func(t *testing.T) {
					d := decideOne(severity, disposition, result, notRun)
					if got := where(d, "F1"); got != want || d.Outcome != reviewverdict.Clear {
						t.Errorf("F1 is %s and the review %q; want %s and %q", got, d.Outcome, want, reviewverdict.Clear)
					}
				})
			}
		}
	}
}

func TestDecideListsEachFindingInTheOrderOfTheResultAndBlockingWinsOverUnverified(t *testing.T) {
	findings := []reviewresult.Finding{
		{ID: "F1", Severity: reviewresult.SeverityBlocker, Disposition: reviewresult.DispositionIntroduced},
		{ID: "F2", Severity: reviewresult.SeverityWarning, Disposition: reviewresult.DispositionIntroduced},
		{ID: "F3", Severity: reviewresult.SeveritySuggestion, Disposition: reviewresult.DispositionIntroduced},
		{ID: "F4", Severity: reviewresult.SeverityCritical, Disposition: reviewresult.DispositionPreExisting},
		{ID: "F5", Severity: reviewresult.SeverityBlocker, Disposition: reviewresult.DispositionWorsened},
		{ID: "F6", Severity: reviewresult.SeverityCritical, Disposition: reviewresult.DispositionBehaviorActivated},
		{ID: "F7", Severity: reviewresult.SeverityBlocker, Disposition: reviewresult.DispositionIntroduced},
	}
	verdicts := []reviewverdict.Verdict{ // not in the order of the findings
		{ID: "F7", Result: notReproduced}, {ID: "F6", Result: reproduced}, {ID: "F5", Result: notRun},
		{ID: "F4", Result: reproduced}, {ID: "F3", Result: reproduced}, {ID: "F2", Result: notRun}, {ID: "F1", Result: reproduced},
	}

	got := reviewverdict.Decide(reviewresult.Result{Findings: findings}, verdicts)

	want := reviewverdict.Decision{
		Outcome: reviewverdict.Blocks, Blocking: []string{"F1", "F6"}, Unverified: []string{"F5"}, Fix: []string{"F2"}, Carry: []string{"F3", "F4"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Decide() = %+v, want %+v", got, want)
	}
}

func TestDecideAFindingWithoutAVerdictIsUnverifiedNeverClear(t *testing.T) {
	result := reviewresult.Result{Findings: []reviewresult.Finding{{ID: "F1", Severity: reviewresult.SeverityBlocker, Disposition: reviewresult.DispositionIntroduced}}}

	for name, verdicts := range map[string][]reviewverdict.Verdict{
		"no verdict at all":         nil,
		"a verdict for another one": {{ID: "F2", Result: notReproduced, Base: notReproduced}},
	} {
		t.Run(name, func(t *testing.T) { expect(t, reviewverdict.Decide(result, verdicts), unverified) })
	}
}

func TestDecideAResultWithoutFindingsIsClear(t *testing.T) {
	want := reviewverdict.Decision{Outcome: reviewverdict.Clear}
	if got := reviewverdict.Decide(reviewresult.Result{}, nil); !reflect.DeepEqual(got, want) {
		t.Errorf("Decide() = %+v, want %+v", got, want)
	}
}
