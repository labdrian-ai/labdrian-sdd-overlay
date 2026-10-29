package app

import (
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
)

// The outcomes of CheckLanding (own-rdd T7a, S7 d-f): a merge that lands the tree of a head whose chain stored reviews
// clear is verified, and one that changes no feature document is told as such and passes. A feature is clear when its
// chain clears base..head and refused, with the reason, when it does not.
const (
	OutcomeVerified  = "verified"
	OutcomeNoFeature = "no-feature"
	FeatureClear     = "clear"
	FeatureRefused   = "refused"
)

// LandedFeature is what CheckChain said of one feature the merge changes: Reason is the text of its error when the
// outcome is FeatureRefused, and empty when it is FeatureClear.
type LandedFeature struct {
	Feature string
	Outcome string
	Reason  string
}

// Landing is what CheckLanding found out about a merge that lands a reviewed tree: the merge and the head in full, the
// base they were told from (the merge base of the two parents), the tree of the head (only when verified), the outcome
// and each feature the merge changes, in order. Features is empty, never nil, when the merge changes no feature document.
type Landing struct {
	Merge, Base, Head, Tree string
	Outcome                 string
	Features                []LandedFeature
}

// CheckLanding decides whether the merge of a pull request, merge (a revision as a person types it) in the repository
// that holds dir (absolute), lands a tree that stored reviews cleared (own-rdd T7a, S7 b-e). The merge must have exactly
// two parents, the first the base and the second the head of the pull request, and those two exactly one merge base B.
// The features are the feature documents the paths of B..H change; the commits are reviewed when at least one feature's
// chain clears B..H (CheckChain, which this does not re-implement), every feature being checked; a merge that changes no
// feature document passes as NoFeature. Once a chain clears, the tree of the merge must be the tree of the head. Whatever
// cannot be told, a failing git included, is a refusal. Nothing is stored.
func (s Service) CheckLanding(dir, merge string) (Landing, error) {
	if err := review.CheckRev(merge); err != nil {
		return Landing{}, err
	}
	repo, err := s.git.Locate(dir)
	if err != nil {
		return Landing{}, fmt.Errorf("locate the repository of %s: %w", dir, err)
	}
	mergeName, err := s.git.Commit(repo, merge)
	if err != nil {
		return Landing{}, fmt.Errorf("resolve merge %q: %w", merge, err)
	}
	parents, err := s.git.Parents(repo, mergeName)
	if err != nil {
		return Landing{}, fmt.Errorf("read the parents of %s: %w", mergeName, err)
	}
	first, head, err := review.MergeHead(mergeName, parents)
	if err != nil {
		return Landing{}, err
	}
	bases, err := s.git.MergeBases(repo, first, head)
	if err != nil {
		return Landing{}, fmt.Errorf("find the merge base of %s and %s: %w", first, head, err)
	}
	base, err := review.OneMergeBase(first, head, bases)
	if err != nil {
		return Landing{}, err
	}
	paths, err := s.git.Paths(repo, base, head)
	if err != nil {
		return Landing{}, fmt.Errorf("list the paths of %s..%s: %w", base, head, err)
	}
	features, err := review.FeaturesOf(paths)
	if err != nil {
		return Landing{}, err
	}
	landing := Landing{Merge: mergeName, Base: base, Head: head, Features: []LandedFeature{}}
	if len(features) == 0 {
		landing.Outcome = OutcomeNoFeature
		return landing, nil
	}
	cleared, reasons := false, make([]error, len(features))
	for i, feature := range features {
		if _, reasons[i] = s.CheckChain(dir, feature, base, head); reasons[i] != nil {
			landing.Features = append(landing.Features, LandedFeature{Feature: feature, Outcome: FeatureRefused, Reason: reasons[i].Error()})
			continue
		}
		cleared = true
		landing.Features = append(landing.Features, LandedFeature{Feature: feature, Outcome: FeatureClear})
	}
	if !cleared {
		return Landing{}, review.NoFeatureClears(base, head, features, reasons)
	}
	mergeTree, err := s.git.Tree(repo, mergeName)
	if err != nil {
		return Landing{}, fmt.Errorf("read the tree of %s: %w", mergeName, err)
	}
	headTree, err := s.git.Tree(repo, head)
	if err != nil {
		return Landing{}, fmt.Errorf("read the tree of %s: %w", head, err)
	}
	if err := review.CheckLandedTree(mergeName, mergeTree, head, headTree, first); err != nil {
		return Landing{}, err
	}
	landing.Tree, landing.Outcome = headTree, OutcomeVerified
	return landing, nil
}
