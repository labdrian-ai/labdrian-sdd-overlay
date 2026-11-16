package runtime_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/hookwire"
)

// The failures of the engine, and what the bridge keeps between calls (T11 correction batch). The
// engine is an executable the bridge cannot trust to answer: it may be missing, hang, print noise or
// too much, and the bridge fails open and says so once, however the first failure came about.

const notRunning = "labdrian hooks are not running"

// scriptEngine writes a stand-in engine with its own script; calls.log is the place the script
// records its calls, as the default stand-in does.
func scriptEngine(t *testing.T, script string) fakeEngine {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "engine")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write script engine: %v", err)
	}
	return fakeEngine{path: path, dir: dir}
}

// resultTexts is the texts of the content parts of a result, decoded from a node run's output.
func decodeResults(t *testing.T, out []byte) map[string][]string {
	t.Helper()
	var got map[string][]string
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode result: %v\nraw=%q", err, out)
	}
	return got
}

// The note must not depend on which call carries it: a first failing call that never completes
// (it is denied by another hook, it errors, the session ends) must not lose it.
func TestOpenCodeBridgeKeepsTheUnavailableNoteUntilItIsAttached(t *testing.T) {
	requireNode(t)
	pluginPath := bridgeFixture(t)
	missing := filepath.Join(t.TempDir(), "no-engine")

	t.Run("a later completed result carries it, once", func(t *testing.T) {
		out := runPluginWithEngine(t, pluginPath, missing, `const afterHook = hooks.get("execute.after");
await beforeHook({ tool: "shell", sessionID: "ses_1", id: "c1", input: { command: "ls" } });
const texts = (result) => result.content.map((part) => part.text);
const second = { content: [{ type: "text", text: "ok" }] };
await afterHook({ tool: "shell", sessionID: "ses_1", id: "c2", input: {}, status: "completed", result: second });
const third = { content: [{ type: "text", text: "ok" }] };
await afterHook({ tool: "shell", sessionID: "ses_1", id: "c3", input: {}, status: "completed", result: third });
console.log(JSON.stringify({ second: texts(second), third: texts(third) }));`)
		got := decodeResults(t, out)
		if len(got["second"]) != 2 || !strings.Contains(got["second"][1], notRunning) || len(got["third"]) != 1 {
			t.Fatalf("the first completed result should carry the note and the next none, got %#v", got)
		}
	})

	t.Run("the next model call carries it when no result came first", func(t *testing.T) {
		out := runPluginWithEngine(t, pluginPath, missing, `const afterHook = hooks.get("execute.after");
await beforeHook({ tool: "shell", sessionID: "ses_1", id: "c1", input: { command: "ls" } });
const system = [];
await hooks.get("context")({ sessionID: "ses_2", system });
const later = { content: [{ type: "text", text: "ok" }] };
await afterHook({ tool: "shell", sessionID: "ses_2", id: "c2", input: {}, status: "completed", result: later });
console.log(JSON.stringify({ system: system.map((part) => part.text), later: later.content.map((part) => part.text) }));`)
		got := decodeResults(t, out)
		if len(got["system"]) != 1 || !strings.Contains(got["system"][0], notRunning) || len(got["later"]) != 1 {
			t.Fatalf("the model call should carry the note and the later result none, got %#v", got)
		}
	})

	t.Run("the note is told once however many model calls follow", func(t *testing.T) {
		out := runPluginWithEngine(t, pluginPath, missing, `const first = [];
await hooks.get("context")({ sessionID: "ses_1", system: first });
const second = [];
await hooks.get("context")({ sessionID: "ses_1", system: second });
console.log(JSON.stringify({ first: first.map((part) => part.text), second: second.map((part) => part.text) }));`)
		got := decodeResults(t, out)
		if len(got["first"]) != 1 || !strings.Contains(got["first"][0], notRunning) || len(got["second"]) != 0 {
			t.Fatalf("the note should join the first model call alone, got %#v", got)
		}
	})
}

