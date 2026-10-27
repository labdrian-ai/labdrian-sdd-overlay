package review_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewresult"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewverdict"
)

// The results of a head are the files <head12>-<n>.json of the directory of its feature, in the order of their
// launches: by number, so that the tenth comes after the ninth and not after the first. A name that is not a
// result of that head is left out, whatever else it looks like: the verdict stored beside a result, a file
// that is still being written, a result of another head, a number the commands never write, any other file.
func TestResultsOfHeadAreTheStoredResultsOfThatHeadInTheOrderOfTheirLaunches(t *testing.T) {
	names := []string{
		"222222222222-10.json",
		"222222222222-2.json",
		"222222222222-2.verifier.json",
		"111111111111-1.json",
		"222222222222-01.json",
		"222222222222-3.json.tmp",
		".222222222222-4.json",
		"README.md",
		"222222222222-1.json",
	}
	given := slices.Clone(names)

	got := review.ResultsOfHead(names, headSHA)

	want := []string{"222222222222-1.json", "222222222222-2.json", "222222222222-10.json"}
	if !slices.Equal(got, want) {
		t.Errorf("ResultsOfHead(%q) = %q, want %q", headSHA, got, want)
	}
	if !slices.Equal(names, given) {
		t.Errorf("ResultsOfHead() reordered the names it was given: %q, were %q", names, given)
	}
}

// A head too short to name a file has no results: an empty prefix must not make every result the result of it.
func TestResultsOfHeadThatNamesNoFileAreNone(t *testing.T) {
	names := []string{"222222222222-1.json"}
	for _, head := range []string{"", "2222"} {
		if got := review.ResultsOfHead(names, head); len(got) != 0 {
			t.Errorf("ResultsOfHead(%q) = %q, want none", head, got)
		}
	}
}

// A stored binding is the candidate of the commits it names or it is not: the check recomputes the candidate
// from git, and a binding that differs in any of its four members, or in the order of its paths, is refused
// with the member that differs and both values.
func TestCheckBindingHoldsAStoredBindingToTheCandidateGitGives(t *testing.T) {
	other := strings.Repeat("3", 40)
	zeros := strings.Repeat("0", 64)
	for _, tc := range []struct {
		name string
		edit func(*reviewresult.Binding)
		want []string // what the message says; none when the binding is the candidate's
	}{
		{"the binding of the candidate", func(*reviewresult.Binding) {}, nil},
		{"another base", func(b *reviewresult.Binding) { b.Base = other }, []string{"base", other, baseSHA}},
		{"another head", func(b *reviewresult.Binding) { b.Head = other }, []string{"head", other, headSHA}},
		{"another diff", func(b *reviewresult.Binding) { b.DiffSHA256 = zeros }, []string{"diff_sha256", zeros, diffSHA}},
		{"fewer paths", func(b *reviewresult.Binding) { b.Paths = b.Paths[:1] }, []string{"paths", "lists 1", "lists 2"}},
		{"the paths in another order", func(b *reviewresult.Binding) { slices.Reverse(b.Paths) }, []string{"paths"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := candidate(t)
			stored := c.Binding()
			stored.Paths = slices.Clone(stored.Paths)
			tc.edit(&stored)

			err := review.CheckBinding(stored, c)

			if tc.want == nil {
				if err != nil {
					t.Errorf("CheckBinding() error = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, review.ErrBindingDiffers) {
				t.Fatalf("CheckBinding() error = %v, want one that wraps ErrBindingDiffers", err)
			}
			for _, w := range tc.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("message %q does not say %q", err, w)
				}
			}
		})
	}
}

// resultOf is the stored result number n of the head, as the check reads it back, with the decision given.
func resultOf(n int, verdict string, d reviewverdict.Decision) review.Reviewed {
	return review.Reviewed{Result: fmt.Sprintf("odd/review-results/own-rdd/222222222222-%d.json", n), Verdict: verdict, Decision: d}
}

// A head is cleared for delivery when a stored result binds it and every result that binds it is clear. A
// result that blocks keeps the head blocked whatever was recorded after it, a result with a severe finding that
// nobody checked is not clear, and so is one whose outcome nobody decided: a check that did not run never
// passes. The message names the result, its verdict and the findings.
func TestClearanceClearsAHeadOnlyWhenEveryResultOfItIsClear(t *testing.T) {
	clear1 := resultOf(1, "", reviewverdict.Decision{Outcome: reviewverdict.Clear, Fix: []string{"F9"}, Carry: []string{"F8"}})
	clear2 := resultOf(2, "odd/review-results/own-rdd/222222222222-2.verifier.json", reviewverdict.Decision{Outcome: reviewverdict.Clear})
	blocks3 := resultOf(3, "odd/review-results/own-rdd/222222222222-3.verifier.json", reviewverdict.Decision{Outcome: reviewverdict.Blocks, Blocking: []string{"F1", "F2"}})
	unchecked4 := resultOf(4, "", reviewverdict.Decision{Outcome: reviewverdict.Unverified, Unverified: []string{"F3"}})
	undecided5 := resultOf(5, "", reviewverdict.Decision{})

	for _, tc := range []struct {
		name     string
		reviewed []review.Reviewed
		wantErr  error
		wantIn   []string
	}{
		{"no result binds the head", nil, review.ErrNoResult, nil},
		{"one clear result", []review.Reviewed{clear1}, nil, nil},
		{"two clear results", []review.Reviewed{clear1, clear2}, nil, nil},
		{"a result that blocks", []review.Reviewed{blocks3}, review.ErrBlocked, []string{blocks3.Result, blocks3.Verdict, "F1", "F2"}},
		{"a result with a finding nobody checked", []review.Reviewed{unchecked4}, review.ErrNotClear, []string{unchecked4.Result, "no verdict", "F3"}},
		{"a result nobody decided", []review.Reviewed{undecided5}, review.ErrNotClear, []string{undecided5.Result}},
		{"a result that blocks after a clear one", []review.Reviewed{clear1, blocks3}, review.ErrBlocked, []string{blocks3.Result}},
		{"a clear result after one that blocks", []review.Reviewed{blocks3, clear2}, review.ErrBlocked, []string{blocks3.Result}},
		{"a clear result after one nobody checked", []review.Reviewed{unchecked4, clear1}, review.ErrNotClear, []string{unchecked4.Result}},
		{"a result that blocks after one nobody checked", []review.Reviewed{unchecked4, blocks3}, review.ErrBlocked, []string{blocks3.Result}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := review.Clearance(tc.reviewed)

			if tc.wantErr == nil {
				if err != nil {
					t.Errorf("Clearance() error = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Clearance() error = %v, want one that wraps %v", err, tc.wantErr)
			}
			for _, w := range tc.wantIn {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("message %q does not say %q", err, w)
				}
			}
		})
	}
}
