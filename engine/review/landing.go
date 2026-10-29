package review

import (
	"errors"
	"fmt"
	"strings"
)

// The decisions of `review anchor` (own-rdd T7a, S7 b-e, g): whether the merge of a pull request lands a tree that
// stored reviews cleared. They are pure: the facts come from git through the use case, and each refusal is a sentence
// with the names in it that wraps one sentinel.
var (
	// ErrNotAMerge is a commit that is not the merge of a pull request: it does not have exactly two parents.
	ErrNotAMerge = errors.New("review: the commit is not the merge of a pull request")
	// ErrMergeBase is a pair of commits that do not have exactly one merge base.
	ErrMergeBase = errors.New("review: the commits do not have exactly one merge base")
	// ErrNoFeatureClears is a merge none of whose feature documents has a chain of reviews that clears it.
	ErrNoFeatureClears = errors.New("review: no feature document the merge changes clears it")
	// ErrTreeMoved is a merge that lands another tree than the one of the reviewed head.
	ErrTreeMoved = errors.New("review: the merge does not land the tree of the reviewed head")
)

// landingRefusal is a refusal whose text is exactly the sentence S7 gives and that wraps its sentinel.
type landingRefusal struct {
	text     string
	sentinel error
}

func (e *landingRefusal) Error() string { return e.text }
func (e *landingRefusal) Unwrap() error { return e.sentinel }

// MergeHead reads the parents of merge, the full name of a commit: a merge of a pull request has exactly two, the first
// the base (the tip the pull request was merged into) and the second the head of the pull request. Any other number is
// refused with ErrNotAMerge (a squash or rebase landing, a direct push, a root commit and an octopus merge cannot be told
// to be reviewed).
func MergeHead(merge string, parents []string) (first, head string, err error) {
	if len(parents) != 2 {
		return "", "", &landingRefusal{
			text: fmt.Sprintf("the commit %s is not the merge of a pull request (the number of its parents is %d, not 2), "+
				"so what it lands cannot be told to be reviewed", merge, len(parents)),
			sentinel: ErrNotAMerge,
		}
	}
	return parents[0], parents[1], nil
}

// OneMergeBase is the merge base of first and head, the two parents of the merge, when the bases are exactly one. None
// or several are refused with ErrMergeBase, since the commits the pull request adds cannot be told.
func OneMergeBase(first, head string, bases []string) (string, error) {
	if len(bases) != 1 {
		return "", &landingRefusal{
			text: fmt.Sprintf("%s and %s do not have exactly one merge base (they have %d), "+
				"so the commits the pull request adds cannot be told", first, head, len(bases)),
			sentinel: ErrMergeBase,
		}
	}
	return bases[0], nil
}

const (
	featureDocumentDir = "odd/tasks/"
	featureDocumentExt = ".md"
)

// FeaturesOf names the features of a change: the <f> of every path odd/tasks/<f>.md that sits directly under odd/tasks/,
// in the order of paths, each once. A name CheckFeature refuses is refused with "odd/tasks/<f>.md: " and its error,
// which wraps ErrInvalidFeature. Paths that are not a feature document are not features.
func FeaturesOf(paths []string) ([]string, error) {
	var features []string
	seen := map[string]bool{}
	for _, p := range paths {
		rest, ok := strings.CutPrefix(p, featureDocumentDir)
		if !ok || strings.Contains(rest, "/") {
			continue
		}
		name, ok := strings.CutSuffix(rest, featureDocumentExt)
		if !ok {
			continue
		}
		if err := CheckFeature(name); err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		if !seen[name] {
			seen[name] = true
			features = append(features, name)
		}
	}
	return features, nil
}

// NoFeatureClears is the refusal of a merge none of whose features clears base..head: it names each feature with the
// reason its chain was refused, in order, joined with "; ". reasons[i] is the reason of features[i]; a feature with no
// reason is told as such, since whatever cannot be told is a refusal.
func NoFeatureClears(base, head string, features []string, reasons []error) error {
	told := make([]string, len(features))
	for i, f := range features {
		reason := "no reason was given"
		if i < len(reasons) && reasons[i] != nil {
			reason = reasons[i].Error()
		}
		told[i] = f + ": " + reason
	}
	return &landingRefusal{
		text:     fmt.Sprintf("no feature document the merge changes clears %s..%s: %s", base, head, strings.Join(told, "; ")),
		sentinel: ErrNoFeatureClears,
	}
}

// CheckLandedTree holds the tree of the merge to the tree of the reviewed head: equal passes, and a different one is
// refused with ErrTreeMoved, since the branch did not hold first, the base, when it was merged.
func CheckLandedTree(merge, mergeTree, head, headTree, first string) error {
	if mergeTree == headTree {
		return nil
	}
	return &landingRefusal{
		text: fmt.Sprintf("the merge %s lands the tree %s, not the tree %s of the reviewed head %s: the branch did not hold %s when it was merged",
			merge, mergeTree, headTree, head, first),
		sentinel: ErrTreeMoved,
	}
}