// An exit-0 engine that prints noise is not silent: only nothing at all is the engine's silence.
func TestOpenCodeBridgeTreatsAnAnswerThatIsNotJSONAsAFailure(t *testing.T) {
	requireNode(t)
	pluginPath := bridgeFixture(t)
	for name, output := range map[string]string{
		"garbage":   `not json`,
		"truncated": `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"de`,
		"an array":  `[]`,
		"a string":  `"deny"`,
	} {
		t.Run(name, func(t *testing.T) {
			engine := scriptEngine(t, "#!/bin/sh\ncat > /dev/null\nprintf '%s' '"+output+"'\nexit 0\n")
			out := runPluginWithEngine(t, pluginPath, engine.path, `const call = { tool: "shell", sessionID: "ses_1", id: "c1", input: { command: "ls" } };
await beforeHook(call);
const result = { content: [{ type: "text", text: "ok" }] };
await hooks.get("execute.after")({ ...call, status: "completed", result });
console.log(JSON.stringify({ result: result.content.map((part) => part.text) }));`)
			got := decodeResults(t, out)
			if len(got["result"]) != 2 || !strings.Contains(got["result"][1], notRunning) {
				t.Fatalf("an answer that is not a JSON object should be the failure of the engine, with the note, got %#v", got)
			}
		})
	}

	t.Run("nothing at all is silence", func(t *testing.T) {
		engine := scriptEngine(t, "#!/bin/sh\ncat > /dev/null\nexit 0\n")
		out := runPluginWithEngine(t, pluginPath, engine.path, `const call = { tool: "shell", sessionID: "ses_1", id: "c1", input: { command: "ls" } };
await beforeHook(call);
const result = { content: [{ type: "text", text: "ok" }] };
await hooks.get("execute.after")({ ...call, status: "completed", result });
console.log(JSON.stringify({ result: result.content.map((part) => part.text) }));`)
		if got := decodeResults(t, out); len(got["result"]) != 1 {
			t.Fatalf("an engine with nothing to say leaves the result as it was, got %#v", got)
		}
	})
}

