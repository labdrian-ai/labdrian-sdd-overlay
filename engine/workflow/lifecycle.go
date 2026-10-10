package workflow

import (
	"context"
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
	// unavailable (the event log never overwrites those; see ClassifyLog).
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
	// hash chain does not verify. ClassifyLog already refuses to classify a
	// broken chain as owned, so this should be unreachable in practice; it
	// exists as a defensive, named failure rather than a silent assumption.
	ErrChainInvalid = errors.New("workflow lifecycle: verify: event hash chain does not verify")
	// ErrProfileInvalid is returned by Verify when the workflow's recorded
	// Workflow Profile name no longer resolves against the catalog. Only a
	// workflow of version 1 can fail so: one of version 2 carries its profile.
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
// driven development, "RDD"). See workflowprofile.WorkflowProfile.ReliesOnGentleReview.
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
// currently available. It should return promptly and honor ctx's deadline;
// see UnavailableProber for the safe default. A capability name is an
// opaque string this package defines (memory sources are named
// "memory:<source>"; native review is named by gentleAIReviewCapability).
//
// observationsFor applies its own bounded deadline (see
// DefaultDependencyProbeTimeout) around every call, independent of whether the
// prober itself honors ctx: a prober that ignores ctx and blocks forever
// still never blocks the calling lifecycle operation past that deadline
// (see Lifecycle.probe), though its goroutine may leak for the remainder of
// the process's life. That is an accepted trade-off for a caller-supplied
// prober that violates its documented contract; Go has no way to force-stop
// a goroutine that never checks ctx.Done().
type DependencyProber interface {
	// Probe returns one Observation per entry in capabilities, in the same
	// order and the same length, or a non-nil error. It must not run a
	// subprocess or make a network call (see UnavailableProber's doc
	// comment for the one documented exception a future prober may choose
	// to take). A returned error, or ctx's deadline expiring first, is
	// treated by observationsFor exactly like every capability being
	// unavailable; Probe need not synthesize Observations for that case
	// itself.
	Probe(ctx context.Context, capabilities []string) ([]Observation, error)
}

// DefaultDependencyProbeTimeout is the default bound observationsFor applies to a
// single DependencyProber.Probe call. 5 seconds is generous for any prober
// that only inspects local state (a PATH lookup, a socket, a config file)
// while still keeping every lifecycle-mutating operation (Create, Start,
// Pause, Resume, RecordStage, Verify, Close) responsive when a prober is
// slow, hung, or misbehaving. NewLifecycle sets this as Lifecycle.probeTimeout;
// tests in this package may lower it to keep a deliberately slow prober test
// fast.
const DefaultDependencyProbeTimeout = 5 * time.Second

// UnavailableProber is the safe default DependencyProber: it reports every
// requested capability as unavailable, without running a subprocess or
// making a network call. A future prober is free to use an
// exec.LookPath-based PATH check (never a network call or another
// subprocess) as long as that choice is documented on the prober itself;
// this default takes the simplest, always-safe option of reporting nothing
// as positively confirmed.
type UnavailableProber struct{}

// Probe implements DependencyProber. It ignores ctx: it never blocks, so it
// has no deadline to honor.
func (UnavailableProber) Probe(_ context.Context, capabilities []string) ([]Observation, error) {
	return unavailableObservations(capabilities, "default prober: availability cannot be positively confirmed without a subprocess or network call"), nil
}

// unavailableObservations builds one Observation per capability, every one
// marked unavailable with detail (truncated to MaxObservationDetailLength
// runes so a verbose prober error or ctx.Err() can never itself make the
// resulting event fail Validate).
func unavailableObservations(capabilities []string, detail string) []Observation {
	if len([]rune(detail)) > MaxObservationDetailLength {
		detail = string([]rune(detail)[:MaxObservationDetailLength])
	}
	observations := make([]Observation, len(capabilities))
	for i, capability := range capabilities {
		observations[i] = Observation{Capability: capability, Status: ObservationUnavailable, Detail: detail}
	}
	return observations
}

// Lifecycle performs the standalone workflow lifecycle operations (create,
// start, pause, resume, stage recording, structural verify, close) over one
// EventLog, with every side-effecting dependency injected: the log itself, the
// ProfileCatalog every profile is resolved from, a clock, static
// caller-supplied provenance (this package never runs git or any other
// subprocess), a GoalReader, a RoleChainReader, and a DependencyProber. Every
// operation appends at most one event through EventLog.Append and returns the
// resulting State; a rejected operation appends nothing.
type Lifecycle struct {
	log        EventLog
	profiles   ProfileCatalog
	clock      func() time.Time
	provenance Provenance
	goals      GoalReader
	chains     RoleChainReader
	prober     DependencyProber
	// probeTimeout bounds a single DependencyProber.Probe call; it is
	// DefaultDependencyProbeTimeout outside tests.
	probeTimeout time.Duration
	// degraded is called, if non-nil, exactly when observationsFor could
	// not get a usable answer from l.prober itself (a Probe error, a
	// probeTimeout expiry, or a returned-observation count mismatch) -- as
	// distinct from the prober succeeding and simply reporting a
	// capability unavailable. See WithDegradedHook.
	degraded DegradedHook
}

// DegradedHook is notified, with a short human-readable detail, whenever a
// dependency probe degrades: the configured DependencyProber returned an
// error, missed its deadline, or returned the wrong number of
// observations. The lifecycle operation still succeeds either way (every
// affected capability is recorded unavailable; see DependencyProber and
// observationsFor), so this hook exists only so a caller such as the CLI
// can surface the degradation somewhere a person will see it (for example,
// one line on stderr) instead of it being visible only by reading the
// "unavailable" detail back out of the appended event later.
type DegradedHook func(detail string)

// WithDegradedHook returns a copy of l with hook wired as its
// DegradedHook (nil clears it). It is a copy-returning setter rather than
// a NewLifecycle parameter so every existing caller and test that builds a
// Lifecycle without a hook keeps compiling and behaving unchanged.
func (l Lifecycle) WithDegradedHook(hook DegradedHook) Lifecycle {
	l.degraded = hook
	return l
}

// notifyDegraded calls l.degraded with detail when a hook is set; it is a
// no-op otherwise.
func (l Lifecycle) notifyDegraded(detail string) {
	if l.degraded != nil {
		l.degraded(detail)
	}
}

// NewLifecycle builds a Lifecycle. log, profiles, clock, goals, and chains
// must not be nil; prober may be nil, in which case UnavailableProber is used.
// provenance is validated once here (the same shape check WorkflowEvent's
// own Validate applies) since it is reused, unchanged, on every event this
// Lifecycle appends.
func NewLifecycle(log EventLog, profiles ProfileCatalog, clock func() time.Time, provenance Provenance, goals GoalReader, chains RoleChainReader, prober DependencyProber) (Lifecycle, error) {
	if log == nil {
		return Lifecycle{}, fmt.Errorf("workflow lifecycle: event log must not be nil")
	}
	if profiles == nil {
		return Lifecycle{}, fmt.Errorf("workflow lifecycle: profile catalog must not be nil")
	}
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
	return Lifecycle{log: log, profiles: profiles, clock: clock, provenance: provenance, goals: goals, chains: chains, prober: prober, probeTimeout: DefaultDependencyProbeTimeout}, nil
}

// loadOwned loads the workflow and requires it to be ClassificationOwned;
// every operation except Create calls this first.
func (l Lifecycle) loadOwned(projectID, workflowID string) (Loaded, error) {
	loaded, err := l.log.Load(projectID, workflowID)
	if err != nil {
		return Loaded{}, err
	}
	if loaded.Classification != ClassificationOwned {
		return Loaded{}, fmt.Errorf("%w: on-disk classification is %q", ErrWorkflowNotOwned, loaded.Classification)
	}
	return loaded, nil
}

// ProfileOf is the Workflow Profile a workflow goes on from. A workflow whose log recorded a
// snapshot of its profile when it was created (version 2) goes on from that snapshot, and the
// catalog is not asked: the workflow is self-contained, so it does not change when the catalog does,
// and it does not stop when the catalog drops the profile. A workflow whose log recorded the name
// alone (version 1) goes on from the profile the catalog gives for that name, as it always did, and
// the catalog's refusal is the error.
func ProfileOf(state State, catalog ProfileCatalog) (workflowprofile.WorkflowProfile, error) {
	if state.ProfileSnapshot != nil {
		return state.ProfileSnapshot.Profile(), nil
	}
	return catalog.Resolve(state.Profile)
}

// profileOf is the profile of the workflow in state, from this Lifecycle's catalog where the log has
// no snapshot of it (see ProfileOf).
func (l Lifecycle) profileOf(state State) (workflowprofile.WorkflowProfile, error) {
	return ProfileOf(state, l.profiles)
}

// baseEvent builds the fields common to every event this Lifecycle appends:
// version, ids, seq/prev_digest linkage derived from loaded, kind, the
// current timestamp, this Lifecycle's static provenance, and dependency
// observations for profile. The caller fills in any kind-specific
// payload fields before appending.
func (l Lifecycle) baseEvent(loaded Loaded, projectID, workflowID string, kind Kind, profile workflowprofile.WorkflowProfile) (WorkflowEvent, error) {
	observations, err := l.observationsFor(profile)
	if err != nil {
		return WorkflowEvent{}, err
	}
	return l.eventWith(loaded, projectID, workflowID, kind, observations)
}

// stateEvent is baseEvent for an event of the workflow loaded holds, which goes on from the profile
// of that workflow (see ProfileOf).
func (l Lifecycle) stateEvent(loaded Loaded, projectID, workflowID string, kind Kind) (WorkflowEvent, error) {
	profile, err := l.profileOf(loaded.State)
	if err != nil {
		return WorkflowEvent{}, fmt.Errorf("workflow lifecycle: %w", err)
	}
	return l.baseEvent(loaded, projectID, workflowID, kind, profile)
}

// eventWith builds the next event of kind for loaded's chain with the given
// observations. Its version is the log's, that of the created event, and the
// current one for the created event of a workflow that has no log yet.
func (l Lifecycle) eventWith(loaded Loaded, projectID, workflowID string, kind Kind, observations []Observation) (WorkflowEvent, error) {
	var seq int
	var prevDigest string
	version := EventVersion
	if n := len(loaded.Events); n > 0 {
		seq = n
		version = loaded.State.Version
		digest, err := EventDigest(loaded.Events[n-1])
		if err != nil {
			return WorkflowEvent{}, fmt.Errorf("workflow lifecycle: %w", err)
		}
		prevDigest = digest
	}
	return WorkflowEvent{
		Version:      version,
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
// applying it to loaded.State. When Append refuses event, nothing is
// appended and commit returns Append's error alongside a zero-value State;
// every caller in this file checks the error first and never reads that
// zero-value State.
func (l Lifecycle) commit(projectID, workflowID string, loaded Loaded, event WorkflowEvent) (State, error) {
	if err := l.log.Append(projectID, workflowID, event); err != nil {
		return State{}, err
	}
	return applyEvent(loaded.State, event), nil
}

// observationsFor probes, and returns as Observations, every dependency a
// Workflow Profile declares: its default memory sources
// (memoryscope.DefaultFor), named "memory:<source>", plus
// gentleAIReviewCapability when the profile ReliesOnGentleReview. Both are
// typed data of the profile the catalog returns, never a function of its name. Every
// dependency this Lifecycle does not positively confirm through l.prober is
// recorded unavailable; it never blocks the operation and is never
// recorded as available on its own authority (see DependencyProber). A
// prober that returns an error, returns the wrong number of observations,
// or does not return within l.probeTimeout is treated the same way: every
// requested capability is recorded unavailable, with a detail explaining
// why (see l.probe).
func (l Lifecycle) observationsFor(profile workflowprofile.WorkflowProfile) ([]Observation, error) {
	directive, err := memoryscope.DefaultFor(profile)
	if err != nil {
		return nil, fmt.Errorf("workflow lifecycle: %w", err)
	}
	capabilities := make([]string, 0, len(directive.Sources)+1)
	for _, source := range directive.Sources {
		capabilities = append(capabilities, "memory:"+string(source))
	}
	if profile.ReliesOnGentleReview {
		capabilities = append(capabilities, gentleAIReviewCapability)
	}

	observed, err := l.probe(capabilities)
	if err != nil {
		l.notifyDegraded(err.Error())
		return unavailableObservations(capabilities, err.Error()), nil
	}
	if len(observed) != len(capabilities) {
		detail := fmt.Sprintf("dependency prober returned %d observations for %d capabilities", len(observed), len(capabilities))
		l.notifyDegraded(detail)
		return unavailableObservations(capabilities, detail), nil
	}
	result := make([]Observation, len(observed))
	copy(result, observed)
	return result, nil
}

// probe calls l.prober.Probe with a deadline of l.probeTimeout, in a
// separate goroutine so that a prober which never checks its context and
// blocks forever still cannot block the caller past that deadline: probe
// returns as soon as either the prober's own goroutine finishes or the
// deadline expires, whichever comes first. A prober that ignores ctx and
// never returns leaks exactly one goroutine per such probe call, for the
// remainder of the process's life (nothing ever collects it, since Go has
// no way to force-stop a goroutine that never checks ctx.Done()); see
// DependencyProber's doc comment for why that is an accepted trade-off for
// a caller-supplied prober violating its documented contract. In practice
// this is bounded and cheap: the CLI (engine/cmd) is a short-lived process
// that calls probe at most once per invocation and exits immediately after,
// so any leaked goroutine dies with the process before it could
// accumulate; only a long-lived embedder of this package driven by a
// non-compliant prober repeatedly would see them add up.
func (l Lifecycle) probe(capabilities []string) ([]Observation, error) {
	ctx, cancel := context.WithTimeout(context.Background(), l.probeTimeout)
	defer cancel()

	type result struct {
		observations []Observation
		err          error
	}
	done := make(chan result, 1)
	go func() {
		observations, err := l.prober.Probe(ctx, capabilities)
		done <- result{observations, err}
	}()

	select {
	case r := <-done:
		return r.observations, r.err
	case <-ctx.Done():
		return nil, fmt.Errorf("dependency prober did not return within %s: %w", l.probeTimeout, ctx.Err())
	}
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

// chainRecordDigest returns the SHA-256 hex digest of one role chain
// record's raw bytes: the same digest the next record in that chain would
// chain to via prev_sha256 (see roles.VerifyChain).
func chainRecordDigest(record roles.ChainRecord) string {
	sum := sha256.Sum256(record.Raw)
	return hex.EncodeToString(sum[:])
}

// roleChainDigest returns records' last record's chainRecordDigest: its
// current head, reused here as the stable digest this package's own
// Checked.role_chain_digest records and WorkflowEvent.RoleChainHead binds
// at creation. records must be non-empty.
func roleChainDigest(records []roles.ChainRecord) string {
	return chainRecordDigest(records[len(records)-1])
}

// roleChainContainsDigest reports whether any record in records has
// chainRecordDigest equal to digest. Verify uses this to require that the
// exact record chained at Create is still present in the (possibly grown)
// chain, rather than merely that some chain with the same id currently
// verifies: an entirely different but internally self-consistent chain
// placed at the same id would still pass roles.VerifyChain, but it would
// not contain the recorded head.
func roleChainContainsDigest(records []roles.ChainRecord, digest string) bool {
	for _, record := range records {
		if chainRecordDigest(record) == digest {
			return true
		}
	}
	return false
}

// loadVerifiedRoleChain loads the role chain identified by projectID,
// goalID, and roleChainID through l.chains and requires it to be
// non-empty and pass roles.VerifyChain; both failure shapes are wrapped in
// ErrRoleChainInvalid. Create and Verify share this helper so the two
// operations' doc comments promising "has no records or fails
// roles.VerifyChain" cannot silently drift apart.
func (l Lifecycle) loadVerifiedRoleChain(projectID, goalID, roleChainID string) ([]roles.ChainRecord, error) {
	records, err := l.chains.LoadChain(projectID, goalID, roleChainID)
	if err != nil {
		return nil, fmt.Errorf("load role chain: %w", err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("%w: role chain %q has no records", ErrRoleChainInvalid, roleChainID)
	}
	if err := roles.VerifyChain(records); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRoleChainInvalid, err)
	}
	return records, nil
}

// Create validates g (a Goal v2) and profileName, optionally verifies a
// referenced role chain, and appends the workflow's created event: data
// only, binding goal_id, goal_digest, profile, a snapshot of the profile the
// catalog gave for profileName (and the snapshot's digest), and (if given)
// role_chain_id. The workflow goes on from that snapshot, not from the
// catalog. It fails, appending nothing, when g is invalid, profileName is unknown,
// roleChainID is non-blank but the chain has no records or fails
// roles.VerifyChain, or on-disk state already exists for this workflow
// (ErrCreateRefused).
func (l Lifecycle) Create(projectID, workflowID string, g goal.Goal, profileName, roleChainID string) (State, error) {
	if err := g.Validate(); err != nil {
		return State{}, fmt.Errorf("workflow lifecycle: create: invalid goal: %w", err)
	}
	profile, err := l.profiles.Resolve(profileName)
	if err != nil {
		return State{}, fmt.Errorf("workflow lifecycle: create: %w", err)
	}
	// A catalog that gives a profile of another name than it was asked for is refused by the created
	// event's own validation (the snapshot must be of the profile the event names).
	snapshot := profile.Snapshot()
	snapshotDigest, err := snapshot.Digest()
	if err != nil {
		return State{}, fmt.Errorf("workflow lifecycle: create: %w", err)
	}
	var roleChainHead string
	if roleChainID != "" {
		if err := ValidateIdentifier("role_chain_id", roleChainID); err != nil {
			return State{}, fmt.Errorf("workflow lifecycle: create: %w", err)
		}
		records, err := l.loadVerifiedRoleChain(projectID, g.GoalID, roleChainID)
		if err != nil {
			return State{}, fmt.Errorf("workflow lifecycle: create: %w", err)
		}
		roleChainHead = roleChainDigest(records)
	}
	digest, err := goalDigest(g)
	if err != nil {
		return State{}, fmt.Errorf("workflow lifecycle: create: %w", err)
	}

	loaded, err := l.log.Load(projectID, workflowID)
	if err != nil {
		return State{}, err
	}
	if loaded.Classification != ClassificationAbsent {
		return State{}, fmt.Errorf("%w: on-disk classification is %q", ErrCreateRefused, loaded.Classification)
	}

	event, err := l.baseEvent(loaded, projectID, workflowID, KindCreated, profile)
	if err != nil {
		return State{}, err
	}
	event.GoalID = g.GoalID
	event.GoalDigest = digest
	event.Profile = profileName
	event.ProfileSnapshot = &snapshot
	event.ProfileDigest = snapshotDigest
	event.RoleChainID = roleChainID
	event.RoleChainHead = roleChainHead

	return l.commit(projectID, workflowID, loaded, event)
}

// Start appends a started event; legal only from status created (see
// CheckTransition).
func (l Lifecycle) Start(projectID, workflowID string) (State, error) {
	loaded, err := l.loadOwned(projectID, workflowID)
	if err != nil {
		return State{}, err
	}
	event, err := l.stateEvent(loaded, projectID, workflowID, KindStarted)
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
	event, err := l.stateEvent(loaded, projectID, workflowID, KindPaused)
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
	event, err := l.stateEvent(loaded, projectID, workflowID, KindResumed)
	if err != nil {
		return State{}, err
	}
	return l.commit(projectID, workflowID, loaded, event)
}

// RecordStage appends a stage_recorded event for stage. It rejects, without
// appending anything, a stage name that is not exactly the Workflow
// Profile's next declared stage: the stage immediately after the last one
// already recorded, in the profile's declared stage order (see
// workflowprofile.WorkflowProfile.NextStage). This also rejects a stage the profile never declares
// (it can never be "next") and a workflow whose profile has already
// recorded every declared stage.
func (l Lifecycle) RecordStage(projectID, workflowID, stage string) (State, error) {
	loaded, err := l.loadOwned(projectID, workflowID)
	if err != nil {
		return State{}, err
	}
	profile, err := l.profileOf(loaded.State)
	if err != nil {
		return State{}, fmt.Errorf("workflow lifecycle: record stage: %w", err)
	}
	want, more := profile.NextStage(len(loaded.State.Stages))
	if !more {
		return State{}, fmt.Errorf("%w: profile %q's declared stages are all recorded", ErrStageOutOfOrder, profile.Name)
	}
	if stage != want {
		return State{}, fmt.Errorf("%w: got %q, want %q (profile %q's next declared stage)", ErrStageOutOfOrder, stage, want, profile.Name)
	}
	event, err := l.baseEvent(loaded, projectID, workflowID, KindStageRecorded, profile)
	if err != nil {
		return State{}, err
	}
	event.Stage = stage
	return l.commit(projectID, workflowID, loaded, event)
}

// Verify performs a structural-only verification (no execution): the
// event hash chain still verifies, the Workflow Profile still resolves
// (ErrProfileInvalid; a workflow of version 2 uses the snapshot in its log, so
// it always does), the recorded stages still respect the Profile's
// declared order (ErrStageOrderInvalid), re-reading the Goal (via
// GoalReader) still produces the digest recorded at creation
// (ErrGoalDigestMismatch), and, if a role chain is referenced, it still has
// records, passes roles.VerifyChain, and still contains the exact record
// chained at creation (ErrRoleChainInvalid; the chain may legitimately grow
// since creation, but the record bound at creation must still be present,
// not merely some record with the same chain id — see
// roleChainContainsDigest). On success it appends one verified event
// recording the digests it checked (Checked.ChainDigest is the digest of
// the last event before this one); on any failure it appends nothing and
// returns a named error identifying which check failed.
func (l Lifecycle) Verify(projectID, workflowID string) (State, error) {
	loaded, err := l.loadOwned(projectID, workflowID)
	if err != nil {
		return State{}, err
	}
	state := loaded.State

	if err := VerifyEvents(loaded.Events); err != nil {
		return State{}, fmt.Errorf("%w: %v", ErrChainInvalid, err)
	}
	profile, err := l.profileOf(state)
	if err != nil {
		return State{}, fmt.Errorf("%w: %v", ErrProfileInvalid, err)
	}
	if err := profile.CheckStagePrefix(state.Stages); err != nil {
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

	var roleChainHeadDigest string
	if state.RoleChainID != "" {
		records, err := l.loadVerifiedRoleChain(projectID, state.GoalID, state.RoleChainID)
		if err != nil {
			return State{}, fmt.Errorf("workflow lifecycle: verify: %w", err)
		}
		if !roleChainContainsDigest(records, state.RoleChainHead) {
			return State{}, fmt.Errorf("%w: role chain %q no longer contains the record bound at creation (%s)", ErrRoleChainInvalid, state.RoleChainID, state.RoleChainHead)
		}
		roleChainHeadDigest = roleChainDigest(records)
	}

	workflowHeadDigest, err := EventDigest(loaded.Events[len(loaded.Events)-1])
	if err != nil {
		return State{}, fmt.Errorf("workflow lifecycle: verify: %w", err)
	}

	event, err := l.baseEvent(loaded, projectID, workflowID, KindVerified, profile)
	if err != nil {
		return State{}, err
	}
	event.Checked = &Checked{
		ChainDigest:     workflowHeadDigest,
		GoalDigest:      digest,
		Profile:         profile.Name,
		RoleChainDigest: roleChainHeadDigest,
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
	event, err := l.stateEvent(loaded, projectID, workflowID, KindClosed)
	if err != nil && outcome == OutcomeAbandoned {
		// Abandoning must stay possible even when the recorded profile no
		// longer resolves (for example after a profile is retired), so the
		// failure is recorded as an unavailable profile observation instead.
		detail := err.Error()
		if len(detail) > MaxObservationDetailLength {
			detail = strings.ToValidUTF8(detail[:MaxObservationDetailLength], "")
		}
		event, err = l.eventWith(loaded, projectID, workflowID, KindClosed, []Observation{{Capability: "profile", Status: ObservationUnavailable, Detail: detail}})
	}
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
	loaded, err := l.log.Load(projectID, workflowID)
	if err != nil {
		return "", State{}, err
	}
	return loaded.Classification, loaded.State, nil
}
