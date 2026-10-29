package app

import "github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"

// NewFeatureListing and FactsOfAnyFeature let the external tests reach the facts of a commit read from every feature
// (own-rdd T6b6a); the variadic listings let them pass values without naming the unexported type.
func NewFeatureListing(feature string, names []string) featureListing {
	return featureListing{feature: feature, names: names}
}

func (s Service) FactsOfAnyFeature(repo Repository, commit string, listings ...featureListing) (review.Facts, string, error) {
	return s.factsOfAnyFeature(repo, listings, commit)
}