// A call the bridge cannot even write down for the engine is not an exception of the tool call.
func TestOpenCodeBridgeFailsOpenWhenTheCallCannotBeSerialized(t *testing.T) {
	requireNode(t)
	pluginPath := bridgeFixture(t)
	engine := newFakeEngine(t, nil)

	out := runPluginWithEngine(t, pluginPath, engine.path, `const circular = {};
circular.self = circular;
const calls = [
  { tool: "shell", sessionID: "ses_1", id: "c1", input: circular },
  { tool: "shell", sessionID: "ses_1", id: "c2", input: { size: 10n } },
];
const thrown = [];
for (const call of calls) {
  try { await beforeHook(call); } catch (error) { thrown.push(String(error)); }
}
const result = { content: [{ type: "text", text: "ok" }] };
await hooks.get("execute.after")({ tool: "shell", sessionID: "ses_1", id: "c3", input: {}, status: "completed", result });
console.log(JSON.stringify({ thrown, result: result.content.map((part) => part.text) }));`)
	var got struct {
		Thrown []string `json:"thrown"`
		Result []string `json:"result"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode result: %v\nraw=%q", err, out)
	}
	if len(got.Thrown) != 0 {
		t.Fatalf("a call that cannot be serialized must go through, got %v", got.Thrown)
	}
	if len(got.Result) != 2 || !strings.Contains(got.Result[1], notRunning) || !strings.Contains(got.Result[1], "serialized") {
		t.Fatalf("the model should be told the hooks did not run, and why, got %#v", got.Result)
	}
	for _, call := range engine.calls(t) {
		if call.Event == eventPreToolUse {
			t.Fatalf("the engine cannot have been asked about a call that was not written, got %#v", call)
		}
	}
}

// aliveAfter polls until pid is gone, for at most d.
func gone(pid int, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); err != nil {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// A hung engine costs one timeout, not one for every call: the bridge stops asking for a while, says
// so, asks again after it, and ends the engine however it ignores the first signal.
func TestOpenCodeBridgeStopsAskingAHungEngineForAWhile(t *testing.T) {
	requireNode(t)
	pluginPath := bridgeFixture(t)
	engine := scriptEngine(t, "#!/bin/sh\ndir=$(dirname \"$0\")\necho \"$$\" >> \"$dir/calls.log\"\ntrap '' TERM\nwhile :; do :; done\n")

	out := runPluginWithEnv(t, pluginPath, []string{engineEnvironment + "=" + engine.path, "LABDRIAN_ENGINE_TIMEOUT_MS=300"}, `const afterHook = hooks.get("execute.after");
const wait = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
const elapsed = [];
const timed = async (call) => { const start = Date.now(); await beforeHook(call); elapsed.push(Date.now() - start); };
await timed({ tool: "shell", sessionID: "ses_1", id: "c1", input: {} });
const result = { content: [{ type: "text", text: "ok" }] };
const start = Date.now();
await afterHook({ tool: "shell", sessionID: "ses_1", id: "c1", input: {}, status: "completed", result });
elapsed.push(Date.now() - start);
await timed({ tool: "shell", sessionID: "ses_1", id: "c2", input: {} });
await wait(900);
console.log(JSON.stringify({ elapsed, result: result.content.map((part) => part.text) }));`)
	var got struct {
		Elapsed []int    `json:"elapsed"`
		Result  []string `json:"result"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode result: %v\nraw=%q", err, out)
	}
	// The gate (PreToolUse) is asked on every call, since a pause there would let a call through that
	// the engine denies; the other events are not asked during the pause.
	if len(got.Elapsed) != 3 || got.Elapsed[0] < 250 || got.Elapsed[0] > 2000 || got.Elapsed[1] > 100 || got.Elapsed[2] < 250 {
		t.Fatalf("each gate call waits one timeout, the PostToolUse during the pause is not asked, got %v ms", got.Elapsed)
	}
	if len(got.Result) != 2 || !strings.Contains(got.Result[1], notRunning) || !strings.Contains(got.Result[1], "did not answer") || !strings.Contains(got.Result[1], "stops asking") {
		t.Fatalf("the note should say the engine hung and that the bridge stops asking, got %#v", got.Result)
	}
	log, err := os.ReadFile(filepath.Join(engine.dir, "calls.log"))
	if err != nil {
		t.Fatalf("read calls: %v", err)
	}
	pids := strings.Fields(string(log))
	if len(pids) != 2 {
		t.Fatalf("the engine should be started for each gate call and not for the PostToolUse in the pause, got pids %v", pids)
	}
	for _, text := range pids {
		pid, _ := strconv.Atoi(text)
		if !gone(pid, 3*time.Second) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			t.Errorf("engine process %d, which ignores SIGTERM, is still running: the bridge must end it with SIGKILL", pid)
		}
	}
}

func TestOpenCodeBridgeRefusesAnEngineThatPrintsTooMuch(t *testing.T) {
	requireNode(t)
	pluginPath := bridgeFixture(t)
	engine := scriptEngine(t, "#!/bin/sh\ncat > /dev/null\nyes x | head -c 3000000\nexit 0\n")

	out := runPluginWithEngine(t, pluginPath, engine.path, `const call = { tool: "shell", sessionID: "ses_1", id: "c1", input: {} };
await beforeHook(call);
const result = { content: [{ type: "text", text: "ok" }] };
await hooks.get("execute.after")({ ...call, status: "completed", result });
console.log(JSON.stringify({ result: result.content.map((part) => part.text) }));`)
	got := decodeResults(t, out)
	if len(got["result"]) != 2 || !strings.Contains(got["result"][1], notRunning) || !strings.Contains(got["result"][1], "2 MiB") {
		t.Fatalf("an answer over 2 MiB should be a failure of the engine, got %#v", got)
	}
}

