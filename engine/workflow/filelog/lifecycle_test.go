package filelog

import (
	"errors"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/goal"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/roles"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflowprofile"
)

// The lifecycle's own tests run it over an in-memory log built on the same pure
// rules as this one; this runs it over the real file, so that what the lifecycle
// does and what the file keeps are proven together, in a fresh process (a new
// Store and a new Lifecycle) for every step.

type oneGoal struct{ g goal.Goal }

func (o oneGoal) LoadGoal(projectID, goalID string) (goal.Goal, error) {
	if projectID != o.g.ProjectID || goalID != o.g.GoalID {
		return goal.Goal{}, errors.New("no such goal")
	}
	return o.g, nil
}

type noChains struct{}

func (noChains) LoadChain(projectID, goalID, chainID string) ([]roles.ChainRecord, error) {
	return nil, nil
}

func TestALifecycleRunsToCompletionOverTheFileLogAcrossRestarts(t *testing.T) {
	root := setStoreEnv(t)
	g := goal.Goal{
		Version: 2, ProjectID: "proj-1", GoalID: "goal-1", Objective: "ship the thing", Scope: "bounded scope",
		Constraints: []string{}, NonGoals: []string{}, AcceptanceCriteria: []string{"it works"},
		MemoryScope: "none", RuntimeScope: "none", DeliveryBoundary: "local",
	}
	profile, err := workflowprofile.Resolve("standalone-minimal")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	clock := func() time.Time { now = now.Add(time.Minute); return now }
	provenance := workflow.Provenance{WorktreeRoot: "/home/labdrian/labdrian-sdd-overlay", GitHead: "e1218c2f00000000000000000000000000000000"}

	// Every call builds its own Store and Lifecycle, as every command does.
	lifecycle := func() workflow.Lifecycle {
		store, err := NewStore(root)
		if err != nil {
			t.Fatal(err)
		}
		lc, err := workflow.NewLifecycle(store, workflow.ProfileCatalogFunc(workflowprofile.Resolve), clock, provenance,
			oneGoal{g}, noChains{}, workflow.UnavailableProber{})
		if err != nil {
			t.Fatal(err)
		}
		return lc
	}

	steps := []struct {
		name string
		do   func(workflow.Lifecycle) (workflow.State, error)
	}{
		{"create", func(lc workflow.Lifecycle) (workflow.State, error) {
			return lc.Create("proj-1", "wf-1", g, profile.Name, "")
		}},
		{"start", func(lc workflow.Lifecycle) (workflow.State, error) { return lc.Start("proj-1", "wf-1") }},
	}
	for _, stage := range profile.Stages {
		stage := stage.Name
		steps = append(steps, struct {
			name string
			do   func(workflow.Lifecycle) (workflow.State, error)
		}{"stage " + stage, func(lc workflow.Lifecycle) (workflow.State, error) { return lc.RecordStage("proj-1", "wf-1", stage) }})
	}
	steps = append(steps, []struct {
		name string
		do   func(workflow.Lifecycle) (workflow.State, error)
	}{
		{"verify", func(lc workflow.Lifecycle) (workflow.State, error) { return lc.Verify("proj-1", "wf-1") }},
		{"close", func(lc workflow.Lifecycle) (workflow.State, error) {
			return lc.Close("proj-1", "wf-1", workflow.OutcomeCompleted, "")
		}},
	}...)
	for _, step := range steps {
		if _, err := step.do(lifecycle()); err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
	}

	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load("proj-1", "wf-1")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Classification != workflow.ClassificationOwned || loaded.State.Status != workflow.StatusClosed ||
		loaded.State.CloseOutcome != workflow.OutcomeCompleted || len(loaded.Events) != len(steps) {
		t.Errorf("log = %q with %d events, state %+v; want the closed, completed workflow of %d events, owned", loaded.Classification, len(loaded.Events), loaded.State, len(steps))
	}

	// A closed workflow takes nothing more, and the refusal leaves the log alone.
	if _, err := lifecycle().Start("proj-1", "wf-1"); err == nil {
		t.Error("Start of a closed workflow = nil, want a refusal")
	}
	again, err := store.Load("proj-1", "wf-1")
	if err != nil || len(again.Events) != len(steps) {
		t.Errorf("after the refused Start the log holds %d events (%v), want %d", len(again.Events), err, len(steps))
	}
}
