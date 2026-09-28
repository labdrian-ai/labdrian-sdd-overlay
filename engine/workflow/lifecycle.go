package workflow

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/goal"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/memoryscope"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/roles"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflowprofile"
)

// Sentinel errors returned by Lifecycle operations. Wrap with %w so callers
// can distinguish the failure with errors.Is.
var (
	// ErrWorkflowNotOwned is returned by every operation except Create when
	// the on-disk state is not ClassificationOwned: either it does not
	// exist yet (create it first) or it is foreign, malformed, drifted, or
	// unavailable (the Store never overwrites those; see Store.Load).
	ErrWorkflowNotOwned = errors.New("workflow lifecycle: workflow state is not owned; create it first, or its on-disk state is foreign, malformed, drifted, or unavailable")
	// ErrCreateRefused is returned by Create when on-disk state already
	// exists for this workflow (owned or otherwise); a workflow may be
	// created only once (see CheckTransition's ErrAlreadyCreated).
	ErrCreateRefused = errors.New("workflow lifecycle: cannot create: on-disk state already exists for this workflow")
	// ErrStageOutOfOrder is returned by RecordStage when stage is not the
	// exact next stage in the Workflow Profile's declared order (including
	// when stage is not one of the Profile's declared stages at all, or
	// when the Profile has no more undeclared stages left to record).
	ErrStageOutOfOrder = errors.New("workflow lifecycle: stage is not the profile's next declared stage")
	// ErrChainInvalid is returned by Verify when the workflow's own event
	// hash chain does not verify. Store.Load already refuses to classify a
	// broken chain as owned, so this should be unreachable in practice; it
	// exists as a defensive, named failure rather than a silent assumption.
	ErrChainInvalid = errors.New("workflow lifecycle: verify: event hash chain does not verify")
	// ErrProfileInvalid is returned by Verify when the workflow's recorded
	// Workflow Profile name no longer resolves against workflowprofile.
	ErrProfileInvalid = errors.New("workflow lifecycle: verify: recorded profile no longer resolves")
	// ErrStageOrderInvalid is returned by Verify when the workflow's
	// recorded stages no longer form a valid prefix of the Profile's
	// declared stage order.
	ErrStageOrderInvalid = errors.New("workflow lifecycle: verify: recorded stages no longer respect the profile's declared order")
	// ErrGoalDigestMismatch is returned by Verify when re-reading the Goal
	// produces a digest different from the one recorded at creation.
	ErrGoalDigestMismatch = errors.New("workflow lifecycle: verify: goal digest no longer matches the digest recorded at creation")
	// ErrRoleChainInvalid is returned by Create and Verify when a
	// referenced role chain has no records or fails roles.VerifyChain.
	ErrRoleChainInvalid = errors.New("workflow lifecycle: referenced role chain does not verify")
)

// gentleAIReviewCapability is the capability name recorded when a Workflow
// Profile's review policy relies on Gentle AI's native review (receipt-
// driven development, "RDD"). See profileReliesOnGentleReview.
const gentleAIReviewCapability = "gentle-ai-review"

// GoalReader loads the current bytes of one Goal, by project and goal id, so
// Verify can re-check the Goal's digest against the one recorded at
// creation. It is caller-supplied: this package never reads a Goal file (or
// any file) itself, and it runs no subprocess. A typical implementation
// reads a Goal v2 file from disk and calls goal.Parse; a test may use an
// in-memory fake.
type GoalReader interface {
	LoadGoal(projectID, goalID string) (goal.Goal, error)
}

// RoleChainReader loads one role handoff chain's records, by project, goal,
// and chain id. Its signature exactly matches
// (roles.ChainStore).LoadChain, so a real roles.ChainStore satisfies this
// interface directly with no wrapper; a test may use an in-memory fake.
type RoleChainReader interface {
	LoadChain(projectID, goalID, chainID string) ([]roles.ChainRecord, error)
}

