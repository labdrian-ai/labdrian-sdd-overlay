package app

import (
	"fmt"
	"slices"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
)

// Range is one range of a chain: what clears Head (Checked) and the commit the range starts from, which is the base
// the first result that binds Head names, or the first exemption when there is no result.
type Range struct {
	Base string
	Checked
}

// Chained is what CheckChain found out about a head that is cleared as the end of an unbroken chain from a base: the
// full names of both and the ranges, from the one that starts at Base to the one that ends at Head, each starting where
// the one before ended.
type Chained struct {
	Base, Head string
	Ranges     []Range
}

// CheckChain decides whether the head being delivered is cleared from base, the integration base of the delivery, in
// the repository that holds dir (absolute). Reviews are per commit (own-rdd, L18): the head is cleared only when clear
// results or exemptions cover base..head as an unbroken chain, each range starting where the one before ended. base and
// head are revisions as a person types them.
//
// The walk goes back from the head. At each commit it asks what Check asks of a head (cleared), so every result and
// every exemption that binds the commit is held to its file and to git again and must be clear (review.Cleared), and
// the base of the range it follows is the next commit. It is done when that base is the delivery base. It is refused
// when no result or exemption ends at a commit (a gap, which is also where a chain that begins after the base ends, and
// where one that passes the base ends), when a range is not clear or does not hold, when git or the store fails, and
// when the walk comes back to a commit it already passed (review.ErrChainLoops): each step takes a commit not passed
// before, so it ends. Whatever cannot be told is a refusal. Nothing is stored.
func (s Service) CheckChain(dir, feature, base, head string) (Chained, error) {
	if err := review.CheckFeature(feature); err != nil {
		return Chained{}, err
	}
	for _, rev := range []string{base, head} {
		if err := review.CheckRev(rev); err != nil {
			return Chained{}, err
		}
	}
	repo, err := s.git.Locate(dir)
	if err != nil {
		return Chained{}, fmt.Errorf("locate the repository of %s: %w", dir, err)
	}
	baseName, err := s.git.Commit(repo, base)
	if err != nil {
		return Chained{}, fmt.Errorf("resolve base %q: %w", base, err)
	}
	headName, err := s.git.Commit(repo, head)
	if err != nil {
		return Chained{}, fmt.Errorf("resolve head %q: %w", head, err)
	}
	names, err := s.results.Names(repo.Root, feature)
	if err != nil {
		return Chained{}, fmt.Errorf("list %s/%s: %w", review.ResultsDir, feature, err)
	}
	refused := func(err error) error { return fmt.Errorf("the chain from %s to %s: %w", baseName, headName, err) }
	chain, passed, point := Chained{Base: baseName, Head: headName}, map[string]bool{}, headName
	for {
		if passed[point] {
			return Chained{}, refused(fmt.Errorf("%w: %s", review.ErrChainLoops, point))
		}
		passed[point] = true
		checked, err := s.cleared(repo, feature, names, point)
		if err != nil {
			return Chained{}, refused(err)
		}
		start := checked.start()
		chain.Ranges = append(chain.Ranges, Range{Base: start, Checked: checked})
		if start == baseName {
			slices.Reverse(chain.Ranges)
			return chain, nil
		}
		point = start
	}
}

// start is the commit the range that clears the head starts from: the base of the first result that binds it, or of the
// first exemption when it has no result. review.Cleared leaves one of the two for a head it cleared.
func (c Checked) start() string {
	if len(c.Results) > 0 {
		return c.Results[0].Candidate.Base
	}
	return c.Exemptions[0].Candidate.Base
}
