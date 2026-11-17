package runtime_test

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/hookwire"
)

// The session events of the hook bridge (S10, T11 commit b): UserPromptSubmit from the `prompt`
// hook, SessionStart from the `context` hook, and Stop from `experimental.idle`. They run against the
// same stand-in engine as the tool events (opencode_bridge_test.go).

const (
	eventUserPromptSubmit = "UserPromptSubmit"
	eventSessionStart     = "SessionStart"
	eventStop             = "Stop"
)

// systemPart is one part the `context` hook pushed onto event.system: a part of type text, with its text.
type systemPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func TestOpenCodeBridgePutsTheSessionStartContextIntoEveryModelCallOfTheSession(t *testing.T) {
	requireNode(t)
	const block = "labdrian memory: workflow wf-1 of project proj-1 allows longterm-mem only"
	engine := newFakeEngine(t, map[string][]byte{eventSessionStart: encoded(t, hookwire.SessionStartReply{Context: block})})
	pluginPath := bridgeFixture(t)

	out := runPluginWithEngine(t, pluginPath, engine.path, `const context = hooks.get("context");
const run = async (sessionID) => { const system = []; await context({ sessionID, system }); return system; };
console.log(JSON.stringify({ first: await run("ses_1"), again: await run("ses_1"), child: await run("ses_2") }));`)
	var got struct {
		First []systemPart `json:"first"`
		Again []systemPart `json:"again"`
		Child []systemPart `json:"child"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode result: %v\nraw=%q", err, out)
	}
	want := []systemPart{{Type: "text", Text: block}}
	for name, parts := range map[string][]systemPart{"first": got.First, "again": got.Again, "child": got.Child} {
		if len(parts) != 1 || parts[0] != want[0] {
			t.Errorf("%s model call should get the engine's block as one system text part, got %#v", name, parts)
		}
	}

	calls := engine.calls(t)
	if len(calls) != 2 {
		t.Fatalf("the engine is asked once for each session, however many model calls it makes, got %#v", calls)
	}
	if calls[0].Event != eventSessionStart || calls[0].Args != "projection hook --event SessionStart "+bridgeSource {
		t.Fatalf("the bridge should run `projection hook --event SessionStart --source opencode`, got %#v", calls[0])
	}
	stdin := calls[0].Stdin
	if stdin["hook_event_name"] != eventSessionStart || stdin["session_id"] != "ses_1" || !strings.HasPrefix(stdin["cwd"].(string), "/") {
		t.Fatalf("the engine should be handed the session as Claude Code's envelope, got %#v", stdin)
	}
}

func TestOpenCodeBridgeAddsNothingToTheModelCallWhenTheEngineHasNoBlock(t *testing.T) {
	requireNode(t)
	engine := newFakeEngine(t, nil)
	pluginPath := bridgeFixture(t)

	out := runPluginWithEngine(t, pluginPath, engine.path, `const system = [{ type: "text", text: "base" }];
await hooks.get("context")({ sessionID: "ses_1", system });
console.log(JSON.stringify(system));`)
	var got []systemPart
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode result: %v\nraw=%q", err, out)
	}
	if len(got) != 1 || got[0].Text != "base" {
		t.Fatalf("a session with no block keeps its system as it was, got %#v", got)
	}
}

func TestOpenCodeBridgeCarriesThePromptContextInTheUsersPromptAndNeverInTheSystemPrompt(t *testing.T) {
	requireNode(t)
	const context = "labdrian: workflow wf-1 is paused"
	const warning = "labdrian: the binding could not be read"
	engine := newFakeEngine(t, map[string][]byte{eventUserPromptSubmit: encoded(t, hookwire.PromptReply{Context: context, Warning: warning})})
	pluginPath := bridgeFixture(t)

	out := runPluginWithEngine(t, pluginPath, engine.path, `const prompt = { sessionID: "ses_1", messageID: "msg_1", prompt: { text: "please continue" }, delivery: "steer" };
await hooks.get("prompt")(prompt);
const system = [];
await hooks.get("context")({ sessionID: "ses_1", system });
const other = [];
await hooks.get("context")({ sessionID: "ses_2", system: other });
console.log(JSON.stringify({ text: prompt.prompt.text, system, other }));`)
	var got struct {
		Text   string       `json:"text"`
		System []systemPart `json:"system"`
		Other  []systemPart `json:"other"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode result: %v\nraw=%q", err, out)
	}
	// The system prompt comes before every message: what changes with each prompt would invalidate the
	// cached prefix of the whole conversation, so it goes after the text the user wrote.
	if want := "please continue\n\n" + warning + "\n" + context; got.Text != want {
		t.Fatalf("the engine's warning and context should follow the user's text in the prompt, got %q want %q", got.Text, want)
	}
	if len(got.System) != 0 || len(got.Other) != 0 {
		t.Fatalf("a prompt's context must not touch the system prompt, got %#v and %#v", got.System, got.Other)
	}

	calls := engine.calls(t)
	if len(calls) < 1 || calls[0].Event != eventUserPromptSubmit || calls[0].Args != "projection hook --event UserPromptSubmit "+bridgeSource {
		t.Fatalf("the bridge should run `projection hook --event UserPromptSubmit --source opencode` first, got %#v", calls)
	}
	stdin := calls[0].Stdin
	if stdin["hook_event_name"] != eventUserPromptSubmit || stdin["session_id"] != "ses_1" || stdin["prompt"] != "please continue" {
		t.Fatalf("the engine should be handed the prompt as Claude Code's envelope, got %#v", stdin)
	}
}

