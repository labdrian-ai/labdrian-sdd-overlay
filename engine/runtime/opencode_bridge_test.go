package runtime_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/enginetest"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/hookwire"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/repotest"
)

// The hook bridge (S10) hands each OpenCode event to the engine command that answers the same event
// for Claude Code, and turns the answer into what OpenCode understands. These tests run the
// installed plugin under node against a stand-in for the engine that records what it was asked
// and answers with what the engine's own encoders (engine/hookwire) write, which are the bytes the
// golden files of the hooks pin; engine/cmd/opencode_bridge_e2e_test.go runs the same plugin against
// the real engine.

// fakeEngineScript is the stand-in for the engine binary. It appends "<event><TAB><arguments>" to
// calls.log, keeps its stdin in stdin.<n> for the n-th call, and prints answer.<event> when there is
// one. answer.<event>.<n> answers the n-th call alone and wins over it. The event is the --event value, SkillPaths for `roles skill-paths`, or Stop for `review stop-hook`, which takes none.
const fakeEngineScript = `#!/bin/sh
dir=$(dirname "$0")
event=Stop
previous=
for argument in "$@"; do
  if [ "$previous" = "--event" ]; then event="$argument"; fi
  previous="$argument"
done
if [ "$1 $2" = "roles skill-paths" ]; then event=SkillPaths; fi
count=$(wc -l < "$dir/calls.log" 2>/dev/null || echo 0)
count=$((count + 1))
printf '%s\t%s\n' "$event" "$*" >> "$dir/calls.log"
cat > "$dir/stdin.$count"
if [ -f "$dir/sleep.$event" ]; then sleep "$(cat "$dir/sleep.$event")"; fi
if [ -f "$dir/exit.$event" ]; then cat "$dir/stderr.$event" >&2; exit "$(cat "$dir/exit.$event")"; fi
if [ -f "$dir/exit" ]; then cat "$dir/stderr" >&2; exit "$(cat "$dir/exit")"; fi
if [ -f "$dir/answer.$event.$count" ]; then cat "$dir/answer.$event.$count"
elif [ -f "$dir/answer.$event" ]; then cat "$dir/answer.$event"; fi
exit 0
`

type fakeEngine struct {
	path string
	dir  string
}

// engineCall is one call the stand-in recorded.
type engineCall struct {
	Event string
	Args  string
	Stdin map[string]any
	Raw   string
}

// newFakeEngine writes the stand-in with the answers it gives, by event name.
func newFakeEngine(t *testing.T, answers map[string][]byte) fakeEngine {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "engine")
	if err := os.WriteFile(path, []byte(fakeEngineScript), 0o755); err != nil {
		t.Fatalf("write fake engine: %v", err)
	}
	for event, answer := range answers {
		if err := os.WriteFile(filepath.Join(dir, "answer."+event), answer, 0o644); err != nil {
			t.Fatalf("write fake engine answer: %v", err)
		}
	}
	return fakeEngine{path: path, dir: dir}
}

// failWith makes the stand-in exit with code and say message on stderr, as an engine too old for a flag does.
func (e fakeEngine) failWith(t *testing.T, code int, message string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(e.dir, "exit"), []byte(fmt.Sprint(code)), 0o644); err != nil {
		t.Fatalf("write fake engine exit: %v", err)
	}
	if err := os.WriteFile(filepath.Join(e.dir, "stderr"), []byte(message), 0o644); err != nil {
		t.Fatalf("write fake engine stderr: %v", err)
	}
}

// failEventWith makes the stand-in fail only the calls of one event, with code and message on stderr.
func (e fakeEngine) failEventWith(t *testing.T, event string, code int, message string) {
	t.Helper()
	for name, content := range map[string]string{"exit." + event: fmt.Sprint(code), "stderr." + event: message} {
		if err := os.WriteFile(filepath.Join(e.dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write fake engine %s: %v", name, err)
		}
	}
}

// hangEventFor makes the stand-in sleep for seconds before it answers a call of one event.
func (e fakeEngine) hangEventFor(t *testing.T, event string, seconds int) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(e.dir, "sleep."+event), []byte(fmt.Sprint(seconds)), 0o644); err != nil {
		t.Fatalf("write fake engine sleep: %v", err)
	}
}

