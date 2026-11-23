package app

import (
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
)

// NewFeatureListing and FactsOfAnyFeature let the external tests reach the facts of a commit read from every feature
// (own-rdd T6b6a); the variadic listings let them pass values without naming the unexported type.
func NewFeatureListing(feature string, names []string) featureListing {
	return featureListing{feature: feature, names: names}
}

func (s Service) FactsOfAnyFeature(repo Repository, commit string, listings ...featureListing) (review.Facts, string, error) {
	return s.factsOfAnyFeature(repo, listings, commit)
}

// KeptResults lists the stored results of the commits of the branch HEAD is on, in the repository that holds dir
// (absolute), that have one kept finding or more (S6): what keptResultsOf finds for the pass of Stop, which is the only
// production caller, so production exports no listing of its own.
func (s Service) KeptResults(dir string) ([]KeptResult, []error, error) {
	repo, err := s.git.Locate(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("locate the repository of %s: %w", dir, err)
	}
	return s.keptResultsOf(repo)
}