func TestOpenCodeBridgeLeavesAPromptAloneWhenTheEngineHasNothingToAdd(t *testing.T) {
	requireNode(t)
	engine := newFakeEngine(t, map[string][]byte{eventUserPromptSubmit: encoded(t, hookwire.PromptReply{Context: "labdrian: paused"})})
	pluginPath := bridgeFixture(t)

	// The second prompt is answered with nothing: the stand-in's answer is emptied between the two.
	out := runPluginWithEngine(t, pluginPath, engine.path, fmt.Sprintf(`const { writeFileSync } = await import("node:fs");
const first = { sessionID: "ses_1", prompt: { text: "one" } };
await hooks.get("prompt")(first);
writeFileSync(%q, "");
const second = { sessionID: "ses_1", prompt: { text: "two" } };
await hooks.get("prompt")(second);
console.log(JSON.stringify({ first: first.prompt.text, second: second.prompt.text }));`, filepath.Join(engine.dir, "answer."+eventUserPromptSubmit)))
	var got struct{ First, Second string }
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode result: %v\nraw=%q", err, out)
	}
	if got.First != "one\n\nlabdrian: paused" || got.Second != "two" {
		t.Fatalf("a prompt carries the answer to itself alone: first=%q second=%q", got.First, got.Second)
	}
}

// The system prompt comes before every message of the conversation, so a byte that changes in it
// invalidates the cache of everything after. Two prompts answered differently leave it identical.
func TestOpenCodeBridgeKeepsTheSystemPromptByteStableAcrossPrompts(t *testing.T) {
	requireNode(t)
	engine := newFakeEngine(t, map[string][]byte{
		eventSessionStart:     encoded(t, hookwire.SessionStartReply{Context: "labdrian: start block"}),
		eventUserPromptSubmit: encoded(t, hookwire.PromptReply{Context: "labdrian: answer to the prompt"}),
	})
	pluginPath := bridgeFixture(t)

	out := runPluginWithEngine(t, pluginPath, engine.path, fmt.Sprintf(`const { writeFileSync } = await import("node:fs");
const systems = [];
await hooks.get("prompt")({ sessionID: "ses_1", prompt: { text: "first prompt" } });
const a = []; await hooks.get("context")({ sessionID: "ses_1", system: a }); systems.push(JSON.stringify(a));
writeFileSync(%q, "");
await hooks.get("prompt")({ sessionID: "ses_1", prompt: { text: "a very different second prompt" } });
const b = []; await hooks.get("context")({ sessionID: "ses_1", system: b }); systems.push(JSON.stringify(b));
console.log(JSON.stringify(systems));`, filepath.Join(engine.dir, "answer."+eventUserPromptSubmit)))
	var systems []string
	if err := json.Unmarshal(out, &systems); err != nil {
		t.Fatalf("decode result: %v\nraw=%q", err, out)
	}
	want := `[{"type":"text","text":"labdrian: start block"}]`
	if len(systems) != 2 || systems[0] != want || systems[1] != want {
		t.Fatalf("the system prompt must be the same bytes whatever the prompt: the start block alone, got %v", systems)
	}
}

