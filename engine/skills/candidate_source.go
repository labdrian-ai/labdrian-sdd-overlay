package skills

// CandidateObservation is one raw memorycard observation returned to the skills use case.
type CandidateObservation struct {
	TopicKey string
	Content  string
}

// CandidateObservations contains the two memorycard topic families used by M10.
type CandidateObservations struct {
	Candidates []CandidateObservation
	Drafts     []CandidateObservation
}

// CandidateSource is the read-only memorycard port owned by the skills domain. Its
// adapter is chosen by the command composition root; callers cannot write records.
type CandidateSource interface {
	ListCandidateObservations(project string) (CandidateObservations, error)
}

// LegacyOccurrenceResolver is the read-only port that says which memorycard memory a legacy
// `engram:<n>` occurrence was imported as. It is given the numbers as written (digits only) and
// returns a map from number to memory id holding only the numbers that were imported; a number
// that is absent was never mapped and counts as itself. The domain translates ids (see
// TranslateOccurrence); an adapter never edits a record.
type LegacyOccurrenceResolver interface {
	ResolveLegacyOccurrences(numbers []string) (map[string]int, error)
}
