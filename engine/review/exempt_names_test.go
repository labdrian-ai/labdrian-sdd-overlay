package review_test

// The names of the files stored for a head now come in three kinds, one directory for all of them (own-rdd T4, S5):
// <head12>-<n>.json is a result of the reviewer, <head12>-<n>.verifier.json the verdict of the verifier beside it, and
// <head12>-<n>.exempt.json the exemption of the head. The three share one numbering, and no reader of one kind ever
// takes a file of another kind for its own.

import (
	"errors"
	"slices"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
)

func TestExemptionNameIsTheFirstTwelveCharactersOfTheHeadThenTheNumberThenExempt(t *testing.T) {
	c := candidate(t)
	for _, tc := range []struct {
		n    int
		want string
	}{{1, "222222222222-1.exempt.json"}, {2, "222222222222-2.exempt.json"}, {12, "222222222222-12.exempt.json"}} {
		if got := review.ExemptionName(c, tc.n); got != tc.want {
			t.Errorf("ExemptionName(_, %d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

// The exemptions of a head are the files <head12>-<n>.exempt.json of the directory of its feature, in the order of
// their numbers, as the results of a head are. A name that is not one of them is left out, whatever else it looks
// like: a result or a verdict with the same number, a file that is still being written, an exemption of another
// head, a number the commands never write, any other file.
func TestExemptionsOfHeadAreTheStoredExemptionsOfThatHeadInTheOrderOfTheirNumbers(t *testing.T) {
	names := []string{
		"222222222222-10.exempt.json",
		"222222222222-2.exempt.json",
		"222222222222-2.json",
		"222222222222-2.verifier.json",
		"111111111111-1.exempt.json",
		"222222222222-01.exempt.json",
		"222222222222-3.exempt.json.tmp",
		".222222222222-4.exempt.json",
		"222222222222-5.exempt",
		"222222222222-x.exempt.json",
		"222222222222-.exempt.json",
		"222222222222.exempt.json",
		"222222222222-6.exempt.verifier.json",
		"README.md",
		"222222222222-1.exempt.json",
	}
	given := slices.Clone(names)

	got := review.ExemptionsOfHead(names, headSHA)

	want := []string{"222222222222-1.exempt.json", "222222222222-2.exempt.json", "222222222222-10.exempt.json"}
	if !slices.Equal(got, want) {
		t.Errorf("ExemptionsOfHead(%q) = %q, want %q", headSHA, got, want)
	}
	if !slices.Equal(names, given) {
		t.Errorf("ExemptionsOfHead() reordered the names it was given: %q, were %q", names, given)
	}
}

// A head too short to name a file has no exemptions: an empty prefix must not make every exemption the exemption of it.
func TestExemptionsOfHeadThatNamesNoFileAreNone(t *testing.T) {
	names := []string{"222222222222-1.exempt.json"}
	for _, head := range []string{"", "2222"} {
		if got := review.ExemptionsOfHead(names, head); len(got) != 0 {
			t.Errorf("ExemptionsOfHead(%q) = %q, want none", head, got)
		}
	}
}

// The reader of a kind never takes a file of another kind for its own, and the exemption of a head never lets the
// head count as reviewed: the files of the three kinds beside each other are each found by their own reader only.
func TestAnExemptionIsNeverTakenForAResultOrAVerdictNorTheOtherWayAround(t *testing.T) {
	const (
		result    = "222222222222-1.json"
		verdict   = "222222222222-1.verifier.json"
		exemption = "222222222222-1.exempt.json"
	)
	all := []string{result, verdict, exemption}

	if got, want := review.ResultsOfHead(all, headSHA), []string{result}; !slices.Equal(got, want) {
		t.Errorf("ResultsOfHead(%q) = %q, want only the result %q", all, got, want)
	}
	if got, want := review.ExemptionsOfHead(all, headSHA), []string{exemption}; !slices.Equal(got, want) {
		t.Errorf("ExemptionsOfHead(%q) = %q, want only the exemption %q", all, got, want)
	}
	if err := review.CheckResultName(exemption); !errors.Is(err, review.ErrInvalidResultName) {
		t.Errorf("CheckResultName(%q) = %v, want an error wrapping ErrInvalidResultName", exemption, err)
	}
	if got, ok := review.ResultOfVerdict(exemption); ok {
		t.Errorf("ResultOfVerdict(%q) = %q, true; the name of an exemption is no verdict", exemption, got)
	}
	// A verdict is beside a result and never beside an exemption, even if a file is named as if it were.
	if name, err := review.VerdictName(exemption); !errors.Is(err, review.ErrInvalidResultName) {
		t.Errorf("VerdictName(%q) = %q, %v; want an error wrapping ErrInvalidResultName", exemption, name, err)
	}
	if got, ok := review.ResultOfVerdict("222222222222-1.exempt.verifier.json"); ok {
		t.Errorf("ResultOfVerdict(an exemption with .verifier.json) = %q, true; want it to be no verdict", got)
	}
}

// CheckExemptionName accepts the name ExemptionName makes and no other: <head12>-<n>.exempt.json with a head of 12
// lowercase hexadecimal characters and a number written the way NextNumber counts it. The name of a result, of a
// verdict, a path and a name that only looks like one are refused with an error that wraps ErrInvalidExemptionName.
func TestCheckExemptionNameAcceptsOnlyWhatExemptionNameMakes(t *testing.T) {
	for n := 1; n <= 12; n++ {
		if name := review.ExemptionName(candidate(t), n); review.CheckExemptionName(name) != nil {
			t.Errorf("CheckExemptionName(%q) = %v, want nil: it is a name ExemptionName makes", name, review.CheckExemptionName(name))
		}
	}
	for _, name := range []string{
		"", "222222222222-1.json", "222222222222-1.verifier.json", "222222222222-1.exempt.verifier.json",
		"222222222222-0.exempt.json", "222222222222-01.exempt.json", "222222222222-x.exempt.json", "222222222222-.exempt.json",
		"22222222222-1.exempt.json", "2222222222222-1.exempt.json", "22222222222G-1.exempt.json", "22222222222A-1.exempt.json",
		"222222222222-1.exempt.json.tmp", ".222222222222-1.exempt.json", "222222222222-1.exempt", "own-rdd/222222222222-1.exempt.json",
		"../222222222222-1.exempt.json", "222222222222-1.exempt.json ", "222222222222-1.exempt.json\n",
	} {
		if err := review.CheckExemptionName(name); !errors.Is(err, review.ErrInvalidExemptionName) {
			t.Errorf("CheckExemptionName(%q) = %v, want an error wrapping ErrInvalidExemptionName", name, err)
		}
	}
}

// The three kinds share one numbering: the number an exemption holds is not given again to a result, a verdict or
// another exemption of the head, as long as it is the highest, and the number of one deleted is not given again as
// long as a later one is there.
func TestNextNumberCountsTheNumberOfAnExemption(t *testing.T) {
	c := candidate(t)
	for _, tc := range []struct {
		name  string
		names []string
		want  int
	}{
		{"an exemption holds its number", []string{"222222222222-1.exempt.json"}, 2},
		{"after a result", []string{"222222222222-1.json", "222222222222-2.exempt.json"}, 3},
		{"before a result", []string{"222222222222-1.exempt.json", "222222222222-2.json"}, 3},
		{"the highest, not the count", []string{"222222222222-4.exempt.json"}, 5},
		{"two exemptions", []string{"222222222222-1.exempt.json", "222222222222-2.exempt.json"}, 3},
		{"an exemption of another head does not count", []string{"333333333333-7.exempt.json", "222222222222-1.json"}, 2},
		{"a number the command never writes does not count", []string{"222222222222-01.exempt.json"}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := review.NextNumber(tc.names, c); got != tc.want {
				t.Errorf("NextNumber(%q) = %d, want %d", tc.names, got, tc.want)
			}
		})
	}
}
