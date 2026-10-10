package projection_test

import (
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflowprofile"
)

// A workflow whose log carries a snapshot of its profile (version 2) is projected and gated by that
// snapshot, not by the catalog: the context states the next stage and the memory plan the snapshot
// declares, and the gate holds a memory query to that plan, so the three agree with the lifecycle
// (workflow.ProfileOf), which goes on from the snapshot. A workflow whose log has none is projected by
// the profile its name resolves to, which every other test of this package shows.

// snapshotProfile is a profile that is in no catalog: two stages, a goal-scoped memory default with a
// single source, no review dependency.
func snapshotProfile() workflowprofile.WorkflowProfile {
	return workflowprofile.WorkflowProfile{
		Name:                 "retired-profile",
		Stages:               []workflowprofile.Stage{{Name: "first", DependsOn: []string{}}, {Name: "second", DependsOn: []string{"first"}}},
		Roles:                []string{"role"},
		Checks:               []string{"check"},
		MemoryPolicy:         "memory",
		ReviewPolicy:         "review",
		DeliveryPolicy:       "delivery",
		MemoryDefault:        workflowprofile.MemoryDefault{Scope: workflowprofile.MemoryScopeGoal, Sources: []workflowprofile.MemorySource{workflowprofile.MemorySourceEngram}},
		ReliesOnGentleReview: false,
	}
}

// withSnapshot makes w the log of a workflow of version 2 whose profile is p.
func withSnapshot(w *workflow.Loaded, p workflowprofile.WorkflowProfile) *workflow.Loaded {
	snapshot := p.Snapshot()
	w.State.Version = workflow.EventVersionSnapshot
	w.State.Profile = p.Name
	w.State.ProfileSnapshot = &snapshot
	return w
}

func TestProjectContextTakesTheNextStageAndThePlanFromTheSnapshotOfAProfileThatIsInNoCatalog(t *testing.T) {
	ctx := project(ownedBinding(), withSnapshot(loadedWorkflow("", workflow.StatusRunning, "first"), snapshotProfile())).Context
	for _, want := range []string{
		"profile: retired-profile",
		"next stage: second",
		"memory plan (read-only: it executes no query and grants no memory write): scope=goal sources=engram project_id=proj-1 goal_id=goal-1 write=none",
	} {
		if !hasLine(ctx, want) {
			t.Errorf("no line %q in:\n%s", want, ctx)
		}
	}
	if strings.Contains(ctx, "not available") {
		t.Errorf("the context says something is not available although the snapshot has all of it:\n%s", ctx)
	}
	done := project(ownedBinding(), withSnapshot(loadedWorkflow("", workflow.StatusRunning, "first", "second"), snapshotProfile())).Context
	if !hasLine(done, "next stage: none: every declared stage is recorded") {
		t.Errorf("no 'every declared stage is recorded' line in:\n%s", done)
	}
}

// The snapshot wins over a catalog profile of the same name, which is what a catalog that has changed
// since the workflow was created is.
func TestProjectContextPrefersTheSnapshotToTheCatalogsProfileOfTheSameName(t *testing.T) {
	changed, err := workflowprofile.Resolve("odd")
	if err != nil {
		t.Fatal(err)
	}
	changed.Stages = []workflowprofile.Stage{{Name: "only", DependsOn: []string{}}}
	changed.MemoryDefault = workflowprofile.MemoryDefault{Scope: workflowprofile.MemoryScopeNone, Sources: []workflowprofile.MemorySource{}}
	ctx := project(ownedBinding(), withSnapshot(loadedWorkflow("odd", workflow.StatusRunning), changed)).Context
	for _, want := range []string{
		"next stage: only",
		"memory plan (read-only: it executes no query and grants no memory write): scope=none sources=none project_id=none goal_id=none write=none",
	} {
		if !hasLine(ctx, want) {
			t.Errorf("no line %q in:\n%s", want, ctx)
		}
	}
	if hasLine(ctx, "next stage: authorize") {
		t.Errorf("the next stage is the catalog's, not the snapshot's:\n%s", ctx)
	}
}

func TestProjectContextStillSaysWhenTheRecordedStagesDoNotFollowTheSnapshot(t *testing.T) {
	ctx := project(ownedBinding(), withSnapshot(loadedWorkflow("", workflow.StatusRunning, "second"), snapshotProfile())).Context
	line := lineWithPrefix(t, ctx, "next stage:")
	if !strings.Contains(line, "not available") || !strings.Contains(line, "retired-profile") {
		t.Errorf("next stage line %q does not say the recorded stages do not follow the declared order of the profile", line)
	}
}

func TestGateHoldsAMemoryQueryToThePlanOfTheSnapshot(t *testing.T) {
	// The catalog's odd would permit proj-1; the snapshot's profile permits no project at all.
	none, err := workflowprofile.Resolve("odd")
	if err != nil {
		t.Fatal(err)
	}
	none.MemoryDefault = workflowprofile.MemoryDefault{Scope: workflowprofile.MemoryScopeNone, Sources: []workflowprofile.MemorySource{}}
	denied(t, "a snapshot with scope none", gate(withSnapshot(loadedWorkflow("odd", workflow.StatusRunning), none), queryTool, named("proj-1")), "scope none")

	// A profile in no catalog is gated by its snapshot: the plan's project is allowed, another is not,
	// and nothing is left unchecked.
	w := withSnapshot(loadedWorkflow("", workflow.StatusRunning), snapshotProfile())
	if got := gate(w, queryTool, named("proj-1")); got.Deny || got.Warning != "" {
		t.Errorf("a query to the plan's project: Gate() = %+v, want an allow without a warning", got)
	}
	denied(t, "another project", gate(w, queryTool, named("proj-2")), "proj-1")
}