// calls is what the stand-in was asked, in order.
func (e fakeEngine) calls(t *testing.T) []engineCall {
	t.Helper()
	log, err := os.ReadFile(filepath.Join(e.dir, "calls.log"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("read fake engine calls: %v", err)
	}
	var calls []engineCall
	for i, line := range strings.Split(strings.TrimSuffix(string(log), "\n"), "\n") {
		event, args, _ := strings.Cut(line, "\t")
		stdin, err := os.ReadFile(filepath.Join(e.dir, fmt.Sprintf("stdin.%d", i+1)))
		if err != nil {
			t.Fatalf("read fake engine stdin: %v", err)
		}
		call := engineCall{Event: event, Args: args, Raw: string(stdin)}
		// A hook is handed a JSON object; `review frozen-context` (event Stop: it takes no --event) is
		// handed the brief as plain text, and so is `roles skill-paths` the prompt.
		if !strings.HasPrefix(args, "review frozen-context") && !strings.HasPrefix(args, "roles skill-paths") {
			if err := json.Unmarshal(stdin, &call.Stdin); err != nil {
				t.Fatalf("fake engine stdin %q is not a JSON object: %v", stdin, err)
			}
		}
		calls = append(calls, call)
	}
	return calls
}

// encoded is the answer an encoder of engine/hookwire writes.
func encoded(t *testing.T, reply interface{ Encode() ([]byte, error) }) []byte {
	t.Helper()
	out, err := reply.Encode()
	if err != nil {
		t.Fatalf("encode golden answer: %v", err)
	}
	return out
}

// bridgeFixture installs the repository plugin with a minimalism config and returns the plugin path.
func bridgeFixture(t *testing.T) string {
	t.Helper()
	root, pluginPath := writePluginFixture(t)
	writeJSONFile(t, filepath.Join(root, "labdrian-runtime-parity.json"), minimalismConfig(pluginPath, root))
	return pluginPath
}

const (
	eventPreToolUse  = "PreToolUse"
	eventPostToolUse = "PostToolUse"
	bridgeSource     = "--source opencode"
)

func TestOpenCodeBridgeDeniesACallTheEngineDeniesWithToolError(t *testing.T) {
	requireNode(t)
	const reason = "workflow wf-1 of project proj-1 is paused: patch is denied"
	engine := newFakeEngine(t, map[string][]byte{eventPreToolUse: encoded(t, hookwire.PreToolUseReply{Deny: true, Reason: reason})})
	pluginPath := bridgeFixture(t)

	out := runPluginWithEngine(t, pluginPath, engine.path, `const call = { tool: "patch", sessionID: "ses_1", agent: "foreman", messageID: "msg_1", id: "call_1", input: { filePath: "/work/repo/main.go", patch: "p" } };
let thrown = null;
try { await beforeHook(call); } catch (error) { thrown = { name: error.name, message: error.message, fields: error.fields }; }
console.log(JSON.stringify({ thrown, input: call.input }));`)
	var got struct {
		Thrown *struct {
			Name    string         `json:"name"`
			Message string         `json:"message"`
			Fields  map[string]any `json:"fields"`
		} `json:"thrown"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode result: %v\nraw=%q", err, out)
	}
	if got.Thrown == nil || got.Thrown.Name != "Tool.Error" || got.Thrown.Message != reason || got.Thrown.Fields["message"] != reason {
		t.Fatalf("the engine's denial should throw a Tool.Error({message: %q}), got %#v", reason, got.Thrown)
	}

	calls := engine.calls(t)
	if len(calls) != 1 || calls[0].Event != eventPreToolUse || calls[0].Args != "projection hook --event PreToolUse "+bridgeSource {
		t.Fatalf("the bridge should run `projection hook --event PreToolUse --source opencode` once, got %#v", calls)
	}
	stdin := calls[0].Stdin
	toolInput, _ := stdin["tool_input"].(map[string]any)
	if stdin["hook_event_name"] != eventPreToolUse || stdin["tool_name"] != "patch" || stdin["session_id"] != "ses_1" ||
		stdin["tool_use_id"] != "call_1" || toolInput["filePath"] != "/work/repo/main.go" {
		t.Fatalf("the engine should be handed the call as Claude Code's envelope, with the OpenCode tool name (the engine adapts it): %#v", stdin)
	}
	if cwd, _ := stdin["cwd"].(string); !filepath.IsAbs(cwd) {
		t.Fatalf("the envelope should carry the absolute working directory, got %#v", stdin["cwd"])
	}
}

func TestOpenCodeBridgeLetsACallThroughWhenTheEngineHasNothingToSay(t *testing.T) {
	requireNode(t)
	engine := newFakeEngine(t, nil)
	pluginPath := bridgeFixture(t)

	out := runPluginWithEngine(t, pluginPath, engine.path, `const call = { tool: "shell", sessionID: "ses_1", id: "call_1", input: { command: "ls" } };
await beforeHook(call);
console.log(JSON.stringify(call));`)
	var got struct {
		Tool  string         `json:"tool"`
		Input map[string]any `json:"input"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode result: %v\nraw=%q", err, out)
	}
	if got.Tool != "shell" || got.Input["command"] != "ls" || len(got.Input) != 1 {
		t.Fatalf("an allowed call must be left as it was, got %#v", got)
	}
	if calls := engine.calls(t); len(calls) != 1 {
		t.Fatalf("every tool call is asked of the engine once, got %#v", calls)
	}
}

func TestOpenCodeBridgeLetsACallThroughWhenTheEngineFailsAndSaysSoOnce(t *testing.T) {
	requireNode(t)
	pluginPath := bridgeFixture(t)
	stale := newFakeEngine(t, nil)
	stale.failWith(t, 1, `error: projection hook: unsupported --source "opencode" (supported: claude, codex)`)

	for name, engine := range map[string]string{"missing": filepath.Join(t.TempDir(), "no-engine"), "stale": stale.path} {
		t.Run(name, func(t *testing.T) {
			out := runPluginWithEngine(t, pluginPath, engine, `const afterHook = hooks.get("execute.after");
const results = [];
for (const id of ["call_1", "call_2"]) {
  const call = { tool: "shell", sessionID: "ses_1", id, input: { command: "ls" } };
  await beforeHook(call);
  const result = { output: "ok", content: [{ type: "text", text: "ok" }] };
  await afterHook({ ...call, status: "completed", result });
  results.push(result.content.map((part) => part.text));
}
console.log(JSON.stringify(results));`)
			var got [][]string
			if err := json.Unmarshal(out, &got); err != nil {
				t.Fatalf("decode result: %v\nraw=%q", err, out)
			}
			if len(got) != 2 || len(got[0]) != 2 || !strings.Contains(got[0][1], "labdrian hooks are not running") {
				t.Fatalf("the first call's result should say the hooks are not running, got %#v", got)
			}
			if len(got[1]) != 1 {
				t.Fatalf("the unavailable engine is reported once, got %#v", got)
			}
		})
	}
}

func TestOpenCodeBridgeSendsACompletedCallToPostToolUse(t *testing.T) {
	requireNode(t)
	const line = "the evidence of this memory read was not recorded"
	engine := newFakeEngine(t, map[string][]byte{eventPostToolUse: encoded(t, hookwire.PostToolUseReply{Context: line})})
	pluginPath := bridgeFixture(t)

	out := runPluginWithEngine(t, pluginPath, engine.path, `const afterHook = hooks.get("execute.after");
const call = { tool: "longterm-mem_query", sessionID: "ses_1", id: "call_7", input: { project: "proj-1", query: "q" } };
const result = { output: "found", content: [{ type: "text", text: "found" }] };
await afterHook({ ...call, status: "completed", result });
const failed = { content: [{ type: "text", text: "boom" }] };
await afterHook({ ...call, id: "call_8", status: "error", error: new Error("boom"), result: failed });
console.log(JSON.stringify({ completed: result.content, failed: failed.content }));`)
	var got struct {
		Completed []map[string]string `json:"completed"`
		Failed    []map[string]string `json:"failed"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode result: %v\nraw=%q", err, out)
	}
	if len(got.Completed) != 2 || got.Completed[0]["text"] != "found" || got.Completed[1]["type"] != "text" || got.Completed[1]["text"] != line {
		t.Fatalf("the engine's context line should join the result as one more text content, got %#v", got.Completed)
	}
	if len(got.Failed) != 1 {
		t.Fatalf("a failed call is not a PostToolUse, got %#v", got.Failed)
	}

	calls := engine.calls(t)
	if len(calls) != 1 || calls[0].Event != eventPostToolUse || calls[0].Args != "projection hook --event PostToolUse "+bridgeSource {
		t.Fatalf("the bridge should run `projection hook --event PostToolUse --source opencode` for the completed call alone, got %#v", calls)
	}
	stdin := calls[0].Stdin
	response, _ := stdin["tool_response"].(map[string]any)
	if stdin["hook_event_name"] != eventPostToolUse || stdin["tool_name"] != "longterm-mem_query" || stdin["tool_use_id"] != "call_7" ||
		stdin["session_id"] != "ses_1" || response["output"] != "found" {
		t.Fatalf("the engine should be handed the call and its result as Claude Code's envelope: %#v", stdin)
	}
}

func TestOpenCodeBridgeCarriesAWarningOfThePreToolUseToTheResultOfTheSameCall(t *testing.T) {
	requireNode(t)
	const warning = "the memory gate could not check this query"
	engine := newFakeEngine(t, map[string][]byte{eventPreToolUse: encoded(t, hookwire.PreToolUseReply{Warning: warning})})
	pluginPath := bridgeFixture(t)

	out := runPluginWithEngine(t, pluginPath, engine.path, `const afterHook = hooks.get("execute.after");
const call = { tool: "memorycard_search", sessionID: "ses_1", id: "call_3", input: { query: "q" } };
await beforeHook(call);
const result = { content: [{ type: "text", text: "ok" }] };
await afterHook({ ...call, status: "completed", result });
const other = { content: [{ type: "text", text: "ok" }] };
await afterHook({ ...call, id: "call_4", status: "completed", result: other });
console.log(JSON.stringify({ result: result.content.map((part) => part.text), other: other.content.map((part) => part.text) }));`)
	var got struct {
		Result []string `json:"result"`
		Other  []string `json:"other"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode result: %v\nraw=%q", err, out)
	}
	if strings.Join(got.Result, "|") != "ok|"+warning {
		t.Fatalf("the warning should reach the result of the call it was about, got %#v", got.Result)
	}
	if len(got.Other) != 1 {
		t.Fatalf("a warning belongs to one call, got %#v", got.Other)
	}
}

func TestOpenCodeBridgeKeepsTheContractInjectionAfterTheGate(t *testing.T) {
	requireNode(t)
	pluginPath := bridgeFixture(t)
	const contract = "skills/_shared/minimalism-contract.md"
	launch := `const call = { tool: "subagent", sessionID: "ses_1", id: "call_9", input: { prompt: %q, agent: "sdd-tasks" } };
let denied = false;
try { await beforeHook(call); } catch (error) { denied = true; }
console.log(JSON.stringify({ denied, prompt: call.input.prompt }));`

	allowing := newFakeEngine(t, nil)
	var got struct {
		Denied bool   `json:"denied"`
		Prompt string `json:"prompt"`
	}
	if err := json.Unmarshal(runPluginWithEngine(t, pluginPath, allowing.path, fmt.Sprintf(launch, basePromptFixture)), &got); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if got.Denied || !strings.Contains(got.Prompt, contract) {
		t.Fatalf("an allowed subagent launch should still get the contract, got %#v", got)
	}
	calls := allowing.calls(t)
	if len(calls) != 1 {
		t.Fatalf("the engine should be asked once, got %#v", calls)
	}
	if asked, _ := calls[0].Stdin["tool_input"].(map[string]any)["prompt"].(string); asked != basePromptFixture {
		t.Fatalf("the engine should judge the launch as the agent wrote it, before the contract is added, got %#v", calls)
	}

	denying := newFakeEngine(t, map[string][]byte{eventPreToolUse: encoded(t, hookwire.PreToolUseReply{Deny: true, Reason: "no"})})
	if err := json.Unmarshal(runPluginWithEngine(t, pluginPath, denying.path, fmt.Sprintf(launch, basePromptFixture)), &got); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if !got.Denied || got.Prompt != basePromptFixture {
		t.Fatalf("a denied launch must not be rewritten, got %#v", got)
	}
}

// Under Code Mode one memorycard read fires two execute.before events: the outer execute carrying
// the model's code, then the inner memorycard_mem_search carrying {query, project} (L102 (1)). The
// bridge hands every one of them to the engine as OpenCode sent it, and a denial of the inner call
// throws the engine's reason.
func TestOpenCodeBridgeJudgesTheInnerMemorycardCallOfACodeModeExecute(t *testing.T) {
	requireNode(t)
	const reason = "this memorycard read names no project"
	engine := newFakeEngine(t, nil)
	// The stand-in answers the second call alone: the outer execute is let through.
	if err := os.WriteFile(filepath.Join(engine.dir, "answer."+eventPreToolUse+".2"), encoded(t, hookwire.PreToolUseReply{Deny: true, Reason: reason}), 0o644); err != nil {
		t.Fatalf("write fake engine answer: %v", err)
	}
	pluginPath := bridgeFixture(t)

	out := runPluginWithEngine(t, pluginPath, engine.path, `const outer = { tool: "execute", sessionID: "ses_1", id: "call_1", input: { code: "await memorycard_mem_search({ query: 'q', project: 'proj-1' })" } };
const inner = { tool: "memorycard_mem_search", sessionID: "ses_1", id: "call_2", input: { query: "q", project: "proj-1" } };
const thrown = {};
for (const [name, call] of [["outer", outer], ["inner", inner]]) {
  thrown[name] = null;
  try { await beforeHook(call); } catch (error) { thrown[name] = { name: error.name, message: error.message, fields: error.fields }; }
}
console.log(JSON.stringify(thrown));`)
	var got map[string]*struct {
		Name    string         `json:"name"`
		Message string         `json:"message"`
		Fields  map[string]any `json:"fields"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode result: %v\nraw=%q", err, out)
	}
	if got["outer"] != nil {
		t.Fatalf("the outer execute should be let through, got %#v", got["outer"])
	}
	if inner := got["inner"]; inner == nil || inner.Name != "Tool.Error" || inner.Message != reason || inner.Fields["message"] != reason {
		t.Fatalf("the engine's denial of the inner call should throw a Tool.Error({message: %q}), got %#v", reason, got["inner"])
	}

	calls := engine.calls(t)
	if len(calls) != 2 {
		t.Fatalf("the engine should be asked twice, once per execute.before event, got %#v", calls)
	}
	for i, want := range []struct {
		tool  string
		field string
		value any
	}{
		{"execute", "code", "await memorycard_mem_search({ query: 'q', project: 'proj-1' })"},
		{"memorycard_mem_search", "project", "proj-1"},
	} {
		call := calls[i]
		toolInput, _ := call.Stdin["tool_input"].(map[string]any)
		if call.Event != eventPreToolUse || call.Args != "projection hook --event PreToolUse "+bridgeSource ||
			call.Stdin["tool_name"] != want.tool || toolInput[want.field] != want.value {
			t.Errorf("call %d: want a PreToolUse of %s with %s %v as OpenCode sent it, got %#v", i+1, want.tool, want.field, want.value, call)
		}
	}
	if inputs, _ := calls[1].Stdin["tool_input"].(map[string]any); inputs["query"] != "q" {
		t.Errorf("the inner call keeps its query, got %#v", inputs)
	}
}

// L115 left the plugin's deny path for an inner read unobserved: the model refused the unscoped call.
// This runs the plugin over the real engine, with a workflow of project proj-1 bound to the repository
// the node process runs in, and sends the inner memorycard read of a Code Mode execute without a model:
// the reads that name no project or another project throw the reason the same engine binary prints for
// that envelope, the read of the workflow's own project and the outer execute go ahead (S9, L117).
func TestOpenCodeBridgeThrowsTheEnginesReasonForADeniedInnerMemorycardRead(t *testing.T) {
	requireNode(t)
	binary := enginetest.Binary(t)
	home, state := t.TempDir(), t.TempDir()
	// The workflow is created from a directory outside any repository, so BindRunning names its project
	// with --project, the same name as the repository's.
	workDir := filepath.Join(t.TempDir(), "proj-1")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	repo := repotest.Repo(t, "proj-1")
	env := []string{"HOME=" + home, "XDG_STATE_HOME=" + state, "PATH=" + t.TempDir()}
	enginetest.BindRunning(t, binary, env, workDir, repo, "proj-1", "wf-1", "sdd")
	before := stateContents(t, state)
	pluginPath := bridgeFixture(t)

	// reasonFor is what the engine prints for the envelope the plugin builds from call.
	reasonFor := func(call map[string]any) string {
		t.Helper()
		stdin, err := json.Marshal(map[string]any{
			"hook_event_name": eventPreToolUse, "session_id": call["sessionID"], "cwd": repo,
			"tool_name": call["tool"], "tool_input": call["input"], "tool_use_id": call["id"],
		})
		if err != nil {
			t.Fatal(err)
		}
		command := exec.Command(binary, "projection", "hook", "--event", eventPreToolUse, "--source", "opencode")
		command.Dir, command.Env, command.Stdin = repo, env, bytes.NewReader(stdin)
		out, err := command.Output()
		if err != nil {
			t.Fatalf("engine hook: %v", err)
		}
		var answer struct {
			Specific struct {
				Decision string `json:"permissionDecision"`
				Reason   string `json:"permissionDecisionReason"`
			} `json:"hookSpecificOutput"`
		}
		if err := json.Unmarshal(out, &answer); err != nil && len(bytes.TrimSpace(out)) > 0 {
			t.Fatalf("decode the engine's answer %q: %v", out, err)
		}
		if answer.Specific.Decision != "deny" {
			return ""
		}
		if answer.Specific.Reason == "" {
			t.Fatalf("the engine denied without a reason: %q", out)
		}
		return answer.Specific.Reason
	}
	cases := []struct {
		name string
		call map[string]any
		deny bool
	}{
		{"the outer execute", map[string]any{"tool": "execute", "sessionID": "ses_1", "id": "call_1", "input": map[string]any{"code": "await memorycard_mem_search({ query: 'q' })"}}, false},
		{"a read with no project", map[string]any{"tool": "memorycard_mem_search", "sessionID": "ses_1", "id": "call_2", "input": map[string]any{"query": "q"}}, true},
		{"a read of another project", map[string]any{"tool": "memorycard_mem_search", "sessionID": "ses_1", "id": "call_3", "input": map[string]any{"query": "q", "project": "other"}}, true},
		{"a read of the workflow's project", map[string]any{"tool": "memorycard_mem_search", "sessionID": "ses_1", "id": "call_4", "input": map[string]any{"query": "q", "project": "proj-1"}}, false},
	}
	calls := make([]map[string]any, len(cases))
	want := make([]string, len(cases))
	for i, c := range cases {
		calls[i], want[i] = c.call, reasonFor(c.call)
		if c.deny == (want[i] == "") {
			t.Fatalf("%s: the engine's own answer should deny=%v, its reason is %q", c.name, c.deny, want[i])
		}
	}
	callsJSON, err := json.Marshal(calls)
	if err != nil {
		t.Fatal(err)
	}

	out := runPluginWithEnv(t, pluginPath, []string{engineEnvironment + "=" + binary, "HOME=" + home, "XDG_STATE_HOME=" + state}, fmt.Sprintf(`process.chdir(%q);
const thrown = [];
for (const call of %s) {
  try { await beforeHook(call); thrown.push(null); } catch (error) { thrown.push({ name: error.name, message: error.message, fields: error.fields }); }
}
console.log(JSON.stringify(thrown));`, repo, callsJSON))
	var got []*struct {
		Name    string         `json:"name"`
		Message string         `json:"message"`
		Fields  map[string]any `json:"fields"`
	}
	if err := json.Unmarshal(out, &got); err != nil || len(got) != len(cases) {
		t.Fatalf("decode result: %v\nraw=%q", err, out)
	}
	for i, c := range cases {
		switch {
		case !c.deny && got[i] != nil:
			t.Errorf("%s should go ahead, got %#v", c.name, got[i])
		case c.deny && (got[i] == nil || got[i].Name != "Tool.Error" || got[i].Message != want[i] || got[i].Fields["message"] != want[i]):
			t.Errorf("%s should throw a Tool.Error with the engine's reason %q, got %#v", c.name, want[i], got[i])
		}
	}
	if !reflect.DeepEqual(before, stateContents(t, state)) {
		t.Error("the gate changed the state home")
	}
}

// stateContents is the content of every file under root, by path.
func stateContents(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		files[path] = string(data)
		return err
	})
	if err != nil {
		t.Fatalf("read the state home: %v", err)
	}
	return files
}

// B4 (L99, L100, L107): on OpenCode the reviewer cannot run git, so on a `subagent` launch of the agent
// `reviewer` the bridge hands the brief to `review frozen-context` and launches the reviewer with what it
// prints, the brief and the frozen context of its candidate. The verb's own refusals and any failure of
// it deny the launch: a review without its diff must not run.

const (
	reviewerBrief = "Review base aaaa head bbbb.\n\"base\": \"aaaa\"\n\"head\": \"bbbb\"\n"
	frozenReply   = reviewerBrief + "\n## Frozen context (base aaaa, head bbbb)\nchanged paths: a.go\n```diff\n+x\n```\n"
	// frozenEvent is the event the stand-in files `review frozen-context` under: the verb takes no --event.
	frozenEvent = "Stop"
)

// launchOf is a node snippet that runs a launch of agent through the hook and prints what came of it.
func launchOf(tool, agent, prompt string) string {
	return fmt.Sprintf(`const call = { tool: %q, sessionID: "ses_1", id: "call_1", input: { prompt: %q, agent: %q } };
let thrown = null;
try { await beforeHook(call); } catch (error) { thrown = { name: error.name, message: error.message, fields: error.fields }; }
console.log(JSON.stringify({ thrown, input: call.input }));`, tool, prompt, agent)
}

type launchOutcome struct {
	Thrown *struct {
		Name    string         `json:"name"`
		Message string         `json:"message"`
		Fields  map[string]any `json:"fields"`
	} `json:"thrown"`
	Input map[string]any `json:"input"`
}

func decodeLaunch(t *testing.T, out []byte) launchOutcome {
	t.Helper()
	var got launchOutcome
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode result: %v\nraw=%q", err, out)
	}
	return got
}

// pluginSHA256 is the sha256 of engine/runtime/labdrian-runtime-parity-plugin.mjs as T60 found it: the
// change that lists the skills a prose names is the engine's, so the plugin file must not change with it.
const pluginSHA256 = "1f4d76a9d3492dbb19e1cc651c92b479c9b18ee6127dbdd77cb329bf908cc058"

// TestOpenCodeBridgeLaunchesABuilderWithTheSkillItsProseNames (T60, S8 L162): through the real engine's
// `roles skill-paths`, a builder launch whose prose names work-unit-commits and lists nothing reaches the
// child with that skill's absolute SKILL.md path under the header, and the plugin is the file it was.
func TestOpenCodeBridgeLaunchesABuilderWithTheSkillItsProseNames(t *testing.T) {
	requireNode(t)
	engineDir, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	built := filepath.Join(t.TempDir(), "engine-real")
	build := exec.Command("go", "build", "-o", built, ".")
	build.Dir = filepath.Join(engineDir, "cmd")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build the engine: %v\n%s", err, out)
	}
	wrapper := filepath.Join(t.TempDir(), "engine")
	script := "#!/bin/sh\nif [ \"$1 $2\" = \"roles skill-paths\" ]; then exec " + built + " \"$@\"; fi\ncat > /dev/null\nexit 0\n"
	if err := os.WriteFile(wrapper, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	overlay := shippedOverlay(t)
	root, pluginPath := writePluginFixture(t)
	config := minimalismConfig(pluginPath, root)
	config["skills_root"] = filepath.Join(overlay, "skills")
	writeJSONFile(t, filepath.Join(root, "labdrian-runtime-parity.json"), config)
	prompt := "Build T1 as small commits, following the work-unit-commits skill.\n"
	path := filepath.Join(overlay, "skills", "work-unit-commits", "SKILL.md")

	got := decodeLaunch(t, runPluginWithEngine(t, pluginPath, wrapper, launchOf("subagent", "builder", prompt)))

	want := strings.TrimRight(prompt, "\n") + "\n\n## Skills to load before work\n" + path + "\n"
	if got.Thrown != nil || got.Input["prompt"] != want {
		t.Fatalf("the child should get the prompt with %s under the header, got %#v", path, got)
	}
	if sum := fmt.Sprintf("%x", sha256.Sum256(repositoryPluginSource(t))); sum != pluginSHA256 {
		t.Fatalf("the plugin changed: sha256 %s, want %s", sum, pluginSHA256)
	}
}

func TestOpenCodeBridgeLaunchesTheReviewerWithTheFrozenContextOfItsBrief(t *testing.T) {
	requireNode(t)
	engine := newFakeEngine(t, map[string][]byte{frozenEvent: []byte(frozenReply)})
	pluginPath := bridgeFixture(t)

	got := decodeLaunch(t, runPluginWithEngine(t, pluginPath, engine.path, launchOf("subagent", "reviewer", reviewerBrief)))
	if got.Thrown != nil {
		t.Fatalf("a reviewer launch whose brief the verb answers should go through, got %#v", got.Thrown)
	}
	if got.Input["prompt"] != frozenReply || got.Input["agent"] != "reviewer" {
		t.Fatalf("the reviewer should be launched with the verb's stdout as its prompt, got %#v", got.Input)
	}

	calls := engine.calls(t)
	if len(calls) != 2 || calls[0].Event != eventPreToolUse {
		t.Fatalf("the gate should be asked first, then the verb, got %#v", calls)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if want := "review frozen-context --cwd " + cwd; calls[1].Args != want {
		t.Errorf("the verb should run as %q, got %q", want, calls[1].Args)
	}
	if calls[1].Raw != reviewerBrief {
		t.Errorf("the verb should be handed the brief as the agent wrote it, as plain text, got %q", calls[1].Raw)
	}
}

func TestOpenCodeBridgeRunsTheReviewerRewriteAfterTheGateAndBeforeTheContract(t *testing.T) {
	requireNode(t)
	const contract = "skills/_shared/minimalism-contract.md"
	reply := reviewerBrief + "## Skills to load before work\n## Frozen context (base aaaa, head bbbb)\n"
	pluginPath := bridgeFixture(t)
	root := filepath.Dir(filepath.Dir(filepath.Dir(pluginPath)))
	config := minimalismConfig(pluginPath, root)
	config["prompt_config"].(map[string]any)["included_phases"] = []string{"reviewer"}
	writeJSONFile(t, filepath.Join(root, "labdrian-runtime-parity.json"), config)

	allowing := newFakeEngine(t, map[string][]byte{frozenEvent: []byte(reply)})
	got := decodeLaunch(t, runPluginWithEngine(t, pluginPath, allowing.path, launchOf("subagent", "reviewer", reviewerBrief)))
	prompt, _ := got.Input["prompt"].(string)
	if got.Thrown != nil || !strings.Contains(prompt, contract) || !strings.Contains(prompt, "## Frozen context (base aaaa, head bbbb)") {
		t.Fatalf("the contract should be added to the prompt the verb printed, got %#v", got)
	}
	if calls := allowing.calls(t); len(calls) != 2 || calls[1].Raw != reviewerBrief {
		t.Fatalf("the verb should be handed the brief before the contract is added, got %#v", calls)
	}

	denying := newFakeEngine(t, map[string][]byte{eventPreToolUse: encoded(t, hookwire.PreToolUseReply{Deny: true, Reason: "paused"}), frozenEvent: []byte(reply)})
	got = decodeLaunch(t, runPluginWithEngine(t, pluginPath, denying.path, launchOf("subagent", "reviewer", reviewerBrief)))
	if got.Thrown == nil || got.Thrown.Message != "paused" || got.Input["prompt"] != reviewerBrief {
		t.Fatalf("a launch the gate denies is neither rewritten nor given its context, got %#v", got)
	}
	if calls := denying.calls(t); len(calls) != 1 {
		t.Fatalf("a denied launch should not run the verb, got %#v", calls)
	}
}

func TestOpenCodeBridgeDeniesAReviewerLaunchTheVerbRefusesWithItsReason(t *testing.T) {
	requireNode(t)
	const reason = "error: review frozen-context: the brief names no base and head, in the form review bind prints"
	engine := newFakeEngine(t, map[string][]byte{frozenEvent: []byte(frozenReply)})
	engine.failEventWith(t, frozenEvent, 2, reason+"\n")
	pluginPath := bridgeFixture(t)

	got := decodeLaunch(t, runPluginWithEngine(t, pluginPath, engine.path, launchOf("subagent", "reviewer", reviewerBrief)))
	if got.Thrown == nil || got.Thrown.Name != "Tool.Error" || got.Thrown.Message != reason || got.Thrown.Fields["message"] != reason {
		t.Fatalf("the verb's refusal should throw a Tool.Error({message: %q}), got %#v", reason, got.Thrown)
	}
	if got.Input["prompt"] != reviewerBrief {
		t.Fatalf("a refused launch keeps the prompt the agent wrote, got %#v", got.Input)
	}
}

func TestOpenCodeBridgeDeniesAReviewerLaunchWhenTheEngineCannotAnswer(t *testing.T) {
	requireNode(t)
	pluginPath := bridgeFixture(t)
	hung := newFakeEngine(t, map[string][]byte{frozenEvent: []byte(frozenReply)})
	hung.hangEventFor(t, frozenEvent, 2)
	silent := newFakeEngine(t, nil)

	for name, run := range map[string]func() []byte{
		"missing": func() []byte {
			return runPluginWithEngine(t, pluginPath, filepath.Join(t.TempDir(), "no-engine"), launchOf("subagent", "reviewer", reviewerBrief))
		},
		"timeout": func() []byte {
			return runPluginWithEnv(t, pluginPath, []string{engineEnvironment + "=" + hung.path, "LABDRIAN_ENGINE_TIMEOUT_MS=400"}, launchOf("subagent", "reviewer", reviewerBrief))
		},
		"empty answer": func() []byte {
			return runPluginWithEngine(t, pluginPath, silent.path, launchOf("subagent", "reviewer", reviewerBrief))
		},
	} {
		t.Run(name, func(t *testing.T) {
			got := decodeLaunch(t, run())
			if got.Thrown == nil || got.Thrown.Name != "Tool.Error" || !strings.Contains(got.Thrown.Message, "frozen context") {
				t.Fatalf("a reviewer launch the engine cannot give its context must be denied with a sentence naming it, got %#v", got.Thrown)
			}
			if got.Input["prompt"] != reviewerBrief {
				t.Fatalf("a denied launch keeps its prompt, got %#v", got.Input)
			}
		})
	}
}

func TestOpenCodeBridgeLeavesEveryOtherLaunchAsItWas(t *testing.T) {
	requireNode(t)
	pluginPath := bridgeFixture(t)
	for name, c := range map[string]struct{ tool, agent string }{
		"builder":              {"subagent", "builder"},
		"agent named like it":  {"subagent", "reviewer-two"},
		"other tool, reviewer": {"shell", "reviewer"},
		"other tool, builder":  {"task", "builder"},
	} {
		t.Run(name, func(t *testing.T) {
			engine := newFakeEngine(t, map[string][]byte{frozenEvent: []byte(frozenReply)})
			got := decodeLaunch(t, runPluginWithEngine(t, pluginPath, engine.path, launchOf(c.tool, c.agent, basePromptFixture)))
			if got.Thrown != nil || got.Input["prompt"] != basePromptFixture || got.Input["agent"] != c.agent || len(got.Input) != 2 {
				t.Fatalf("a %s %s launch must be left byte-identical, got %#v", c.tool, c.agent, got)
			}
			if calls := engine.calls(t); len(calls) != 1 || calls[0].Event != eventPreToolUse {
				t.Fatalf("only the gate should be asked, got %#v", calls)
			}
		})
	}
}
