package skills

import (
	"strings"
	"testing"
)

// The vocabularies are the ones of skills/_shared/procedural-candidate-detection.md section 9 (candidate)
// and section 8 (draft); ADR C23 M10, decision 1.

func TestParseCandidateStatusKnowsExactlyTheSevenValues(t *testing.T) {
	want := []CandidateStatus{StatusObserving, StatusEmitted, StatusRejected, StatusDrafted, StatusRegistered, StatusPromoted, StatusRetired}
	spelled := []string{"observing", "emitted", "rejected", "drafted", "registered", "promoted", "retired"}
	for i, s := range spelled {
		got, err := ParseCandidateStatus(s)
		if err != nil || got != want[i] {
			t.Errorf("ParseCandidateStatus(%q) = %q, %v; want %q", s, got, err, want[i])
		}
	}
	for _, bad := range []string{"", "Observing", "open", "abandoned", " emitted", "emitted "} {
		if _, err := ParseCandidateStatus(bad); err == nil || !strings.Contains(err.Error(), "candidate status") {
			t.Errorf("ParseCandidateStatus(%q) error = %v; want a refusal naming the candidate status", bad, err)
		}
	}
}

func TestParseDraftStatusKnowsExactlyTheThreeValues(t *testing.T) {
	for want, s := range map[DraftStatus]string{DraftOpen: "open", DraftRegistered: "registered", DraftAbandoned: "abandoned"} {
		got, err := ParseDraftStatus(s)
		if err != nil || got != want {
			t.Errorf("ParseDraftStatus(%q) = %q, %v; want %q", s, got, err, want)
		}
	}
	for _, bad := range []string{"", "Open", "emitted", "drafted"} {
		if _, err := ParseDraftStatus(bad); err == nil || !strings.Contains(err.Error(), "draft status") {
			t.Errorf("ParseDraftStatus(%q) error = %v; want a refusal naming the draft status", bad, err)
		}
	}
}

// legal is the transition table of section 9, one row per line, and nothing else.
var legalTransitions = []struct {
	from, to CandidateStatus
	actor    Actor
}{
	{"", StatusObserving, ActorAgent},
	{StatusObserving, StatusEmitted, ActorAgent},
	{StatusObserving, StatusRejected, ActorAgent},
	{StatusEmitted, StatusRejected, ActorAgent},
	{StatusEmitted, StatusDrafted, ActorAgent},
	{StatusDrafted, StatusRegistered, ActorAgent},
	{StatusRegistered, StatusRegistered, ActorAgent},
	{StatusDrafted, StatusPromoted, ActorHuman},
	{StatusRegistered, StatusPromoted, ActorHuman},
	{StatusRegistered, StatusRetired, ActorAgent},
	{StatusPromoted, StatusRetired, ActorHuman},
}

func TestCheckTransitionAcceptsEveryRowOfTheTable(t *testing.T) {
	for _, row := range legalTransitions {
		if err := CheckTransition(row.from, row.to, row.actor); err != nil {
			t.Errorf("CheckTransition(%q, %q, %s) = %v; want nil", row.from, row.to, row.actor, err)
		}
	}
}

func TestCheckTransitionRefusesEveryOtherPair(t *testing.T) {
	all := []CandidateStatus{"", StatusObserving, StatusEmitted, StatusRejected, StatusDrafted, StatusRegistered, StatusPromoted, StatusRetired}
	legal := map[[3]string]bool{}
	for _, row := range legalTransitions {
		legal[[3]string{string(row.from), string(row.to), string(row.actor)}] = true
	}
	for _, from := range all {
		for _, to := range all {
			for _, actor := range []Actor{ActorAgent, ActorHuman} {
				if from == to && from != "" {
					continue // same-state lines are the non-transition events, asserted below
				}
				if legal[[3]string{string(from), string(to), string(actor)}] {
					continue
				}
				if err := CheckTransition(from, to, actor); err == nil {
					t.Errorf("CheckTransition(%q, %q, %s) = nil; the table has no such row", from, to, actor)
				}
			}
		}
	}
}

// A line that keeps the state ("registered -> registered", "promoted -> promoted") records an event that is not a
// transition (section 9); it is legal in every state once a candidate exists, for either actor, and a candidate
// that does not exist yet cannot have one.
func TestCheckTransitionAllowsASameStateLineOnceTheCandidateExists(t *testing.T) {
	for _, s := range []CandidateStatus{StatusObserving, StatusEmitted, StatusRejected, StatusDrafted, StatusRegistered, StatusPromoted, StatusRetired} {
		for _, actor := range []Actor{ActorAgent, ActorHuman} {
			if err := CheckTransition(s, s, actor); err != nil {
				t.Errorf("CheckTransition(%q, %q, %s) = %v; want nil", s, s, actor, err)
			}
		}
	}
	if err := CheckTransition("", "", ActorAgent); err == nil {
		t.Error("CheckTransition(none, none) = nil; a candidate that does not exist has no same-state line")
	}
}

func TestCheckTransitionRefusalSaysWhoMayAndWhat(t *testing.T) {
	err := CheckTransition(StatusDrafted, StatusPromoted, ActorAgent)
	if err == nil || !strings.Contains(err.Error(), "drafted -> promoted") || !strings.Contains(err.Error(), "human") {
		t.Errorf("an agent promoting: error = %v; want it to name the pair and the human", err)
	}
	err = CheckTransition(StatusRetired, StatusObserving, ActorAgent)
	if err == nil || !strings.Contains(err.Error(), "retired -> observing") {
		t.Errorf("a reopened candidate: error = %v; want it to name the pair", err)
	}
	err = CheckTransition("", StatusEmitted, ActorAgent)
	if err == nil || !strings.Contains(err.Error(), "none -> emitted") {
		t.Errorf("a candidate born emitted: error = %v; want it to name the pair as from none", err)
	}
}

func TestParseActorKnowsAgentAndHuman(t *testing.T) {
	for want, s := range map[Actor]string{ActorAgent: "agent", ActorHuman: "human"} {
		if got, err := ParseActor(s); err != nil || got != want {
			t.Errorf("ParseActor(%q) = %q, %v; want %q", s, got, err, want)
		}
	}
	if _, err := ParseActor("robot"); err == nil {
		t.Error(`ParseActor("robot") = nil; want a refusal`)
	}
}
