package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"slices"
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
// signature as roles.ChainLog.LoadChain, so a real chain store satisfies
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
func newTestLifecycle(t *testing.T, store EventLog, clock func() time.Time, goals GoalReader, chains RoleChainReader, prober DependencyProber) Lifecycle {
	t.Helper()
	lc, err := NewLifecycle(store, builtInProfiles, clock, testProvenance(), goals, chains, prober)
	if err != nil {
		t.Fatalf("NewLifecycle() = %v, want nil", err)
	}
	return lc
}

// builtInProfiles is the catalog of the built-in Workflow Profiles, the one the
// program is wired with.
var builtInProfiles = ProfileCatalogFunc(workflowprofile.Resolve)

func TestNewLifecycleRejectsMissingDependencies(t *testing.T) {
	store := newMemEventLog()
	goals := newFakeGoalReader()
	chains := newFakeChainReader()
	clock := stepClock()

	if _, err := NewLifecycle(nil, builtInProfiles, clock, testProvenance(), goals, chains, nil); err == nil {
		t.Fatalf("NewLifecycle() = nil, want error for a nil event log")
	}
	if _, err := NewLifecycle(store, nil, clock, testProvenance(), goals, chains, nil); err == nil {
		t.Fatalf("NewLifecycle() = nil, want error for a nil profile catalog")
	}
	if _, err := NewLifecycle(store, builtInProfiles, nil, testProvenance(), goals, chains, nil); err == nil {
		t.Fatalf("NewLifecycle() = nil, want error for a nil clock")
	}
	if _, err := NewLifecycle(store, builtInProfiles, clock, testProvenance(), nil, chains, nil); err == nil {
		t.Fatalf("NewLifecycle() = nil, want error for a nil goal reader")
	}
	if _, err := NewLifecycle(store, builtInProfiles, clock, testProvenance(), goals, nil, nil); err == nil {
		t.Fatalf("NewLifecycle() = nil, want error for a nil role chain reader")
	}
	// A nil prober is accepted and defaults to UnavailableProber.
	if _, err := NewLifecycle(store, builtInProfiles, clock, testProvenance(), goals, chains, nil); err != nil {
		t.Fatalf("NewLifecycle() = %v, want nil for a nil prober (defaults to UnavailableProber)", err)
	}
}

// memEventLog is an in-memory EventLog: the lifecycle needs a log, not a file
// system, and this one proves it. It keeps each workflow's log as the same JSONL
// bytes a file would hold and decides everything with the domain's own pure rules
// (ClassifyLog to read them, AdmitAppend to extend them), so it refuses what a
// real log refuses. What it leaves out is the file: that is the adapter's, and
// engine/workflow/filelog tests it, including a Lifecycle running over it.
type memEventLog struct {
	logs    map[string][]byte
	loads   int
	appends int
}

func newMemEventLog() *memEventLog { return &memEventLog{logs: map[string][]byte{}} }

func (m *memEventLog) classify(projectID, workflowID string) Loaded {
	raw := m.logs[projectID+"/"+workflowID]
	if raw == nil {
		return Loaded{Classification: ClassificationAbsent}
	}
	return ClassifyLog(projectID, workflowID, raw)
}

func (m *memEventLog) Load(projectID, workflowID string) (Loaded, error) {
	m.loads++
	return m.classify(projectID, workflowID), nil
}

func (m *memEventLog) Append(projectID, workflowID string, next WorkflowEvent) error {
	m.appends++
	line, err := AdmitAppend(projectID, workflowID, m.classify(projectID, workflowID), next)
	if err != nil {
		return err
	}
	key := projectID + "/" + workflowID
	m.logs[key] = append(append([]byte(nil), m.logs[key]...), line...)
	return nil
}

// countingCatalog counts the profiles asked of it.
type countingCatalog struct {
	inner ProfileCatalog
	asked int
}

func (c *countingCatalog) Resolve(name string) (workflowprofile.WorkflowProfile, error) {
	c.asked++
	return c.inner.Resolve(name)
}

