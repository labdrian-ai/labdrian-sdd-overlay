package app

import (
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
)

// CheckOnto decides whether the head being delivered is cleared from the integration branch the delivery chose, in the
// repository that holds dir (absolute): the engine finds where the chain starts instead of the role (own-rdd S11 a).
// onto and head are revisions as a person types them. The start is the one merge base of onto and head
// (review.OneMergeBase), so the commits the pull request adds are the ones after it even when onto moved on after the
// branch was made; none or several merge bases are refused. The chain from that commit is then CheckChain's, as if the
// merge base had been given as the base. onto is still a revision the agent names: a commit of the branch itself given
// as onto shortens the chain as a wrong base does. Nothing is stored.
func (s Service) CheckOnto(dir, feature, onto, head string) (Chained, error) {
	if err := review.CheckFeature(feature); err != nil {
		return Chained{}, err
	}
	for _, rev := range []string{onto, head} {
		if err := review.CheckRev(rev); err != nil {
			return Chained{}, err
		}
	}
	repo, err := s.git.Locate(dir)
	if err != nil {
		return Chained{}, fmt.Errorf("locate the repository of %s: %w", dir, err)
	}
	ontoName, err := s.git.Commit(repo, onto)
	if err != nil {
		return Chained{}, fmt.Errorf("resolve onto %q: %w", onto, err)
	}
	headName, err := s.git.Commit(repo, head)
	if err != nil {
		return Chained{}, fmt.Errorf("resolve head %q: %w", head, err)
	}
	bases, err := s.git.MergeBases(repo, ontoName, headName)
	if err != nil {
		return Chained{}, fmt.Errorf("find the merge base of %s and %s: %w", ontoName, headName, err)
	}
	base, err := review.OneMergeBase(ontoName, headName, bases)
	if err != nil {
		return Chained{}, err
	}
	return s.CheckChain(dir, feature, base, headName)
}
