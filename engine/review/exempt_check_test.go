package review_test

// What the delivery of a head is held to once a head can be exempted (own-rdd T4, S5, L16): `review check` clears a
// head with a non-blocking review result or a valid exemption bound to it, and a blocking review wins over any
// exemption. These are the pure rules; the use case that reads the files and asks git is review/app's Check.

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewresult"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewverdict"
)

// storedExemptionOf is a stored exemption, as DecodeExemption reads it back, with the risk and the report given.
func storedExemptionOf(t *testing.T, risk, report string) review.StoredExemption {
	t.Helper()
	data, err := review.EncodeExemption(candidate(t), risk, []byte(report))
	if err != nil {
		t.Fatalf("setup: EncodeExemption() error = %v", err)
	}
	stored, err := review.DecodeExemption(data)
	if err != nil {
		t.Fatalf("setup: DecodeExemption() error = %v", err)
	}
	return stored
}

// A stored exemption holds when the rules it was made under still hold for what it stores: the declared risk names
// no high-risk item and the report is clear. A file that was edited to say something else, or that says what no
// command makes it say, does not: the engine decides from the stored risk and report again and never from the fact
// that the file exists.
func TestCheckExemptionHoldsAStoredExemptionToTheRulesItWasMadeUnder(t *testing.T) {
	for _, tc := range []struct {
		name, risk, report string
		want               error // the reason the error wraps; nil for an exemption that holds
	}{
		{"the exemption `review exempt` writes", "none", cleanReportText(), nil},
		{"a risk edited to name an item", "3", cleanReportText(), review.ErrHighRisk},
		{"a risk edited to name several items", "1,5", cleanReportText(), review.ErrHighRisk},
		{"a risk that is no declaration", "no", cleanReportText(), review.ErrInvalidRisk},
		{"a report edited to a check that failed", "none", `{"checks":[{"command":"c","result":"fail","output":"o"}]}`, reviewverdict.ErrNotClear},
		{"a report edited to a check that did not run", "none", `{"checks":[{"command":"c","result":"not-run","output":"o"}]}`, reviewverdict.ErrNotClear},
		{"a report edited to name no check", "none", `{"checks":[]}`, reviewverdict.ErrNotClear},
		{"a report with a member the contract does not name", "none", `{"checks":[],"outcome":"clear"}`, reviewverdict.ErrUnknownMember},
		{"a report that is not the object of the contract", "none", `"clear"`, reviewverdict.ErrUnparseable},
		{"a report whose check has no output", "none", `{"checks":[{"command":"c","result":"pass","output":" "}]}`, reviewverdict.ErrMissingEvidence},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := review.CheckExemption(storedExemptionOf(t, tc.risk, tc.report))

			if tc.want == nil {
				if err != nil {
					t.Errorf("CheckExemption() error = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tc.want) {
				t.Errorf("CheckExemption() error = %v, want one that wraps %v", err, tc.want)
			}
		})
	}
}

// A declared risk is checked before the report: when both are wrong, the refusal is the risk, the first thing
// `review exempt` is asked and the thing the owner decided can never be exempt.
func TestCheckExemptionRefusesTheRiskBeforeTheReport(t *testing.T) {
	err := review.CheckExemption(storedExemptionOf(t, "2", `{"checks":[]}`))

	if !errors.Is(err, review.ErrHighRisk) || errors.Is(err, reviewverdict.ErrNotClear) {
		t.Errorf("CheckExemption() error = %v, want the high risk and not the report", err)
	}
}

// The binding a stored exemption names must name a candidate before anything is done with it, as the binding of a
// stored result must before its reply is read: a base that is not a full object name, as an edited file could hold,
// is refused here and never reaches git, and it is refused before the risk and the report are looked at.
func TestCheckExemptionRefusesABindingThatNamesNoCandidateBeforeTheRiskAndTheReport(t *testing.T) {
	for name, edit := range map[string]func(*review.StoredExemption){
		"a base that would be read as an option": func(e *review.StoredExemption) { e.Binding.Base = "--output=forged" },
		"a base that is a ref":                   func(e *review.StoredExemption) { e.Binding.Base = "main" },
		"an abbreviated head":                    func(e *review.StoredExemption) { e.Binding.Head = e.Binding.Head[:12] },
		"a digest that is not one":               func(e *review.StoredExemption) { e.Binding.DiffSHA256 = "not a digest" },
		"a path listed twice":                    func(e *review.StoredExemption) { e.Binding.Paths = []string{"a.go", "a.go"} },
	} {
		t.Run(name, func(t *testing.T) {
			e := storedExemptionOf(t, "2", `{"checks":[]}`) // a risk and a report that are refused as well
			edit(&e)

			err := review.CheckExemption(e)

			if !errors.Is(err, reviewresult.ErrInvalidBinding) || errors.Is(err, review.ErrHighRisk) || errors.Is(err, reviewverdict.ErrNotClear) {
				t.Errorf("CheckExemption() error = %v, want the invalid binding and neither the risk nor the report", err)
			}
		})
	}
}

