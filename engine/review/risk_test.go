package review_test

// The Risk trailer (own-rdd T6b1, S6 a, L34 a, L40). A commit declares its risk as the git trailer "Risk: none" or
// "Risk: <items>", in the grammar of "review exempt --risk", and the engine reads that one declaration from the
// raw message and the trailers git parsed from it. A commit whose trailer fails is unknown risk, never a pass.

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
)

const riskCommit = "1111111111111111111111111111111111111111"

func TestParseRiskReadsNoneAndTheListOfItems(t *testing.T) {
	for _, tc := range []struct {
		declared string
		want     []int
	}{
		{"none", []int{}},
		{"3", []int{3}},
		{"4,2", []int{4, 2}},
		{"1,2,3,4,5,6", []int{1, 2, 3, 4, 5, 6}},
	} {
		got, err := review.ParseRisk(tc.declared)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("ParseRisk(%q) = %v, %v, want %v, nil", tc.declared, got, err, tc.want)
		}
	}
}

func TestParseRiskRefusesWhatTheGrammarOfExemptRefuses(t *testing.T) {
	for _, declared := range []string{"", "None", "0", "7", "3, 5", "3,3", "3,", ",3", "12", "none,3", " none"} {
		_, err := review.ParseRisk(declared)
		if !errors.Is(err, review.ErrInvalidRisk) {
			t.Errorf("ParseRisk(%q) error = %v, want one that wraps ErrInvalidRisk", declared, err)
			continue
		}
		if want := review.CheckRisk(declared); want == nil || want.Error() != err.Error() {
			t.Errorf("ParseRisk(%q) message %q is not the one of CheckRisk %q", declared, err, want)
		}
	}
}

// CheckRisk keeps every message of "review exempt --risk" it had before it shared the parser.
func TestCheckRiskKeepsItsMessagesAfterSharingTheParser(t *testing.T) {
	invalid := `review: invalid risk declaration: "3, 5" is not "none" or a list of the numbers 1 to 6 of the high-risk items, separated by commas with no space and none written twice (for example "2,4")`
	if err := review.CheckRisk("3, 5"); err == nil || err.Error() != invalid {
		t.Errorf("CheckRisk(%q) = %v, want %q", "3, 5", err, invalid)
	}
	high := "review: a change that names a high-risk item is never exempt from review: the declared risk names 4 (concurrency), 2 (security)"
	if err := review.CheckRisk("4,2"); err == nil || err.Error() != high {
		t.Errorf("CheckRisk(%q) = %v, want %q", "4,2", err, high)
	}
	if err := review.CheckRisk("none"); err != nil {
		t.Errorf("CheckRisk(none) = %v, want nil", err)
	}
}

func TestDeclaredRiskReadsTheOneRiskTrailerOfACommit(t *testing.T) {
	for _, tc := range []struct {
		name     string
		message  string
		trailers []review.Trailer
		want     []int
	}{
		{"none", "feat: x\n\nbody\n\nRisk: none\n", []review.Trailer{{Key: "Risk", Value: "none"}}, []int{}},
		{"items", "feat: x\n\nRisk: 3,5\n", []review.Trailer{{Key: "Risk", Value: "3,5"}}, []int{3, 5}},
		{"the key in another case", "feat: x\n\nrisk: 2\n", []review.Trailer{{Key: "risk", Value: "2"}}, []int{2}},
		{"among other trailers", "feat: x\n\nSigned-off-by: A\nRisk: none\nReviewed-by: B\n",
			[]review.Trailer{{Key: "Signed-off-by", Value: "A"}, {Key: "Risk", Value: "none"}, {Key: "Reviewed-by", Value: "B"}}, []int{}},
	} {
		got, err := review.DeclaredRisk(riskCommit, tc.message, tc.trailers)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: DeclaredRisk = %v, %v, want %v, nil", tc.name, got, err, tc.want)
		}
	}
}

func TestDeclaredRiskFailsFourWaysAndNamesTheCommit(t *testing.T) {
	for _, tc := range []struct {
		name     string
		message  string
		trailers []review.Trailer
		say      string
	}{
		{"no trailer", "feat: x\n\nbody\n", nil,
			`it has no "Risk:" trailer in the final trailer block of its message`},
		{"only other trailers", "feat: x\n\nSigned-off-by: A\n", []review.Trailer{{Key: "Signed-off-by", Value: "A"}},
			`it has no "Risk:" trailer in the final trailer block of its message`},
		{"two trailers", "feat: x\n\nRisk: none\nRisk: 2\n", []review.Trailer{{Key: "Risk", Value: "none"}, {Key: "Risk", Value: "2"}},
			`it has more than one "Risk:" trailer`},
		{"two trailers in two cases", "feat: x\n\nRisk: none\nrisk: 2\n", []review.Trailer{{Key: "Risk", Value: "none"}, {Key: "risk", Value: "2"}},
			`it has more than one "Risk:" trailer`},
		{"a wrapped trailer", "feat: x\n\nRisk: 3,\n 5\n", []review.Trailer{{Key: "Risk", Value: "3, 5"}},
			`its "Risk:" trailer wraps onto more than one line`},
		{"a value outside the grammar", "feat: x\n\nRisk: 3, 5\n", []review.Trailer{{Key: "Risk", Value: "3, 5"}},
			`its "Risk:" trailer "3, 5" is not "none" or a list of the numbers 1 to 6 separated by commas with no space (for example "2,4")`},
		{"a value that is empty", "feat: x\n\nRisk:\n", []review.Trailer{{Key: "Risk", Value: ""}},
			`its "Risk:" trailer "" is not "none"`},
	} {
		got, err := review.DeclaredRisk(riskCommit, tc.message, tc.trailers)
		if got != nil || !errors.Is(err, review.ErrRiskTrailer) {
			t.Errorf("%s: DeclaredRisk = %v, %v, want nil and an error that wraps ErrRiskTrailer", tc.name, got, err)
			continue
		}
		for _, want := range []string{riskCommit, tc.say} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("%s: message %q does not say %q", tc.name, err, want)
			}
		}
	}
}
