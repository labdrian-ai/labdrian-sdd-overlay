package main

import "testing"

// runningUnderGoal creates a running odd workflow whose Goal (version 3) narrows its plan to scope
// goal over longterm-mem, goal goal-1 of project, and binds the repository to it.
func (e hookEnv) runningUnderGoal(t *testing.T, project, wf string) {
	t.Helper()
	goal := writeMemoryTestFile(t, e.dir, project+"-"+wf+"-goal.json",
		memoryTestGoalV3JSON(project, "goal-1", directiveJSONOf("goal", "longterm-mem")))
	phase6MustExitZeroWith(t, e.deps, "create", []string{"create", "--project", project, "--workflow", wf, "--goal", goal, "--profile", "odd"}, e.dir)
	e.step(t, project, wf, "start")
	mustBindOK(t, e.repo, project, wf)
}

// queryArgsWithGoal is queryArgs with a goal argument, none when goal is nil.
func queryArgsWithGoal(project, goal any) map[string]any {
	in := queryArgs(project)
	if goal != nil {
		in["goal"] = goal
	}
	return in
}

// TestPreToolUseChecksTheGoalOfALongtermMemQueryUnderScopeGoal: under scope goal a query must name the
// plan's goal as well as its project (C23 M5 decision 3); the project check comes first.
func TestPreToolUseChecksTheGoalOfALongtermMemQueryUnderScopeGoal(t *testing.T) {
	for _, tool := range queryTools {
		t.Run(tool, func(t *testing.T) {
			e := newHookEnv(t)
			e.runningUnderGoal(t, "proj-1", "wf-1")
			assertSilent(t, "the plan's project and goal", e.gate(t, e.repo, tool, queryArgsWithGoal("proj-1", "goal-1")))
			assertDenied(t, "no goal", e.gate(t, e.repo, tool, queryArgsWithGoal("proj-1", nil)),
				"names no goal", `"goal-1"`, `Query project "proj-1" with goal "goal-1"`, "labdrian workflow unbind")
			assertDenied(t, "another goal", e.gate(t, e.repo, tool, queryArgsWithGoal("proj-1", "goal-2")),
				`asks for goal "goal-2"`, `permits only goal "goal-1"`)
			assertDenied(t, "a goal of the wrong type", e.gate(t, e.repo, tool, queryArgsWithGoal("proj-1", 7)), "names no goal")
			assertDenied(t, "another project", e.gate(t, e.repo, tool, queryArgsWithGoal("proj-2", "goal-1")),
				`"proj-2"`, `"proj-1"`, "Query project", "labdrian workflow unbind")
		})
	}
}

// TestPreToolUseNeverChecksTheGoalOfAQueryUnderScopeProject: odd is scope project, so the goal
// argument passes unchecked.
func TestPreToolUseNeverChecksTheGoalOfAQueryUnderScopeProject(t *testing.T) {
	e := newHookEnv(t)
	e.running(t, "proj-1", "wf-1", "odd")
	for _, goal := range []any{nil, "goal-2", "goal-1", 7} {
		assertSilent(t, "scope project", e.gate(t, e.repo, "mcp__longterm-mem__query", queryArgsWithGoal("proj-1", goal)))
	}
}