// The lifecycle reaches its log and its profiles only through the two ports: a
// whole workflow runs over an in-memory log with the catalog counted, and no state
// directory is touched.
func TestLifecycleRunsOverAnyEventLogAndProfileCatalog(t *testing.T) {
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)
	log := newMemEventLog()
	catalog := &countingCatalog{inner: builtInProfiles}
	goals := newFakeGoalReader()
	g := validGoal("proj-1", "goal-1")
	goals.set("proj-1", "goal-1", g)
	lc, err := NewLifecycle(log, catalog, stepClock(), testProvenance(), goals, newFakeChainReader(), UnavailableProber{})
	if err != nil {
		t.Fatalf("NewLifecycle() = %v, want nil", err)
	}

	profile, err := workflowprofile.Resolve("standalone-minimal")
	if err != nil {
		t.Fatal(err)
	}
	steps := []struct {
		name string
		do   func() (State, error)
	}{
		{"create", func() (State, error) { return lc.Create("proj-1", "wf-1", g, profile.Name, "") }},
		{"start", func() (State, error) { return lc.Start("proj-1", "wf-1") }},
		{"stage", func() (State, error) { return lc.RecordStage("proj-1", "wf-1", profile.Stages[0].Name) }},
		{"verify", func() (State, error) { return lc.Verify("proj-1", "wf-1") }},
		{"close", func() (State, error) { return lc.Close("proj-1", "wf-1", OutcomeCompleted, "") }},
	}
	var last State
	for _, step := range steps {
		if last, err = step.do(); err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
	}
	if last.Status != StatusClosed {
		t.Errorf("final status = %q, want %q", last.Status, StatusClosed)
	}
	if log.appends != len(steps) {
		t.Errorf("appends to the injected log = %d, want %d (one per operation)", log.appends, len(steps))
	}
	if log.loads == 0 {
		t.Error("the injected log was never read")
	}
	if catalog.asked == 0 {
		t.Error("the injected catalog was never asked")
	}
	if cls, state, err := lc.Status("proj-1", "wf-1"); err != nil || cls != ClassificationOwned || state.Status != StatusClosed {
		t.Errorf("Status() = (%q, %q, %v), want the closed workflow read back through the log", cls, state.Status, err)
	}
	entries, err := os.ReadDir(stateHome)
	if err != nil || len(entries) != 0 {
		t.Errorf("state home holds %v (%v), want nothing: no file store was used", entries, err)
	}
}

// TestLifecycleHappyPathAcrossRestarts drives create through close(completed)
// using a fresh Lifecycle instance for every operation, over the same
// backing log, to simulate the process restarting between each step.
func TestLifecycleHappyPathAcrossRestarts(t *testing.T) {
	store := newMemEventLog()
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
	store := newMemEventLog()
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
	store := newMemEventLog()
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
	store := newMemEventLog()
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
	store := newMemEventLog()
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
	store := newMemEventLog()
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
	store := newMemEventLog()
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
	store := newMemEventLog()
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
	store := newMemEventLog()
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
			store := newMemEventLog()
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
	store := newMemEventLog()
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
// gentle-ai-review capability is probed for every profile that relies on
// Gentle review and is absent for standalone-minimal, which
// explicitly disclaims Gentle review.
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
			store := newMemEventLog()
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
	store := newMemEventLog()
	goals := newFakeGoalReader()
	chains := newFakeChainReader()
	lc := newTestLifecycle(t, store, stepClock(), goals, chains, nil)

	if _, err := lc.Start("proj-1", "wf-1"); !errors.Is(err, ErrWorkflowNotOwned) {
		t.Fatalf("Start() on an absent workflow err = %v, want ErrWorkflowNotOwned", err)
	}
}

