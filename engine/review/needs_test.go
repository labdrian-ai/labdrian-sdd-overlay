package review_test

// What a commit needs before it is ready for delivery (own-rdd T6b5, S6 d and f, L63). One pure function lists what
// the facts of a commit miss, in the order of S6 (d), and the hook and `review check` both read it.

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
)

const needsCommit = "2222222222222222222222222222222222222222"

// ready is the facts of a commit that misses nothing: it declares Risk: none, its sweep is clean and a review covers it.
func ready() review.Facts {
	return review.Facts{
		Commit: needsCommit, Risk: []int{},
		Sweep: "odd/review-results/own-rdd/222222222222-3.sweep.json", AskCoverage: true,
	}
}

func kinds(needs []review.Need) []review.Missing {
	var got []review.Missing
	for _, n := range needs {
		got = append(got, n.Missing)
	}
	return got
}

func TestNeedsListsWhatTheFactsMissInTheOrderOfTheDesign(t *testing.T) {
	trailer := errors.New("trailer failed")
	for _, tc := range []struct {
		name  string
		facts func(f *review.Facts)
		want  []review.Missing
	}{
		{"a commit that misses nothing", func(*review.Facts) {}, nil},
		{"a trailer that fails asks for the risk and the sweep only", func(f *review.Facts) {
			f.RiskErr, f.Risk, f.Sweep = trailer, nil, ""
			f.Coverage = review.ErrNoResult
		}, []review.Missing{review.MissingRisk, review.MissingSweep}},
		{"a trailer that fails but has a sweep asks for the risk only", func(f *review.Facts) {
			f.RiskErr, f.Risk, f.SweepFired = trailer, nil, true
		}, []review.Missing{review.MissingRisk}},
		{"no sweep record", func(f *review.Facts) { f.Sweep = "" }, []review.Missing{review.MissingSweep}},
		{"a fired sweep that no sweeper answers", func(f *review.Facts) { f.SweepFired = true }, []review.Missing{review.MissingSweeper}},
		{"a fired sweep that a sweeper answers", func(f *review.Facts) { f.SweepFired, f.Answered = true, true }, nil},
		{"a declared item with no polisher", func(f *review.Facts) { f.Risk = []int{3, 5} }, []review.Missing{review.MissingPolisher}},
		{"a declared item with a polisher", func(f *review.Facts) { f.Risk, f.Polished = []int{3}, true }, nil},
		{"no review covers it", func(f *review.Facts) { f.Coverage = review.ErrNoResult }, []review.Missing{review.MissingReview}},
		{"the review is not asked for", func(f *review.Facts) { f.AskCoverage, f.Coverage = false, review.ErrNoResult }, nil},
		{"everything is missing", func(f *review.Facts) {
			f.Sweep, f.Risk, f.Coverage = "", []int{1}, review.ErrNoResult
		}, []review.Missing{review.MissingSweep, review.MissingPolisher, review.MissingReview}},
		{"a fired sweep with no sweeper, a polisher and a review missing", func(f *review.Facts) {
			f.SweepFired, f.Risk, f.Coverage = true, []int{2}, review.ErrNoResult
		}, []review.Missing{review.MissingSweeper, review.MissingPolisher, review.MissingReview}},
		{"a sweep commit needs no sweep of its own", func(f *review.Facts) { f.SweepCommit, f.Sweep = true, "" }, nil},
		{"a sweep commit still needs its polisher and its review", func(f *review.Facts) {
			f.SweepCommit, f.Sweep, f.Risk, f.Coverage = true, "", []int{3}, review.ErrNoResult
		}, []review.Missing{review.MissingPolisher, review.MissingReview}},
		{"a passive commit needs nothing, not even a review", func(f *review.Facts) {
			f.Passive, f.Sweep, f.Risk, f.Coverage = true, "", []int{1}, review.ErrNoResult
		}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := ready()
			tc.facts(&f)

			got := kinds(review.Needs(f))

			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Needs() = %v, want %v", got, tc.want)
			}
		})
	}
}

