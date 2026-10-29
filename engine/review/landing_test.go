package review_test

// The pure decisions of `review anchor` (own-rdd T7a, S7 b-e, g): what a merge is, which merge base counts, which
// features a merge changes, why no feature clears it, and whether the tree that landed is the tree reviewed.

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
)

const (
	landM = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	landP = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	landH = "cccccccccccccccccccccccccccccccccccccccc"
	landB = "dddddddddddddddddddddddddddddddddddddddd"
)

func TestMergeHeadGivesTheFirstAndTheSecondParentOfAMerge(t *testing.T) {
	first, head, err := review.MergeHead(landM, []string{landP, landH})

	if err != nil || first != landP || head != landH {
		t.Errorf("MergeHead() = %q, %q, %v; want %q, %q, nil", first, head, err, landP, landH)
	}
}

func TestMergeHeadRefusesACommitThatDoesNotHaveTwoParents(t *testing.T) {
	for _, tc := range []struct {
		name    string
		parents []string
		n       string
	}{
		{"one parent", []string{landP}, "1"},
		{"three parents", []string{landP, landH, landB}, "3"},
		{"no parent", nil, "0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			first, head, err := review.MergeHead(landM, tc.parents)

			want := "the commit " + landM + " is not the merge of a pull request (the number of its parents is " + tc.n +
				", not 2), so what it lands cannot be told to be reviewed"
			if !errors.Is(err, review.ErrNotAMerge) || err.Error() != want || first != "" || head != "" {
				t.Errorf("MergeHead() = %q, %q, %v; want no names and the refusal %q", first, head, err, want)
			}
		})
	}
}

func TestOneMergeBaseGivesTheOnlyMergeBase(t *testing.T) {
	got, err := review.OneMergeBase(landP, landH, []string{landB})

	if err != nil || got != landB {
		t.Errorf("OneMergeBase() = %q, %v; want %q, nil", got, err, landB)
	}
}

func TestOneMergeBaseRefusesNoneAndSeveral(t *testing.T) {
	for _, tc := range []struct {
		name  string
		bases []string
		n     string
	}{
		{"none", nil, "0"},
		{"two", []string{landB, landM}, "2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := review.OneMergeBase(landP, landH, tc.bases)

			want := landP + " and " + landH + " do not have exactly one merge base (they have " + tc.n +
				"), so the commits the pull request adds cannot be told"
			if !errors.Is(err, review.ErrMergeBase) || err.Error() != want || got != "" {
				t.Errorf("OneMergeBase() = %q, %v; want no name and the refusal %q", got, err, want)
			}
		})
	}
}

func TestFeaturesOfNamesTheFeatureDocumentsDirectlyUnderOddTasks(t *testing.T) {
	got, err := review.FeaturesOf([]string{
		"README.md", "odd/review-results/a/x.json", "odd/tasks/a.md", "odd/tasks/b.md", "odd/tasks/a.md",
		"odd/tasks/sub/c.md", "odd/tasks/x.txt",
	})

	if err != nil || !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("FeaturesOf() = %q, %v; want [a b] in order, each once", got, err)
	}
}

func TestFeaturesOfGivesNoFeatureForPathsOfNoFeatureDocument(t *testing.T) {
	got, err := review.FeaturesOf([]string{"README.md", "odd/tasks/x.txt"})

	if err != nil || len(got) != 0 {
		t.Errorf("FeaturesOf() = %q, %v; want no feature", got, err)
	}
}

func TestFeaturesOfRefusesANameThatCheckFeatureRefuses(t *testing.T) {
	bad := "-bad"

	got, err := review.FeaturesOf([]string{"odd/tasks/a.md", "odd/tasks/" + bad + ".md"})

	reason := review.CheckFeature(bad)
	want := "odd/tasks/" + bad + ".md: " + reason.Error()
	if !errors.Is(err, review.ErrInvalidFeature) || err == nil || err.Error() != want || got != nil {
		t.Errorf("FeaturesOf() = %q, %v; want no names and the refusal %q", got, err, want)
	}
}

func TestNoFeatureClearsJoinsTheReasonsInOrder(t *testing.T) {
	err := review.NoFeatureClears(landB, landH, []string{"a", "b"}, []error{errors.New("no sweep"), errors.New("a gap")})

	want := "no feature document the merge changes clears " + landB + ".." + landH + ": a: no sweep; b: a gap"
	if !errors.Is(err, review.ErrNoFeatureClears) || err == nil || err.Error() != want {
		t.Errorf("NoFeatureClears() = %v; want the refusal %q", err, want)
	}
}

func TestNoFeatureClearsNamesTheOneFeature(t *testing.T) {
	err := review.NoFeatureClears(landB, landH, []string{"a"}, []error{errors.New("no sweep")})

	want := "no feature document the merge changes clears " + landB + ".." + landH + ": a: no sweep"
	if !errors.Is(err, review.ErrNoFeatureClears) || err == nil || err.Error() != want {
		t.Errorf("NoFeatureClears() = %v; want the refusal %q", err, want)
	}
}

func TestCheckLandedTreePassesEqualTrees(t *testing.T) {
	if err := review.CheckLandedTree(landM, "t1", landH, "t1", landP); err != nil {
		t.Errorf("CheckLandedTree() = %v; want nil for equal trees", err)
	}
}

func TestCheckLandedTreeRefusesDifferentTrees(t *testing.T) {
	err := review.CheckLandedTree(landM, "tm", landH, "th", landP)

	want := "the merge " + landM + " lands the tree tm, not the tree th of the reviewed head " + landH +
		": the branch did not hold " + landP + " when it was merged"
	if !errors.Is(err, review.ErrTreeMoved) || err == nil || err.Error() != want {
		t.Errorf("CheckLandedTree() = %v; want the refusal %q", err, want)
	}
}

func TestLandingRefusalsNameTheirOwnSentinelOnly(t *testing.T) {
	_, _, notMerge := review.MergeHead(landM, nil)
	moved := review.CheckLandedTree(landM, "a", landH, "b", landP)

	if errors.Is(notMerge, review.ErrTreeMoved) || errors.Is(moved, review.ErrNotAMerge) ||
		strings.Contains(notMerge.Error(), "review:") {
		t.Errorf("the refusals mix their sentinels or carry a prefix: %v / %v", notMerge, moved)
	}
}