// The dependencies a workflow records are those the profile the catalog gives declares as typed
// data (its memory default's sources, and whether it relies on Gentle review), whatever the
// profile is called: a profile named like a built-in one but declaring something else is
// observed as it declares itself. (That the typed review dependency agrees with the review_policy
// prose is checked where the profiles are declared, in workflowprofile.)
func TestLifecycleObservesTheDependenciesTheCatalogsProfileDeclares(t *testing.T) {
	maintenance, err := workflowprofile.Resolve("maintenance")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		edit   func(p *workflowprofile.WorkflowProfile)
		wanted []string
	}{
		{"as the built-in profile", func(*workflowprofile.WorkflowProfile) {}, []string{"memory:engram", "memory:longterm-mem", "memory:procedural-skills", gentleAIReviewCapability}},
		{"with a narrower memory default", func(p *workflowprofile.WorkflowProfile) {
			p.MemoryDefault.Sources = []workflowprofile.MemorySource{workflowprofile.MemorySourceEngram}
		}, []string{"memory:engram", gentleAIReviewCapability}},
		{"with no review dependency", func(p *workflowprofile.WorkflowProfile) { p.ReliesOnGentleReview = false }, []string{"memory:engram", "memory:longterm-mem", "memory:procedural-skills"}},
		{"with a review dependency and no memory", func(p *workflowprofile.WorkflowProfile) {
			p.MemoryDefault = workflowprofile.MemoryDefault{Scope: workflowprofile.MemoryScopeNone, Sources: []workflowprofile.MemorySource{}}
		}, []string{gentleAIReviewCapability}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newMemEventLog()
			goals := newFakeGoalReader()
			lc := newTestLifecycle(t, store, stepClock(), goals, newFakeChainReader(), UnavailableProber{})
			custom := maintenance
			custom.MemoryDefault.Sources = append([]workflowprofile.MemorySource(nil), maintenance.MemoryDefault.Sources...)
			tc.edit(&custom)
			lc.profiles = ProfileCatalogFunc(func(string) (workflowprofile.WorkflowProfile, error) { return custom, nil })
			g := validGoal("proj-1", "goal-1")
			goals.set("proj-1", "goal-1", g)
			if _, err := lc.Create("proj-1", "wf-1", g, "maintenance", ""); err != nil {
				t.Fatalf("Create() = %v, want nil", err)
			}
			loaded, err := store.Load("proj-1", "wf-1")
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, o := range loaded.Events[0].Observations {
				got = append(got, o.Capability)
			}
			if !slices.Equal(got, tc.wanted) {
				t.Errorf("observed %v, want %v", got, tc.wanted)
			}
		})
	}
}

// TestLifecycleAbandonSucceedsWhenProfileNoLongerResolves checks that closing
// a workflow as abandoned never depends on its profile still resolving: the
// close is recorded with a "profile" observation marked unavailable instead
// of failing, while a completed close still requires a successful verify. The
// workflow is of version 1, whose profile is the catalog's (a version 2 workflow
// has its profile in its log and needs no such fallback).
func TestLifecycleAbandonSucceedsWhenProfileNoLongerResolves(t *testing.T) {
	store := newMemEventLog()
	goals := newFakeGoalReader()
	chains := newFakeChainReader()
	lc := newTestLifecycle(t, store, stepClock(), goals, chains, UnavailableProber{})

	g := validGoal("proj-1", "goal-1")
	goals.set("proj-1", "goal-1", g)
	createVersion1Workflow(t, store, "proj-1", "wf-1", g, "odd")
	if _, err := lc.Start("proj-1", "wf-1"); err != nil {
		t.Fatalf("Start() = %v, want nil", err)
	}

	lc.profiles = ProfileCatalogFunc(func(name string) (workflowprofile.WorkflowProfile, error) {
		return workflowprofile.WorkflowProfile{}, errors.New("profile retired")
	})
	state, err := lc.Close("proj-1", "wf-1", OutcomeAbandoned, "profile retired upstream")
	if err != nil {
		t.Fatalf("Close(abandoned) = %v, want nil when the profile no longer resolves", err)
	}
	if state.Status != StatusClosed {
		t.Fatalf("state.Status = %q, want %q", state.Status, StatusClosed)
	}
	loaded, err := store.Load("proj-1", "wf-1")
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	closed := loaded.Events[len(loaded.Events)-1]
	found := false
	for _, o := range closed.Observations {
		if o.Capability == "profile" && o.Status == ObservationUnavailable && o.Detail != "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("closed observations = %+v, want a profile observation marked unavailable with a detail", closed.Observations)
	}
}

