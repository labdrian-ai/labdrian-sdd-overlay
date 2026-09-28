package workflow

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/goal"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/roles"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflowprofile"
)

// stepClock returns a clock that advances by one minute on every call, so
// events in a test never collide on the same "at" timestamp.
func stepClock() func() time.Time {
	current := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	return func() time.Time {
		current = current.Add(time.Minute)
		return current
	}
}

func testProvenance() Provenance {
	return Provenance{
		WorktreeRoot: "/home/labdrian/labdrian-sdd-overlay",
		GitHead:      strings.Repeat("a", 40),
	}
}

func validGoal(projectID, goalID string) goal.Goal {
	return goal.Goal{
		Version:            2,
		ProjectID:          projectID,
		GoalID:             goalID,
		Objective:          "ship the thing",
		Scope:              "bounded scope",
		Constraints:        []string{},
		NonGoals:           []string{},
		AcceptanceCriteria: []string{"it works"},
		MemoryScope:        "none",
		RuntimeScope:       "none",
		DeliveryBoundary:   "local",
	}
}

// fakeGoalReader is an injectable, in-memory GoalReader: the lifecycle never
// reads a Goal file directly, so a real one is unnecessary here.
type fakeGoalReader struct {
	goals map[string]goal.Goal
}

func newFakeGoalReader() *fakeGoalReader {
	return &fakeGoalReader{goals: map[string]goal.Goal{}}
}

func (f *fakeGoalReader) set(projectID, goalID string, g goal.Goal) {
	f.goals[projectID+"/"+goalID] = g
}

func (f *fakeGoalReader) LoadGoal(projectID, goalID string) (goal.Goal, error) {
	g, ok := f.goals[projectID+"/"+goalID]
	if !ok {
		return goal.Goal{}, errors.New("fake goal reader: no goal for " + projectID + "/" + goalID)
	}
	return g, nil
}

// fakeChainReader is an injectable, in-memory RoleChainReader with the same
// signature as roles.ChainStore.LoadChain, so a real ChainStore satisfies
// RoleChainReader directly; this fake exists only to control chain contents
// deterministically in tests.
type fakeChainReader struct {
	chains map[string][]roles.ChainRecord
}

func newFakeChainReader() *fakeChainReader {
	return &fakeChainReader{chains: map[string][]roles.ChainRecord{}}
}

func (f *fakeChainReader) set(projectID, goalID, chainID string, records []roles.ChainRecord) {
	f.chains[projectID+"/"+goalID+"/"+chainID] = records
}

func (f *fakeChainReader) LoadChain(projectID, goalID, chainID string) ([]roles.ChainRecord, error) {
	return f.chains[projectID+"/"+goalID+"/"+chainID], nil
}

// validRoleChain returns a one-record chain that VerifyChain accepts.
func validRoleChain(projectID, goalID, chainID string) []roles.ChainRecord {
	h := roles.RoleHandoff{
		Version:       roles.HandoffVersion,
		ProjectID:     projectID,
		GoalID:        goalID,
		ChainID:       chainID,
		Seq:           1,
		FromRole:      roles.RolePrototyper,
		ToRole:        roles.RoleShaper,
		PrevSHA256:    roles.EmptyChainDigest,
		PayloadKind:   "plan",
		PayloadSHA256: strings.Repeat("a", 64),
		Evidence:      []roles.Evidence{},
		Context:       roles.Context{Summary: "s", Decisions: []string{}, OpenQuestions: []string{}},
		Status:        roles.StatusCompleted,
	}
	raw, err := json.Marshal(h)
	if err != nil {
		panic(err)
	}
	return []roles.ChainRecord{{Raw: raw, Handoff: h}}
}

// newTestLifecycle builds a Lifecycle over store, failing the test on error.
func newTestLifecycle(t *testing.T, store Store, clock func() time.Time, goals GoalReader, chains RoleChainReader, prober DependencyProber) Lifecycle {
	t.Helper()
	lc, err := NewLifecycle(store, clock, testProvenance(), goals, chains, prober)
	if err != nil {
		t.Fatalf("NewLifecycle() = %v, want nil", err)
	}
	return lc
}

