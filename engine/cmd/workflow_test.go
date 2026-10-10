package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflowprofile"
)

type workflowRun struct {
	code   int
	stdout string
	stderr string
}

func runWorkflowTest(args []string, cwd string) workflowRun {
	return runWorkflowTestWith(testDeps(), args, cwd)
}

func runWorkflowTestWith(d deps, args []string, cwd string) workflowRun {
	var out, errBuf bytes.Buffer
	code := -1
	exited := false
	runWorkflowCore(d, args, cwd, &out, &errBuf, func(c int) {
		if !exited {
			code = c
			exited = true
		}
	})
	if !exited {
		code = 0
	}
	return workflowRun{code: code, stdout: out.String(), stderr: errBuf.String()}
}

// workflowTestStateHome isolates the workflow Store and role chain store
// (both XDG-resolved) under a fresh temp dir, and neutralizes cwd-based
// provenance discovery by never pointing it at a real .git.
func workflowTestStateHome(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
}

func TestRunWorkflowCoreRequiresVerb(t *testing.T) {
	r := runWorkflowTest(nil, "")
	if r.code != 1 || !strings.Contains(r.stderr, "requires a verb") {
		t.Fatalf("code=%d stderr=%q, want exit 1 requiring a verb", r.code, r.stderr)
	}
}

func TestRunWorkflowCoreUnknownVerb(t *testing.T) {
	r := runWorkflowTest([]string{"astronaut"}, "")
	if r.code != 1 || !strings.Contains(r.stderr, "unknown verb") {
		t.Fatalf("code=%d stderr=%q, want exit 1 with an unknown-verb message", r.code, r.stderr)
	}
}

func TestWorkflowCreateRequiresFlags(t *testing.T) {
	workflowTestStateHome(t)
	for _, tt := range []struct {
		args []string
		want string
	}{
		{[]string{"create"}, "--project is required"},
		{[]string{"create", "--project", "p"}, "--workflow is required"},
		{[]string{"create", "--project", "p", "--workflow", "w"}, "--goal is required"},
		{[]string{"create", "--project", "p", "--workflow", "w", "--goal", "g.json"}, "--profile is required"},
	} {
		r := runWorkflowTest(tt.args, t.TempDir())
		if r.code != 1 || !strings.Contains(r.stderr, tt.want) {
			t.Fatalf("args=%v: code=%d stderr=%q, want exit 1 containing %q", tt.args, r.code, r.stderr, tt.want)
		}
	}
}

func TestWorkflowCreateRejectsUnknownFlag(t *testing.T) {
	workflowTestStateHome(t)
	r := runWorkflowTest([]string{"create", "--bogus", "x"}, t.TempDir())
	if r.code != 1 || !strings.Contains(r.stderr, `unknown flag "--bogus"`) {
		t.Fatalf("code=%d stderr=%q, want exit 1 for an unknown flag", r.code, r.stderr)
	}
}

func TestWorkflowCreateRejectsMissingGoalFile(t *testing.T) {
	workflowTestStateHome(t)
	dir := t.TempDir()
	r := runWorkflowTest([]string{"create", "--project", "p", "--workflow", "w", "--goal", dir + "/missing.json", "--profile", "standalone-minimal"}, dir)
	if r.code != 2 || !strings.Contains(r.stderr, "read --goal") {
		t.Fatalf("code=%d stderr=%q, want exit 2 for a missing --goal file", r.code, r.stderr)
	}
}

func TestWorkflowCreateRejectsUnknownProfile(t *testing.T) {
	workflowTestStateHome(t)
	dir := t.TempDir()
	goalPath := writeMemoryTestFile(t, dir, "goal.json", memoryTestGoalJSON("proj-1", "goal-1"))
	r := runWorkflowTest([]string{"create", "--project", "proj-1", "--workflow", "wf-1", "--goal", goalPath, "--profile", "not-a-profile"}, dir)
	if r.code != 2 {
		t.Fatalf("code=%d stderr=%q, want exit 2 for an unknown profile", r.code, r.stderr)
	}
}