// growRoleChain returns records with one more record appended, chained onto
// records' current head, transitioning fromRole (the last record's to_role)
// to toRole. It exists so tests can simulate a role chain that legitimately
// grows after a workflow's Create bound its head.
func growRoleChain(records []roles.ChainRecord, fromRole, toRole roles.Role) []roles.ChainRecord {
	last := records[len(records)-1]
	next := roles.RoleHandoff{
		Version:       roles.HandoffVersion,
		ProjectID:     last.Handoff.ProjectID,
		GoalID:        last.Handoff.GoalID,
		ChainID:       last.Handoff.ChainID,
		Seq:           last.Handoff.Seq + 1,
		FromRole:      fromRole,
		ToRole:        toRole,
		PrevSHA256:    chainRecordDigest(last),
		PayloadKind:   "plan",
		PayloadSHA256: strings.Repeat("a", 64),
		Evidence:      []roles.Evidence{},
		Context:       roles.Context{Summary: "s", Decisions: []string{}, OpenQuestions: []string{}},
		Status:        roles.StatusCompleted,
	}
	raw, err := json.Marshal(next)
	if err != nil {
		panic(err)
	}
	return append(records, roles.ChainRecord{Raw: raw, Handoff: next})
}

// TestLifecycleCreateBindsRoleChainHeadAndVerifyRequiresIt covers R1-001: a
// workflow's created event binds the role chain's current head digest, and
// Verify requires that exact record to still be present in the (possibly
// grown) chain, not merely that some chain with the same id currently
// verifies.
func TestLifecycleCreateBindsRoleChainHeadAndVerifyRequiresIt(t *testing.T) {
	store := newMemEventLog()
	clock := stepClock()
	goals := newFakeGoalReader()
	chains := newFakeChainReader()
	newLC := func() Lifecycle { return newTestLifecycle(t, store, clock, goals, chains, nil) }

	g := validGoal("proj-1", "goal-1")
	goals.set("proj-1", "goal-1", g)
	original := validRoleChain("proj-1", "goal-1", "chain-1")
	chains.set("proj-1", "goal-1", "chain-1", original)

	if _, err := newLC().Create("proj-1", "wf-1", g, "odd", "chain-1"); err != nil {
		t.Fatalf("Create() = %v, want nil", err)
	}
	if _, err := newLC().Start("proj-1", "wf-1"); err != nil {
		t.Fatalf("Start() = %v, want nil", err)
	}

	loaded, err := store.Load("proj-1", "wf-1")
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	created := loaded.Events[0]
	wantHead := chainRecordDigest(original[len(original)-1])
	if created.RoleChainHead != wantHead {
		t.Fatalf("created.RoleChainHead = %q, want %q (the chain's head digest at creation)", created.RoleChainHead, wantHead)
	}

	// The chain legitimately grows after Create: Verify must still succeed,
	// since the record bound at creation is still present.
	grown := growRoleChain(original, roles.RoleShaper, roles.RoleEstimator)
	chains.set("proj-1", "goal-1", "chain-1", grown)
	if _, err := newLC().Verify("proj-1", "wf-1"); err != nil {
		t.Fatalf("Verify() = %v, want nil when the chain has only grown", err)
	}

	// The chain is rewritten from scratch: a different, internally
	// self-consistent chain at the same id no longer contains the record
	// bound at creation, so Verify must fail.
	rewritten := validRoleChain("proj-1", "goal-1", "chain-1")
	rewritten[0].Handoff.PayloadSHA256 = strings.Repeat("b", 64)
	raw, err := json.Marshal(rewritten[0].Handoff)
	if err != nil {
		t.Fatalf("json.Marshal() = %v, want nil", err)
	}
	rewritten[0].Raw = raw
	chains.set("proj-1", "goal-1", "chain-1", rewritten)
	if _, err := newLC().Verify("proj-1", "wf-1"); !errors.Is(err, ErrRoleChainInvalid) {
		t.Fatalf("Verify() err = %v, want ErrRoleChainInvalid for a rewritten chain", err)
	}
}

