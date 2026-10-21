package skills

import "fmt"

// The lifecycle of a procedural skill candidate and of its draft, as skills/_shared/procedural-candidate-detection.md
// states it (section 9 for the candidate, section 8 for the draft; ADR C23 M10, decision 1). The engine only reads
// these records and judges them: "no engine verb ever writes Status", so nothing here moves a record, it only says
// whether a move that a record claims was one the table allows.

// CandidateStatus is the Status of a candidate record.
type CandidateStatus string

// The seven values of a candidate's Status.
const (
	StatusObserving  CandidateStatus = "observing"
	StatusEmitted    CandidateStatus = "emitted"
	StatusRejected   CandidateStatus = "rejected"
	StatusDrafted    CandidateStatus = "drafted"
	StatusRegistered CandidateStatus = "registered"
	StatusPromoted   CandidateStatus = "promoted"
	StatusRetired    CandidateStatus = "retired"
)

// ParseCandidateStatus reads one value of the vocabulary, spelled exactly.
func ParseCandidateStatus(s string) (CandidateStatus, error) {
	switch v := CandidateStatus(s); v {
	case StatusObserving, StatusEmitted, StatusRejected, StatusDrafted, StatusRegistered, StatusPromoted, StatusRetired:
		return v, nil
	}
	return "", fmt.Errorf("candidate status %q is not one of observing, emitted, rejected, drafted, registered, promoted, retired", s)
}

// DraftStatus is the Status of a draft record, a vocabulary of its own.
type DraftStatus string

// The three values of a draft's Status.
const (
	DraftOpen       DraftStatus = "open"
	DraftRegistered DraftStatus = "registered"
	DraftAbandoned  DraftStatus = "abandoned"
)

// ParseDraftStatus reads one value of the draft vocabulary, spelled exactly.
func ParseDraftStatus(s string) (DraftStatus, error) {
	switch v := DraftStatus(s); v {
	case DraftOpen, DraftRegistered, DraftAbandoned:
		return v, nil
	}
	return "", fmt.Errorf("draft status %q is not one of open, registered, abandoned", s)
}

// Actor is who makes a transition: the agent in a session, or a human.
type Actor string

// The two actors of the transition table.
const (
	ActorAgent Actor = "agent"
	ActorHuman Actor = "human"
)

// ParseActor reads the actor column of a History line.
func ParseActor(s string) (Actor, error) {
	switch v := Actor(s); v {
	case ActorAgent, ActorHuman:
		return v, nil
	}
	return "", fmt.Errorf("actor %q is not agent or human", s)
}

// transitionKey is one row of the table of section 9.
type transitionKey struct {
	from, to CandidateStatus
	actor    Actor
}

// noStatus is the state before a candidate exists: the "none" of the first row.
const noStatus CandidateStatus = ""

// transitions is the table of section 9, row for row. The two same-state rows the section lists
// (registered -> registered on a revision) are covered by the same-state rule of CheckTransition.
var transitions = map[transitionKey]bool{
	{noStatus, StatusObserving, ActorAgent}:        true,
	{StatusObserving, StatusEmitted, ActorAgent}:   true,
	{StatusObserving, StatusRejected, ActorAgent}:  true,
	{StatusEmitted, StatusRejected, ActorAgent}:    true,
	{StatusEmitted, StatusDrafted, ActorAgent}:     true,
	{StatusDrafted, StatusRegistered, ActorAgent}:  true,
	{StatusDrafted, StatusPromoted, ActorHuman}:    true,
	{StatusRegistered, StatusPromoted, ActorHuman}: true,
	{StatusRegistered, StatusRetired, ActorAgent}:  true,
	{StatusPromoted, StatusRetired, ActorHuman}:    true,
}

// CheckTransition says whether actor may move a candidate from one Status to another. from is the empty
// status for a candidate that does not exist yet ("none" in the table). A line that keeps the Status is
// an event that is not a transition (section 9: ownership lost, a disposition computed, a revision draft
// opened, a recurrence after retirement, the removal of a project copy after promotion), so it is legal
// in every state once the candidate exists, for either actor.
func CheckTransition(from, to CandidateStatus, actor Actor) error {
	if from == to && from != noStatus {
		return nil
	}
	if transitions[transitionKey{from, to, actor}] {
		return nil
	}
	return fmt.Errorf("transition %s -> %s by %s is not in the table of the candidate lifecycle%s",
		statusWord(from), statusWord(to), actor, whoMayHint(from, to, actor))
}

func statusWord(s CandidateStatus) string {
	if s == noStatus {
		return "none"
	}
	return string(s)
}

// whoMayHint names the actor that does hold the row when the pair is in the table for the other one.
func whoMayHint(from, to CandidateStatus, actor Actor) string {
	other := ActorHuman
	if actor == ActorHuman {
		other = ActorAgent
	}
	if transitions[transitionKey{from, to, other}] {
		return fmt.Sprintf(" (only the %s makes it)", other)
	}
	return ""
}