func TestNewLifecycleRejectsMissingDependencies(t *testing.T) {
	store := newTestStore(t)
	goals := newFakeGoalReader()
	chains := newFakeChainReader()
	clock := stepClock()

	if _, err := NewLifecycle(store, nil, testProvenance(), goals, chains, nil); err == nil {
		t.Fatalf("NewLifecycle() = nil, want error for a nil clock")
	}
	if _, err := NewLifecycle(store, clock, testProvenance(), nil, chains, nil); err == nil {
		t.Fatalf("NewLifecycle() = nil, want error for a nil goal reader")
	}
	if _, err := NewLifecycle(store, clock, testProvenance(), goals, nil, nil); err == nil {
		t.Fatalf("NewLifecycle() = nil, want error for a nil role chain reader")
	}
	// A nil prober is accepted and defaults to UnavailableProber.
	if _, err := NewLifecycle(store, clock, testProvenance(), goals, chains, nil); err != nil {
		t.Fatalf("NewLifecycle() = %v, want nil for a nil prober (defaults to UnavailableProber)", err)
	}
}

// TestLifecycleHappyPathAcrossRestarts drives create through close(completed)
// using a fresh Lifecycle instance for every operation, over the same
// backing Store, to simulate the process restarting between each step.
func TestLifecycleHappyPathAcrossRestarts(t *testing.T) {
	store := newTestStore(t)
	clock := stepClock()
	goals := newFakeGoalReader()
	chains := newFakeChainReader()
	prober := UnavailableProber{}

	newLC := func() Lifecycle { return newTestLifecycle(t, store, clock, goals, chains, prober) }

	const projectID, workflowID, goalID = "proj-1", "wf-1", "goal-1"
	g := validGoal(projectID, goalID)
	goals.set(projectID, goalID, g)

	// maintenance declares: inspect, isolate-smallest-change, repair, validate, report.
	profile, err := workflowprofile.Resolve("maintenance")
	if err != nil {
		t.Fatalf("workflowprofile.Resolve() = %v, want nil", err)
	}

	state, err := newLC().Create(projectID, workflowID, g, profile.Name, "")
	if err != nil {
		t.Fatalf("Create() = %v, want nil", err)
	}
	if state.Status != StatusCreated || state.GoalID != goalID || state.Profile != profile.Name {
		t.Fatalf("Create() state = %+v, want created/%s/%s", state, goalID, profile.Name)
	}

	state, err = newLC().Start(projectID, workflowID)
	if err != nil {
		t.Fatalf("Start() = %v, want nil", err)
	}
	if state.Status != StatusRunning {
		t.Fatalf("Start() state.Status = %q, want %q", state.Status, StatusRunning)
	}

	state, err = newLC().RecordStage(projectID, workflowID, profile.Stages[0].Name)
	if err != nil {
		t.Fatalf("RecordStage(%q) = %v, want nil", profile.Stages[0].Name, err)
	}
	if len(state.Stages) != 1 || state.Stages[0] != profile.Stages[0].Name {
		t.Fatalf("RecordStage() stages = %v, want [%s]", state.Stages, profile.Stages[0].Name)
	}

	state, err = newLC().Pause(projectID, workflowID)
	if err != nil {
		t.Fatalf("Pause() = %v, want nil", err)
	}
	if state.Status != StatusPaused {
		t.Fatalf("Pause() state.Status = %q, want %q", state.Status, StatusPaused)
	}

	state, err = newLC().Resume(projectID, workflowID)
	if err != nil {
		t.Fatalf("Resume() = %v, want nil", err)
	}
	if state.Status != StatusRunning {
		t.Fatalf("Resume() state.Status = %q, want %q", state.Status, StatusRunning)
	}

	for _, stage := range profile.Stages[1:] {
		state, err = newLC().RecordStage(projectID, workflowID, stage.Name)
		if err != nil {
			t.Fatalf("RecordStage(%q) = %v, want nil", stage.Name, err)
		}
	}
	if len(state.Stages) != len(profile.Stages) {
		t.Fatalf("recorded stages = %v, want all of %v", state.Stages, profile.Stages)
	}

	state, err = newLC().Verify(projectID, workflowID)
	if err != nil {
		t.Fatalf("Verify() = %v, want nil", err)
	}
	if state.LastVerifiedSeq < 0 {
		t.Fatalf("Verify() state.LastVerifiedSeq = %d, want >= 0", state.LastVerifiedSeq)
	}
	loaded, err := store.Load(projectID, workflowID)
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	lastEvent := loaded.Events[len(loaded.Events)-1]
	if lastEvent.Kind != KindVerified || lastEvent.Checked == nil {
		t.Fatalf("last event = %+v, want a verified event with checked", lastEvent)
	}
	wantGoalDigest, err := goalDigest(g)
	if err != nil {
		t.Fatalf("goalDigest() = %v, want nil", err)
	}
	if lastEvent.Checked.GoalDigest != wantGoalDigest || lastEvent.Checked.Profile != profile.Name {
		t.Fatalf("checked = %+v, want goal_digest=%s profile=%s", lastEvent.Checked, wantGoalDigest, profile.Name)
	}

	state, err = newLC().Close(projectID, workflowID, OutcomeCompleted, "")
	if err != nil {
		t.Fatalf("Close(completed) = %v, want nil", err)
	}
	if state.Status != StatusClosed || state.CloseOutcome != OutcomeCompleted {
		t.Fatalf("Close() state = %+v, want closed/completed", state)
	}

	classification, statusState, err := newLC().Status(projectID, workflowID)
	if err != nil {
		t.Fatalf("Status() = %v, want nil", err)
	}
	if classification != ClassificationOwned || statusState.Status != StatusClosed {
		t.Fatalf("Status() = (%q, %+v), want (owned, closed)", classification, statusState)
	}
}