// blockingProber is a DependencyProber that never returns on its own: it
// blocks until ctx is done, then reports ctx's error. It exists to prove
// observationsFor's own bounded deadline (Lifecycle.probeTimeout), not the
// prober's cooperation, is what keeps a lifecycle operation from hanging.
type blockingProber struct{}

func (blockingProber) Probe(ctx context.Context, capabilities []string) ([]Observation, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// TestLifecycleObservationsForProbeTimeout covers R4-probe-no-timeout: a
// prober that blocks past Lifecycle.probeTimeout never blocks the calling
// operation, and every declared dependency is recorded unavailable with a
// detail explaining why.
func TestLifecycleObservationsForProbeTimeout(t *testing.T) {
	store := newMemEventLog()
	goals := newFakeGoalReader()
	chains := newFakeChainReader()
	g := validGoal("proj-1", "goal-1")
	goals.set("proj-1", "goal-1", g)

	lc := newTestLifecycle(t, store, stepClock(), goals, chains, blockingProber{})
	lc.probeTimeout = 20 * time.Millisecond

	start := time.Now()
	state, err := lc.Create("proj-1", "wf-1", g, "odd", "")
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Create() = %v, want nil even when the prober blocks past the deadline", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("Create() took %s, want it bounded near probeTimeout (20ms), not hung on the prober", elapsed)
	}
	if state.Status != StatusCreated {
		t.Fatalf("state.Status = %q, want %q", state.Status, StatusCreated)
	}

	loaded, err := store.Load("proj-1", "wf-1")
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	created := loaded.Events[0]
	if len(created.Observations) == 0 {
		t.Fatalf("created.Observations is empty, want one unavailable observation per declared dependency")
	}
	for _, o := range created.Observations {
		if o.Status != ObservationUnavailable || o.Detail == "" {
			t.Fatalf("observation %+v, want unavailable with a non-empty detail", o)
		}
	}
}

// mismatchedCountProber always returns fewer observations than requested,
// without an error: a misbehaving prober that violates DependencyProber's
// documented contract in a different way than an error or a timeout.
type mismatchedCountProber struct{}

func (mismatchedCountProber) Probe(_ context.Context, capabilities []string) ([]Observation, error) {
	if len(capabilities) == 0 {
		return nil, nil
	}
	return []Observation{{Capability: capabilities[0], Status: ObservationAvailable}}, nil
}

// TestLifecycleObservationsForTreatsMismatchedProberCountAsUnavailable
// covers R3-4: a prober returning the wrong number of observations never
// fails or blocks the operation and is never trusted for
// ObservationAvailable; every declared dependency is instead recorded
// unavailable with a detail naming the mismatch.
func TestLifecycleObservationsForTreatsMismatchedProberCountAsUnavailable(t *testing.T) {
	store := newMemEventLog()
	goals := newFakeGoalReader()
	chains := newFakeChainReader()
	g := validGoal("proj-1", "goal-1")
	goals.set("proj-1", "goal-1", g)

	lc := newTestLifecycle(t, store, stepClock(), goals, chains, mismatchedCountProber{})
	if _, err := lc.Create("proj-1", "wf-1", g, "odd", ""); err != nil {
		t.Fatalf("Create() = %v, want nil even when the prober returns the wrong observation count", err)
	}
	loaded, err := store.Load("proj-1", "wf-1")
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	created := loaded.Events[0]
	if len(created.Observations) < 2 {
		t.Fatalf("created.Observations = %+v, want one per declared dependency (odd declares more than one)", created.Observations)
	}
	for _, o := range created.Observations {
		if o.Status != ObservationUnavailable {
			t.Fatalf("observation %+v status = %q, want %q (a mismatched-count prober is never trusted)", o, o.Status, ObservationUnavailable)
		}
	}
}