// DependencyProber reports, for each named capability, whether it is
// currently available. It must not block on or assume any particular
// capability; see UnavailableProber for the safe default. A capability name
// is an opaque string this package defines (memory sources are named
// "memory:<source>"; native review is named by gentleAIReviewCapability).
type DependencyProber interface {
	// Probe returns one Observation per entry in capabilities, in the same
	// order and the same length. It must not run a subprocess or make a
	// network call (see UnavailableProber's doc comment for the one
	// documented exception a future prober may choose to take).
	Probe(capabilities []string) []Observation
}

// UnavailableProber is the safe default DependencyProber: it reports every
// requested capability as unavailable, without running a subprocess or
// making a network call. A future prober is free to use an
// exec.LookPath-based PATH check (never a network call or another
// subprocess) as long as that choice is documented on the prober itself;
// this default takes the simplest, always-safe option of reporting nothing
// as positively confirmed.
type UnavailableProber struct{}

// Probe implements DependencyProber.
func (UnavailableProber) Probe(capabilities []string) []Observation {
	observations := make([]Observation, len(capabilities))
	for i, capability := range capabilities {
		observations[i] = Observation{
			Capability: capability,
			Status:     ObservationUnavailable,
			Detail:     "default prober: availability cannot be positively confirmed without a subprocess or network call",
		}
	}
	return observations
}

// Lifecycle performs the standalone workflow lifecycle operations (create,
// start, pause, resume, stage recording, structural verify, close) over one
// Store, with every side-effecting dependency injected for testability: a
// clock, static caller-supplied provenance (this package never runs git or
// any other subprocess), a GoalReader, a RoleChainReader, and a
// DependencyProber. Every operation appends at most one event through
// Store.Append and returns the resulting State; a rejected operation
// appends nothing.
type Lifecycle struct {
	store      Store
	clock      func() time.Time
	provenance Provenance
	goals      GoalReader
	chains     RoleChainReader
	prober     DependencyProber
}

// NewLifecycle builds a Lifecycle. clock, goals, and chains must not be
// nil; prober may be nil, in which case UnavailableProber is used.
// provenance is validated once here (the same shape check WorkflowEvent's
// own Validate applies) since it is reused, unchanged, on every event this
// Lifecycle appends.
func NewLifecycle(store Store, clock func() time.Time, provenance Provenance, goals GoalReader, chains RoleChainReader, prober DependencyProber) (Lifecycle, error) {
	if clock == nil {
		return Lifecycle{}, fmt.Errorf("workflow lifecycle: clock must not be nil")
	}
	if goals == nil {
		return Lifecycle{}, fmt.Errorf("workflow lifecycle: goal reader must not be nil")
	}
	if chains == nil {
		return Lifecycle{}, fmt.Errorf("workflow lifecycle: role chain reader must not be nil")
	}
	if err := provenance.validate(); err != nil {
		return Lifecycle{}, fmt.Errorf("workflow lifecycle: %w", err)
	}
	if prober == nil {
		prober = UnavailableProber{}
	}
	return Lifecycle{store: store, clock: clock, provenance: provenance, goals: goals, chains: chains, prober: prober}, nil
}

// loadOwned loads the workflow and requires it to be ClassificationOwned;
// every operation except Create calls this first.
func (l Lifecycle) loadOwned(projectID, workflowID string) (Loaded, error) {
	loaded, err := l.store.Load(projectID, workflowID)
	if err != nil {
		return Loaded{}, err
	}
	if loaded.Classification != ClassificationOwned {
		return Loaded{}, fmt.Errorf("%w: on-disk classification is %q", ErrWorkflowNotOwned, loaded.Classification)
	}
	return loaded, nil
}