func TestLifecycleCreateRejectsUnknownProfile(t *testing.T) {
	store := newTestStore(t)
	goals := newFakeGoalReader()
	chains := newFakeChainReader()
	lc := newTestLifecycle(t, store, stepClock(), goals, chains, nil)

	g := validGoal("proj-1", "goal-1")
	goals.set("proj-1", "goal-1", g)

	if _, err := lc.Create("proj-1", "wf-1", g, "not-a-real-profile", ""); err == nil {
		t.Fatalf("Create() = nil, want error for an unknown profile")
	}
	loaded, err := store.Load("proj-1", "wf-1")
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	if loaded.Classification != ClassificationAbsent {
		t.Fatalf("Load() classification = %q, want %q (a rejected create must append nothing)", loaded.Classification, ClassificationAbsent)
	}
}

func TestLifecycleCreateRejectsInvalidGoal(t *testing.T) {
	store := newTestStore(t)
	goals := newFakeGoalReader()
	chains := newFakeChainReader()
	lc := newTestLifecycle(t, store, stepClock(), goals, chains, nil)

	g := validGoal("proj-1", "goal-1")
	g.Objective = "" // invalid: blank objective
	if _, err := lc.Create("proj-1", "wf-1", g, "odd", ""); err == nil {
		t.Fatalf("Create() = nil, want error for an invalid goal")
	}
}

func TestLifecycleCreateRejectsBrokenRoleChain(t *testing.T) {
	store := newTestStore(t)
	goals := newFakeGoalReader()
	chains := newFakeChainReader()
	lc := newTestLifecycle(t, store, stepClock(), goals, chains, nil)

	g := validGoal("proj-1", "goal-1")
	goals.set("proj-1", "goal-1", g)
	// No chain registered under this id: LoadChain returns an empty slice.
	if _, err := lc.Create("proj-1", "wf-1", g, "odd", "chain-1"); err == nil {
		t.Fatalf("Create() = nil, want error for a missing role chain")
	}
	loaded, err := store.Load("proj-1", "wf-1")
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	if loaded.Classification != ClassificationAbsent {
		t.Fatalf("Load() classification = %q, want %q", loaded.Classification, ClassificationAbsent)
	}
}

