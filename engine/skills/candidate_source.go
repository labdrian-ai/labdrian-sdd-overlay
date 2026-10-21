package skills

// CandidateObservation is one raw Engram observation returned to the skills use case.
type CandidateObservation struct {
	TopicKey string
	Content  string
}

// CandidateObservations contains the two Engram topic families used by M10.
type CandidateObservations struct {
	Candidates []CandidateObservation
	Drafts     []CandidateObservation
}

// CandidateSource is the read-only Engram port owned by the skills domain. Its
// adapter is chosen by the command composition root; callers cannot write records.
type CandidateSource interface {
	ListCandidateObservations(project string) (CandidateObservations, error)
}
