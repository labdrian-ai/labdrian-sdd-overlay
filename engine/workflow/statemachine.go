package workflow

import (
	"errors"
	"fmt"
)

// Status is a workflow's current lifecycle status, derived by Replay.
type Status string

const (
	// StatusNone is the zero value: no event has been applied yet. It is the
	// single signal CheckTransition consults to decide whether next must be
	// the workflow's created event; there is no separate "has an event"
	// flag, since Status can only ever be StatusNone before the first event
	// is applied (the first applied event is always created, which sets
	// Status to StatusCreated) and never reverts to StatusNone afterward.
	StatusNone    Status = ""
	StatusCreated Status = "created"
	StatusRunning Status = "running"
	StatusPaused  Status = "paused"
	StatusClosed  Status = "closed"
)

// Sentinel errors returned by CheckTransition and Replay. Wrap with %w so
// callers can distinguish the failure with errors.Is.
var (
	ErrNotCreatedFirst         = errors.New("workflow: the first event must be a created event at seq 0")
	ErrAlreadyCreated          = errors.New("workflow: a workflow may be created only once")
	ErrClosed                  = errors.New("workflow: no event may follow a closed workflow")
	ErrInvalidTransition       = errors.New("workflow: invalid lifecycle transition")
	ErrDuplicateStage          = errors.New("workflow: stage already recorded")
	ErrCompletedRequiresVerify = errors.New("workflow: a completed close requires an immediately preceding verified event")
	ErrTooManyStages           = errors.New("workflow: stage limit exceeded")
)

// MaxStages bounds how many stages a single workflow may record. The
// largest built-in Workflow Profile (engine/workflowprofile) declares seven
// stages; 256 gives generous headroom for a future profile with a much
// longer stage chain while still bounding the per-event copy cost in
// applyEvent (see State's doc comment) to a small, predictable amount of
// work.
const MaxStages = 256

// State is the pure state derived by replaying a workflow's event log: its
// current status, the profile and goal it was created against, its recorded
// stages in order, the seq of the last verified event (-1 if none), and its
// close outcome once closed.
//
// Branch ownership: applyEvent copies Stages and stageSet fresh per
// stage_recorded event, so branches from a common ancestor never alias.
// MaxStages bounds the copy cost.
type State struct {
	Status          Status
	Profile         string
	GoalID          string
	GoalDigest      string
	RoleChainID     string
	RoleChainHead   string
	Stages          []string
	LastVerifiedSeq int
	CloseOutcome    Outcome
	CloseReason     string

	// lastEventKind is the kind of the most recently applied event; it is
	// used only to enforce that a completed close immediately follows a
	// verified event.
	lastEventKind Kind

	// stageSet mirrors Stages as a set, giving hasStage an O(1) duplicate
	// check instead of an O(len(Stages)) scan. See the Replay invariant
	// above: it is grown in place, not copied, on the assumption that State
	// is used linearly.
	stageSet map[string]bool
}

// hasStage reports whether stage was already recorded, in O(1) via
// stageSet. A zero-value State (stageSet == nil) safely reports false for
// every stage, since reading from a nil map is valid Go.
func (s State) hasStage(stage string) bool {
	return s.stageSet[stage]
}

