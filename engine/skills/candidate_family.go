package skills

import "strings"

// CandidateFamily is the record namespace a topic key belongs to.
type CandidateFamily int

const (
	// FamilyNone is any topic that is neither a candidate nor a draft.
	FamilyNone CandidateFamily = iota
	// FamilyCandidate is a topic under procedural/candidates/.
	FamilyCandidate
	// FamilyDraft is a topic under procedural/drafts/.
	FamilyDraft
)

// CandidateTopicFamily names the family of a topic key, ignoring the goal prefix a record
// created under a goal carries. A source adapter uses it to keep only the two namespaces the
// candidate use case owns.
func CandidateTopicFamily(topic string) CandidateFamily {
	_, rest := splitGoalPrefix(topic)
	switch {
	case strings.HasPrefix(rest, "procedural/candidates/"):
		return FamilyCandidate
	case strings.HasPrefix(rest, "procedural/drafts/"):
		return FamilyDraft
	}
	return FamilyNone
}