func exemptionOf(n int) review.Exempted {
	return review.Exempted{Exemption: fmt.Sprintf("odd/review-results/own-rdd/222222222222-%d.exempt.json", n)}
}

// A head is cleared by what is stored for it. With no result, an exemption clears it, one is enough. With a
// result, the results decide as they always did, and an exemption changes nothing: a result that blocks keeps the
// head blocked, and so does a result nobody decided or a finding nobody checked, because an exemption is for work
// that takes no review and not a way around a review that left something unchecked. With neither, the head was
// not reviewed.
func TestClearedClearsAHeadByItsResultsOrByAnExemptionAndAResultThatBlocksWins(t *testing.T) {
	clear1 := resultOf(1, "", reviewverdict.Decision{Outcome: reviewverdict.Clear})
	blocks3 := resultOf(3, "odd/review-results/own-rdd/222222222222-3.verifier.json", reviewverdict.Decision{Outcome: reviewverdict.Blocks, Blocking: []string{"F1", "F2"}})
	unchecked4 := resultOf(4, "", reviewverdict.Decision{Outcome: reviewverdict.Unverified, Unverified: []string{"F3"}})
	undecided5 := resultOf(5, "", reviewverdict.Decision{})

	for _, tc := range []struct {
		name      string
		reviewed  []review.Reviewed
		exempted  []review.Exempted
		wantErr   error
		wantIn    []string
		sameAsOld bool // the answer is the one Clearance gives for the results alone
	}{
		{"nothing is stored for the head", nil, nil, review.ErrNoResult, nil, true},
		{"one exemption", nil, []review.Exempted{exemptionOf(1)}, nil, nil, false},
		{"two exemptions", nil, []review.Exempted{exemptionOf(1), exemptionOf(2)}, nil, nil, false},
		{"a clear result", []review.Reviewed{clear1}, nil, nil, nil, true},
		{"a clear result and an exemption", []review.Reviewed{clear1}, []review.Exempted{exemptionOf(2)}, nil, nil, true},
		{"a result that blocks", []review.Reviewed{blocks3}, nil, review.ErrBlocked, []string{blocks3.Result, "F1", "F2"}, true},
		{"a result that blocks and an exemption", []review.Reviewed{blocks3}, []review.Exempted{exemptionOf(1)}, review.ErrBlocked, []string{blocks3.Result, blocks3.Verdict, "F1", "F2"}, true},
		{"a result that blocks after a clear one and an exemption", []review.Reviewed{clear1, blocks3}, []review.Exempted{exemptionOf(2)}, review.ErrBlocked, []string{blocks3.Result}, true},
		{"a result with a finding nobody checked", []review.Reviewed{unchecked4}, nil, review.ErrNotClear, []string{unchecked4.Result, "F3"}, true},
		{"a result with a finding nobody checked and an exemption", []review.Reviewed{unchecked4}, []review.Exempted{exemptionOf(1)}, review.ErrNotClear, []string{unchecked4.Result, "F3"}, true},
		{"a result nobody decided and an exemption", []review.Reviewed{undecided5}, []review.Exempted{exemptionOf(1)}, review.ErrNotClear, []string{undecided5.Result}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := review.Cleared(tc.reviewed, tc.exempted)

			if tc.wantErr == nil {
				if err != nil {
					t.Errorf("Cleared() error = %v, want nil", err)
				}
			} else {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("Cleared() error = %v, want one that wraps %v", err, tc.wantErr)
				}
				for _, w := range tc.wantIn {
					if !strings.Contains(err.Error(), w) {
						t.Errorf("message %q does not say %q", err, w)
					}
				}
			}
			if tc.sameAsOld {
				// The answer for a head with no exemption, or with a result, is the one the delivery check gave before
				// an exemption existed, word for word.
				if old := review.Clearance(tc.reviewed); fmt.Sprint(old) != fmt.Sprint(err) {
					t.Errorf("Cleared() = %v, want what Clearance() gives for the results alone: %v", err, old)
				}
			}
		})
	}
}