func TestLifecycleCreateWithValidRoleChainSucceeds(t *testing.T) {
	store := newTestStore(t)
	goals := newFakeGoalReader()
	chains := newFakeChainReader()
	lc := newTestLifecycle(t, store, stepClock(), goals, chains, nil)

	g := validGoal("proj-1", "goal-1")
	goals.set("proj-1", "goal-1", g)
	chains.set("proj-1", "goal-1", "chain-1", validRoleChain("proj-1", "goal-1", "chain-1"))

	state, err := lc.Create("proj-1", "wf-1", g, "odd", "chain-1")
	if err != nil {
		t.Fatalf("Create() = %v, want nil", err)
	}
	if state.RoleChainID != "chain-1" {
		t.Fatalf("Create() state.RoleChainID = %q, want %q", state.RoleChainID, "chain-1")
	}
}

func TestLifecycleVerifyFailsWhenGoalDigestChanged(t *testing.T) {
	store := newTestStore(t)
	clock := stepClock()
	goals := newFakeGoalReader()
	chains := newFakeChainReader()

	newLC := func() Lifecycle { return newTestLifecycle(t, store, clock, goals, chains, nil) }

	g := validGoal("proj-1", "goal-1")
	goals.set("proj-1", "goal-1", g)
	if _, err := newLC().Create("proj-1", "wf-1", g, "standalone-minimal", ""); err != nil {
		t.Fatalf("Create() = %v, want nil", err)
	}
	if _, err := newLC().Start("proj-1", "wf-1"); err != nil {
		t.Fatalf("Start() = %v, want nil", err)
	}

	before, err := store.Load("proj-1", "wf-1")
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}

	// Simulate the Goal file changing after creation.
	changed := g
	changed.Objective = "a different objective"
	goals.set("proj-1", "goal-1", changed)

	if _, err := newLC().Verify("proj-1", "wf-1"); !errors.Is(err, ErrGoalDigestMismatch) {
		t.Fatalf("Verify() err = %v, want ErrGoalDigestMismatch", err)
	}

	after, err := store.Load("proj-1", "wf-1")
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	if len(after.Events) != len(before.Events) {
		t.Fatalf("Verify() failure appended %d events, want 0 new events (had %d, now %d)", len(after.Events)-len(before.Events), len(before.Events), len(after.Events))
	}
}

func TestLifecycleVerifyFailsWhenRoleChainBreaks(t *testing.T) {
	store := newTestStore(t)
	clock := stepClock()
	goals := newFakeGoalReader()
	chains := newFakeChainReader()

	newLC := func() Lifecycle { return newTestLifecycle(t, store, clock, goals, chains, nil) }

	g := validGoal("proj-1", "goal-1")
	goals.set("proj-1", "goal-1", g)
	chains.set("proj-1", "goal-1", "chain-1", validRoleChain("proj-1", "goal-1", "chain-1"))

	if _, err := newLC().Create("proj-1", "wf-1", g, "odd", "chain-1"); err != nil {
		t.Fatalf("Create() = %v, want nil", err)
	}
	if _, err := newLC().Start("proj-1", "wf-1"); err != nil {
		t.Fatalf("Start() = %v, want nil", err)
	}

	before, err := store.Load("proj-1", "wf-1")
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}

	// Break the chain: seq 2 with no seq 1.
	broken := validRoleChain("proj-1", "goal-1", "chain-1")
	broken[0].Handoff.Seq = 2
	chains.set("proj-1", "goal-1", "chain-1", broken)

	if _, err := newLC().Verify("proj-1", "wf-1"); !errors.Is(err, ErrRoleChainInvalid) {
		t.Fatalf("Verify() err = %v, want ErrRoleChainInvalid", err)
	}

	after, err := store.Load("proj-1", "wf-1")
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	if len(after.Events) != len(before.Events) {
		t.Fatalf("Verify() failure appended events, want none (had %d, now %d)", len(before.Events), len(after.Events))
	}
}

