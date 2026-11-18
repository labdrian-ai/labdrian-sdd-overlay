package enginetest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// GoalJSON is a goal file of the project, as `workflow create --goal` reads it.
func GoalJSON(projectID, goalID string) string {
	return `{
  "version": 2,
  "project_id": "` + projectID + `",
  "goal_id": "` + goalID + `",
  "objective": "obj",
  "scope": "scope",
  "constraints": [],
  "non_goals": [],
  "acceptance_criteria": ["done"],
  "memory_scope": "goal",
  "runtime_scope": "local",
  "delivery_boundary": "local"
}`
}

// BindRunning creates a workflow of project in the state of env, starts it, records the stages, and
// binds repo to it, every step in its own process of the engine binary. workDir is where the verbs
// other than the binding run and where the goal file is written; it must exist.
func BindRunning(t testing.TB, binary string, env []string, workDir, repo, project, workflow, profile string, stages ...string) {
	t.Helper()
	goal := filepath.Join(workDir, project+"-"+workflow+"-goal.json")
	if err := os.WriteFile(goal, []byte(GoalJSON(project, "goal-1")), 0o600); err != nil {
		t.Fatal(err)
	}
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command(binary, args...)
		cmd.Dir = dir
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("engine %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	verb := func(name string, flags ...string) {
		t.Helper()
		run(workDir, append([]string{"workflow", name, "--project", project, "--workflow", workflow}, flags...)...)
	}
	verb("create", "--goal", goal, "--profile", profile)
	verb("start")
	for _, stage := range stages {
		verb("stage", "--stage", stage)
	}
	run(repo, "workflow", "bind", "--project", project, "--workflow", workflow)
}
