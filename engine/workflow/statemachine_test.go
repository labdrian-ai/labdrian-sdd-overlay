package workflow

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// newEventOfKind returns a valid base event of kind, with the created-only
// payload cleared for every other kind. It does not set Seq or PrevDigest:
// every call site either runs the result through withDigestChain (which
// assigns both from the event's position in the chain) or sets them itself
// when testing CheckTransition directly against a specific position.
func newEventOfKind(kind Kind) WorkflowEvent {
	e := validCreatedEvent()
	e.Kind = kind
	if kind != KindCreated {
		e.GoalID, e.GoalDigest, e.Profile = "", "", ""
	}
	return e
}

// seq is a short alias for newEventOfKind, used throughout this file's event
// chain literals.
func seq(kind Kind) WorkflowEvent {
	return newEventOfKind(kind)
}

func withDigestChain(events []WorkflowEvent) []WorkflowEvent {
	prev := ""
	for i := range events {
		events[i].Seq = i
		events[i].PrevDigest = prev
		d, err := EventDigest(events[i])
		if err != nil {
			panic(err)
		}
		prev = d
	}
	return events
}

func TestReplayHappyPath(t *testing.T) {
	events := []WorkflowEvent{
		seq(KindCreated),
		seq(KindStarted),
		func() WorkflowEvent { e := seq(KindStageRecorded); e.Stage = "explore"; return e }(),
		func() WorkflowEvent { e := seq(KindStageRecorded); e.Stage = "implement"; return e }(),
		seq(KindPaused),
		seq(KindResumed),
		func() WorkflowEvent {
			e := seq(KindVerified)
			e.Checked = &Checked{ChainDigest: strings.Repeat("b", 64), GoalDigest: strings.Repeat("a", 64), Profile: "odd"}
			return e
		}(),
		func() WorkflowEvent { e := seq(KindClosed); e.Outcome = string(OutcomeCompleted); return e }(),
	}
	events = withDigestChain(events)

	state, err := Replay(events)
	if err != nil {
		t.Fatalf("Replay() = %v, want nil", err)
	}
	if state.Status != StatusClosed {
		t.Fatalf("Replay() status = %q, want %q", state.Status, StatusClosed)
	}
	if state.CloseOutcome != OutcomeCompleted {
		t.Fatalf("Replay() close outcome = %q, want %q", state.CloseOutcome, OutcomeCompleted)
	}
	wantStages := []string{"explore", "implement"}
	if len(state.Stages) != len(wantStages) || state.Stages[0] != wantStages[0] || state.Stages[1] != wantStages[1] {
		t.Fatalf("Replay() stages = %v, want %v", state.Stages, wantStages)
	}
	if state.Profile != "odd" || state.GoalID != "goal-1" {
		t.Fatalf("Replay() profile/goal = %q/%q, want odd/goal-1", state.Profile, state.GoalID)
	}
	if state.LastVerifiedSeq != 6 {
		t.Fatalf("Replay() last verified seq = %d, want 6", state.LastVerifiedSeq)
	}
}

func TestReplayFirstEventMustBeCreated(t *testing.T) {
	events := withDigestChain([]WorkflowEvent{seq(KindStarted)})
	if _, err := Replay(events); !errors.Is(err, ErrNotCreatedFirst) {
		t.Fatalf("Replay() err = %v, want ErrNotCreatedFirst", err)
	}
}

func TestReplayRejectsSecondCreated(t *testing.T) {
	events := withDigestChain([]WorkflowEvent{
		seq(KindCreated),
		seq(KindCreated),
	})
	if _, err := Replay(events); !errors.Is(err, ErrAlreadyCreated) {
		t.Fatalf("Replay() err = %v, want ErrAlreadyCreated", err)
	}
}

func TestReplayRejectsEventsAfterClose(t *testing.T) {
	events := withDigestChain([]WorkflowEvent{
		seq(KindCreated),
		func() WorkflowEvent {
			e := seq(KindClosed)
			e.Outcome = string(OutcomeAbandoned)
			e.Reason = "stop"
			return e
		}(),
		seq(KindStarted),
	})
	if _, err := Replay(events); !errors.Is(err, ErrClosed) {
		t.Fatalf("Replay() err = %v, want ErrClosed", err)
	}
}

