package projection_test

import (
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/memoryscope"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// The context line and the memory gate state and enforce the plan the log's narrowers make
// (workflow.PlanOf, C23 M3): the Goal's and the shaper handoff's directives, then the role directive
// in force. A narrower that the plan cannot apply is a plan that cannot be computed, which the
// context says and the gate allows with a warning, as before.

func narrowerDirective(scope memoryscope.Scope, sources ...memoryscope.Source) *memoryscope.Directive {
	if sources == nil {
		sources = []memoryscope.Source{}
	}
	return &memoryscope.Directive{Version: 1, Scope: scope, Sources: sources, Write: "none"}
}

// narrowedWorkflow is a running version 3 workflow of the odd profile with the given narrowers.
func narrowedWorkflow(goal, handoff, role *memoryscope.Directive) *workflow.Loaded {
	w := loadedWorkflow("odd", workflow.StatusRunning)
	w.State.Version = workflow.EventVersionNarrowers
	w.State.GoalDirective = goal
	w.State.HandoffDirective = handoff
	w.State.RoleDirective = role
	return w
}

const planPrefix = "memory plan (read-only: it executes no query and grants no memory write): "

func TestTheContextStatesThePlanTheStoredNarrowersMake(t *testing.T) {
	eng, ltm := memoryscope.SourceEngram, memoryscope.SourceLongtermMem
	tests := []struct {
		name string
		w    *workflow.Loaded
		want string
	}{
		{"no narrower", narrowedWorkflow(nil, nil, nil), "scope=project sources=engram,longterm-mem,procedural-skills project_id=proj-1 goal_id=none write=none"},
		{"the goal", narrowedWorkflow(narrowerDirective(memoryscope.ScopeProject, eng, ltm), nil, nil), "scope=project sources=engram,longterm-mem project_id=proj-1 goal_id=none write=none"},
		{"the goal and the shaper handoff", narrowedWorkflow(narrowerDirective(memoryscope.ScopeProject, eng, ltm), narrowerDirective(memoryscope.ScopeGoal, ltm), nil),
			"scope=goal sources=longterm-mem project_id=proj-1 goal_id=goal-1 write=none"},
		{"a narrowed role over the base", func() *workflow.Loaded {
			w := narrowedWorkflow(narrowerDirective(memoryscope.ScopeProject, eng, ltm), nil, narrowerDirective(memoryscope.ScopeNone))
			w.State.NarrowedHead = hex64("2")
			w.State.NarrowedDirective = narrowerDirective(memoryscope.ScopeProject, eng)
			return w
		}(), "scope=project sources=engram project_id=proj-1 goal_id=none write=none"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := project(ownedBinding(), tt.w).Context
			if !hasLine(ctx, planPrefix+tt.want) {
				t.Errorf("no line %q in:\n%s", planPrefix+tt.want, ctx)
			}
		})
	}

	widening := narrowedWorkflow(nil, nil, narrowerDirective(memoryscope.ScopeProject, eng))
	widening.State.Profile = "sdd"
	widening.State.GoalDirective = narrowerDirective(memoryscope.ScopeGoal, eng)
	line := lineWithPrefix(t, project(ownedBinding(), widening).Context, "memory plan:")
	if line != "memory plan: not available: the role handoff memory directive widens the plan: widens scope from goal to project" {
		t.Errorf("the plan line of a log whose narrower widens is %q", line)
	}
}

func TestTheGateEnforcesThePlanTheStoredNarrowersMake(t *testing.T) {
	// The ceiling of odd permits proj-1; the role directive narrows the plan to scope none.
	none := narrowedWorkflow(narrowerDirective(memoryscope.ScopeProject, memoryscope.SourceEngram), nil, narrowerDirective(memoryscope.ScopeNone))
	denied(t, "a role directive of scope none", gate(none, queryTool, named("proj-1")), "scope none")

	// A later role handoff with no directive gets back to the base, which permits proj-1 from
	// longterm-mem.
	back := narrowedWorkflow(narrowerDirective(memoryscope.ScopeProject, memoryscope.SourceEngram, memoryscope.SourceLongtermMem), nil, narrowerDirective(memoryscope.ScopeNone))
	back.State.NarrowedHead = hex64("2")
	if got := gate(back, queryTool, named("proj-1")); got.Deny || got.Warning != "" {
		t.Errorf("after a role with no directive: Gate() = %+v, want an allow without a warning", got)
	}
	// The sources a narrower keeps are enforced too: a goal directive of engram alone denies the
	// query (C23 M4, decision 1).
	engramOnly := narrowedWorkflow(narrowerDirective(memoryscope.ScopeProject, memoryscope.SourceEngram), nil, nil)
	denied(t, "a goal directive of engram alone", gate(engramOnly, queryTool, named("proj-1")), `does not include source "longterm-mem"`)

	// A narrower the plan cannot apply: the gate allows, and says it could not check.
	widening := narrowedWorkflow(narrowerDirective(memoryscope.ScopeGoal, memoryscope.SourceEngram), narrowerDirective(memoryscope.ScopeProject, memoryscope.SourceEngram), nil)
	got := gate(widening, queryTool, named("proj-2"))
	if got.Deny || !strings.Contains(got.Warning, "the shaper handoff memory directive widens the plan") {
		t.Errorf("a widening narrower: Gate() = %+v, want an allow warning that names the shaper handoff", got)
	}
}