func TestLifecycleRecordStageRejectsOutOfOrder(t *testing.T) {
	store := newTestStore(t)
	clock := stepClock()
	goals := newFakeGoalReader()
	chains := newFakeChainReader()
	newLC := func() Lifecycle { return newTestLifecycle(t, store, clock, goals, chains, nil) }

	g := validGoal("proj-1", "goal-1")
	goals.set("proj-1", "goal-1", g)
	profile, err := workflowprofile.Resolve("maintenance")
	if err != nil {
		t.Fatalf("workflowprofile.Resolve() = %v, want nil", err)
	}
	if _, err := newLC().Create("proj-1", "wf-1", g, profile.Name, ""); err != nil {
		t.Fatalf("Create() = %v, want nil", err)
	}
	if _, err := newLC().Start("proj-1", "wf-1"); err != nil {
		t.Fatalf("Start() = %v, want nil", err)
	}

	// Skip ahead to the third declared stage instead of the first.
	if _, err := newLC().RecordStage("proj-1", "wf-1", profile.Stages[2].Name); !errors.Is(err, ErrStageOutOfOrder) {
		t.Fatalf("RecordStage() err = %v, want ErrStageOutOfOrder", err)
	}

	// A name the profile never declares is rejected the same way.
	if _, err := newLC().RecordStage("proj-1", "wf-1", "not-a-declared-stage"); !errors.Is(err, ErrStageOutOfOrder) {
		t.Fatalf("RecordStage() err = %v, want ErrStageOutOfOrder", err)
	}

	loaded, err := store.Load("proj-1", "wf-1")
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	if len(loaded.State.Stages) != 0 {
		t.Fatalf("Stages = %v, want none recorded after rejected attempts", loaded.State.Stages)
	}
}

func TestLifecycleCloseCompletedRequiresPriorVerify(t *testing.T) {
	store := newTestStore(t)
	clock := stepClock()
	goals := newFakeGoalReader()
	chains := newFakeChainReader()
	newLC := func() Lifecycle { return newTestLifecycle(t, store, clock, goals, chains, nil) }

	g := validGoal("proj-1", "goal-1")
	goals.set("proj-1", "goal-1", g)
	if _, err := newLC().Create("proj-1", "wf-1", g, "standalone-minimal", ""); err != nil {
		t.Fatalf("Create() = %v, want nil", err)
	}
	if _, err := newLC().Start("proj-1", "wf-1"); err != nil {
		t.Fatalf("Start() = %v, want nil", err)
	}
	if _, err := newLC().Close("proj-1", "wf-1", OutcomeCompleted, ""); !errors.Is(err, ErrCompletedRequiresVerify) {
		t.Fatalf("Close(completed) err = %v, want ErrCompletedRequiresVerify", err)
	}
}

func TestLifecycleCloseAbandonedFromEachState(t *testing.T) {
	tests := []struct {
		name string
		run  func(lc Lifecycle, projectID, workflowID string) error
	}{
		{"from created", func(lc Lifecycle, p, w string) error { return nil }},
		{"from running", func(lc Lifecycle, p, w string) error { _, err := lc.Start(p, w); return err }},
		{"from paused", func(lc Lifecycle, p, w string) error {
			if _, err := lc.Start(p, w); err != nil {
				return err
			}
			_, err := lc.Pause(p, w)
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newTestStore(t)
			clock := stepClock()
			goals := newFakeGoalReader()
			chains := newFakeChainReader()
			lc := newTestLifecycle(t, store, clock, goals, chains, nil)

			g := validGoal("proj-1", "goal-1")
			goals.set("proj-1", "goal-1", g)
			if _, err := lc.Create("proj-1", "wf-1", g, "standalone-minimal", ""); err != nil {
				t.Fatalf("Create() = %v, want nil", err)
			}
			if err := tt.run(lc, "proj-1", "wf-1"); err != nil {
				t.Fatalf("setup step = %v, want nil", err)
			}
			state, err := lc.Close("proj-1", "wf-1", OutcomeAbandoned, "no longer needed")
			if err != nil {
				t.Fatalf("Close(abandoned) = %v, want nil", err)
			}
			if state.Status != StatusClosed || state.CloseOutcome != OutcomeAbandoned {
				t.Fatalf("Close() state = %+v, want closed/abandoned", state)
			}
		})
	}
}

