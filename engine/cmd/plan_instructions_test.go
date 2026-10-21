package main

// The memory instructions of C23 M6 (docs/architecture/adr-c23-m6-plan-instructions.md) through the
// commands: 'memory plan --instructions', the SessionStart event of 'projection hook', and the
// gate-task rewrite of a sub-agent's prompt. The three print the one block engine/projection
// renders, while a workflow is bound; without a binding the hooks say nothing new and gate-task
// answers as it did. Workflows are created through the real verbs in a state home of their own.

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// oddInstructions is the block of a running odd workflow wf-1 of proj-1 created in a bindEnv,
// whose identity, and so Engram name, is proj-1.
const oddInstructions = "labdrian memory instructions (workflow wf-1, scope project):\n" +
	"- Read memory only from: engram, longterm-mem, procedural-skills. A read from any other source is denied.\n" +
	`- Engram reads (mem_search, mem_context) name project "proj-1"; longterm-mem query and get name project "proj-1".` + "\n" +
	"- Memory writes (mem_save, promote) are not limited by the plan.\n" +
	"- Procedural candidates: T1 right after a proactive mem_save of type bugfix, pattern, discovery or decision; T2 at session close, beside mem_session_summary (skills/_shared/procedural-candidate-detection.md, section 5). Load skills/procedural-candidates/SKILL.md for the procedure; run `skills candidates` before creating or updating a candidate."

const noWorkflowBound = "labdrian memory instructions: no workflow is bound to this repository, so the memory plan sets no rule.\n"

// runMemoryIn runs the memory verb as a process whose working directory is cwd.
func runMemoryIn(d deps, cwd string, args ...string) memoryRun {
	var out, errBuf bytes.Buffer
	code := -1
	runMemoryCore(d, cwd, args, &out, &errBuf, func(c int) {
		if code == -1 {
			code = c
		}
	})
	return memoryRun{code: code, stdout: out.String(), stderr: errBuf.String()}
}

// --- memory plan --instructions ---------------------------------------------------------------

func TestMemoryPlanInstructionsOfANamedWorkflow(t *testing.T) {
	e := newHookEnv(t)
	e.running(t, "proj-1", "wf-1", "odd")
	r := runMemoryIn(e.deps, "", "plan", "--project", "proj-1", "--workflow", "wf-1", "--instructions")
	if r.code != 0 || r.stderr != "" || r.stdout != oddInstructions+"\n" {
		t.Errorf("code=%d stdout=%q stderr=%q, want exit 0 and the block", r.code, r.stdout, r.stderr)
	}
}

func TestMemoryPlanInstructionsReadTheRepositorysBinding(t *testing.T) {
	e := newHookEnv(t)
	e.running(t, "proj-1", "wf-1", "odd")
	r := runMemoryIn(e.deps, e.repo, "plan", "--instructions")
	if r.code != 0 || r.stderr != "" || r.stdout != oddInstructions+"\n" {
		t.Errorf("code=%d stdout=%q stderr=%q, want exit 0 and the block of the bound workflow", r.code, r.stdout, r.stderr)
	}
}

func TestMemoryPlanInstructionsWithNoBindingSayNoWorkflowIsBound(t *testing.T) {
	e := newHookEnv(t)
	e.create(t, "proj-1", "wf-1", "odd") // a workflow exists, but nothing is bound to it
	for _, cwd := range []string{e.repo, e.dir} {
		r := runMemoryIn(e.deps, cwd, "plan", "--instructions")
		if r.code != 0 || r.stderr != "" || r.stdout != noWorkflowBound {
			t.Errorf("cwd %s: code=%d stdout=%q stderr=%q, want exit 0 and %q", cwd, r.code, r.stdout, r.stderr, noWorkflowBound)
		}
	}
}