// TestLifecycleVerifyFailsWhenProfileNoLongerResolves covers R3-2: Verify's
// ErrProfileInvalid branch, driven through the injectable ProfileCatalog the
// same way TestLifecycleAbandonSucceedsWhenProfileNoLongerResolves drives
// Close's profile-observation fallback. The workflow is of version 1.
func TestLifecycleVerifyFailsWhenProfileNoLongerResolves(t *testing.T) {
	store := newMemEventLog()
	clock := stepClock()
	goals := newFakeGoalReader()
	chains := newFakeChainReader()
	newLC := func() Lifecycle { return newTestLifecycle(t, store, clock, goals, chains, nil) }

	g := validGoal("proj-1", "goal-1")
	goals.set("proj-1", "goal-1", g)
	createVersion1Workflow(t, store, "proj-1", "wf-1", g, "standalone-minimal")
	if _, err := newLC().Start("proj-1", "wf-1"); err != nil {
		t.Fatalf("Start() = %v, want nil", err)
	}
	before, err := store.Load("proj-1", "wf-1")
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}

	lc := newLC()
	lc.profiles = ProfileCatalogFunc(func(name string) (workflowprofile.WorkflowProfile, error) {
		return workflowprofile.WorkflowProfile{}, errors.New("profile retired")
	})
	if _, err := lc.Verify("proj-1", "wf-1"); !errors.Is(err, ErrProfileInvalid) {
		t.Fatalf("Verify() err = %v, want ErrProfileInvalid", err)
	}

	after, err := store.Load("proj-1", "wf-1")
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	if len(after.Events) != len(before.Events) {
		t.Fatalf("Verify() failure appended events, want none (had %d, now %d)", len(before.Events), len(after.Events))
	}
}

// TestLifecycleVerifyFailsWhenStageOrderInvalid covers R3-2's other branch:
// ErrStageOrderInvalid, driven by an injected ProfileCatalog that reports a
// profile whose declared stage order no longer matches what was actually
// recorded. The workflow is of version 1.
func TestLifecycleVerifyFailsWhenStageOrderInvalid(t *testing.T) {
	store := newMemEventLog()
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
	createVersion1Workflow(t, store, "proj-1", "wf-1", g, profile.Name)
	if _, err := newLC().Start("proj-1", "wf-1"); err != nil {
		t.Fatalf("Start() = %v, want nil", err)
	}
	if _, err := newLC().RecordStage("proj-1", "wf-1", profile.Stages[0].Name); err != nil {
		t.Fatalf("RecordStage() = %v, want nil", err)
	}
	before, err := store.Load("proj-1", "wf-1")
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}

	lc := newLC()
	lc.profiles = ProfileCatalogFunc(func(name string) (workflowprofile.WorkflowProfile, error) {
		mutated := profile
		mutated.Stages = append([]workflowprofile.Stage{}, profile.Stages...)
		mutated.Stages[0].Name = "not-" + profile.Stages[0].Name
		return mutated, nil
	})
	if _, err := lc.Verify("proj-1", "wf-1"); !errors.Is(err, ErrStageOrderInvalid) {
		t.Fatalf("Verify() err = %v, want ErrStageOrderInvalid", err)
	}

	after, err := store.Load("proj-1", "wf-1")
	if err != nil {
		t.Fatalf("Load() = %v, want nil", err)
	}
	if len(after.Events) != len(before.Events) {
		t.Fatalf("Verify() failure appended events, want none (had %d, now %d)", len(before.Events), len(after.Events))
	}
}