// Each need carries the reason `review check` refuses with, and each wraps exactly one sentinel.
func TestNeedsCarryTheReasonEachWrapsOneSentinel(t *testing.T) {
	trailer := errors.New("review: the commit does not declare its risk in a Risk trailer: commit x")
	sweepPath := "odd/review-results/own-rdd/222222222222-3.sweep.json"
	for _, tc := range []struct {
		name     string
		facts    func(f *review.Facts)
		sentinel error
		text     string
	}{
		{"the trailer", func(f *review.Facts) { f.RiskErr, f.Risk = trailer, nil }, trailer, trailer.Error()},
		{"the sweep", func(f *review.Facts) { f.Sweep = "" }, review.ErrNoSweep,
			"commit " + needsCommit + ": review: no sweep record binds the commit"},
		{"the sweeper", func(f *review.Facts) { f.SweepFired = true }, review.ErrNoSweeper,
			"commit " + needsCommit + ": review: no sweeper record answers the fired sweep record: " + sweepPath},
		{"the polisher", func(f *review.Facts) { f.Risk = []int{3, 5} }, review.ErrNoPolisher,
			"commit " + needsCommit + ": review: no polisher record binds a commit that declares a high-risk item: Risk: 3,5"},
		{"the review", func(f *review.Facts) { f.Coverage = review.ErrNoResult }, review.ErrNoResult, review.ErrNoResult.Error()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := ready()
			tc.facts(&f)

			needs := review.Needs(f)

			if len(needs) != 1 || !errors.Is(needs[0].Err, tc.sentinel) || needs[0].Err.Error() != tc.text {
				t.Fatalf("Needs() = %+v, want one need whose reason wraps %v and reads %q", needs, tc.sentinel, tc.text)
			}
			for _, other := range []error{review.ErrNoSweep, review.ErrNoSweeper, review.ErrNoPolisher} {
				if other != tc.sentinel && errors.Is(needs[0].Err, other) {
					t.Errorf("the reason %q also wraps %v, want exactly one sentinel", needs[0].Err, other)
				}
			}
		})
	}
}

func TestIsPassiveHoldsTheFourConditionsOfAPassiveCommit(t *testing.T) {
	failed := errors.New("trailer failed")
	for _, tc := range []struct {
		name    string
		risk    []int
		riskErr error
		paths   []string
		removed []string
		want    bool
	}{
		{"every path under odd/tasks", []int{}, nil, []string{"odd/tasks/own-rdd.md"}, nil, true},
		{"the three passive directories", []int{}, nil, []string{"odd/tasks/a.md", "odd/review-results/f/x.json", "odd/review-receipts/r.json"}, nil, true},
		{"a commit that changes nothing", []int{}, nil, nil, nil, true},
		{"a path outside the passive directories", []int{}, nil, []string{"odd/tasks/a.md", "engine/a.go"}, nil, false},
		{"a directory that only begins like a passive one", []int{}, nil, []string{"odd/tasks-old/a.md"}, nil, false},
		{"the passive directory itself is not a path under it", []int{}, nil, []string{"odd/tasks"}, nil, false},
		{"a trailer that names an item", []int{3}, nil, []string{"odd/tasks/a.md"}, nil, false},
		{"a trailer that fails", nil, failed, []string{"odd/tasks/a.md"}, nil, false},
		{"a deleted or renamed path", []int{}, nil, []string{"odd/tasks/a.md"}, []string{"odd/tasks/old.md"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := review.IsPassive(tc.risk, tc.riskErr, tc.paths, tc.removed); got != tc.want {
				t.Errorf("IsPassive(%v, %v, %q, %q) = %v, want %v", tc.risk, tc.riskErr, tc.paths, tc.removed, got, tc.want)
			}
		})
	}
}

func TestTheSentinelsOfTheNeedsAreOneEachAndNamedLikeTheReasonsOfCheck(t *testing.T) {
	for _, err := range []error{review.ErrNoSweep, review.ErrNoSweeper, review.ErrNoPolisher} {
		if !strings.HasPrefix(err.Error(), "review: no ") {
			t.Errorf("%q does not begin like the other reasons of review check", err)
		}
	}
	if errors.Is(review.ErrNoSweep, review.ErrNoSweeper) || errors.Is(review.ErrNoSweeper, review.ErrNoPolisher) {
		t.Error("the sentinels are not distinct")
	}
}
