package projection_test

// The goal check of a longterm-mem query (C23 M5 decision 3, docs/architecture/adr-c23-m5-goal-filter.md):
// under scope goal a query must name the plan's goal as well as its project; under any other scope its
// goal argument passes unchecked. The project check comes first, so a query to another project is
// denied for its project whatever goal it names.

import (
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/memoryscope"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// goalScoped is a running odd workflow whose Goal narrows its plan to scope goal over longterm-mem:
// the plan's project is proj-1 and its goal goal-1 (loadedWorkflow's).
func goalScoped() *workflow.Loaded {
	return narrowedWorkflow(narrowerDirective(memoryscope.ScopeGoal, memoryscope.SourceLongtermMem), nil, nil)
}

// namedWithGoal is a query that names project and goal ("" when it names none: missing, null, not
// text, empty).
func namedWithGoal(project, goal string) projection.QueryArguments {
	return projection.QueryArguments{Named: true, Project: project, Goal: goal}
}

func TestGateComparesTheQueryGoalWithThePlanGoalUnderScopeGoal(t *testing.T) {
	w := goalScoped()
	for _, tool := range []string{queryTool, pluginQueryTool} {
		allowed(t, tool+" the plan's project and goal", gate(w, tool, namedWithGoal("proj-1", "goal-1")))
		denied(t, tool+" no goal", gate(w, tool, namedWithGoal("proj-1", "")),
			"names no goal", "the goal argument is missing, not a string, or empty", `"goal-1"`, "Query project \"proj-1\" with goal \"goal-1\"", "labdrian workflow unbind")
		denied(t, tool+" another goal", gate(w, tool, namedWithGoal("proj-1", "goal-2")),
			`asks for goal "goal-2"`, `permits only goal "goal-1"`, "labdrian workflow unbind")
		denied(t, tool+" a goal that differs only in case", gate(w, tool, namedWithGoal("proj-1", "GOAL-1")), `"GOAL-1"`, `"goal-1"`)
		denied(t, tool+" a goal that differs by whitespace", gate(w, tool, namedWithGoal("proj-1", "goal-1 ")), `"goal-1 "`, `"goal-1"`)
	}
}

// The project check comes first: a query to another project, or to none, is denied for its project,
// whatever goal it names, with the project denial unchanged.
func TestTheProjectCheckComesBeforeTheGoalCheck(t *testing.T) {
	w := goalScoped()
	for _, goal := range []string{"goal-1", "goal-2", ""} {
		got := gate(w, queryTool, namedWithGoal("proj-2", goal))
		denied(t, "another project, goal "+goal, got, `asks for project "proj-2"`, `"proj-1"`, "Query project")
		if strings.Contains(got.Reason, "goal") {
			t.Errorf("another project, goal %q: reason %q speaks of the goal, want the project denial alone", goal, got.Reason)
		}
		denied(t, "no project, goal "+goal, gate(w, queryTool, namedWithGoal("", goal)), "names no project", `"proj-1"`)
	}
}

// Under any scope but goal the goal argument passes unchecked: missing, another, or the workflow's.
func TestTheGoalOfAQueryIsNotCheckedUnderAnotherScope(t *testing.T) {
	w := loadedWorkflow("odd", workflow.StatusRunning) // scope project
	for _, goal := range []string{"", "goal-2", "goal-1"} {
		allowed(t, "scope project, goal "+goal, gate(w, queryTool, namedWithGoal("proj-1", goal)))
	}
	// Scope none still denies every query, for the plan, not the goal.
	minimal := loadedWorkflow("standalone-minimal", workflow.StatusRunning)
	denied(t, "scope none", gate(minimal, queryTool, namedWithGoal("proj-1", "goal-1")), "permits no project")
}

// Arguments that are not named cannot be read: the gate allows the query with the one warning it
// gives today, under scope goal too, and checks neither project nor goal.
func TestAQueryWhoseArgumentsCannotBeReadIsAllowedUnderScopeGoal(t *testing.T) {
	got := gate(goalScoped(), queryTool, unnamed)
	if got.Deny || got.Reason != "" || !strings.Contains(got.Warning, "not a JSON object") || !strings.Contains(got.Warning, "not checked") {
		t.Errorf("Gate() = %+v, want an allow with the one-line warning that the query was not checked", got)
	}
}

// A goal-scoped plan whose goal is somehow empty cannot be checked: an allow with a warning, never a
// denial on what the gate cannot explain. Resolve never makes such a plan; this is the gate staying
// total, as for a plan with no project.
func TestTheGoalCheckOfAPlanWithNoGoalIsAnAllow(t *testing.T) {
	w := goalScoped()
	w.State.GoalID = ""
	got := gate(w, queryTool, namedWithGoal("proj-1", "goal-2"))
	if got.Deny || got.Warning == "" {
		t.Errorf("Gate() = %+v, want an allow with a warning", got)
	}
}