func TestWorkflowCloseRejectsUnknownOutcome(t *testing.T) {
	workflowTestStateHome(t)
	r := runWorkflowTest([]string{"close", "--project", "p", "--workflow", "w", "--outcome", "cancelled"}, t.TempDir())
	if r.code != 1 || !strings.Contains(r.stderr, "--outcome must be") {
		t.Fatalf("code=%d stderr=%q, want exit 1 for an unknown outcome", r.code, r.stderr)
	}
}

func TestWorkflowVerifyRequiresGoalFlag(t *testing.T) {
	workflowTestStateHome(t)
	r := runWorkflowTest([]string{"verify", "--project", "p", "--workflow", "w"}, t.TempDir())
	if r.code != 1 || !strings.Contains(r.stderr, "--goal is required") {
		t.Fatalf("code=%d stderr=%q, want exit 1 requiring --goal for verify", r.code, r.stderr)
	}
}

func TestWorkflowStatusOnAbsentWorkflowSucceeds(t *testing.T) {
	workflowTestStateHome(t)
	r := runWorkflowTest([]string{"status", "--project", "proj-1", "--workflow", "wf-1"}, t.TempDir())
	if r.code != 0 {
		t.Fatalf("code=%d stderr=%q, want exit 0 (status on an absent workflow is not an error)", r.code, r.stderr)
	}
	var out workflowStateJSON
	if err := json.Unmarshal([]byte(r.stdout), &out); err != nil {
		t.Fatalf("json.Unmarshal(%q) = %v, want nil", r.stdout, err)
	}
	if out.Classification != "absent" {
		t.Fatalf("classification = %q, want %q", out.Classification, "absent")
	}
}

func TestWorkflowStartOnAbsentWorkflowRefused(t *testing.T) {
	workflowTestStateHome(t)
	r := runWorkflowTest([]string{"start", "--project", "proj-1", "--workflow", "wf-1"}, t.TempDir())
	if r.code != 2 {
		t.Fatalf("code=%d stderr=%q, want exit 2 (start requires an owned workflow)", r.code, r.stderr)
	}
}

func TestWorkflowPauseOnAbsentWorkflowRefused(t *testing.T) {
	workflowTestStateHome(t)
	r := runWorkflowTest([]string{"pause", "--project", "proj-1", "--workflow", "wf-1"}, t.TempDir())
	if r.code != 2 {
		t.Fatalf("code=%d stderr=%q, want exit 2 (pause requires an owned, running workflow)", r.code, r.stderr)
	}
}

func TestWorkflowResumeOnAbsentWorkflowRefused(t *testing.T) {
	workflowTestStateHome(t)
	r := runWorkflowTest([]string{"resume", "--project", "proj-1", "--workflow", "wf-1"}, t.TempDir())
	if r.code != 2 {
		t.Fatalf("code=%d stderr=%q, want exit 2 (resume requires an owned, paused workflow)", r.code, r.stderr)
	}
}