// baseEvent builds the fields common to every event this Lifecycle appends:
// version, ids, seq/prev_digest linkage derived from loaded, kind, the
// current timestamp, this Lifecycle's static provenance, and dependency
// observations for profileName. The caller fills in any kind-specific
// payload fields before appending.
func (l Lifecycle) baseEvent(loaded Loaded, projectID, workflowID string, kind Kind, profileName string) (WorkflowEvent, error) {
	var seq int
	var prevDigest string
	if n := len(loaded.Events); n > 0 {
		seq = n
		digest, err := EventDigest(loaded.Events[n-1])
		if err != nil {
			return WorkflowEvent{}, fmt.Errorf("workflow lifecycle: %w", err)
		}
		prevDigest = digest
	}
	observations, err := l.observationsFor(profileName)
	if err != nil {
		return WorkflowEvent{}, err
	}
	return WorkflowEvent{
		Version:      EventVersion,
		WorkflowID:   workflowID,
		ProjectID:    projectID,
		Seq:          seq,
		PrevDigest:   prevDigest,
		Kind:         kind,
		At:           l.clock().UTC().Format(time.RFC3339),
		Provenance:   l.provenance,
		Observations: observations,
	}, nil
}

// commit appends event and, on success, returns the State that results from
// applying it to loaded.State. It appends nothing and returns loaded's
// unmodified failure path when Append refuses event.
func (l Lifecycle) commit(projectID, workflowID string, loaded Loaded, event WorkflowEvent) (State, error) {
	if err := l.store.Append(projectID, workflowID, event); err != nil {
		return State{}, err
	}
	return applyEvent(loaded.State, event), nil
}

// observationsFor probes, and returns as Observations, every dependency a
// Workflow Profile declares: its default memory sources
// (memoryscope.DefaultFor), named "memory:<source>", plus
// gentleAIReviewCapability when profileReliesOnGentleReview. Every
// dependency this Lifecycle does not positively confirm through l.prober is
// recorded unavailable; it never blocks the operation and is never
// recorded as available on its own authority (see DependencyProber).
func (l Lifecycle) observationsFor(profileName string) ([]Observation, error) {
	profile, err := workflowprofile.Resolve(profileName)
	if err != nil {
		return nil, fmt.Errorf("workflow lifecycle: %w", err)
	}
	directive, err := memoryscope.DefaultFor(profile.Name)
	if err != nil {
		return nil, fmt.Errorf("workflow lifecycle: %w", err)
	}
	capabilities := make([]string, 0, len(directive.Sources)+1)
	for _, source := range directive.Sources {
		capabilities = append(capabilities, "memory:"+string(source))
	}
	if profileReliesOnGentleReview(profile) {
		capabilities = append(capabilities, gentleAIReviewCapability)
	}
	observed := l.prober.Probe(capabilities)
	if len(observed) != len(capabilities) {
		return nil, fmt.Errorf("workflow lifecycle: dependency prober returned %d observations for %d capabilities", len(observed), len(capabilities))
	}
	result := make([]Observation, len(observed))
	copy(result, observed)
	return result, nil
}

// profileReliesOnGentleReview reports whether profile's declared
// review_policy (engine/workflowprofile.go) relies on Gentle AI's native
// review (receipt-driven development, "RDD"), so gentleAIReviewCapability
// should be probed for it. It is derived from the policy's own text, not a
// hardcoded profile list: a policy that names "RDD" without explicitly
// disclaiming Gentle relies on it; standalone-minimal's policy states "no
// Gentle/RDD dependency" and is excluded.
//
// PRODUCT DECISION FLAG: this derivation selects four of the five built-in
// profiles (odd, sdd, maintenance, incident-recovery all mention "RDD"
// without disclaiming it; only standalone-minimal disclaims it) rather than
// just the two profiles (odd, sdd) named as examples in this feature's task
// description. This was a genuine ambiguity in the task text ("derive from
// the profile data, do not hardcode") versus its parenthetical example; the
// derivation-from-data instruction was followed literally. If only odd and
// sdd should gate this capability, that is a narrower rule than "contains
// RDD" and should be made explicit (for example, an exact profile allowlist)
// rather than derived from substring matching.
func profileReliesOnGentleReview(profile workflowprofile.WorkflowProfile) bool {
	if strings.Contains(profile.ReviewPolicy, "no Gentle") {
		return false
	}
	return strings.Contains(profile.ReviewPolicy, "RDD")
}