func TestMemoryPlanInstructionsOfABindingThatCannotBeFollowedAreRefused(t *testing.T) {
	e := newHookEnv(t)
	e.running(t, "proj-1", "wf-1", "odd")
	e.step(t, "proj-1", "wf-1", "close", "--outcome", "abandoned", "--reason", "stopped")
	r := runMemoryIn(e.deps, e.repo, "plan", "--instructions")
	if r.code != 2 || r.stdout != "" || !strings.HasPrefix(r.stderr, "error: memory plan: labdrian workflow projection: workflow wf-1 of project proj-1 is closed (abandoned).") {
		t.Errorf("code=%d stdout=%q stderr=%q, want exit 2 and the note of the closed workflow", r.code, r.stdout, r.stderr)
	}
	if _, err := os.Stat(e.bindingFile(t, e.repo)); err != nil {
		t.Errorf("memory plan removed the binding of a closed workflow")
	}
}

func TestMemoryPlanInstructionsDoNotCombineWithTheProfileForm(t *testing.T) {
	want := "error: memory plan: --instructions reads the plan of a workflow and does not combine with --profile, --goal, --goal-directive or --handoff-directive\n"
	for _, args := range [][]string{
		{"plan", "--instructions", "--profile", "odd"},
		{"plan", "--instructions", "--goal", "g.json"},
		{"plan", "--instructions", "--handoff-directive", "d.json"},
	} {
		if r := runMemoryTest(args); r.code != 1 || r.stdout != "" || r.stderr != want {
			t.Errorf("%v: code=%d stdout=%q stderr=%q, want exit 1 and %q", args, r.code, r.stdout, r.stderr, want)
		}
	}
}

// Without --instructions, --project --workflow prints the plan as JSON, as before M6.
func TestMemoryPlanOfANamedWorkflowWithoutInstructionsIsStillTheJSONPlan(t *testing.T) {
	e := newHookEnv(t)
	e.running(t, "proj-1", "wf-1", "odd")
	r := runMemoryIn(e.deps, e.repo, "plan", "--project", "proj-1", "--workflow", "wf-1")
	var plan map[string]any
	if r.code != 0 || r.stderr != "" || json.Unmarshal([]byte(r.stdout), &plan) != nil || plan["scope"] != "project" {
		t.Errorf("code=%d stdout=%q stderr=%q, want exit 0 and the plan as JSON", r.code, r.stdout, r.stderr)
	}
}

// --- projection hook --event SessionStart -----------------------------------------------------

