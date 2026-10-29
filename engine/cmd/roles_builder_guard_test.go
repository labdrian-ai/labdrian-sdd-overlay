package main

// Tests for 'roles builder-guard' (own-rdd S14 d-f), the Claude Code PreToolUse hook that denies launching
// the builder on a task the shaper did not design. The decision is engine/oddtask's and is tested there;
// these tests pin the hook's contract with Claude Code through the public verb: what it prints, its exit
// code, which agents it leaves alone, and that a builder launch it cannot check is denied. Everything runs in
// process on injected streams, in temporary repositories.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/hookwire"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/oddtask"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/roles"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/sha256hex"
)

const bgDoc = "## Tasks\n\n- [ ] T1 — S14 — delegated — designed — shaped: L5 — commit: -\n- [ ] T2 — S14 — delegated — not designed — commit: -\n" +
	"- [ ] T6b1 — S14 — delegated — lettered and numbered — shaped: L5 — commit: -\n- [ ] T6b — S14 — delegated — prefix, not designed — commit: -\n\n" +
	"## Log\n\n- **L5** (2026-10-28, shaper): design\n"

// bgBareRepo is a git repository whose odd/tasks/f.md is bgDoc and odd/goals/f.json is a goal of the feature f,
// in a world of its own: HOME, XDG_STATE_HOME and XDG_CONFIG_HOME are under a temporary directory, so no
// workflow, binding or chain outside it is read or written. Nothing is bound in it. It returns its root.
func bgBareRepo(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	for env, dir := range map[string]string{"HOME": "home", "XDG_STATE_HOME": "state", "XDG_CONFIG_HOME": "config"} {
		if err := os.MkdirAll(filepath.Join(base, dir), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv(env, filepath.Join(base, dir))
	}
	root := fixtureRepo(t, "repo")
	for rel, content := range map[string]string{"odd/tasks/f.md": bgDoc, "odd/goals/f.json": memoryTestGoalJSON("proj-1", "f")} {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// bgRepo is bgBareRepo with the workflow f-t1 opened and bound (S19 b): running, of the goal f, its chain
// handing the work to the builder. This is the world where a builder launch is allowed.
func bgRepo(t *testing.T) string {
	t.Helper()
	root := bgBareRepo(t)
	if r := runWorkflowTestWith(testDeps(), []string{"open", "--feature", "f", "--task", "T1"}, root); r.code != 0 {
		t.Fatalf("workflow open: code=%d stderr=%q", r.code, r.stderr)
	}
	return root
}

// bgCall is the hook input of an Agent call.
func bgCall(t *testing.T, cwd, agent, prompt string) string {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"session_id":      "s-1",
		"cwd":             cwd,
		"hook_event_name": "PreToolUse",
		"tool_name":       "Agent",
		"tool_input":      map[string]any{"description": "d", "subagent_type": agent, "prompt": prompt},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func bgRun(stdin string, args ...string) guardRun {
	var out, errBuf bytes.Buffer
	var codes []int
	runRolesCore(append([]string{"builder-guard"}, args...), strings.NewReader(stdin), &out, &errBuf, func(c int) { codes = append(codes, c) }, nil)
	return guardRun{codes: codes, stdout: out.String(), stderr: errBuf.String()}
}

func TestBuilderGuardAllowsABuilderLaunchOnATaskTheShaperDesigned(t *testing.T) {
	root := bgRepo(t)
	for name, prompt := range map[string]string{
		"a relative path":  "Do T1.\nSpec: odd/tasks/f.md, T1, S14\n",
		"an absolute path": "Spec: " + filepath.Join(root, "odd", "tasks", "f.md") + ", T1",
	} {
		assertGuardSilent(t, name, bgRun(bgCall(t, root, "builder", prompt)))
	}
}

func TestBuilderGuardAllowsABuilderLaunchCitingATaskIdWithLettersAndDigits(t *testing.T) {
	root := bgRepo(t)
	assertGuardSilent(t, "T6b1", bgRun(bgCall(t, root, "builder", "Spec: odd/tasks/f.md, T6b1, S6")))
	// T6b1 is designed; T6b is its prefix and is not: the lookup is by the whole id.
	reason := decodeGuardDenial(t, bgRun(bgCall(t, root, "builder", "Spec: odd/tasks/f.md, T6b, S6")))
	if !strings.Contains(reason, "task T6b of ") || !strings.Contains(reason, "is not marked as designed by the shaper") {
		t.Fatalf("reason = %q, want T6b refused as not designed", reason)
	}
}

func TestBuilderGuardDeniesABuilderLaunchWithTheExactJSONAndExit0(t *testing.T) {
	root := bgRepo(t)
	r := bgRun(bgCall(t, root, "builder", "Spec: odd/tasks/f.md, T2"))
	reason := decodeGuardDenial(t, r)
	path := filepath.Join(root, "odd", "tasks", "f.md")
	want := "labdrian builder guard: task T2 of " + path + ` is not marked as designed by the shaper (its line has no "shaped: L<n>"). ` +
		`The builder never designs: launch the shaper first to design the task and mark its line "shaped: L<n>", then launch the builder with the line "Spec: odd/tasks/<feature>.md, T<n>" in its prompt. ` +
		`This guard reads text only, so it is a speed bump, not a security boundary.`
	if reason != want {
		t.Fatalf("reason = %q, want %q", reason, want)
	}
	var out struct {
		HookSpecificOutput struct {
			HookEventName, PermissionDecision, PermissionDecisionReason string
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &out); err != nil {
		t.Fatal(err)
	}
	if got := out.HookSpecificOutput; got.HookEventName != "PreToolUse" || got.PermissionDecision != "deny" {
		t.Fatalf("hookSpecificOutput = %+v, want a PreToolUse deny", got)
	}
}

func TestBuilderGuardDeniesABuilderLaunchThatCitesNothing(t *testing.T) {
	root := bgRepo(t)
	for name, c := range map[string]struct {
		prompt string
		reason oddtask.Reason
	}{
		"no citation":  {"just build it", oddtask.ReasonNoCitation},
		"two citation": {"Spec: odd/tasks/f.md, T1\nSpec: odd/tasks/f.md, T1", oddtask.ReasonManyCitations},
		"a bad one":    {"Spec: odd/tasks/f.md", oddtask.ReasonBadCitation},
	} {
		if got := decodeGuardDenial(t, bgRun(bgCall(t, root, "builder", c.prompt))); got != oddtask.Message(c.reason) {
			t.Errorf("%s: reason = %q, want %q", name, got, oddtask.Message(c.reason))
		}
	}
}

func TestBuilderGuardDeniesWhenTheDocumentCannotBeRead(t *testing.T) {
	root := bgRepo(t)
	gone := filepath.Join(root, "odd", "tasks", "gone.md")
	got := decodeGuardDenial(t, bgRun(bgCall(t, root, "builder", "Spec: odd/tasks/gone.md, T1")))
	if want := oddtask.Message(oddtask.UnreadableReason(gone, oddtask.CauseNotFound)); got != want {
		t.Fatalf("reason = %q, want %q", got, want)
	}
	// A relative path with a session that has no cwd cannot be resolved.
	got = decodeGuardDenial(t, bgRun(bgCall(t, "", "builder", "Spec: odd/tasks/f.md, T1")))
	if want := oddtask.Message(oddtask.UnreadableReason("odd/tasks/f.md", oddtask.CauseNoCwd)); got != want {
		t.Fatalf("reason = %q, want %q", got, want)
	}
}

func TestBuilderGuardLeavesEveryOtherAgentAloneAndReadsNothing(t *testing.T) {
	root := bgRepo(t)
	for _, agent := range []string{"verifier", "reviewer", "shaper", "general-purpose", "Builder", "builder ", "builders", ""} {
		for _, prompt := range []string{"no citation at all", "Spec: odd/tasks/gone.md, T9", "Spec: odd/tasks/f.md, T2"} {
			assertGuardSilent(t, agent+" "+prompt, bgRun(bgCall(t, root, agent, prompt)))
		}
	}
}

func TestBuilderGuardDeniesABuilderLaunchWhenItPanics(t *testing.T) {
	var out, errBuf bytes.Buffer
	var codes []int
	runRolesBuilderGuard(nil, strings.NewReader(bgCall(t, "/repo", "builder", "Spec: odd/tasks/f.md, T1")), &out, &errBuf,
		func(c int) { codes = append(codes, c) }, panickingDocs{})
	r := guardRun{codes: codes, stdout: out.String(), stderr: errBuf.String()}
	if !strings.Contains(r.stderr, "internal error") {
		t.Errorf("stderr = %q, want the panic reported", r.stderr)
	}
	r.stderr = ""
	if got := decodeGuardDenial(t, r); got != oddtask.Message(oddtask.ReasonInternal) {
		t.Fatalf("reason = %q, want %q", got, oddtask.Message(oddtask.ReasonInternal))
	}
}

type panickingDocs struct{}

func (panickingDocs) Read(string, string) (oddtask.File, error) { panic("boom") }

func TestBuilderGuardLetsThroughWhatItCannotTellIsABuilder(t *testing.T) {
	for name, in := range map[string]string{
		"not json":             "not json",
		"empty":                "",
		"no tool_input":        `{"tool_name":"Agent"}`,
		"a prompt of a number": `{"tool_name":"Agent","tool_input":{"subagent_type":"builder","prompt":3}}`,
		"a type of a number":   `{"tool_name":"Agent","tool_input":{"subagent_type":3,"prompt":"x"}}`,
		"input cut short":      `{"tool_name":"Agent","tool_input":{"subagent_type":"builder","prompt":"Spec`,
	} {
		r := bgRun(in)
		if r.stdout != "" || !reflect.DeepEqual(r.codes, []int{0}) {
			t.Errorf("%s: exits %v, stdout %q, want an allow with exit 0", name, r.codes, r.stdout)
		}
		if strings.Count(r.stderr, "\n") != 1 || !strings.HasPrefix(r.stderr, "roles builder-guard:") {
			t.Errorf("%s: stderr = %q, want one line that says the call was not checked", name, r.stderr)
		}
	}
}

func TestBuilderGuardLetsThroughAStdinThatFails(t *testing.T) {
	var out, errBuf bytes.Buffer
	var codes []int
	runRolesCore([]string{"builder-guard"}, failingReader{}, &out, &errBuf, func(c int) { codes = append(codes, c) }, nil)
	if out.String() != "" || !reflect.DeepEqual(codes, []int{0}) || strings.Count(errBuf.String(), "\n") != 1 {
		t.Fatalf("exits %v, stdout %q, stderr %q, want an allow with exit 0 and one line on stderr", codes, out.String(), errBuf.String())
	}
}

func TestBuilderGuardDeniesAnAgentCallOverTheBoundWhateverTheAgent(t *testing.T) {
	root := bgRepo(t)
	want := oddtask.Message(oddtask.TooLargeReason(hookwire.MaxAgentCallBytes))
	big := strings.Repeat("x", hookwire.MaxAgentCallBytes)
	for _, agent := range []string{"builder", "verifier"} {
		if got := decodeGuardDenial(t, bgRun(bgCall(t, root, agent, "Spec: odd/tasks/f.md, T1\n"+big))); got != want {
			t.Errorf("%s: reason = %q, want %q", agent, got, want)
		}
	}
	// An endless input is cut at the bound, not read to its end.
	if got := decodeGuardDenial(t, bgRun(strings.Repeat(" ", hookwire.MaxAgentCallBytes+10))); got != want {
		t.Errorf("padding: reason = %q, want %q", got, want)
	}
}

// bgEndlessReader never ends and counts what is read from it.
type bgEndlessReader struct{ n int }

func (e *bgEndlessReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = ' '
	}
	e.n += len(p)
	return len(p), nil
}

func TestBuilderGuardReadsOneByteMoreThanTheBoundOfAnEndlessInput(t *testing.T) {
	in := &bgEndlessReader{}
	var out, errBuf bytes.Buffer
	var codes []int
	runRolesCore([]string{"builder-guard"}, in, &out, &errBuf, func(c int) { codes = append(codes, c) }, nil)
	if in.n != hookwire.MaxAgentCallBytes+1 {
		t.Fatalf("read %d bytes of an endless input, want %d", in.n, hookwire.MaxAgentCallBytes+1)
	}
	if !strings.Contains(out.String(), `"permissionDecision":"deny"`) || !reflect.DeepEqual(codes, []int{0}) {
		t.Fatalf("exits %v, stdout %q, want a denial with exit 0", codes, out.String())
	}
}

func TestBuilderGuardTakesNoArgument(t *testing.T) {
	r := bgRun(bgCall(t, "/repo", "builder", "x"), "--now")
	if !reflect.DeepEqual(r.codes, []int{1}) || r.stdout != "" || !strings.Contains(r.stderr, `unexpected argument "--now"`) {
		t.Fatalf("exits %v, stdout %q, stderr %q, want exit 1 and the argument named", r.codes, r.stdout, r.stderr)
	}
}

// The verbs that existed before keep their answers when no verb or an unknown one is given: builder-guard is a
// hook command, not one a person runs, so the usage lines do not list it.
func TestRolesPreviousVerbsStillAnswerAsBefore(t *testing.T) {
	cases := map[string]struct {
		args []string
		want string
	}{
		"no verb":      {nil, "error: roles requires a verb: validate, next, resume, append, match-shaper\n"},
		"unknown verb": {[]string{"bogus"}, "error: roles: unknown verb \"bogus\" (expected validate, next, resume, append, or match-shaper)\n"},
		"validate":     {[]string{"validate"}, "error: roles validate: --file is required\n"},
		"match-shaper": {[]string{"match-shaper"}, "error: roles match-shaper: --root is required\n"},
	}
	for name, c := range cases {
		var out, errBuf bytes.Buffer
		var codes []int
		runRolesCore(c.args, strings.NewReader(""), &out, &errBuf, func(n int) { codes = append(codes, n) }, nil)
		if errBuf.String() != c.want || out.String() != "" || !reflect.DeepEqual(codes, []int{1}) {
			t.Errorf("%s: exits %v, stdout %q, stderr %q, want exit 1 and %q", name, codes, out.String(), errBuf.String(), c.want)
		}
	}
}

// wantWorkflowDenial is the exact message of S19 (c) for reason, spelled out here and not built by the
// code under test.
func wantWorkflowDenial(reason, feature, task string) string {
	return "labdrian builder guard: " + reason + ". Every feature runs inside its workflow: run labdrian workflow open --feature " +
		feature + " --task " + task + " (it records the shaper and estimator hops, then creates, starts and binds the workflow of the task), " +
		"then launch the builder again. This guard reads text and local state only, so it is a speed bump, not a security boundary."
}

// bgDenied runs a builder launch citing feature f's task T1 at root and returns the reason of its denial.
func bgDenied(t *testing.T, root string) string {
	t.Helper()
	return decodeGuardDenial(t, bgRun(bgCall(t, root, "builder", "Spec: odd/tasks/f.md, T1")))
}

// bgWorkflowCmd runs a workflow verb in the world of the test and requires exit 0.
func bgWorkflowCmd(t *testing.T, root string, args ...string) {
	t.Helper()
	if r := runWorkflowTestWith(testDeps(), args, root); r.code != 0 {
		t.Fatalf("workflow %v: code=%d stderr=%q", args, r.code, r.stderr)
	}
}

// S19 (a): a shaped launch inside the running workflow of its feature, whose chain hands the work to the
// builder, is allowed with nothing on stdout.
func TestBuilderGuardAllowsABuilderLaunchInsideItsRunningWorkflow(t *testing.T) {
	root := bgRepo(t)
	binding := decodeBindingReport(t, runWorkflowTestWith(testDeps(), []string{"binding"}, root))
	if binding.Classification != "owned" || binding.Binding == nil || binding.Binding.WorkflowID != "f-t1" || binding.Workflow == nil || binding.Workflow.Status != "running" {
		t.Fatalf("the world is not the one under test: binding = %+v", binding)
	}
	assertGuardSilent(t, "bound to f-t1, running, chain at builder", bgRun(bgCall(t, root, "builder", "Spec: odd/tasks/f.md, T1, S19")))
}

// S19 (a): no binding, and a binding that is not owned.
func TestBuilderGuardDeniesABuilderLaunchOfARepositoryBoundToNoWorkflow(t *testing.T) {
	root := bgBareRepo(t)
	want := wantWorkflowDenial("the repository of "+root+" is bound to no workflow (labdrian workflow binding: absent)", "f", "T1")
	if got := bgDenied(t, root); got != want {
		t.Fatalf("reason = %q, want %q", got, want)
	}

	// A binding file that is not ours is classified, not followed: the repository is bound to no workflow.
	key := mustRepoKey(t, root)
	dir := filepath.Join(os.Getenv("XDG_STATE_HOME"), "labdrian", "bindings")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, key+".json"), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	want = wantWorkflowDenial("the repository of "+root+" is bound to no workflow (labdrian workflow binding: malformed)", "f", "T1")
	if got := bgDenied(t, root); got != want {
		t.Fatalf("a malformed binding: reason = %q, want %q", got, want)
	}
}

// S19 (a): the bound workflow is not running.
func TestBuilderGuardDeniesABuilderLaunchOfAWorkflowThatIsNotRunning(t *testing.T) {
	root := bgRepo(t)
	bgWorkflowCmd(t, root, "pause", "--project", "proj-1", "--workflow", "f-t1")
	want := wantWorkflowDenial("the workflow f-t1 bound to the repository is paused, not running", "f", "T1")
	if got := bgDenied(t, root); got != want {
		t.Fatalf("reason = %q, want %q", got, want)
	}
}

// S19 (a): the bound workflow is of another goal than the feature the prompt cites.
func TestBuilderGuardDeniesABuilderLaunchCitingAFeatureOtherThanTheGoalOfTheWorkflow(t *testing.T) {
	root := bgRepo(t)
	if err := os.WriteFile(filepath.Join(root, "odd", "tasks", "g.md"), []byte(bgDoc), 0o644); err != nil {
		t.Fatal(err)
	}
	got := decodeGuardDenial(t, bgRun(bgCall(t, root, "builder", "Spec: odd/tasks/g.md, T1")))
	want := wantWorkflowDenial("the workflow f-t1 bound to the repository is of goal f, not of the feature g the prompt cites", "g", "T1")
	if got != want {
		t.Fatalf("reason = %q, want %q", got, want)
	}
}

// S19 (a): the bound workflow follows no role chain.
func TestBuilderGuardDeniesABuilderLaunchOfAWorkflowWithNoRoleChain(t *testing.T) {
	root := bgBareRepo(t)
	goal := filepath.Join(root, "odd", "goals", "f.json")
	bgWorkflowCmd(t, root, "create", "--project", "proj-1", "--workflow", "plain", "--goal", goal, "--profile", "odd")
	bgWorkflowCmd(t, root, "start", "--project", "proj-1", "--workflow", "plain")
	bgWorkflowCmd(t, root, "bind", "--project", "proj-1", "--workflow", "plain")
	want := wantWorkflowDenial("the workflow plain bound to the repository follows no role chain", "f", "T1")
	if got := bgDenied(t, root); got != want {
		t.Fatalf("reason = %q, want %q", got, want)
	}
}

// S19 (a): the chain's current role is not the builder.
func TestBuilderGuardDeniesABuilderLaunchWhenTheChainHandsTheWorkToAnotherRole(t *testing.T) {
	root := bgRepo(t)
	store, err := newRoleChainStore()
	if err != nil {
		t.Fatal(err)
	}
	records, err := store.LoadChain("proj-1", "f", "f-t1")
	if err != nil || len(records) != 2 {
		t.Fatalf("chain f-t1: %d records, err=%v, want 2", len(records), err)
	}
	second := records[1].Handoff
	raw, err := json.Marshal(roles.RoleHandoff{
		Version: roles.HandoffVersion, ProjectID: "proj-1", GoalID: "f", ChainID: "f-t1", Seq: 3,
		FromRole: roles.RoleBuilder, ToRole: roles.RoleReviewer, PrevSHA256: sha256hex.Sum(records[1].Raw),
		PayloadKind: "progress", PayloadSHA256: second.PayloadSHA256, Evidence: []roles.Evidence{},
		Context: roles.Context{Summary: "built", Decisions: []string{}, OpenQuestions: []string{}}, Status: roles.StatusCompleted,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Append(raw); err != nil {
		t.Fatal(err)
	}
	want := wantWorkflowDenial("the role chain f-t1 of the workflow f-t1 hands the work to reviewer, not to the builder", "f", "T1")
	if got := bgDenied(t, root); got != want {
		t.Fatalf("reason = %q, want %q", got, want)
	}
}

// S19 (a): a chain that fails roles.VerifyChain, and one with no records.
func TestBuilderGuardDeniesABuilderLaunchWhenTheChainDoesNotVerify(t *testing.T) {
	root := bgRepo(t)
	chainDir := filepath.Join(os.Getenv("XDG_STATE_HOME"), "labdrian", "role-chains", "proj-1", "f", "f-t1")
	second := filepath.Join(chainDir, "000002.json")
	original, err := os.ReadFile(second)
	if err != nil {
		t.Fatal(err)
	}
	var rec roles.RoleHandoff
	if err := json.Unmarshal(original, &rec); err != nil {
		t.Fatal(err)
	}
	wrong := strings.Replace(string(original), rec.PrevSHA256, strings.Repeat("f", 64), 1)
	if err := os.WriteFile(second, []byte(wrong), 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(filepath.Join(chainDir, "000001.json"))
	if err != nil {
		t.Fatal(err)
	}
	cause := `role chain store: role chain: seq 2 prev_sha256 "` + strings.Repeat("f", 64) + `" does not match seq 1 bytes digest "` + sha256hex.Sum(first) + `"`
	want := wantWorkflowDenial("the role chain f-t1 of the workflow f-t1 does not verify ("+cause+")", "f", "T1")
	if got := bgDenied(t, root); got != want {
		t.Fatalf("a wrong prev_sha256: reason = %q, want %q", got, want)
	}

	// No records at all: the chain directory is gone.
	if err := os.RemoveAll(chainDir); err != nil {
		t.Fatal(err)
	}
	want = wantWorkflowDenial("the role chain f-t1 of the workflow f-t1 does not verify (it has no records)", "f", "T1")
	if got := bgDenied(t, root); got != want {
		t.Fatalf("no records: reason = %q, want %q", got, want)
	}
}

// S19 (a): any failure to read the binding denies. Here the state home cannot be resolved at all.
func TestBuilderGuardDeniesABuilderLaunchWhenTheBindingCannotBeRead(t *testing.T) {
	root := bgRepo(t)
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", "")
	got := bgDenied(t, root)
	if !strings.HasPrefix(got, "labdrian builder guard: the workflow of the repository cannot be read (") ||
		!strings.HasSuffix(got, "). Every feature runs inside its workflow: run labdrian workflow open --feature f --task T1 (it records the shaper and estimator hops, then creates, starts and binds the workflow of the task), then launch the builder again. This guard reads text and local state only, so it is a speed bump, not a security boundary.") {
		t.Fatalf("reason = %q, want the unreadable workflow reason in the message of S19 (c)", got)
	}
}

// S14 first: a reason of the shaper's mark wins over the workflow's, in the words S14 always used, and
// every other agent is left alone, in a repository bound to nothing.
func TestBuilderGuardChecksTheWorkflowOnlyAfterTheDesignAndOnlyForTheBuilder(t *testing.T) {
	root := bgBareRepo(t)
	path := filepath.Join(root, "odd", "tasks", "f.md")
	want := "labdrian builder guard: task T2 of " + path + ` is not marked as designed by the shaper (its line has no "shaped: L<n>"). ` +
		`The builder never designs: launch the shaper first to design the task and mark its line "shaped: L<n>", then launch the builder with the line "Spec: odd/tasks/<feature>.md, T<n>" in its prompt. ` +
		`This guard reads text only, so it is a speed bump, not a security boundary.`
	if got := decodeGuardDenial(t, bgRun(bgCall(t, root, "builder", "Spec: odd/tasks/f.md, T2"))); got != want {
		t.Fatalf("reason = %q, want %q", got, want)
	}
	for _, agent := range []string{"verifier", "reviewer", "shaper", "general-purpose"} {
		assertGuardSilent(t, agent, bgRun(bgCall(t, root, agent, "Spec: odd/tasks/f.md, T1")))
	}
}