// goalDigest returns the SHA-256 hex digest of g's canonical Marshal
// encoding. engine/goal defines no digest helper of its own, so this
// package defines one clearly: Marshal already validates g and produces a
// deterministic, field-ordered encoding, so hashing its bytes gives a
// digest that changes if and only if g's contents change.
func goalDigest(g goal.Goal) (string, error) {
	data, err := g.Marshal()
	if err != nil {
		return "", fmt.Errorf("workflow lifecycle: goal digest: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// roleChainDigest returns the SHA-256 hex digest of records' last record's
// raw bytes: the same digest the next record in that chain would chain to
// via prev_sha256 (see roles.VerifyChain), reused here as the stable digest
// this package's own Checked.role_chain_digest records. records must be
// non-empty.
func roleChainDigest(records []roles.ChainRecord) string {
	sum := sha256.Sum256(records[len(records)-1].Raw)
	return hex.EncodeToString(sum[:])
}

// stageOrderPrefix checks that stages is exactly a prefix of profile's
// declared stage order: stages[i] must equal profile.Stages[i].Name for
// every i, and there must be no more recorded stages than the profile
// declares. See RecordStage for how new stages are admitted one at a time
// against this same order.
func stageOrderPrefix(profile workflowprofile.WorkflowProfile, stages []string) error {
	if len(stages) > len(profile.Stages) {
		return fmt.Errorf("recorded %d stages exceeds profile %q's %d declared stages", len(stages), profile.Name, len(profile.Stages))
	}
	for i, stage := range stages {
		if profile.Stages[i].Name != stage {
			return fmt.Errorf("recorded stage %d is %q, want %q per profile %q's declared order", i, stage, profile.Stages[i].Name, profile.Name)
		}
	}
	return nil
}

// Create validates g (a Goal v2) and profileName, optionally verifies a
// referenced role chain, and appends the workflow's created event: data
// only, binding goal_id, goal_digest, profile, and (if given) role_chain_id.
// It fails, appending nothing, when g is invalid, profileName is unknown,
// roleChainID is non-blank but the chain has no records or fails
// roles.VerifyChain, or on-disk state already exists for this workflow
// (ErrCreateRefused).
func (l Lifecycle) Create(projectID, workflowID string, g goal.Goal, profileName, roleChainID string) (State, error) {
	if err := g.Validate(); err != nil {
		return State{}, fmt.Errorf("workflow lifecycle: create: invalid goal: %w", err)
	}
	if _, err := workflowprofile.Resolve(profileName); err != nil {
		return State{}, fmt.Errorf("workflow lifecycle: create: %w", err)
	}
	if roleChainID != "" {
		if err := ValidateIdentifier("role_chain_id", roleChainID); err != nil {
			return State{}, fmt.Errorf("workflow lifecycle: create: %w", err)
		}
		records, err := l.chains.LoadChain(projectID, g.GoalID, roleChainID)
		if err != nil {
			return State{}, fmt.Errorf("workflow lifecycle: create: load role chain: %w", err)
		}
		if len(records) == 0 {
			return State{}, fmt.Errorf("%w: role chain %q has no records", ErrRoleChainInvalid, roleChainID)
		}
		if err := roles.VerifyChain(records); err != nil {
			return State{}, fmt.Errorf("%w: %v", ErrRoleChainInvalid, err)
		}
	}
	digest, err := goalDigest(g)
	if err != nil {
		return State{}, fmt.Errorf("workflow lifecycle: create: %w", err)
	}

	loaded, err := l.store.Load(projectID, workflowID)
	if err != nil {
		return State{}, err
	}
	if loaded.Classification != ClassificationAbsent {
		return State{}, fmt.Errorf("%w: on-disk classification is %q", ErrCreateRefused, loaded.Classification)
	}

	event, err := l.baseEvent(loaded, projectID, workflowID, KindCreated, profileName)
	if err != nil {
		return State{}, err
	}
	event.GoalID = g.GoalID
	event.GoalDigest = digest
	event.Profile = profileName
	event.RoleChainID = roleChainID

	return l.commit(projectID, workflowID, loaded, event)
}

// Start appends a started event; legal only from status created (see
// CheckTransition).
func (l Lifecycle) Start(projectID, workflowID string) (State, error) {
	loaded, err := l.loadOwned(projectID, workflowID)
	if err != nil {
		return State{}, err
	}
	event, err := l.baseEvent(loaded, projectID, workflowID, KindStarted, loaded.State.Profile)
	if err != nil {
		return State{}, err
	}
	return l.commit(projectID, workflowID, loaded, event)
}

// Pause appends a paused event; legal only from status running.
func (l Lifecycle) Pause(projectID, workflowID string) (State, error) {
	loaded, err := l.loadOwned(projectID, workflowID)
	if err != nil {
		return State{}, err
	}
	event, err := l.baseEvent(loaded, projectID, workflowID, KindPaused, loaded.State.Profile)
	if err != nil {
		return State{}, err
	}
	return l.commit(projectID, workflowID, loaded, event)
}

// Resume appends a resumed event; legal only from status paused.
func (l Lifecycle) Resume(projectID, workflowID string) (State, error) {
	loaded, err := l.loadOwned(projectID, workflowID)
	if err != nil {
		return State{}, err
	}
	event, err := l.baseEvent(loaded, projectID, workflowID, KindResumed, loaded.State.Profile)
	if err != nil {
		return State{}, err
	}
	return l.commit(projectID, workflowID, loaded, event)
}

// RecordStage appends a stage_recorded event for stage. It rejects, without
// appending anything, a stage name that is not exactly the Workflow
// Profile's next declared stage: the stage immediately after the last one
// already recorded, in the profile's declared stage order (see
// stageOrderPrefix). This also rejects a stage the profile never declares
// (it can never be "next") and a workflow whose profile has already
// recorded every declared stage.
func (l Lifecycle) RecordStage(projectID, workflowID, stage string) (State, error) {
	loaded, err := l.loadOwned(projectID, workflowID)
	if err != nil {
		return State{}, err
	}
	profile, err := workflowprofile.Resolve(loaded.State.Profile)
	if err != nil {
		return State{}, fmt.Errorf("workflow lifecycle: record stage: %w", err)
	}
	if len(loaded.State.Stages) >= len(profile.Stages) {
		return State{}, fmt.Errorf("%w: profile %q's declared stages are all recorded", ErrStageOutOfOrder, profile.Name)
	}
	want := profile.Stages[len(loaded.State.Stages)].Name
	if stage != want {
		return State{}, fmt.Errorf("%w: got %q, want %q (profile %q's next declared stage)", ErrStageOutOfOrder, stage, want, profile.Name)
	}
	event, err := l.baseEvent(loaded, projectID, workflowID, KindStageRecorded, profile.Name)
	if err != nil {
		return State{}, err
	}
	event.Stage = stage
	return l.commit(projectID, workflowID, loaded, event)
}

// Verify performs a structural-only verification (no execution): the
// event hash chain still verifies, the Workflow Profile still resolves, the
// recorded stages still respect the Profile's declared order, re-reading
// the Goal (via GoalReader) still produces the digest recorded at creation,
// and, if a role chain is referenced, it still has records and passes
// roles.VerifyChain. On success it appends one verified event recording the
// digests it checked (Checked.ChainDigest is the digest of the last event
// before this one); on any failure it appends nothing and returns a named
// error identifying which check failed.
func (l Lifecycle) Verify(projectID, workflowID string) (State, error) {
	loaded, err := l.loadOwned(projectID, workflowID)
	if err != nil {
		return State{}, err
	}
	state := loaded.State

	if err := VerifyEvents(loaded.Events); err != nil {
		return State{}, fmt.Errorf("%w: %v", ErrChainInvalid, err)
	}
	profile, err := workflowprofile.Resolve(state.Profile)
	if err != nil {
		return State{}, fmt.Errorf("%w: %v", ErrProfileInvalid, err)
	}
	if err := stageOrderPrefix(profile, state.Stages); err != nil {
		return State{}, fmt.Errorf("%w: %v", ErrStageOrderInvalid, err)
	}
	g, err := l.goals.LoadGoal(projectID, state.GoalID)
	if err != nil {
		return State{}, fmt.Errorf("workflow lifecycle: verify: load goal: %w", err)
	}
	if err := g.Validate(); err != nil {
		return State{}, fmt.Errorf("workflow lifecycle: verify: invalid goal: %w", err)
	}
	digest, err := goalDigest(g)
	if err != nil {
		return State{}, fmt.Errorf("workflow lifecycle: verify: %w", err)
	}
	if digest != state.GoalDigest {
		return State{}, fmt.Errorf("%w: goal %q digest is now %s, recorded %s", ErrGoalDigestMismatch, state.GoalID, digest, state.GoalDigest)
	}

	var roleChainDigestValue string
	if state.RoleChainID != "" {
		records, err := l.chains.LoadChain(projectID, state.GoalID, state.RoleChainID)
		if err != nil {
			return State{}, fmt.Errorf("workflow lifecycle: verify: load role chain: %w", err)
		}
		if len(records) == 0 {
			return State{}, fmt.Errorf("%w: role chain %q has no records", ErrRoleChainInvalid, state.RoleChainID)
		}
		if err := roles.VerifyChain(records); err != nil {
			return State{}, fmt.Errorf("%w: %v", ErrRoleChainInvalid, err)
		}
		roleChainDigestValue = roleChainDigest(records)
	}

	chainDigestValue, err := EventDigest(loaded.Events[len(loaded.Events)-1])
	if err != nil {
		return State{}, fmt.Errorf("workflow lifecycle: verify: %w", err)
	}

	event, err := l.baseEvent(loaded, projectID, workflowID, KindVerified, profile.Name)
	if err != nil {
		return State{}, err
	}
	event.Checked = &Checked{
		ChainDigest:     chainDigestValue,
		GoalDigest:      digest,
		Profile:         profile.Name,
		RoleChainDigest: roleChainDigestValue,
	}
	return l.commit(projectID, workflowID, loaded, event)
}

// Close appends a closed event with outcome and reason. Outcome completed
// is legal only when the immediately preceding event is a verified event
// (see CheckTransition's ErrCompletedRequiresVerify); outcome abandoned is
// legal from any non-closed state and requires a non-blank reason (see
// WorkflowEvent.Validate).
func (l Lifecycle) Close(projectID, workflowID string, outcome Outcome, reason string) (State, error) {
	loaded, err := l.loadOwned(projectID, workflowID)
	if err != nil {
		return State{}, err
	}
	event, err := l.baseEvent(loaded, projectID, workflowID, KindClosed, loaded.State.Profile)
	if err != nil {
		return State{}, err
	}
	event.Outcome = string(outcome)
	event.Reason = reason
	return l.commit(projectID, workflowID, loaded, event)
}

// Status is a read-only report of one workflow's on-disk classification and
// (when owned) its replayed State. It never appends anything.
func (l Lifecycle) Status(projectID, workflowID string) (Classification, State, error) {
	loaded, err := l.store.Load(projectID, workflowID)
	if err != nil {
		return "", State{}, err
	}
	return loaded.Classification, loaded.State, nil
}