func TestReplayRejectsStartedFromNonCreated(t *testing.T) {
	events := withDigestChain([]WorkflowEvent{
		seq(KindCreated),
		seq(KindStarted),
		seq(KindStarted),
	})
	if _, err := Replay(events); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("Replay() err = %v, want ErrInvalidTransition", err)
	}
}

func TestReplayRejectsPausedFromNonRunning(t *testing.T) {
	events := withDigestChain([]WorkflowEvent{
		seq(KindCreated),
		seq(KindPaused),
	})
	if _, err := Replay(events); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("Replay() err = %v, want ErrInvalidTransition", err)
	}
}

func TestReplayRejectsResumedFromNonPaused(t *testing.T) {
	events := withDigestChain([]WorkflowEvent{
		seq(KindCreated),
		seq(KindStarted),
		seq(KindResumed),
	})
	if _, err := Replay(events); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("Replay() err = %v, want ErrInvalidTransition", err)
	}
}

func TestReplayRejectsStageRecordedWhileNotRunning(t *testing.T) {
	events := withDigestChain([]WorkflowEvent{
		seq(KindCreated),
		func() WorkflowEvent { e := seq(KindStageRecorded); e.Stage = "explore"; return e }(),
	})
	if _, err := Replay(events); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("Replay() err = %v, want ErrInvalidTransition", err)
	}
}

func TestReplayRejectsDuplicateStage(t *testing.T) {
	events := withDigestChain([]WorkflowEvent{
		seq(KindCreated),
		seq(KindStarted),
		func() WorkflowEvent { e := seq(KindStageRecorded); e.Stage = "explore"; return e }(),
		func() WorkflowEvent { e := seq(KindStageRecorded); e.Stage = "explore"; return e }(),
	})
	if _, err := Replay(events); !errors.Is(err, ErrDuplicateStage) {
		t.Fatalf("Replay() err = %v, want ErrDuplicateStage", err)
	}
}

func TestReplayAllowsVerifiedInAnyNonClosedState(t *testing.T) {
	for _, kind := range []Kind{KindCreated, KindStarted, KindPaused} {
		t.Run(string(kind), func(t *testing.T) {
			var events []WorkflowEvent
			events = append(events, seq(KindCreated))
			switch kind {
			case KindStarted:
				events = append(events, seq(KindStarted))
			case KindPaused:
				events = append(events, seq(KindStarted), seq(KindPaused))
			}
			verified := seq(KindVerified)
			verified.Checked = &Checked{ChainDigest: strings.Repeat("b", 64), GoalDigest: strings.Repeat("a", 64), Profile: "odd"}
			events = append(events, verified)
			events = withDigestChain(events)
			if _, err := Replay(events); err != nil {
				t.Fatalf("Replay() = %v, want nil", err)
			}
		})
	}
}

func TestReplayClosedCompletedRequiresImmediatelyPrecedingVerified(t *testing.T) {
	events := withDigestChain([]WorkflowEvent{
		seq(KindCreated),
		seq(KindStarted),
		func() WorkflowEvent { e := seq(KindClosed); e.Outcome = string(OutcomeCompleted); return e }(),
	})
	if _, err := Replay(events); !errors.Is(err, ErrCompletedRequiresVerify) {
		t.Fatalf("Replay() err = %v, want ErrCompletedRequiresVerify", err)
	}
}

func TestReplayClosedCompletedRejectsStaleVerify(t *testing.T) {
	events := withDigestChain([]WorkflowEvent{
		seq(KindCreated),
		seq(KindStarted),
		func() WorkflowEvent {
			e := seq(KindVerified)
			e.Checked = &Checked{ChainDigest: strings.Repeat("b", 64), GoalDigest: strings.Repeat("a", 64), Profile: "odd"}
			return e
		}(),
		func() WorkflowEvent { e := seq(KindStageRecorded); e.Stage = "explore"; return e }(),
		func() WorkflowEvent { e := seq(KindClosed); e.Outcome = string(OutcomeCompleted); return e }(),
	})
	if _, err := Replay(events); !errors.Is(err, ErrCompletedRequiresVerify) {
		t.Fatalf("Replay() err = %v, want ErrCompletedRequiresVerify", err)
	}
}