// TestLifecycleRecordStageRejectsWhenAllStagesRecorded covers R3-3: once
// every declared stage is recorded, one more RecordStage call is rejected
// (there is no "next declared stage" left), distinct from
// TestLifecycleRecordStageRejectsOutOfOrder's skip-ahead and undeclared-name
// cases.
func TestLifecycleRecordStageRejectsWhenAllStagesRecorded(t *testing.T) {
	store := newMemEventLog()
	clock := stepClock()
	goals := newFakeGoalReader()
	chains := newFakeChainReader()
	newLC := func() Lifecycle { return newTestLifecycle(t, store, clock, goals, chains, nil) }

	g := validGoal("proj-1", "goal-1")
	goals.set("proj-1", "goal-1", g)
	profile, err := workflowprofile.Resolve("standalone-minimal")
	if err != nil {
		t.Fatalf("workflowprofile.Resolve() = %v, want nil", err)
	}
	if _, err := newLC().Create("proj-1", "wf-1", g, profile.Name, ""); err != nil {
		t.Fatalf("Create() = %v, want nil", err)
	}
	if _, err := newLC().Start("proj-1", "wf-1"); err != nil {
		t.Fatalf("Start() = %v, want nil", err)
	}
	for _, stage := range profile.Stages {
		if _, err := newLC().RecordStage("proj-1", "wf-1", stage.Name); err != nil {
			t.Fatalf("RecordStage(%q) = %v, want nil", stage.Name, err)
		}
	}

	if _, err := newLC().RecordStage("proj-1", "wf-1", "one-more"); !errors.Is(err, ErrStageOutOfOrder) {
		t.Fatalf("RecordStage() err = %v, want ErrStageOutOfOrder once every declared stage is recorded", err)
	}
}

// TestLifecycleVerifyRecordsExactLastVerifiedSeq covers R3-5:
// TestLifecycleHappyPathAcrossRestarts only asserts LastVerifiedSeq is
// non-negative; this test asserts the exact seq of the appended verified
// event.
func TestLifecycleVerifyRecordsExactLastVerifiedSeq(t *testing.T) {
	store := newMemEventLog()
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
	wantSeq := len(before.Events) // the verified event's own seq, about to be appended.

	state, err := newLC().Verify("proj-1", "wf-1")
	if err != nil {
		t.Fatalf("Verify() = %v, want nil", err)
	}
	if state.LastVerifiedSeq != wantSeq {
		t.Fatalf("Verify() state.LastVerifiedSeq = %d, want exactly %d", state.LastVerifiedSeq, wantSeq)
	}
}

// erroringProber always fails: used to drive DegradedHook's error path.
type erroringProber struct{ err error }

func (p erroringProber) Probe(_ context.Context, capabilities []string) ([]Observation, error) {
	return nil, p.err
}

// TestLifecycleDegradedHookNotifiesOnProberError covers R4-1: a caller that
// wires WithDegradedHook is notified exactly when observationsFor could not
// get a usable answer from the prober (here, a Probe error), while the
// operation itself still succeeds (dependency unavailability never blocks a
// lifecycle operation).
func TestLifecycleDegradedHookNotifiesOnProberError(t *testing.T) {
	store := newMemEventLog()
	goals := newFakeGoalReader()
	chains := newFakeChainReader()
	g := validGoal("proj-1", "goal-1")
	goals.set("proj-1", "goal-1", g)

	proberErr := errors.New("boom")
	lc := newTestLifecycle(t, store, stepClock(), goals, chains, erroringProber{err: proberErr})
	var calls []string
	lc = lc.WithDegradedHook(func(detail string) { calls = append(calls, detail) })

	state, err := lc.Create("proj-1", "wf-1", g, "odd", "")
	if err != nil {
		t.Fatalf("Create() = %v, want nil even when the prober errors", err)
	}
	if state.Status != StatusCreated {
		t.Fatalf("state.Status = %q, want %q", state.Status, StatusCreated)
	}
	if len(calls) != 1 {
		t.Fatalf("DegradedHook calls = %v, want exactly one call", calls)
	}
	if !strings.Contains(calls[0], proberErr.Error()) {
		t.Fatalf("DegradedHook detail = %q, want it to mention %q", calls[0], proberErr.Error())
	}
}

// TestLifecycleDegradedHookNotifiesOnMismatchedCount covers the same
// notification for the mismatched-observation-count degradation shape.
func TestLifecycleDegradedHookNotifiesOnMismatchedCount(t *testing.T) {
	store := newMemEventLog()
	goals := newFakeGoalReader()
	chains := newFakeChainReader()
	g := validGoal("proj-1", "goal-1")
	goals.set("proj-1", "goal-1", g)

	lc := newTestLifecycle(t, store, stepClock(), goals, chains, mismatchedCountProber{})
	calls := 0
	lc = lc.WithDegradedHook(func(string) { calls++ })

	if _, err := lc.Create("proj-1", "wf-1", g, "odd", ""); err != nil {
		t.Fatalf("Create() = %v, want nil", err)
	}
	if calls != 1 {
		t.Fatalf("DegradedHook calls = %d, want exactly 1", calls)
	}
}

