package main

// Tests for 'projection hook --event PreToolUse'. As for the prompt hook,
// workflows are created and advanced through the real 'workflow' verbs,
// repositories are hand-made fixtures, and every test runs in its own state home
// (bindEnv sets XDG_STATE_HOME), so nothing here can reach real state. The gate
// is strictly read-only: the state directory must be byte-identical after every
// call, a denial included.

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
)

// --- harness ----------------------------------------------------------------

var gateArgs = []string{"hook", "--event", "PreToolUse"}

func runGateArgs(args []string, stdin, processCwd string) hookRun {
	return runProjectionArgs(args, stdin, processCwd)
}

// gateInput is the JSON Claude Code sends a PreToolUse hook.
func gateInput(t *testing.T, cwd, session, tool string, toolInput any) string {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"session_id":      session,
		"transcript_path": "/tmp/transcript.jsonl",
		"cwd":             cwd,
		"permission_mode": "default",
		"hook_event_name": "PreToolUse",
		"tool_name":       tool,
		"tool_input":      toolInput,
		"tool_use_id":     "toolu_01ABC",
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// gate runs the gate for the repository at cwd, as Claude Code would.
func (e hookEnv) gate(t *testing.T, cwd, tool string, toolInput any) hookRun {
	t.Helper()
	return runGateArgs(gateArgs, gateInput(t, cwd, "session-1", tool, toolInput), e.dir)
}

var (
	editInput  = map[string]any{"file_path": "/repo/main.go", "old_string": "a", "new_string": "b"}
	queryTools = []string{"mcp__longterm-mem__query", "mcp__plugin_x_longterm-mem__query"}
)

func queryArgs(project any) map[string]any {
	in := map[string]any{"query": "how did we decide", "limit": 5}
	if project != nil {
		in["project"] = project
	}
	return in
}

// decodeDeny requires stdout to be exactly the documented denial and returns its
// reason. Exit 0 (never 2) and an empty stderr are part of the contract.
func decodeDeny(t *testing.T, r hookRun) string {
	t.Helper()
	if !reflect.DeepEqual(r.codes, []int{0}) || r.stderr != "" {
		t.Fatalf("exits %v, stderr %q, want exactly one exit 0 (the JSON decides; exit 2 is not used) and no stderr", r.codes, r.stderr)
	}
	if !strings.HasPrefix(r.stdout, "{") || !strings.HasSuffix(r.stdout, "}\n") || strings.Count(r.stdout, "\n") != 1 || !json.Valid([]byte(r.stdout)) {
		t.Fatalf("stdout %q is not exactly one JSON object on one line", r.stdout)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(r.stdout), &top); err != nil {
		t.Fatal(err)
	}
	if len(top) != 1 || top["hookSpecificOutput"] == nil {
		t.Fatalf("stdout %s, want exactly one key, hookSpecificOutput", r.stdout)
	}
	var specific map[string]string
	if err := json.Unmarshal(top["hookSpecificOutput"], &specific); err != nil {
		t.Fatalf("hookSpecificOutput = %s: %v", top["hookSpecificOutput"], err)
	}
	if len(specific) != 3 || specific["hookEventName"] != "PreToolUse" || specific["permissionDecision"] != "deny" || specific["permissionDecisionReason"] == "" {
		t.Fatalf("hookSpecificOutput = %v, want exactly hookEventName PreToolUse, permissionDecision deny, and a reason", specific)
	}
	if strings.Contains(r.stdout, `"allow"`) {
		t.Errorf("stdout %s says allow, which would bypass the normal permission flow", r.stdout)
	}
	return specific["permissionDecisionReason"]
}

func assertDenied(t *testing.T, label string, r hookRun, wantIn ...string) {
	t.Helper()
	reason := decodeDeny(t, r)
	for _, want := range wantIn {
		if !strings.Contains(reason, want) {
			t.Errorf("%s: reason %q does not contain %q", label, reason, want)
		}
	}
}

// pausedEnv is a repository bound to a paused workflow of the profile.
func pausedEnv(t *testing.T, profile string) hookEnv {
	t.Helper()
	e := newHookEnv(t)
	e.running(t, "proj-1", "wf-1", profile)
	e.step(t, "proj-1", "wf-1", "pause")
	return e
}

// --- silence ----------------------------------------------------------------

func TestPreToolUseIsSilentWithoutABinding(t *testing.T) {
	e := newHookEnv(t)
	for _, tool := range append([]string{"Edit", "Write", "Bash"}, queryTools...) {
		assertSilent(t, "unbound "+tool, e.gate(t, e.repo, tool, queryArgs("other")))
	}
	// Outside every repository.
	assertSilent(t, "outside a repository", runGateArgs(gateArgs, gateInput(t, t.TempDir(), "s", "Edit", editInput), t.TempDir()))
	// A repository that is not the bound one.
	e2 := pausedEnv(t, "standalone-minimal")
	other := fixtureRepo(t, "unbound")
	assertSilent(t, "another repository", e2.gate(t, other, "Edit", editInput))
}

func TestPreToolUseIsSilentForInputItCannotUse(t *testing.T) {
	e := pausedEnv(t, "standalone-minimal")
	good := gateInput(t, e.repo, "s", "Edit", editInput)
	assertDenied(t, "the good input", runGateArgs(gateArgs, good, e.dir))

	oversized := `{"cwd":"` + e.repo + `","tool_name":"Edit","tool_input":{"content":"` + strings.Repeat("x", projection.MaxHookInputBytes) + `"}}`
	for name, stdin := range map[string]string{
		"empty stdin":                   "",
		"whitespace":                    " \n ",
		"not JSON":                      "hello",
		"an array":                      "[" + good + "]",
		"null":                          "null",
		"a truncated object":            good[:len(good)-10],
		"two objects":                   good + good,
		"a tool name of the wrong type": `{"cwd":"` + e.repo + `","tool_name":7,"tool_input":{}}`,
		"a cwd of the wrong type":       `{"cwd":42,"tool_name":"Edit"}`,
		"input over the size cap":       oversized,
		"another event's input":         strings.Replace(good, `"hook_event_name":"PreToolUse"`, `"hook_event_name":"UserPromptSubmit"`, 1),
		"an event name of wrong type":   `{"hook_event_name":7,"cwd":"` + e.repo + `","tool_name":"Edit"}`,
		"no tool name":                  `{"cwd":"` + e.repo + `","tool_input":{}}`,
	} {
		assertSilent(t, name, runGateArgs(gateArgs, stdin, e.dir))
	}
}

func TestPreToolUseReadsNoMoreThanTheCapPlusOneByte(t *testing.T) {
	e := pausedEnv(t, "standalone-minimal")
	stdin := &endlessReader{}
	var out, errBuf bytes.Buffer
	var codes []int
	runProjectionCore(gateArgs, e.repo, stdin, &out, &errBuf, func(c int) { codes = append(codes, c) })
	assertSilent(t, "an endless input", hookRun{codes: codes, stdout: out.String(), stderr: errBuf.String()})
	if stdin.read > projection.MaxHookInputBytes+1 {
		t.Errorf("the gate read %d bytes of an endless input, want at most %d", stdin.read, projection.MaxHookInputBytes+1)
	}
}

func TestPreToolUseStaysSilentWhenTheReaderFails(t *testing.T) {
	e := pausedEnv(t, "standalone-minimal")
	var out, errBuf bytes.Buffer
	var codes []int
	runProjectionCore(gateArgs, e.dir, failingReader{}, &out, &errBuf, func(c int) { codes = append(codes, c) })
	assertSilent(t, "a failing stdin", hookRun{codes: codes, stdout: out.String(), stderr: errBuf.String()})
}

// --- the paused edit gate ---------------------------------------------------

func TestPreToolUseDeniesTheEditToolsOfAPausedWorkflow(t *testing.T) {
	e := pausedEnv(t, "standalone-minimal")
	for _, tool := range []string{"Write", "Edit", "MultiEdit", "NotebookEdit"} {
		t.Run(tool, func(t *testing.T) {
			assertDenied(t, tool, e.gate(t, e.repo, tool, editInput),
				"wf-1", "proj-1", "paused", tool,
				"labdrian workflow resume --project proj-1 --workflow wf-1",
				"labdrian workflow unbind")
		})
	}
}

func TestPreToolUseNeverDeniesOtherToolsOfAPausedWorkflow(t *testing.T) {
	e := pausedEnv(t, "odd")
	for _, tool := range []string{
		"Bash", "Read", "Grep", "Glob", "Task", "WebFetch", "TodoWrite", "write", "Writer",
		"mcp__longterm-mem__get", "mcp__longterm-mem__promote", "mcp__engram__mem_save", "mcp__engram__mem_search",
	} {
		assertSilent(t, "paused/"+tool, e.gate(t, e.repo, tool, map[string]any{"command": "ls", "id": "x"}))
	}
}

func TestPreToolUseAllowsEditsOnceTheWorkflowIsResumedOrWhileItRuns(t *testing.T) {
	e := newHookEnv(t)
	e.create(t, "proj-1", "wf-1", "standalone-minimal")
	mustBindOK(t, e.repo, "proj-1", "wf-1")
	assertSilent(t, "created", e.gate(t, e.repo, "Edit", editInput))
	e.step(t, "proj-1", "wf-1", "start")
	assertSilent(t, "running", e.gate(t, e.repo, "Edit", editInput))
	e.step(t, "proj-1", "wf-1", "pause")
	assertDenied(t, "paused", e.gate(t, e.repo, "Edit", editInput), "paused")
	e.step(t, "proj-1", "wf-1", "resume")
	assertSilent(t, "resumed", e.gate(t, e.repo, "Edit", editInput))
}

// --- the memory gate --------------------------------------------------------

func TestPreToolUseChecksTheProjectOfALongtermMemQuery(t *testing.T) {
	for _, tool := range queryTools {
		t.Run(tool, func(t *testing.T) {
			e := newHookEnv(t)
			e.running(t, "proj-1", "wf-1", "odd")
			assertSilent(t, "the plan's project", e.gate(t, e.repo, tool, queryArgs("proj-1")))
			assertDenied(t, "another project", e.gate(t, e.repo, tool, queryArgs("proj-2")),
				`"proj-2"`, `"proj-1"`, "Query project", "labdrian workflow unbind")
			assertDenied(t, "no project", e.gate(t, e.repo, tool, queryArgs(nil)), "no project", `"proj-1"`)
			assertDenied(t, "an empty project", e.gate(t, e.repo, tool, queryArgs("")), "no project")
			assertDenied(t, "a project of the wrong type", e.gate(t, e.repo, tool, queryArgs(12)), "no project")
		})
	}
}

func TestPreToolUseNeverTouchesTheMemoryToolsThatCarryNoProject(t *testing.T) {
	e := newHookEnv(t)
	e.running(t, "proj-1", "wf-1", "odd")
	for _, tool := range []string{
		"mcp__longterm-mem__get", "mcp__longterm-mem__promote", "mcp__engram__mem_save", "mcp__engram__mem_search",
		"mcp__longterm-memx__query", "mcp__other__query", "longterm-mem__query",
	} {
		assertSilent(t, tool, e.gate(t, e.repo, tool, map[string]any{"engine_id": "e1", "project": "other"}))
	}
}

func TestPreToolUseDeniesEveryQueryWhenThePlanHasNoProject(t *testing.T) {
	e := newHookEnv(t)
	e.running(t, "proj-1", "wf-1", "standalone-minimal")
	for _, project := range []any{"proj-1", "other", nil, ""} {
		assertDenied(t, "scope none", e.gate(t, e.repo, "mcp__longterm-mem__query", queryArgs(project)),
			"standalone-minimal", "permits no project", "labdrian workflow unbind")
	}
	assertSilent(t, "get under scope none", e.gate(t, e.repo, "mcp__longterm-mem__get", map[string]any{"engine_id": "e1"}))
}

func TestPreToolUseAppliesTheMemoryGateWhilePausedToo(t *testing.T) {
	e := pausedEnv(t, "odd")
	assertDenied(t, "paused, wrong project", e.gate(t, e.repo, "mcp__longterm-mem__query", queryArgs("other")), `"other"`)
	assertSilent(t, "paused, right project", e.gate(t, e.repo, "mcp__longterm-mem__query", queryArgs("proj-1")))
}

// --- states it cannot follow ------------------------------------------------

// TestPreToolUseNeverUnbindsAClosedWorkflow: the prompt hook unbinds a closed
// workflow; the gate only stops gating. It changes nothing on disk.
func TestPreToolUseNeverUnbindsAClosedWorkflow(t *testing.T) {
	e := newHookEnv(t)
	e.running(t, "proj-1", "wf-1", "odd")
	e.step(t, "proj-1", "wf-1", "pause")
	e.step(t, "proj-1", "wf-1", "close", "--outcome", "abandoned", "--reason", "done")
	contents, tree := snapshotContents(t, e.state), snapshotTree(t, e.state)

	assertSilent(t, "Edit on a closed workflow", e.gate(t, e.repo, "Edit", editInput))
	assertSilent(t, "a wrong-project query on a closed workflow", e.gate(t, e.repo, "mcp__longterm-mem__query", queryArgs("other")))
	if loaded := loadStoredBinding(t, e.repo); loaded.Classification != projection.ClassificationOwned {
		t.Errorf("binding = %+v, want it kept: the gate never unbinds", loaded)
	}
	if !reflect.DeepEqual(snapshotContents(t, e.state), contents) || !reflect.DeepEqual(snapshotTree(t, e.state), tree) {
		t.Error("the gate changed the state directory")
	}
}

func TestPreToolUseAllowsWhatItCannotFollow(t *testing.T) {
	breaks := []struct {
		name   string
		break_ func(t *testing.T, e hookEnv)
	}{
		{"the workflow log is gone", func(t *testing.T, e hookEnv) {
			if err := os.Remove(e.workflowLog("proj-1", "wf-1")); err != nil {
				t.Fatal(err)
			}
		}},
		{"the workflow log is foreign", func(t *testing.T, e hookEnv) {
			writeFixtureFile(t, e.workflowLog("proj-1", "wf-1"), `{"hello":"world"}`+"\n")
		}},
		{"the workflow log is malformed", func(t *testing.T, e hookEnv) {
			writeFixtureFile(t, e.workflowLog("proj-1", "wf-1"), "not json\n")
		}},
		{"the workflow log drifted", func(t *testing.T, e hookEnv) {
			driftWorkflowLog(t, e.workflowLog("proj-1", "wf-1"))
		}},
		{"the binding file is foreign", func(t *testing.T, e hookEnv) {
			writeFixtureFile(t, e.bindingFile(t, e.repo), `{"hello":"world"}`+"\n")
		}},
		{"the binding file is malformed", func(t *testing.T, e hookEnv) {
			writeFixtureFile(t, e.bindingFile(t, e.repo), "hand-written notes\n")
		}},
		{"the binding file is gone", func(t *testing.T, e hookEnv) {
			if err := os.Remove(e.bindingFile(t, e.repo)); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tt := range breaks {
		t.Run(tt.name, func(t *testing.T) {
			e := pausedEnv(t, "standalone-minimal")
			tt.break_(t, e)
			contents := snapshotContents(t, e.state)
			// A paused workflow's edit and a query that scope none would deny are
			// both allowed, silently: the prompt hook is the one that warns.
			assertSilent(t, "Edit", e.gate(t, e.repo, "Edit", editInput))
			assertSilent(t, "query", e.gate(t, e.repo, "mcp__longterm-mem__query", queryArgs("other")))
			if !reflect.DeepEqual(snapshotContents(t, e.state), contents) {
				t.Error("the gate changed the state directory")
			}
		})
	}
}

// TestPreToolUseStaysSilentWhenTheStoreCannotBeOpened: the prompt hook warns
// about a store it cannot open on every prompt; the gate runs on every tool call,
// so it allows quietly instead of repeating that warning dozens of times.
func TestPreToolUseStaysSilentWhenTheStoreCannotBeOpened(t *testing.T) {
	e := pausedEnv(t, "standalone-minimal")
	t.Setenv("XDG_STATE_HOME", "relative/state")
	assertSilent(t, "Edit", e.gate(t, e.repo, "Edit", editInput))
}

// --- read-only, deterministic, restartable ----------------------------------

func TestPreToolUseChangesNothingOnDisk(t *testing.T) {
	type call struct {
		tool  string
		input any
	}
	calls := []call{
		{"Edit", editInput}, {"Write", editInput}, {"Bash", map[string]any{"command": "ls"}},
		{"mcp__longterm-mem__query", queryArgs("proj-1")}, {"mcp__longterm-mem__query", queryArgs("other")},
		{"mcp__longterm-mem__get", map[string]any{"engine_id": "e1"}},
	}
	for _, profile := range []string{"odd", "standalone-minimal"} {
		for _, status := range []string{"created", "running", "paused", "closed"} {
			t.Run(profile+"/"+status, func(t *testing.T) {
				e := newHookEnv(t)
				e.create(t, "proj-1", "wf-1", profile)
				mustBindOK(t, e.repo, "proj-1", "wf-1")
				switch status {
				case "running":
					e.step(t, "proj-1", "wf-1", "start")
				case "paused":
					e.step(t, "proj-1", "wf-1", "start")
					e.step(t, "proj-1", "wf-1", "pause")
				case "closed":
					e.step(t, "proj-1", "wf-1", "close", "--outcome", "abandoned", "--reason", "done")
				}
				contents, tree := snapshotContents(t, e.state), snapshotTree(t, e.state)
				denials := 0
				for i := 0; i < 2; i++ {
					for _, c := range calls {
						r := e.gate(t, e.repo, c.tool, c.input)
						if !reflect.DeepEqual(r.codes, []int{0}) || r.stderr != "" {
							t.Fatalf("%s: exits %v, stderr %q, want a plain exit 0", c.tool, r.codes, r.stderr)
						}
						if r.stdout != "" {
							decodeDeny(t, r)
							denials++
						}
					}
				}
				if status == "paused" && denials == 0 {
					t.Fatal("test bug: a paused workflow denied nothing, so the read-only proof covers no denial")
				}
				if after := snapshotContents(t, e.state); !reflect.DeepEqual(after, contents) {
					t.Errorf("the gate changed file contents:\nbefore %v\nafter  %v", contents, after)
				}
				if after := snapshotTree(t, e.state); !reflect.DeepEqual(after, tree) {
					t.Errorf("the gate changed a mode, size, or time:\nbefore %v\nafter  %v", tree, after)
				}
			})
		}
	}
}

func TestPreToolUseDecisionIsTheSameForEverySessionAndWorktree(t *testing.T) {
	e := pausedEnv(t, "standalone-minimal")
	want := e.gate(t, e.repo, "Edit", editInput)
	decodeDeny(t, want)
	for _, session := range []string{"a", "b", strings.Repeat("s", 500)} {
		got := runGateArgs(gateArgs, gateInput(t, e.repo, session, "Edit", editInput), e.dir)
		if got.stdout != want.stdout {
			t.Fatalf("session %.10q: %q, want %q", session, got.stdout, want.stdout)
		}
	}
	worktree := fixtureLinkedWorktree(t, e.repo, "wt")
	if got := e.gate(t, worktree, "Edit", editInput); got.stdout != want.stdout {
		t.Errorf("from a worktree: %q, want %q", got.stdout, want.stdout)
	}
}

// TestPreToolUseFallsBackToTheProcessDirectory: with no usable cwd in the input,
// the gate works in the directory the process runs in.
func TestPreToolUseFallsBackToTheProcessDirectory(t *testing.T) {
	e := pausedEnv(t, "standalone-minimal")
	want := e.gate(t, e.repo, "Edit", editInput)
	decodeDeny(t, want)
	for _, cwd := range []string{"", "relative/dir", "."} {
		in := `{"hook_event_name":"PreToolUse","tool_name":"Edit","tool_input":{},"cwd":"` + cwd + `"}`
		if got := runGateArgs(gateArgs, in, e.repo); got.stdout != want.stdout {
			t.Errorf("cwd %q: %q, want the denial %q", cwd, got.stdout, want.stdout)
		}
	}
	noCwd := `{"tool_name":"Edit","tool_input":{}}`
	if got := runGateArgs(gateArgs, noCwd, e.repo); got.stdout != want.stdout {
		t.Errorf("no cwd: %q, want the denial", got.stdout)
	}
	assertSilent(t, "the process outside a repository", runGateArgs(gateArgs, noCwd, t.TempDir()))
}

// --- failures never block the tool call -------------------------------------

// TestPreToolUseTurnsAPanicIntoAnAllowWithAWarning: a Go panic ends a process with
// status 2, which Claude Code reads as "block this tool call". The seam panics
// where a bug in the gate would, and the call must still go through: exit 0, the
// cause on stderr, no denial, and one short sanitized systemMessage.
func TestPreToolUseTurnsAPanicIntoAnAllowWithAWarning(t *testing.T) {
	e := pausedEnv(t, "standalone-minimal")
	beforeGateDecision = func() { panic("boom\nsecond line \x1b[31mred " + strings.Repeat("x", 5000)) }
	t.Cleanup(func() { beforeGateDecision = nil })

	r := e.gate(t, e.repo, "Edit", editInput)
	if !reflect.DeepEqual(r.codes, []int{0}) || !strings.Contains(r.stderr, "internal error: boom") {
		t.Fatalf("exits %v, stderr %q, want exit 0 and the panic reported on stderr", r.codes, r.stderr)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(r.stdout), &top); err != nil || len(top) != 1 || top["systemMessage"] == nil {
		t.Fatalf("stdout %q (%v), want only a systemMessage: the call is allowed", r.stdout, err)
	}
	var warning string
	if err := json.Unmarshal(top["systemMessage"], &warning); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(warning, "boom second line") || strings.ContainsAny(warning, "\n\x1b") || len(warning) > 700 {
		t.Errorf("warning %q is not one short clean line naming the error", warning)
	}
}

// TestPreToolUsePanicWarningSpeaksOfTheToolCallNotThePrompt: the warning of a
// recovered panic in the gate used to say "the prompt was not affected", which
// is about another event. It names the tool call, is a systemMessage only, and
// never carries a permission decision: a panic is not a reason to deny.
func TestPreToolUsePanicWarningSpeaksOfTheToolCallNotThePrompt(t *testing.T) {
	e := pausedEnv(t, "standalone-minimal")
	beforeGateDecision = func() { panic("boom") }
	t.Cleanup(func() { beforeGateDecision = nil })

	r := e.gate(t, e.repo, "Edit", editInput)
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(r.stdout), &top); err != nil || len(top) != 1 || top["systemMessage"] == nil {
		t.Fatalf("stdout %q (%v), want only a systemMessage", r.stdout, err)
	}
	var warning string
	if err := json.Unmarshal(top["systemMessage"], &warning); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(warning, "tool call") || strings.Contains(warning, "prompt") {
		t.Errorf("warning %q, want it to speak of the tool call and not of a prompt", warning)
	}
	if strings.Contains(r.stdout, "permissionDecision") {
		t.Errorf("stdout %q carries a permission decision; a panic must never decide", r.stdout)
	}
}

// TestPreToolUseTouchesNoStoreForAToolTheGateDoesNotCheck: relevance is decided
// from the tool name first. Only a file-edit tool or a longterm-mem query needs
// the binding and the workflow; for every other tool the hook answers before it
// opens either store, because it runs on every tool call of a bound session. The
// seam counts the moments the hook goes to the stores.
func TestPreToolUseTouchesNoStoreForAToolTheGateDoesNotCheck(t *testing.T) {
	e := pausedEnv(t, "standalone-minimal")
	accesses := 0
	onGateStoreAccess = func() { accesses++ }
	t.Cleanup(func() { onGateStoreAccess = nil })

	for _, tool := range []string{"Bash", "Read", "Grep", "Task", "mcp__longterm-mem__get", "mcp__engram__mem_save", ""} {
		assertSilent(t, "irrelevant/"+tool, e.gate(t, e.repo, tool, map[string]any{"command": "ls"}))
	}
	if accesses != 0 {
		t.Fatalf("the hook went to the stores %d times for tools the gate never checks, want 0", accesses)
	}

	// The seam is live: the tools the gate does check do reach the stores.
	for _, tool := range append([]string{"Write", "Edit", "MultiEdit", "NotebookEdit"}, queryTools...) {
		before := accesses
		e.gate(t, e.repo, tool, editInput)
		if accesses != before+1 {
			t.Errorf("%s: the hook went to the stores %d times, want exactly once", tool, accesses-before)
		}
	}
}

func TestPreToolUseExitsZeroEvenWhenStdoutCannotBeWritten(t *testing.T) {
	e := pausedEnv(t, "standalone-minimal")
	var errBuf bytes.Buffer
	var codes []int
	runProjectionCore(gateArgs, e.dir, strings.NewReader(gateInput(t, e.repo, "s", "Edit", editInput)), failingMemoryWriter{}, &errBuf, func(c int) { codes = append(codes, c) })
	if !reflect.DeepEqual(codes, []int{0}) || errBuf.String() != "" {
		t.Errorf("exits %v, stderr %q, want exit 0 and no stderr: a hook must never fail the call over its own output", codes, errBuf.String())
	}
}

// --- the command line and help ----------------------------------------------

func TestPreToolUseTakesTheLastOfARepeatedEventFlag(t *testing.T) {
	e := pausedEnv(t, "standalone-minimal")
	in := gateInput(t, e.repo, "s", "Edit", editInput)
	assertDenied(t, "PreToolUse last", runGateArgs([]string{"hook", "--event", "UserPromptSubmit", "--event", "PreToolUse"}, in, e.dir), "paused")
	// The other way round, the prompt hook answers, and the PreToolUse input is
	// not its input: silent.
	assertSilent(t, "UserPromptSubmit last", runGateArgs([]string{"hook", "--event", "PreToolUse", "--event", "UserPromptSubmit"}, in, e.dir))
}

func TestUsageDocumentsThePreToolUseGate(t *testing.T) {
	text := strings.Join(strings.Fields(captureUsage(t)), " ")
	for _, want := range []string{
		"engine projection hook --event UserPromptSubmit|PreToolUse",
		"permissionDecision",
		"Write, Edit, MultiEdit, and NotebookEdit",
		"never Bash",
		"longterm-mem query",
		"get, promote",
		"exit 0 always",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("usage does not contain %q", want)
		}
	}
}

// --- a separate process -----------------------------------------------------

// TestPreToolUseHookRunsAsASeparateProcess builds the engine binary and runs it
// the way Claude Code would: one process per call, the hook JSON on stdin, the
// answer on stdout, and a denial that is exit 0. Safety: HOME and
// XDG_STATE_HOME are this test's own temporary directories.
func TestPreToolUseHookRunsAsASeparateProcess(t *testing.T) {
	e := pausedEnv(t, "odd")
	binary := phase6BuildEngineBinary(t)
	home := t.TempDir()

	run := func(cwd, stdin string, args ...string) (string, string, int) {
		t.Helper()
		cmd := exec.Command(binary, args...)
		cmd.Dir = cwd
		cmd.Env = []string{"HOME=" + home, "XDG_STATE_HOME=" + e.state, "PATH=" + os.Getenv("PATH")}
		cmd.Stdin = strings.NewReader(stdin)
		var out, errBuf bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errBuf
		code := 0
		if err := cmd.Run(); err != nil {
			exitErr, ok := err.(*exec.ExitError)
			if !ok {
				t.Fatalf("run the binary: %v", err)
			}
			code = exitErr.ExitCode()
		}
		return out.String(), errBuf.String(), code
	}
	hook := []string{"projection", "hook", "--event", "PreToolUse"}
	contents := snapshotContents(t, e.state)

	// A denial: exit 0 and the documented JSON, the same bytes as in process.
	inProcess := e.gate(t, e.repo, "Edit", editInput)
	decodeDeny(t, inProcess)
	stdout, stderr, code := run(e.dir, gateInput(t, e.repo, "session-a", "Edit", editInput), hook...)
	if code != 0 || stderr != "" || stdout != inProcess.stdout {
		t.Fatalf("a denial: exit %d, stderr %q, stdout\n%s\nwant exit 0 and the in-process output\n%s", code, stderr, stdout, inProcess.stdout)
	}
	// An allow: nothing on either stream, exit 0.
	for _, tool := range []string{"Bash", "Read"} {
		if stdout, stderr, code := run(e.dir, gateInput(t, e.repo, "session-a", tool, map[string]any{"command": "ls"}), hook...); code != 0 || stderr != "" || stdout != "" {
			t.Errorf("%s: exit %d, stdout %q, stderr %q, want a silent exit 0", tool, code, stdout, stderr)
		}
	}
	// The memory gate through the process, with a plugin-prefixed name.
	if stdout, _, code := run(e.dir, gateInput(t, e.repo, "s", "mcp__plugin_x_longterm-mem__query", queryArgs("other")), hook...); code != 0 || !strings.Contains(stdout, `\"other\"`) || !strings.Contains(stdout, `"permissionDecision":"deny"`) {
		t.Errorf("a wrong-project query: exit %d, stdout %q, want exit 0 and a denial naming the project", code, stdout)
	}
	if stdout, stderr, code := run(e.dir, gateInput(t, e.repo, "s", "mcp__longterm-mem__query", queryArgs("proj-1")), hook...); code != 0 || stdout != "" || stderr != "" {
		t.Errorf("a right-project query: exit %d, stdout %q, stderr %q, want a silent exit 0", code, stdout, stderr)
	}
	// Malformed input is silent and exit 0, and a usage error is exit 1.
	if stdout, stderr, code := run(e.dir, "not json", hook...); code != 0 || stdout != "" || stderr != "" {
		t.Errorf("malformed input: exit %d, stdout %q, stderr %q, want a silent exit 0", code, stdout, stderr)
	}
	if _, _, code := run(e.dir, "", "projection", "hook", "--event", "Stop"); code != 1 {
		t.Errorf("an unsupported event: exit %d, want 1", code)
	}
	if !reflect.DeepEqual(snapshotContents(t, e.state), contents) {
		t.Error("the gate process changed the state directory")
	}
}