func TestOpenCodeBridgeForcesAnotherTurnWhenTheEngineBlocksTheStop(t *testing.T) {
	requireNode(t)
	const reason = "Commit abc of branch feature/stop has unfinished review steps."
	engine := newFakeEngine(t, map[string][]byte{eventStop: encoded(t, hookwire.StopReply{Block: true, Reason: reason})})
	pluginPath := bridgeFixture(t)

	out := runPluginWithEngine(t, pluginPath, engine.path, `const idle = hooks.get("experimental.idle");
await idle({ sessionID: "ses_1", background: [] });
await idle({ sessionID: "ses_1", background: [] });
await hooks.get("prompt")({ sessionID: "ses_1", prompt: { text: "next" } });
await idle({ sessionID: "ses_1", background: [] });
console.log(JSON.stringify(synthetic));`)
	var sent []struct {
		SessionID   string `json:"sessionID"`
		Text        string `json:"text"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(out, &sent); err != nil {
		t.Fatalf("decode result: %v\nraw=%q", err, out)
	}
	if len(sent) != 3 || sent[0].SessionID != "ses_1" || sent[0].Text != reason || sent[0].Description == "" {
		t.Fatalf("a block should be sent as a synthetic message of the session with the engine's reason, got %#v", sent)
	}

	var stops []engineCall
	for _, call := range engine.calls(t) {
		if call.Event == eventStop {
			stops = append(stops, call)
		}
	}
	if len(stops) != 3 {
		t.Fatalf("each stop is asked of the engine, got %#v", stops)
	}
	for i, wantActive := range []bool{false, true, false} {
		call := stops[i]
		if call.Args != "review stop-hook" {
			t.Fatalf("the Stop belongs to `review stop-hook` (projection hook has no Stop), got %q", call.Args)
		}
		if call.Stdin["hook_event_name"] != eventStop || call.Stdin["session_id"] != "ses_1" || call.Stdin["stop_hook_active"] != wantActive {
			t.Fatalf("stop %d: the envelope should say stop_hook_active=%v after a block that no prompt has followed, got %#v", i, wantActive, call.Stdin)
		}
	}
}

func TestOpenCodeBridgeLetsTheSessionStopWhenTheEngineDoesNotBlock(t *testing.T) {
	requireNode(t)
	engine := newFakeEngine(t, nil)
	pluginPath := bridgeFixture(t)

	out := runPluginWithEngine(t, pluginPath, engine.path, `await hooks.get("experimental.idle")({ sessionID: "ses_1", background: [] });
console.log(JSON.stringify(synthetic));`)
	if strings.TrimSpace(string(out)) != "[]" {
		t.Fatalf("an engine with nothing to block sends nothing, got %q", out)
	}
	if calls := engine.calls(t); len(calls) != 1 || calls[0].Args != "review stop-hook" {
		t.Fatalf("the stop is asked of the engine once, got %#v", calls)
	}
}

func TestOpenCodeBridgeSessionEventsSurviveAnEngineThatFails(t *testing.T) {
	requireNode(t)
	engine := newFakeEngine(t, nil)
	engine.failWith(t, 1, "error: review stop-hook: unknown flag")
	pluginPath := bridgeFixture(t)

	out := runPluginWithEngine(t, pluginPath, engine.path, `const system = [];
await hooks.get("prompt")({ sessionID: "ses_1", prompt: { text: "hi" } });
await hooks.get("context")({ sessionID: "ses_1", system });
await hooks.get("experimental.idle")({ sessionID: "ses_1", background: [] });
console.log(JSON.stringify({ system, synthetic }));`)
	var got struct {
		System    []systemPart `json:"system"`
		Synthetic []any        `json:"synthetic"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode result: %v\nraw=%q", err, out)
	}
	if len(got.Synthetic) != 0 {
		t.Fatalf("an engine that fails never forces a turn, got %#v", got.Synthetic)
	}
	if len(got.System) != 1 || !strings.Contains(got.System[0].Text, "labdrian hooks are not running") {
		t.Fatalf("a session whose hooks do not run should be told so once, got %#v", got.System)
	}
}
