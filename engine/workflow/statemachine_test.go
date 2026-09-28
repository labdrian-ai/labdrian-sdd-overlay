package workflow

import (
	"errors"
	"strings"
	"testing"
)

func seq(kind Kind, n int, prev string) WorkflowEvent {
	e := validCreatedEvent()
	e.Seq = n
	e.PrevDigest = prev
	e.Kind = kind
	if kind != KindCreated {
		e.GoalID, e.GoalDigest, e.Profile = "", "", ""
	}
	return e
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
		seq(KindCreated, 0, ""),
		seq(KindStarted, 1, ""),
		func() WorkflowEvent { e := seq(KindStageRecorded, 2, ""); e.Stage = "explore"; return e }(),
		func() WorkflowEvent { e := seq(KindStageRecorded, 3, ""); e.Stage = "implement"; return e }(),
		seq(KindPaused, 4, ""),
		seq(KindResumed, 5, ""),
		func() WorkflowEvent {
			e := seq(KindVerified, 6, "")
			e.Checked = &Checked{ChainDigest: strings.Repeat("b", 64), GoalDigest: strings.Repeat("a", 64), Profile: "odd"}
			return e
		}(),
		func() WorkflowEvent { e := seq(KindClosed, 7, ""); e.Outcome = string(OutcomeCompleted); return e }(),
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
	events := withDigestChain([]WorkflowEvent{seq(KindStarted, 0, "")})
	if _, err := Replay(events); !errors.Is(err, ErrNotCreatedFirst) {
		t.Fatalf("Replay() err = %v, want ErrNotCreatedFirst", err)
	}
}

func TestReplayRejectsSecondCreated(t *testing.T) {
	events := withDigestChain([]WorkflowEvent{
		seq(KindCreated, 0, ""),
		seq(KindCreated, 1, ""),
	})
	if _, err := Replay(events); !errors.Is(err, ErrAlreadyCreated) {
		t.Fatalf("Replay() err = %v, want ErrAlreadyCreated", err)
	}
}

func TestReplayRejectsEventsAfterClose(t *testing.T) {
	events := withDigestChain([]WorkflowEvent{
		seq(KindCreated, 0, ""),
		func() WorkflowEvent {
			e := seq(KindClosed, 1, "")
			e.Outcome = string(OutcomeAbandoned)
			e.Reason = "stop"
			return e
		}(),
		seq(KindStarted, 2, ""),
	})
	if _, err := Replay(events); !errors.Is(err, ErrClosed) {
		t.Fatalf("Replay() err = %v, want ErrClosed", err)
	}
}

func TestReplayRejectsStartedFromNonCreated(t *testing.T) {
	events := withDigestChain([]WorkflowEvent{
		seq(KindCreated, 0, ""),
		seq(KindStarted, 1, ""),
		seq(KindStarted, 2, ""),
	})
	if _, err := Replay(events); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("Replay() err = %v, want ErrInvalidTransition", err)
	}
}

func TestReplayRejectsPausedFromNonRunning(t *testing.T) {
	events := withDigestChain([]WorkflowEvent{
		seq(KindCreated, 0, ""),
		seq(KindPaused, 1, ""),
	})
	if _, err := Replay(events); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("Replay() err = %v, want ErrInvalidTransition", err)
	}
}

func TestReplayRejectsResumedFromNonPaused(t *testing.T) {
	events := withDigestChain([]WorkflowEvent{
		seq(KindCreated, 0, ""),
		seq(KindStarted, 1, ""),
		seq(KindResumed, 2, ""),
	})
	if _, err := Replay(events); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("Replay() err = %v, want ErrInvalidTransition", err)
	}
}

func TestReplayRejectsStageRecordedWhileNotRunning(t *testing.T) {
	events := withDigestChain([]WorkflowEvent{
		seq(KindCreated, 0, ""),
		func() WorkflowEvent { e := seq(KindStageRecorded, 1, ""); e.Stage = "explore"; return e }(),
	})
	if _, err := Replay(events); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("Replay() err = %v, want ErrInvalidTransition", err)
	}
}

func TestReplayRejectsDuplicateStage(t *testing.T) {
	events := withDigestChain([]WorkflowEvent{
		seq(KindCreated, 0, ""),
		seq(KindStarted, 1, ""),
		func() WorkflowEvent { e := seq(KindStageRecorded, 2, ""); e.Stage = "explore"; return e }(),
		func() WorkflowEvent { e := seq(KindStageRecorded, 3, ""); e.Stage = "explore"; return e }(),
	})
	if _, err := Replay(events); !errors.Is(err, ErrDuplicateStage) {
		t.Fatalf("Replay() err = %v, want ErrDuplicateStage", err)
	}
}

func TestReplayAllowsVerifiedInAnyNonClosedState(t *testing.T) {
	for _, kind := range []Kind{KindCreated, KindStarted, KindPaused} {
		t.Run(string(kind), func(t *testing.T) {
			var events []WorkflowEvent
			events = append(events, seq(KindCreated, 0, ""))
			switch kind {
			case KindStarted:
				events = append(events, seq(KindStarted, 1, ""))
			case KindPaused:
				events = append(events, seq(KindStarted, 1, ""), seq(KindPaused, 2, ""))
			}
			verified := seq(KindVerified, len(events), "")
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
		seq(KindCreated, 0, ""),
		seq(KindStarted, 1, ""),
		func() WorkflowEvent { e := seq(KindClosed, 2, ""); e.Outcome = string(OutcomeCompleted); return e }(),
	})
	if _, err := Replay(events); !errors.Is(err, ErrCompletedRequiresVerify) {
		t.Fatalf("Replay() err = %v, want ErrCompletedRequiresVerify", err)
	}
}

func TestReplayClosedCompletedRejectsStaleVerify(t *testing.T) {
	events := withDigestChain([]WorkflowEvent{
		seq(KindCreated, 0, ""),
		seq(KindStarted, 1, ""),
		func() WorkflowEvent {
			e := seq(KindVerified, 2, "")
			e.Checked = &Checked{ChainDigest: strings.Repeat("b", 64), GoalDigest: strings.Repeat("a", 64), Profile: "odd"}
			return e
		}(),
		func() WorkflowEvent { e := seq(KindStageRecorded, 3, ""); e.Stage = "explore"; return e }(),
		func() WorkflowEvent { e := seq(KindClosed, 4, ""); e.Outcome = string(OutcomeCompleted); return e }(),
	})
	if _, err := Replay(events); !errors.Is(err, ErrCompletedRequiresVerify) {
		t.Fatalf("Replay() err = %v, want ErrCompletedRequiresVerify", err)
	}
}

func TestReplayClosedAbandonedAllowedFromAnyNonClosedState(t *testing.T) {
	events := withDigestChain([]WorkflowEvent{
		seq(KindCreated, 0, ""),
		func() WorkflowEvent {
			e := seq(KindClosed, 1, "")
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
