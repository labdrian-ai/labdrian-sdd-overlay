package app

import "github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"

// featureListing is a feature of the tree with the listing of its results directory.
type featureListing struct {
	feature string
	names   []string
}

// factsOfAnyFeature reads the facts of a commit for the stop hook (own-rdd T6b6a, S6 e, L66): what git says once, and its
// role records from every listing, in the order given. A passive commit is returned with no record read. A record that
// does not read back or hold counts as absent, whatever the reason, so no record error is returned. The sweep (Sweep,
// SweepFired and Answered) is that of the first listing that has one; the commit is a sweep commit, and polished, when any
// listing says so. It returns the first parent of the commit. Nothing is stored.
func (s Service) factsOfAnyFeature(repo Repository, listings []featureListing, commit string) (review.Facts, string, error) {
	facts, parent, subject, err := s.gitFacts(repo, commit)
	if err != nil || facts.Passive {
		return facts, parent, err
	}
	merged, haveSweep := facts, false
	for _, listing := range listings {
		own := facts
		if err := s.roleFacts(repo, listing.feature, listing.names, commit, parent, subject, true, &own); err != nil {
			return review.Facts{}, "", err
		}
		merged.SweepCommit = merged.SweepCommit || own.SweepCommit
		merged.Polished = merged.Polished || own.Polished
		if !haveSweep && own.Sweep != "" {
			merged.Sweep, merged.SweepFired, merged.Answered = own.Sweep, own.SweepFired, own.Answered
			haveSweep = true
		}
	}
	return merged, parent, nil
}