// CheckTransition reports whether next is a legal event to append after
// state. It enforces:
//   - the first event of a workflow must be a created event at seq 0, and a
//     workflow may be created only once;
//   - started is legal only from created;
//   - paused is legal only from running;
//   - resumed is legal only from paused;
//   - stage_recorded is legal only while running, and never records the
//     same stage twice;
//   - verified is legal from any non-closed state;
//   - closed with outcome completed is legal only when the immediately
//     preceding event was a verified event (a verify after the last
//     change); closed with outcome abandoned is legal from any non-closed
//     state;
//   - nothing follows a closed workflow.
//
// It assumes next already passed WorkflowEvent.Validate; it does not
// re-validate field shapes.
func CheckTransition(state State, next WorkflowEvent) error {
	if state.Status == StatusNone {
		if next.Kind != KindCreated || next.Seq != 0 {
			return fmt.Errorf("%w, got kind %q at seq %d", ErrNotCreatedFirst, next.Kind, next.Seq)
		}
		return nil
	}
	if state.Status == StatusClosed {
		return fmt.Errorf("%w: workflow is already closed", ErrClosed)
	}
	if next.Kind == KindCreated {
		return fmt.Errorf("%w", ErrAlreadyCreated)
	}

	switch next.Kind {
	case KindStarted:
		if state.Status != StatusCreated {
			return fmt.Errorf("%w: started is only legal from %q, current status is %q", ErrInvalidTransition, StatusCreated, state.Status)
		}
	case KindPaused:
		if state.Status != StatusRunning {
			return fmt.Errorf("%w: paused is only legal from %q, current status is %q", ErrInvalidTransition, StatusRunning, state.Status)
		}
	case KindResumed:
		if state.Status != StatusPaused {
			return fmt.Errorf("%w: resumed is only legal from %q, current status is %q", ErrInvalidTransition, StatusPaused, state.Status)
		}
	case KindStageRecorded:
		if state.Status != StatusRunning {
			return fmt.Errorf("%w: stage_recorded is only legal while %q, current status is %q", ErrInvalidTransition, StatusRunning, state.Status)
		}
		if state.hasStage(next.Stage) {
			return fmt.Errorf("%w: %q was already recorded", ErrDuplicateStage, next.Stage)
		}
		if len(state.Stages) >= MaxStages {
			return fmt.Errorf("%w: limit is %d", ErrTooManyStages, MaxStages)
		}
	case KindVerified:
		// Legal from any non-closed state; StatusClosed already rejected above.
	case KindClosed:
		switch Outcome(next.Outcome) {
		case OutcomeCompleted:
			if state.lastEventKind != KindVerified {
				return fmt.Errorf("%w: last event was %q", ErrCompletedRequiresVerify, state.lastEventKind)
			}
		case OutcomeAbandoned:
			// Legal from any non-closed state.
		default:
			return fmt.Errorf("%w: unknown close outcome %q", ErrInvalidTransition, next.Outcome)
		}
	default:
		return fmt.Errorf("%w: unknown kind %q", ErrInvalidTransition, next.Kind)
	}
	return nil
}

// Replay derives the current State of a workflow by validating and applying
// events in order, starting from the empty state. It calls
// WorkflowEvent.Validate and CheckTransition for every event; the first
// failure stops replay and is returned wrapped with the event's index.
// Replay does not check hash-chain linkage (prev_digest) or cross-event
// identifier consistency; call VerifyEvents first when events come from an
// untrusted source.
func Replay(events []WorkflowEvent) (State, error) {
	if len(events) == 0 {
		return State{}, fmt.Errorf("workflow: replay: at least one event is required")
	}
	var state State
	for i, e := range events {
		if err := e.Validate(); err != nil {
			return State{}, fmt.Errorf("workflow: replay: event %d: %w", i, err)
		}
		if err := CheckTransition(state, e); err != nil {
			return State{}, fmt.Errorf("workflow: replay: event %d: %w", i, err)
		}
		state = applyEvent(state, e)
	}
	return state, nil
}

// applyEvent returns the State that results from applying e to state. It
// assumes CheckTransition already accepted e.
func applyEvent(state State, e WorkflowEvent) State {
	switch e.Kind {
	case KindCreated:
		state.Status = StatusCreated
		state.GoalID = e.GoalID
		state.GoalDigest = e.GoalDigest
		state.Profile = e.Profile
		state.RoleChainID = e.RoleChainID
		state.RoleChainHead = e.RoleChainHead
		state.Stages = []string{}
		state.LastVerifiedSeq = -1
	case KindStarted, KindResumed:
		state.Status = StatusRunning
	case KindPaused:
		state.Status = StatusPaused
	case KindStageRecorded:
		stages := make([]string, len(state.Stages), len(state.Stages)+1)
		copy(stages, state.Stages)
		state.Stages = append(stages, e.Stage)

		stageSet := make(map[string]bool, len(state.stageSet)+1)
		for stage := range state.stageSet {
			stageSet[stage] = true
		}
		stageSet[e.Stage] = true
		state.stageSet = stageSet
	case KindVerified:
		state.LastVerifiedSeq = e.Seq
	case KindClosed:
		state.Status = StatusClosed
		state.CloseOutcome = Outcome(e.Outcome)
		state.CloseReason = e.Reason
	}
	state.lastEventKind = e.Kind
	return state
}
