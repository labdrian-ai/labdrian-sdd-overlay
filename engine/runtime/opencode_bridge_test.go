package runtime_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/hookwire"
)

// The hook bridge (S10) hands each OpenCode event to the engine command that answers the same event
// for Claude Code, and turns the answer into what OpenCode understands. These tests run the
// installed plugin under node against a stand-in for the engine that records what it was asked
// and answers with what the engine's own encoders (engine/hookwire) write, which are the bytes the
// golden files of the hooks pin; engine/cmd/opencode_bridge_e2e_test.go runs the same plugin against
// the real engine.

// fakeEngineScript is the stand-in for the engine binary. It appends "<event><TAB><arguments>" to
// calls.log, keeps its stdin in stdin.<n> for the n-th call, and prints answer.<event> when there is
// one. The event is the --event value, or Stop for `review stop-hook`, which takes none.
const fakeEngineScript = `#!/bin/sh
dir=$(dirname "$0")
event=Stop
previous=
for argument in "$@"; do
  if [ "$previous" = "--event" ]; then event="$argument"; fi
  previous="$argument"
done
count=$(wc -l < "$dir/calls.log" 2>/dev/null || echo 0)
count=$((count + 1))
printf '%s\t%s\n' "$event" "$*" >> "$dir/calls.log"
cat > "$dir/stdin.$count"
if [ -f "$dir/exit" ]; then cat "$dir/stderr" >&2; exit "$(cat "$dir/exit")"; fi
if [ -f "$dir/answer.$event" ]; then cat "$dir/answer.$event"; fi
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
		call := engineCall{Event: event, Args: args}
		if err := json.Unmarshal(stdin, &call.Stdin); err != nil {
			t.Fatalf("fake engine stdin %q is not a JSON object: %v", stdin, err)
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