// What the gate warned about a call waits for its result; a call that never gets one must not make
// the bridge hold its warning for ever.
func TestOpenCodeBridgeDoesNotKeepTheWarningsOfCallsThatNeverCompleted(t *testing.T) {
	requireNode(t)
	engine := newFakeEngine(t, map[string][]byte{eventPreToolUse: encoded(t, hookwire.PreToolUseReply{Warning: "warned"})})
	pluginPath := bridgeFixture(t)

	out := runPluginWithEngine(t, pluginPath, engine.path, `const afterHook = hooks.get("execute.after");
const texts = (result) => result.content.map((part) => part.text);
for (let i = 0; i < 300; i++) await beforeHook({ tool: "shell", sessionID: "ses_1", id: "c" + i, input: {} });
const oldest = { content: [{ type: "text", text: "ok" }] };
await afterHook({ tool: "shell", sessionID: "ses_1", id: "c0", input: {}, status: "completed", result: oldest });
const newest = { content: [{ type: "text", text: "ok" }] };
await afterHook({ tool: "shell", sessionID: "ses_1", id: "c299", input: {}, status: "completed", result: newest });
await beforeHook({ tool: "shell", sessionID: "ses_1", id: "failed", input: {} });
await afterHook({ tool: "shell", sessionID: "ses_1", id: "failed", input: {}, status: "error", error: new Error("boom") });
const again = { content: [{ type: "text", text: "ok" }] };
await afterHook({ tool: "shell", sessionID: "ses_1", id: "failed", input: {}, status: "completed", result: again });
console.log(JSON.stringify({ oldest: texts(oldest), newest: texts(newest), again: texts(again) }));`)
	got := decodeResults(t, out)
	if len(got["oldest"]) != 1 {
		t.Errorf("the warning of a call that never completed should have been let go, got %#v", got["oldest"])
	}
	if len(got["newest"]) != 2 || got["newest"][1] != "warned" {
		t.Errorf("the warning of a recent call should wait for its result, got %#v", got["newest"])
	}
	if len(got["again"]) != 1 {
		t.Errorf("a call that errored leaves nothing behind, got %#v", got["again"])
	}
}

// A SessionStart that could not be asked is asked again at the next model call, never remembered as
// an empty answer.
func TestOpenCodeBridgeAsksTheSessionStartAgainAfterAFailure(t *testing.T) {
	requireNode(t)
	const block = "labdrian memory: allowed sources"
	engine := newFakeEngine(t, nil)
	engine.failWith(t, 1, "engine too old")
	pluginPath := bridgeFixture(t)

	out := runPluginWithEngine(t, pluginPath, engine.path, fmt.Sprintf(`const { rmSync, writeFileSync } = await import("node:fs");
const first = [];
await hooks.get("context")({ sessionID: "ses_1", system: first });
rmSync(%q);
writeFileSync(%q, %q);
const second = [];
await hooks.get("context")({ sessionID: "ses_1", system: second });
console.log(JSON.stringify({ first: first.map((part) => part.text), second: second.map((part) => part.text) }));`,
		filepath.Join(engine.dir, "exit"), filepath.Join(engine.dir, "answer."+eventSessionStart),
		string(encoded(t, hookwire.SessionStartReply{Context: block}))))
	got := decodeResults(t, out)
	if len(got["first"]) != 1 || !strings.Contains(got["first"][0], notRunning) {
		t.Fatalf("the failed ask should be told, got %#v", got)
	}
	if len(got["second"]) != 1 || got["second"][0] != block {
		t.Fatalf("the next model call should ask again and get the block, got %#v", got)
	}
}

func TestOpenCodeBridgeKeepsTheStopGuardAcrossThePromptItCaused(t *testing.T) {
	requireNode(t)
	const reason = "Commit abc has unfinished review steps."
	engine := newFakeEngine(t, map[string][]byte{eventStop: encoded(t, hookwire.StopReply{Block: true, Reason: reason})})
	pluginPath := bridgeFixture(t)

	runPluginWithEngine(t, pluginPath, engine.path, fmt.Sprintf(`const idle = hooks.get("experimental.idle");
await idle({ sessionID: "ses_1", background: [] });
await hooks.get("prompt")({ sessionID: "ses_1", prompt: { text: %q } });
await idle({ sessionID: "ses_1", background: [] });
await hooks.get("prompt")({ sessionID: "ses_1", prompt: { text: "a prompt of the user" } });
await idle({ sessionID: "ses_1", background: [] });`, reason))
	var order []string
	for _, call := range engine.calls(t) {
		entry := call.Event
		if call.Event == eventStop {
			entry += fmt.Sprintf("(active=%v)", call.Stdin["stop_hook_active"])
		}
		order = append(order, entry)
	}
	want := "Stop(active=false) Stop(active=true) UserPromptSubmit Stop(active=false)"
	if strings.Join(order, " ") != want {
		t.Fatalf("the prompt the bridge caused is not a prompt of the user: it neither asks UserPromptSubmit nor clears the guard,\n got %v\nwant %s", order, want)
	}
}

