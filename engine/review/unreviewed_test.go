package review_test

// What the stop hook asks for (own-rdd T6a, S6, L18): of the commits a branch has that the default branch has not, the
// next one that no review covers, with the commit it follows as the base. The commits are named f0 (the commit the
// branch was made on) and c1, c2, c3 (the commits of the branch, oldest first); the rule only compares the names.

import (
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
)

func TestFirstUnreviewedIsTheNextCommitAfterTheLastOneTheChainOfReviewsReaches(t *testing.T) {
	commits := []string{"c1", "c2", "c3"}
	for _, tc := range []struct {
		name        string
		links       []review.Link
		base, head  string
		wantNothing bool
	}{
		{"nothing is reviewed, so the first commit is next and the branch point is its base", nil, "f0", "c1", false},
		{"the first commit is reviewed", []review.Link{{Base: "f0", Head: "c1"}}, "c1", "c2", false},
		{"two commits are reviewed one by one", []review.Link{{Base: "f0", Head: "c1"}, {Base: "c1", Head: "c2"}}, "c2", "c3", false},
		{"a review of two commits at once covers both", []review.Link{{Base: "f0", Head: "c2"}}, "c2", "c3", false},
		{"a review of all three commits at once", []review.Link{{Base: "f0", Head: "c3"}}, "", "", true},
		{"every commit is reviewed one by one", []review.Link{{Base: "f0", Head: "c1"}, {Base: "c1", Head: "c2"}, {Base: "c2", Head: "c3"}}, "", "", true},
		{"a review of two commits and then one", []review.Link{{Base: "f0", Head: "c2"}, {Base: "c2", Head: "c3"}}, "", "", true},
		{"a review that starts at a commit nothing reached does not count", []review.Link{{Base: "c1", Head: "c2"}}, "f0", "c1", false},
		{"a gap: the review after it starts at a commit the chain did not reach", []review.Link{{Base: "f0", Head: "c1"}, {Base: "c2", Head: "c3"}}, "c1", "c2", false},
		{"the order of the links does not matter", []review.Link{{Base: "c2", Head: "c3"}, {Base: "c1", Head: "c2"}, {Base: "f0", Head: "c1"}}, "", "", true},
		{"a review made backwards reaches nothing", []review.Link{{Base: "c2", Head: "c1"}}, "f0", "c1", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base, head, found := review.FirstUnreviewed("f0", commits, tc.links)

			if base != tc.base || head != tc.head || found == tc.wantNothing {
				t.Errorf("FirstUnreviewed() = %q, %q, %v; want %q, %q, %v", base, head, found, tc.base, tc.head, !tc.wantNothing)
			}
		})
	}
}

// A branch with no commit of its own has nothing to review, whatever reviews are stored.
func TestFirstUnreviewedOfNoCommitIsNothing(t *testing.T) {
	base, head, found := review.FirstUnreviewed("f0", nil, []review.Link{{Base: "f0", Head: "c1"}})

	if found || base != "" || head != "" {
		t.Errorf("FirstUnreviewed() = %q, %q, %v; want nothing", base, head, found)
	}
}