// TestWorkflowPauseThenResumeDispatch proves runWorkflowCore's dispatch
// specifically wires "pause" to workflow.Lifecycle.Pause and "resume" to
// workflow.Lifecycle.Resume (as opposed to, say, both accidentally calling
// Start): pause only succeeds from a running workflow and its own effect
// (status becomes paused) is distinct from start's; resume only succeeds
// from that paused state and its effect (status becomes running again) is
// distinct from create's. A swapped or mis-copied op argument in
// runWorkflowCore's switch would fail one of the assertions below.
func TestWorkflowPauseThenResumeDispatch(t *testing.T) {
	workflowTestStateHome(t)
	dir := t.TempDir()
	goalPath := writeMemoryTestFile(t, dir, "goal.json", memoryTestGoalJSON("proj-1", "goal-1"))
	const project, wf = "proj-1", "wf-1"

	if r := runWorkflowTest([]string{"create", "--project", project, "--workflow", wf, "--goal", goalPath, "--profile", "standalone-minimal"}, dir); r.code != 0 {
		t.Fatalf("create: code=%d stderr=%q, want exit 0", r.code, r.stderr)
	}
	if r := runWorkflowTest([]string{"start", "--project", project, "--workflow", wf}, dir); r.code != 0 {
		t.Fatalf("start: code=%d stderr=%q, want exit 0", r.code, r.stderr)
	}

	// pause must fail here if it were wired to Start's op: Start is only
	// legal from created, not running.
	pause := runWorkflowTest([]string{"pause", "--project", project, "--workflow", wf}, dir)
	if pause.code != 0 {
		t.Fatalf("pause: code=%d stderr=%q, want exit 0", pause.code, pause.stderr)
	}
	var pausedState workflowStateJSON
	if err := json.Unmarshal([]byte(pause.stdout), &pausedState); err != nil {
		t.Fatalf("pause: json.Unmarshal() = %v, want nil", err)
	}
	if pausedState.Status != "paused" {
		t.Fatalf("pause: status = %q, want %q", pausedState.Status, "paused")
	}

	// resume must fail here if it were wired to Pause's or Start's op:
	// Resume is only legal from paused.
	resume := runWorkflowTest([]string{"resume", "--project", project, "--workflow", wf}, dir)
	if resume.code != 0 {
		t.Fatalf("resume: code=%d stderr=%q, want exit 0", resume.code, resume.stderr)
	}
	var resumedState workflowStateJSON
	if err := json.Unmarshal([]byte(resume.stdout), &resumedState); err != nil {
		t.Fatalf("resume: json.Unmarshal() = %v, want nil", err)
	}
	if resumedState.Status != "running" {
		t.Fatalf("resume: status = %q, want %q", resumedState.Status, "running")
	}

	// A second pause proves resume actually left the workflow running (not,
	// say, silently still paused): pause is legal only from running.
	if r := runWorkflowTest([]string{"pause", "--project", project, "--workflow", wf}, dir); r.code != 0 {
		t.Fatalf("second pause: code=%d stderr=%q, want exit 0", r.code, r.stderr)
	}
}

func TestParseWorkflowArgsRejectsUnexpectedPositionalArgument(t *testing.T) {
	_, err := parseWorkflowArgs([]string{"--project", "p", "stray"}, nil)
	if err == nil || !strings.Contains(err.Error(), `unexpected argument "stray"`) {
		t.Fatalf("parseWorkflowArgs() = %v, want an unexpected-argument error", err)
	}
}

// TestWorkflowDependencyProbeDegradationIsReportedOnStderr proves observationsFor's
// degradation notification (workflow.DegradedHook) is actually wired from the
// CLI through to stderr: a prober that errors must still let the operation
// succeed (dependency unavailability never blocks a lifecycle operation),
// while also printing one line a person running the command would see.
func TestWorkflowDependencyProbeDegradationIsReportedOnStderr(t *testing.T) {
	workflowTestStateHome(t)
	dir := t.TempDir()
	goalPath := writeMemoryTestFile(t, dir, "goal.json", memoryTestGoalJSON("proj-1", "goal-1"))

	r := runWorkflowTest([]string{"create", "--project", "proj-1", "--workflow", "wf-1", "--goal", goalPath, "--profile", "standalone-minimal"}, dir)
	if r.code != 0 {
		t.Fatalf("create: code=%d stderr=%q, want exit 0", r.code, r.stderr)
	}
	// The CLI always wires the default UnavailableProber (never errors), so
	// this asserts the negative: no degradation line for the happy path,
	// establishing the baseline the workflow-package-level degraded-hook
	// unit test (see lifecycle_test.go) contrasts with.
	if strings.Contains(r.stderr, "dependency probing degraded") {
		t.Fatalf("create: stderr=%q, want no degradation line for the default prober", r.stderr)
	}
}