func sessionStartInput(t *testing.T, cwd, source string) string {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"session_id": "session-1", "transcript_path": "/tmp/transcript.jsonl", "cwd": cwd,
		"hook_event_name": "SessionStart", "source": source,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestSessionStartGivesTheInstructionsOfTheBoundWorkflow(t *testing.T) {
	e := newHookEnv(t)
	e.running(t, "proj-1", "wf-1", "odd")
	want, _ := json.Marshal(map[string]any{"hookSpecificOutput": map[string]string{"hookEventName": "SessionStart", "additionalContext": oddInstructions}})
	for _, source := range []string{"startup", "resume", "clear", "compact"} {
		r := runProjectionArgsWith(e.deps, []string{"hook", "--event", "SessionStart"}, sessionStartInput(t, e.repo, source), e.dir)
		var got, wantObj any
		_ = json.Unmarshal(want, &wantObj)
		if len(r.codes) != 1 || r.codes[0] != 0 || r.stderr != "" || json.Unmarshal([]byte(r.stdout), &got) != nil || strings.Count(r.stdout, "\n") != 1 {
			t.Fatalf("%s: exits %v, stdout %q, stderr %q, want one JSON line and exit 0", source, r.codes, r.stdout, r.stderr)
		}
		if g, _ := json.Marshal(got); string(g) != string(want) {
			t.Errorf("%s: stdout %s, want %s", source, g, want)
		}
	}
}

func TestSessionStartIsSilentWithoutAWorkflowToFollow(t *testing.T) {
	e := newHookEnv(t)
	e.create(t, "proj-1", "wf-1", "odd")
	args := []string{"hook", "--event", "SessionStart"}
	assertSilent(t, "nothing bound", runProjectionArgsWith(e.deps, args, sessionStartInput(t, e.repo, "startup"), e.dir))
	assertSilent(t, "outside every repository", runProjectionArgsWith(e.deps, args, sessionStartInput(t, e.dir, "startup"), e.dir))
	assertSilent(t, "input that is not JSON", runProjectionArgsWith(e.deps, args, "not json", e.dir))

	mustBindOK(t, e.repo, "proj-1", "wf-1")
	assertSilent(t, "another event's input", runProjectionArgsWith(e.deps, args, hookInput(t, e.repo, "session-1", "a prompt"), e.dir))
	e.step(t, "proj-1", "wf-1", "start")
	e.step(t, "proj-1", "wf-1", "close", "--outcome", "abandoned", "--reason", "stopped")
	assertSilent(t, "a closed workflow", runProjectionArgsWith(e.deps, args, sessionStartInput(t, e.repo, "resume"), e.dir))
	if _, err := os.Stat(e.bindingFile(t, e.repo)); err != nil {
		t.Errorf("SessionStart removed the binding of a closed workflow")
	}
}

func TestProjectionHookNamesSessionStartAmongTheSupportedEvents(t *testing.T) {
	r := runProjectionArgs([]string{"hook", "--event", "Stop"}, "", "")
	want := "error: projection hook: unsupported --event \"Stop\" (supported: UserPromptSubmit, PreToolUse, SessionStart, PostToolUse)\n"
	if len(r.codes) != 1 || r.codes[0] != 1 || r.stderr != want {
		t.Errorf("exits %v, stderr %q, want exit 1 and %q", r.codes, r.stderr, want)
	}
}

// --- help and README --------------------------------------------------------------------------

func TestUsageAndReadmeDocumentTheMemoryInstructions(t *testing.T) {
	readme, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	flat := func(s string) string { return strings.Join(strings.Fields(strings.ReplaceAll(s, "`", "")), " ") }
	for name, tt := range map[string]struct {
		text string
		want []string
	}{
		"usage()": {flat(captureUsage(t)), []string{
			"engine memory plan --instructions [--project <id> --workflow <id>]",
			"no workflow is bound",
			"engine projection hook --event UserPromptSubmit|PreToolUse|SessionStart|PostToolUse",
			`SessionStart prints {"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":...}}`,
			"appends the memory instructions of the bound workflow to every sub-agent's prompt",
		}},
		"README": {flat(string(readme)), []string{
			"labdrian memory plan --instructions",
			"no workflow is bound",
			"SessionStart",
			"appends the memory instructions",
		}},
	} {
		for _, want := range tt.want {
			if !strings.Contains(tt.text, want) {
				t.Errorf("%s does not contain %q", name, want)
			}
		}
	}
}

// --- gate-task --------------------------------------------------------------------------------

// gateTaskIn runs gate-task as a process in processCwd over the deps of e, with the managed
// contract of the e2e tests, and returns stdout.
func gateTaskIn(e hookEnv, processCwd, input string, files map[string]string) string {
	var stdout, stderr bytes.Buffer
	gateTaskCore([]string{"--contract-file", "/virtual/contract.md", "--contract-path", absoluteContractPath},
		strings.NewReader(input), &stdout, &stderr, inMemoryFiles(files), boundInstructions(e.deps, processCwd))
	return stdout.String()
}

// agentInputIn is an Agent call of the session whose working directory is cwd.
func agentInputIn(cwd, subagentType, prompt string) string {
	data, _ := json.Marshal(map[string]any{
		"cwd": cwd, "hook_event_name": "PreToolUse", "tool_name": "Agent",
		"tool_input": map[string]any{"description": "d", "prompt": prompt, "subagent_type": subagentType},
	})
	return string(data)
}

func rewrittenPrompt(t *testing.T, out string) string {
	t.Helper()
	var answer struct {
		HookSpecificOutput struct {
			UpdatedInput struct {
				Prompt string `json:"prompt"`
			} `json:"updatedInput"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(out), &answer); err != nil {
		t.Fatalf("gate-task printed %q: %v", out, err)
	}
	return answer.HookSpecificOutput.UpdatedInput.Prompt
}

func TestGateTaskAppendsTheInstructionsWhileAWorkflowIsBound(t *testing.T) {
	e := newHookEnv(t)
	e.running(t, "proj-1", "wf-1", "odd")
	files := map[string]string{"/virtual/contract.md": contractContentForE2E}

	got := rewrittenPrompt(t, gateTaskIn(e, "", agentInputIn(e.repo, "sdd-apply", "Do the task."), files))
	if want := "Do the task.\n\n## Skills to load before work\n" + absoluteContractPath + "\n\n" + oddInstructions + "\n"; got != want {
		t.Errorf("sdd-apply prompt = %q, want the contract and then the block %q", got, want)
	}
	got = rewrittenPrompt(t, gateTaskIn(e, "", agentInputIn(e.repo, "general-purpose", "Look around."), files))
	if want := "Look around.\n\n" + oddInstructions + "\n"; got != want {
		t.Errorf("general-purpose prompt = %q, want %q", got, want)
	}
	// The process's directory stands in for a call that names none, and a contract file that cannot
	// be read still lets the block through.
	got = rewrittenPrompt(t, gateTaskIn(e, e.repo, agentInputIn("", "sdd-apply", "Do the task."), nil))
	if want := "Do the task.\n\n" + oddInstructions + "\n"; got != want {
		t.Errorf("no cwd and no contract: prompt = %q, want %q", got, want)
	}
	// A prompt that carries the block already is given it once.
	once := "Do the task.\n\n" + oddInstructions + "\n"
	if out := gateTaskIn(e, "", agentInputIn(e.repo, "general-purpose", once), files); out != "{}\n" {
		t.Errorf("a prompt with the block: gate-task printed %q, want the pass-through", out)
	}
}

// Two gate-task entries run on every Agent call, in parallel, and Claude Code names no winner
// between two rewritten inputs (C23 M6, one rewrite per Agent call). The block rides the
// --contract-file entry alone: the --embedded-contract entry answers while bound exactly as it
// answers unbound, so it never rewrites a call the other entry would answer otherwise.
func TestGateTaskWithAnEmbeddedContractNeverAppendsTheInstructions(t *testing.T) {
	embedded := func(e hookEnv, input string) string {
		var stdout, stderr bytes.Buffer
		gateTaskCore([]string{"--embedded-contract", "anti-generic-design"},
			strings.NewReader(input), &stdout, &stderr, inMemoryFiles(nil), boundInstructions(e.deps, ""))
		return stdout.String()
	}
	unbound := newHookEnv(t)
	unbound.create(t, "proj-1", "wf-1", "odd")
	bound := newHookEnv(t)
	bound.running(t, "proj-1", "wf-1", "odd")
	for _, subagentType := range []string{"general-purpose", "sdd-verify", "sdd-apply", "frontend", ""} {
		got := embedded(bound, agentInputIn(bound.repo, subagentType, "Do the task."))
		want := embedded(unbound, agentInputIn(unbound.repo, subagentType, "Do the task."))
		if got != want || strings.Contains(got, "labdrian memory instructions") {
			t.Errorf("%q: bound gate-task --embedded-contract printed %q, want the unbound answer %q", subagentType, got, want)
		}
	}
}

// Without a binding gate-task answers exactly as it did before M6.
func TestGateTaskWithoutABindingAnswersAsBefore(t *testing.T) {
	e := newHookEnv(t)
	e.create(t, "proj-1", "wf-1", "odd")
	files := map[string]string{"/virtual/contract.md": contractContentForE2E}
	for _, tt := range []struct{ input, want string }{
		{agentInputIn(e.repo, "general-purpose", "Look around."), "{}\n"},
		{agentInputIn(e.repo, "sdd-apply", "Do the task."), gateTaskAnswer(t, agentInputIn(e.repo, "sdd-apply", "Do the task."), contractContentForE2E) + "\n"},
	} {
		if got := gateTaskIn(e, e.repo, tt.input, files); got != tt.want {
			t.Errorf("gate-task printed %q, want %q", got, tt.want)
		}
	}
	if got := gateTaskIn(e, e.repo, agentInputIn(e.repo, "sdd-apply", "Do the task."), nil); got != "{}\n" {
		t.Errorf("a contract that cannot be read: gate-task printed %q, want the pass-through", got)
	}
}