// TestLifecycleNilDegradedHookIsNoop proves a Lifecycle built without
// WithDegradedHook (the default from NewLifecycle) tolerates a degraded
// probe with no panic and no notification, since l.degraded is nil.
func TestLifecycleNilDegradedHookIsNoop(t *testing.T) {
	store := newMemEventLog()
	goals := newFakeGoalReader()
	chains := newFakeChainReader()
	g := validGoal("proj-1", "goal-1")
	goals.set("proj-1", "goal-1", g)

	lc := newTestLifecycle(t, store, stepClock(), goals, chains, erroringProber{err: errors.New("boom")})
	if _, err := lc.Create("proj-1", "wf-1", g, "odd", ""); err != nil {
		t.Fatalf("Create() = %v, want nil", err)
	}
}

// Every operation resolves a profile through the one resolver the Lifecycle was
// built with. Create used to resolve it directly for its first check and
// RecordStage for its stage order, so a resolver other than the built-in
// catalog was honoured by some operations and ignored by others.

// A profile the resolver refuses is refused by Create before anything else is
// read: the role chain is never loaded for it.
func TestLifecycleCreateRefusesAProfileTheResolverRefusesBeforeReadingAnythingElse(t *testing.T) {
	store := newMemEventLog()
	goals := newFakeGoalReader()
	chains := newFakeChainReader() // holds no chain: loading "chain-1" would fail as ErrRoleChainInvalid
	lc := newTestLifecycle(t, store, stepClock(), goals, chains, nil)
	retired := errors.New("profile retired")
	lc.profiles = ProfileCatalogFunc(func(string) (workflowprofile.WorkflowProfile, error) {
		return workflowprofile.WorkflowProfile{}, retired
	})

	_, err := lc.Create("proj-1", "wf-1", validGoal("proj-1", "goal-1"), "odd", "chain-1")
	if !errors.Is(err, retired) {
		t.Fatalf("Create() err = %v, want the resolver's refusal", err)
	}
	if errors.Is(err, ErrRoleChainInvalid) {
		t.Fatalf("Create() err = %v: the role chain was read although the resolver refused the profile", err)
	}
}

// The next stage RecordStage admits is the one the resolver's profile declares, for a workflow of
// version 1, whose profile is the catalog's.
func TestLifecycleRecordStageFollowsTheResolversProfile(t *testing.T) {
	store := newMemEventLog()
	goals := newFakeGoalReader()
	chains := newFakeChainReader()
	lc := newTestLifecycle(t, store, stepClock(), goals, chains, nil)
	g := validGoal("proj-1", "goal-1")
	createVersion1Workflow(t, store, "proj-1", "wf-1", g, "maintenance")
	if _, err := lc.Start("proj-1", "wf-1"); err != nil {
		t.Fatalf("Start() = %v, want nil", err)
	}

	builtIn, err := workflowprofile.Resolve("maintenance")
	if err != nil {
		t.Fatal(err)
	}
	lc.profiles = ProfileCatalogFunc(func(name string) (workflowprofile.WorkflowProfile, error) {
		custom := builtIn
		custom.Stages = append([]workflowprofile.Stage{{Name: "custom-first"}}, builtIn.Stages...)
		return custom, nil
	})
	state, err := lc.RecordStage("proj-1", "wf-1", "custom-first")
	if err != nil {
		t.Fatalf("RecordStage(custom-first) = %v, want nil: the resolver's profile declares it first", err)
	}
	if len(state.Stages) != 1 || state.Stages[0] != "custom-first" {
		t.Fatalf("state.Stages = %v, want [custom-first]", state.Stages)
	}
}