func TestOpenCodeBridgeAsksTheStopOfMainSessionsOnly(t *testing.T) {
	requireNode(t)
	engine := newFakeEngine(t, map[string][]byte{eventStop: encoded(t, hookwire.StopReply{Block: true, Reason: "r"})})
	pluginPath := bridgeFixture(t)

	out := runPluginWithEngine(t, pluginPath, engine.path, `sessionInfo.set("ses_child", { id: "ses_child", parentID: "ses_main" });
sessionInfo.set("ses_wrapped", { session: { id: "ses_wrapped", parentID: "ses_main" } });
sessionInfo.set("ses_unknown", new Error("session lookup failed"));
const idle = hooks.get("experimental.idle");
for (const sessionID of ["ses_child", "ses_wrapped", "ses_main", "ses_unknown"]) await idle({ sessionID, background: [] });
console.log(JSON.stringify(synthetic.map((message) => message.sessionID)));`)
	var sent []string
	if err := json.Unmarshal(out, &sent); err != nil {
		t.Fatalf("decode result: %v\nraw=%q", err, out)
	}
	if strings.Join(sent, ",") != "ses_main,ses_unknown" {
		t.Fatalf("child sessions never stop on a hook, and a session that cannot be told is taken as a main one, got %v", sent)
	}
}

// OpenCode v2 names its tool arguments its own way; the engine reads the ones Claude Code sends.
func TestOpenCodeBridgeNamesTheArgumentsOfTheReadAndSkillToolsAsClaudeCodeDoes(t *testing.T) {
	requireNode(t)
	engine := newFakeEngine(t, nil)
	pluginPath := bridgeFixture(t)

	runPluginWithEngine(t, pluginPath, engine.path, `for (const call of [
  { tool: "read", sessionID: "ses_1", id: "c1", input: { path: "/repo/.claude/skills/deploy/SKILL.md", offset: 3 } },
  { tool: "skill", sessionID: "ses_1", id: "c2", input: { id: "deploy" } },
  { tool: "read", sessionID: "ses_1", id: "c3", input: { path: "/a", file_path: "/b" } },
  { tool: "grep", sessionID: "ses_1", id: "c4", input: { path: "/a", id: "x" } },
  { tool: "skill", sessionID: "ses_1", id: "c5", input: { id: "x", skill: "y" } },
]) await beforeHook(call);`)
	calls := engine.calls(t)
	if len(calls) != 5 {
		t.Fatalf("each call is asked of the engine, got %#v", calls)
	}
	input := func(i int) map[string]any { return calls[i].Stdin["tool_input"].(map[string]any) }
	if input(0)["file_path"] != "/repo/.claude/skills/deploy/SKILL.md" || input(0)["path"] != "/repo/.claude/skills/deploy/SKILL.md" || input(0)["offset"] != float64(3) {
		t.Errorf("a read names its file as file_path too, and keeps the rest, got %#v", input(0))
	}
	if input(1)["skill"] != "deploy" || input(1)["id"] != "deploy" {
		t.Errorf("a skill call names its skill as skill too, got %#v", input(1))
	}
	// OpenCode runs the read on path and the skill named by id, so those are what the policies judge,
	// whatever a Claude-named argument the call also carries says.
	if input(2)["file_path"] != "/a" {
		t.Errorf("the path OpenCode reads is the file_path the engine judges, got %#v", input(2))
	}
	if input(4)["skill"] != "x" {
		t.Errorf("the skill OpenCode loads is the skill the engine judges, got %#v", input(4))
	}
	if _, ok := input(3)["file_path"]; ok || input(3)["skill"] != nil {
		t.Errorf("only the read and skill tools are renamed, got %#v", input(3))
	}
}