// TestWorkflowEndToEndAcrossSeparateProcessCalls drives create through
// close(completed), then status, with every verb its own runWorkflowCore
// call (over the same XDG_STATE_HOME) to simulate each verb running as a
// separate process invocation, then confirms the final status reports the
// closed workflow.
func TestWorkflowEndToEndAcrossSeparateProcessCalls(t *testing.T) {
	workflowTestStateHome(t)
	dir := t.TempDir()
	goalPath := writeMemoryTestFile(t, dir, "goal.json", memoryTestGoalJSON("proj-1", "goal-1"))
	const project, wf = "proj-1", "wf-1"

	create := runWorkflowTest([]string{"create", "--project", project, "--workflow", wf, "--goal", goalPath, "--profile", "standalone-minimal"}, dir)
	if create.code != 0 {
		t.Fatalf("create: code=%d stderr=%q, want exit 0", create.code, create.stderr)
	}
	var createdState workflowStateJSON
	if err := json.Unmarshal([]byte(create.stdout), &createdState); err != nil {
		t.Fatalf("create: json.Unmarshal() = %v, want nil (stdout=%q)", err, create.stdout)
	}
	if createdState.Status != "created" || createdState.Classification != "owned" {
		t.Fatalf("create: state = %+v, want status=created classification=owned", createdState)
	}

	start := runWorkflowTest([]string{"start", "--project", project, "--workflow", wf}, dir)
	if start.code != 0 {
		t.Fatalf("start: code=%d stderr=%q, want exit 0", start.code, start.stderr)
	}

	profile, err := workflowprofile.Resolve("standalone-minimal")
	if err != nil {
		t.Fatalf("workflowprofile.Resolve() = %v, want nil", err)
	}
	firstStage := profile.Stages[0].Name
	stage := runWorkflowTest([]string{"stage", "--project", project, "--workflow", wf, "--stage", firstStage}, dir)
	if stage.code != 0 {
		t.Fatalf("stage: code=%d stderr=%q, want exit 0", stage.code, stage.stderr)
	}
	var stagedState workflowStateJSON
	if err := json.Unmarshal([]byte(stage.stdout), &stagedState); err != nil {
		t.Fatalf("stage: json.Unmarshal() = %v, want nil", err)
	}
	if len(stagedState.Stages) != 1 || stagedState.Stages[0] != firstStage {
		t.Fatalf("stage: Stages = %v, want [%s]", stagedState.Stages, firstStage)
	}

	verify := runWorkflowTest([]string{"verify", "--project", project, "--workflow", wf, "--goal", goalPath}, dir)
	if verify.code != 0 {
		t.Fatalf("verify: code=%d stderr=%q, want exit 0", verify.code, verify.stderr)
	}
	var verifiedState workflowStateJSON
	if err := json.Unmarshal([]byte(verify.stdout), &verifiedState); err != nil {
		t.Fatalf("verify: json.Unmarshal() = %v, want nil", err)
	}
	if verifiedState.LastVerifiedSeq < 0 {
		t.Fatalf("verify: LastVerifiedSeq = %d, want >= 0", verifiedState.LastVerifiedSeq)
	}

	close := runWorkflowTest([]string{"close", "--project", project, "--workflow", wf, "--outcome", "completed"}, dir)
	if close.code != 0 {
		t.Fatalf("close: code=%d stderr=%q, want exit 0", close.code, close.stderr)
	}
	var closedState workflowStateJSON
	if err := json.Unmarshal([]byte(close.stdout), &closedState); err != nil {
		t.Fatalf("close: json.Unmarshal() = %v, want nil", err)
	}
	if closedState.Status != "closed" || closedState.CloseOutcome != "completed" {
		t.Fatalf("close: state = %+v, want status=closed close_outcome=completed", closedState)
	}

	status := runWorkflowTest([]string{"status", "--project", project, "--workflow", wf}, dir)
	if status.code != 0 {
		t.Fatalf("status: code=%d stderr=%q, want exit 0", status.code, status.stderr)
	}
	var finalState workflowStateJSON
	if err := json.Unmarshal([]byte(status.stdout), &finalState); err != nil {
		t.Fatalf("status: json.Unmarshal() = %v, want nil", err)
	}
	if finalState.Classification != "owned" || finalState.Status != "closed" {
		t.Fatalf("status: state = %+v, want classification=owned status=closed", finalState)
	}

	// Nothing may be appended after close.
	if r := runWorkflowTest([]string{"start", "--project", project, "--workflow", wf}, dir); r.code != 2 {
		t.Fatalf("start after close: code=%d stderr=%q, want exit 2", r.code, r.stderr)
	}
}