func TestReplayClosedAbandonedAllowedFromAnyNonClosedState(t *testing.T) {
	events := withDigestChain([]WorkflowEvent{
		seq(KindCreated),
		func() WorkflowEvent {
			e := seq(KindClosed)
			e.Outcome = string(OutcomeAbandoned)
			e.Reason = "no longer needed"
			return e
		}(),
	})
	state, err := Replay(events)
	if err != nil {
		t.Fatalf("Replay() = %v, want nil", err)
	}
	if state.Status != StatusClosed || state.CloseOutcome != OutcomeAbandoned {
		t.Fatalf("Replay() = %+v, want closed/abandoned", state)
	}
}

func TestReplayEmptyEventsErrors(t *testing.T) {
	if _, err := Replay(nil); err == nil {
		t.Fatalf("Replay() = nil, want error for empty events")
	}
}

// CheckTransition's defensive branches (an unknown close outcome, an unknown
// event kind) are exercised directly.

func TestCheckTransitionRejectsUnknownCloseOutcome(t *testing.T) {
	events := withDigestChain([]WorkflowEvent{
		seq(KindCreated),
		seq(KindStarted),
	})
	state, err := Replay(events)
	if err != nil {
		t.Fatalf("Replay() = %v, want nil", err)
	}
	next := seq(KindClosed)
	next.Outcome = "bogus-outcome"
	if err := CheckTransition(state, next); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("CheckTransition() err = %v, want ErrInvalidTransition", err)
	}
}

func TestApplyEventStageRecordedDoesNotAliasAcrossBranches(t *testing.T) {
	events := withDigestChain([]WorkflowEvent{
		seq(KindCreated),
		seq(KindStarted),
		func() WorkflowEvent { e := seq(KindStageRecorded); e.Stage = "setup"; return e }(),
	})
	base, err := Replay(events)
	if err != nil {
		t.Fatalf("Replay() = %v, want nil", err)
	}

	mk := func(stage string) WorkflowEvent { e := seq(KindStageRecorded); e.Stage = stage; return e }
	branchA := applyEvent(base, mk("explore"))
	branchB := applyEvent(base, mk("implement"))

	if base.hasStage("explore") || base.hasStage("implement") {
		t.Fatalf("base mutated by a branch: %+v", base)
	}
	if !branchA.hasStage("explore") || branchA.hasStage("implement") || len(branchA.Stages) != 2 {
		t.Fatalf("branchA = %+v, want only setup+explore", branchA)
	}
	if !branchB.hasStage("implement") || branchB.hasStage("explore") || len(branchB.Stages) != 2 {
		t.Fatalf("branchB = %+v, want only setup+implement", branchB)
	}
}

func TestCheckTransitionRejectsStageRecordedBeyondMaxStages(t *testing.T) {
	events := []WorkflowEvent{seq(KindCreated), seq(KindStarted)}
	for i := 0; i < MaxStages; i++ {
		e := seq(KindStageRecorded)
		e.Stage = fmt.Sprintf("stage-%d", i)
		events = append(events, e)
	}
	state, err := Replay(withDigestChain(events))
	if err != nil {
		t.Fatalf("Replay() = %v, want nil", err)
	}
	if len(state.Stages) != MaxStages {
		t.Fatalf("len(state.Stages) = %d, want %d", len(state.Stages), MaxStages)
	}
	next := seq(KindStageRecorded)
	next.Stage = "overflow"
	if err := CheckTransition(state, next); !errors.Is(err, ErrTooManyStages) {
		t.Fatalf("CheckTransition() err = %v, want ErrTooManyStages", err)
	}
}

func TestCheckTransitionRejectsUnknownKind(t *testing.T) {
	events := withDigestChain([]WorkflowEvent{
		seq(KindCreated),
		seq(KindStarted),
	})
	state, err := Replay(events)
	if err != nil {
		t.Fatalf("Replay() = %v, want nil", err)
	}
	next := seq(KindStarted)
	next.Kind = "bogus-kind"
	if err := CheckTransition(state, next); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("CheckTransition() err = %v, want ErrInvalidTransition", err)
	}
}