// TestLifecycleDependenciesUnavailableNeverBlock exercises the default
// UnavailableProber across every lifecycle operation kind: every operation
// still succeeds even though every declared dependency is reported
// unavailable, and every unavailable observation is recorded, never
// silently approved or hidden.
func TestLifecycleDependenciesUnavailableNeverBlock(t *testing.T) {
	store := newTestStore(t)
	clock := stepClock()
	goals := newFakeGoalReader()
	chains := newFakeChainReader()
	newLC := func() Lifecycle { return newTestLifecycle(t, store, clock, goals, chains, UnavailableProber{}) }

	g := validGoal("proj-1", "goal-1")
	goals.set("proj-1", "goal-1", g)

	if _, err := newLC().Create("proj-1", "wf-1", g, "odd", ""); err != nil {
		t.Fatalf("Create() = %v, want nil", err)
	}
	if _, err := newLC().Start("proj-1", "wf-1"); err != nil {
		t.Fatalf("Start() = %v, want nil", err)
	}
	if _, err := newLC().Close("proj-1", "wf-1", OutcomeAbandoned, "done for this test"); err != nil {
		t.Fatalf("Close(abandoned) = %v, want nil", err)
	}

	loaded, err := store.Load("proj-1", "wf-1")
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	for _, e := range loaded.Events {
		if len(e.Observations) == 0 {
			t.Fatalf("event %q has no observations, want at least the odd profile's declared dependencies", e.Kind)
		}
		for _, o := range e.Observations {
			if o.Status != ObservationUnavailable {
				t.Fatalf("event %q observation %+v status = %q, want %q (UnavailableProber never reports available)", e.Kind, o, o.Status, ObservationUnavailable)
			}
		}
	}
}

// TestLifecycleReviewCapabilityDerivedFromProfileData checks that the
// gentle-ai-review capability is probed for profiles whose review_policy
// names RDD without disclaiming Gentle, and is absent for
// standalone-minimal, which explicitly disclaims it. See
// profileReliesOnGentleReview's doc comment for the exact derivation rule
// and the resulting profile set, which is broader than the two profiles
// named in this feature's task description.
func TestLifecycleReviewCapabilityDerivedFromProfileData(t *testing.T) {
	tests := []struct {
		profile  string
		wantGate bool
	}{
		{"odd", true},
		{"sdd", true},
		{"maintenance", true},
		{"incident-recovery", true},
		{"standalone-minimal", false},
	}
	for _, tt := range tests {
		t.Run(tt.profile, func(t *testing.T) {
			store := newTestStore(t)
			goals := newFakeGoalReader()
			chains := newFakeChainReader()
			lc := newTestLifecycle(t, store, stepClock(), goals, chains, UnavailableProber{})

			g := validGoal("proj-1", "goal-1")
			goals.set("proj-1", "goal-1", g)
			if _, err := lc.Create("proj-1", "wf-1", g, tt.profile, ""); err != nil {
				t.Fatalf("Create() = %v, want nil", err)
			}
			loaded, err := store.Load("proj-1", "wf-1")
			if err != nil {
				t.Fatalf("Load() = %v, want nil", err)
			}
			created := loaded.Events[0]
			found := false
			for _, o := range created.Observations {
				if o.Capability == gentleAIReviewCapability {
					found = true
				}
			}
			if found != tt.wantGate {
				t.Fatalf("profile %q: gentle-ai-review observed = %v, want %v", tt.profile, found, tt.wantGate)
			}
		})
	}
}

func TestLifecycleLoadOwnedRefusesNonOwnedClassifications(t *testing.T) {
	store := newTestStore(t)
	goals := newFakeGoalReader()
	chains := newFakeChainReader()
	lc := newTestLifecycle(t, store, stepClock(), goals, chains, nil)

	if _, err := lc.Start("proj-1", "wf-1"); !errors.Is(err, ErrWorkflowNotOwned) {
		t.Fatalf("Start() on an absent workflow err = %v, want ErrWorkflowNotOwned", err)
	}
}
