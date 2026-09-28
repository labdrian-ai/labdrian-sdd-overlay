package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type memoryRun struct {
	code   int
	stdout string
	stderr string
}

func runMemoryTest(args []string) memoryRun {
	var out, errBuf bytes.Buffer
	code := -1
	exited := false
	runMemoryCore(args, &out, &errBuf, func(c int) {
		if !exited {
			code = c
			exited = true
		}
	})
	if !exited {
		code = 0
	}
	return memoryRun{code: code, stdout: out.String(), stderr: errBuf.String()}
}

func writeMemoryTestFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func memoryTestGoalJSON(projectID, goalID string) string {
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

func memoryTestDirectiveJSON(scope string, sources ...string) string {
	if sources == nil {
		sources = []string{}
	}
	sourcesJSON, _ := json.Marshal(sources)
	return `{"version":1,"scope":"` + scope + `","sources":` + string(sourcesJSON) + `,"write":"none"}`
}

func TestRunMemoryCoreRequiresVerb(t *testing.T) {
	r := runMemoryTest(nil)
	if r.code != 1 {
		t.Fatalf("code = %d, want 1", r.code)
	}
}

func TestRunMemoryCoreUnknownVerb(t *testing.T) {
	r := runMemoryTest([]string{"astronaut"})
	if r.code != 1 || !strings.Contains(r.stderr, "unknown verb") {
		t.Fatalf("code=%d stderr=%q, want exit 1 with an unknown-verb message", r.code, r.stderr)
	}
}

func TestMemoryPlanRequiresProfileFlag(t *testing.T) {
	r := runMemoryTest([]string{"plan"})
	if r.code != 1 || !strings.Contains(r.stderr, "--profile is required") {
		t.Fatalf("code=%d stderr=%q, want exit 1 requiring --profile", r.code, r.stderr)
	}
}

func TestMemoryPlanRejectsUnknownFlag(t *testing.T) {
	r := runMemoryTest([]string{"plan", "--bogus", "x"})
	if r.code != 1 {
		t.Fatalf("code = %d, want 1", r.code)
	}
}

func TestMemoryPlanStandaloneMinimalDefaultIsNoneWithoutGoal(t *testing.T) {
	r := runMemoryTest([]string{"plan", "--profile", "standalone-minimal"})
	if r.code != 0 {
		t.Fatalf("code=%d stderr=%q, want exit 0", r.code, r.stderr)
	}
	var plan struct {
		Scope   string   `json:"scope"`
		Sources []string `json:"sources"`
		Write   string   `json:"write"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &plan); err != nil {
		t.Fatalf("unmarshal stdout: %v\nstdout=%s", err, r.stdout)
	}
	if plan.Scope != "none" || len(plan.Sources) != 0 || plan.Write != "none" {
		t.Errorf("plan = %+v, want scope=none, no sources, write=none", plan)
	}
}

func TestMemoryPlanIncidentRecoveryDefaultRequiresGoalForFilters(t *testing.T) {
	dir := t.TempDir()
	goalPath := writeMemoryTestFile(t, dir, "goal.json", memoryTestGoalJSON("proj-1", "goal-1"))

	r := runMemoryTest([]string{"plan", "--profile", "incident-recovery", "--goal", goalPath})
	if r.code != 0 {
		t.Fatalf("code=%d stderr=%q, want exit 0", r.code, r.stderr)
	}
	var plan struct {
		Scope   string `json:"scope"`
		Filters struct {
			ProjectID string `json:"project_id"`
			GoalID    string `json:"goal_id"`
		} `json:"filters"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &plan); err != nil {
		t.Fatalf("unmarshal stdout: %v\nstdout=%s", err, r.stdout)
	}
	if plan.Scope != "goal" || plan.Filters.ProjectID != "proj-1" || plan.Filters.GoalID != "goal-1" {
		t.Errorf("plan = %+v, want scope=goal with proj-1/goal-1 filters", plan)
	}
}

func TestMemoryPlanOddDefaultWithoutGoalRefusesForMissingProjectID(t *testing.T) {
	r := runMemoryTest([]string{"plan", "--profile", "odd"})
	if r.code != 2 {
		t.Fatalf("code=%d stderr=%q, want exit 2", r.code, r.stderr)
	}
}

func TestMemoryPlanNarrowsWithGoalAndHandoffDirectives(t *testing.T) {
	dir := t.TempDir()
	goalPath := writeMemoryTestFile(t, dir, "goal.json", memoryTestGoalJSON("proj-1", "goal-1"))
	goalDirectivePath := writeMemoryTestFile(t, dir, "goal-directive.json", memoryTestDirectiveJSON("goal", "engram"))
	handoffDirectivePath := writeMemoryTestFile(t, dir, "handoff-directive.json", memoryTestDirectiveJSON("none"))

	r := runMemoryTest([]string{
		"plan", "--profile", "odd",
		"--goal", goalPath,
		"--goal-directive", goalDirectivePath,
		"--handoff-directive", handoffDirectivePath,
	})
	if r.code != 0 {
		t.Fatalf("code=%d stderr=%q, want exit 0", r.code, r.stderr)
	}
	var plan struct {
		Scope   string   `json:"scope"`
		Sources []string `json:"sources"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &plan); err != nil {
		t.Fatalf("unmarshal stdout: %v\nstdout=%s", err, r.stdout)
	}
	if plan.Scope != "none" || len(plan.Sources) != 0 {
		t.Errorf("plan = %+v, want the handoff directive's tighter scope none to win", plan)
	}
}

func TestMemoryPlanRefusesWideningNarrower(t *testing.T) {
	dir := t.TempDir()
	goalPath := writeMemoryTestFile(t, dir, "goal.json", memoryTestGoalJSON("proj-1", "goal-1"))
	wideningDirectivePath := writeMemoryTestFile(t, dir, "widening.json", memoryTestDirectiveJSON("project"))

	r := runMemoryTest([]string{
		"plan", "--profile", "incident-recovery",
		"--goal", goalPath,
		"--goal-directive", wideningDirectivePath,
	})
	if r.code != 2 || !strings.Contains(r.stderr, "widens scope") {
		t.Fatalf("code=%d stderr=%q, want exit 2 naming the widening", r.code, r.stderr)
	}
}

func TestMemoryPlanOddReachesProceduralSkillsThroughANarrower(t *testing.T) {
	dir := t.TempDir()
	goalPath := writeMemoryTestFile(t, dir, "goal.json", memoryTestGoalJSON("proj-1", "goal-1"))
	goalDirectivePath := writeMemoryTestFile(t, dir, "goal-directive.json", memoryTestDirectiveJSON("goal", "procedural-skills"))

	r := runMemoryTest([]string{
		"plan", "--profile", "odd",
		"--goal", goalPath,
		"--goal-directive", goalDirectivePath,
	})
	if r.code != 0 {
		t.Fatalf("code=%d stderr=%q, want exit 0", r.code, r.stderr)
	}
	var plan struct {
		Scope   string   `json:"scope"`
		Sources []string `json:"sources"`
		Filters struct {
			ProjectID string `json:"project_id"`
			GoalID    string `json:"goal_id"`
		} `json:"filters"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &plan); err != nil {
		t.Fatalf("unmarshal stdout: %v\nstdout=%s", err, r.stdout)
	}
	if plan.Scope != "goal" || len(plan.Sources) != 1 || plan.Sources[0] != "procedural-skills" ||
		plan.Filters.ProjectID != "proj-1" || plan.Filters.GoalID != "goal-1" {
		t.Errorf("plan = %+v, want scope=goal, sources=[procedural-skills], proj-1/goal-1 filters", plan)
	}
}

type failingMemoryWriter struct{}

func (failingMemoryWriter) Write([]byte) (int, error) {
	return 0, errors.New("broken pipe")
}

func TestMemoryPlanReportsFailedStdoutWrite(t *testing.T) {
	var errBuf bytes.Buffer
	code := -1
	runMemoryCore([]string{"plan", "--profile", "standalone-minimal"}, failingMemoryWriter{}, &errBuf, func(c int) {
		if code == -1 {
			code = c
		}
	})
	if code != 1 || !strings.Contains(errBuf.String(), "broken pipe") {
		t.Fatalf("code=%d stderr=%q, want exit 1 naming the failed write", code, errBuf.String())
	}
}

func TestMemoryPlanRejectsUnknownProfile(t *testing.T) {
	r := runMemoryTest([]string{"plan", "--profile", "nonexistent"})
	if r.code != 2 {
		t.Fatalf("code=%d stderr=%q, want exit 2", r.code, r.stderr)
	}
}

func TestMemoryPlanRejectsMissingGoalFile(t *testing.T) {
	r := runMemoryTest([]string{"plan", "--profile", "odd", "--goal", "/nonexistent/goal.json"})
	if r.code != 2 {
		t.Fatalf("code=%d stderr=%q, want exit 2", r.code, r.stderr)
	}
}

// A --goal file that reads fine but fails Goal v2 schema/semantic parsing
// (here: a blank objective) must be refused with exit 2, distinct from a
// missing file.
func TestMemoryPlanRejectsGoalFileFailingParse(t *testing.T) {
	dir := t.TempDir()
	goalPath := writeMemoryTestFile(t, dir, "goal.json", `{
  "version": 2,
  "project_id": "proj-1",
  "goal_id": "goal-1",
  "objective": "",
  "scope": "scope",
  "constraints": [],
  "non_goals": [],
  "acceptance_criteria": ["done"],
  "memory_scope": "goal",
  "runtime_scope": "local",
  "delivery_boundary": "local"
}`)

	r := runMemoryTest([]string{"plan", "--profile", "odd", "--goal", goalPath})
	if r.code != 2 || !strings.Contains(r.stderr, "parse --goal") {
		t.Fatalf("code=%d stderr=%q, want exit 2 naming a --goal parse failure", r.code, r.stderr)
	}
}

func TestMemoryPlanRejectsMissingGoalDirectiveFile(t *testing.T) {
	r := runMemoryTest([]string{
		"plan", "--profile", "incident-recovery",
		"--goal-directive", "/nonexistent/goal-directive.json",
	})
	if r.code != 2 || !strings.Contains(r.stderr, "read --goal-directive") {
		t.Fatalf("code=%d stderr=%q, want exit 2 naming a --goal-directive read failure", r.code, r.stderr)
	}
}

func TestMemoryPlanRejectsMissingHandoffDirectiveFile(t *testing.T) {
	r := runMemoryTest([]string{
		"plan", "--profile", "incident-recovery",
		"--handoff-directive", "/nonexistent/handoff-directive.json",
	})
	if r.code != 2 || !strings.Contains(r.stderr, "read --handoff-directive") {
		t.Fatalf("code=%d stderr=%q, want exit 2 naming a --handoff-directive read failure", r.code, r.stderr)
	}
}

func TestMemoryPlanRejectsGoalDirectiveFileFailingParse(t *testing.T) {
	dir := t.TempDir()
	malformedPath := writeMemoryTestFile(t, dir, "goal-directive.json", `{"version":1,"scope":"bogus","sources":[],"write":"none"}`)

	r := runMemoryTest([]string{
		"plan", "--profile", "incident-recovery",
		"--goal-directive", malformedPath,
	})
	if r.code != 2 || !strings.Contains(r.stderr, "parse --goal-directive") {
		t.Fatalf("code=%d stderr=%q, want exit 2 naming a --goal-directive parse failure", r.code, r.stderr)
	}
}

func TestMemoryPlanRejectsHandoffDirectiveFileFailingParse(t *testing.T) {
	dir := t.TempDir()
	malformedPath := writeMemoryTestFile(t, dir, "handoff-directive.json", `{"version":1,"scope":"bogus","sources":[],"write":"none"}`)

	r := runMemoryTest([]string{
		"plan", "--profile", "incident-recovery",
		"--handoff-directive", malformedPath,
	})
	if r.code != 2 || !strings.Contains(r.stderr, "parse --handoff-directive") {
		t.Fatalf("code=%d stderr=%q, want exit 2 naming a --handoff-directive parse failure", r.code, r.stderr)
	}
}

// A value flag given as the last, valueless argument must be refused with
// exit 1, for every flag that takes a value.
func TestMemoryPlanRejectsValueFlagWithoutValue(t *testing.T) {
	for _, flag := range []string{"--profile", "--goal", "--goal-directive", "--handoff-directive"} {
		t.Run(flag, func(t *testing.T) {
			r := runMemoryTest([]string{"plan", flag})
			if r.code != 1 || !strings.Contains(r.stderr, flag+" requires a value") {
				t.Fatalf("code=%d stderr=%q, want exit 1 naming %q requires a value", r.code, r.stderr, flag)
			}
		})
	}
}

func TestMemoryPlanOutputNeverMentionsAQuery(t *testing.T) {
	r := runMemoryTest([]string{"plan", "--profile", "standalone-minimal"})
	if r.code != 0 {
		t.Fatalf("code=%d stderr=%q, want exit 0", r.code, r.stderr)
	}
	if !strings.Contains(r.stdout, "\"write\": \"none\"") {
		t.Errorf("stdout=%q, want it to state write is none", r.stdout)
	}
	if !strings.Contains(r.stdout, "executes no query") {
		t.Errorf("stdout=%q, want the no-execution authority statement", r.stdout)
	}
}
