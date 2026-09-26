package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/pipkg"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/shaper"
)

// runPiGateScript copies the embedded labdrian-gate.ts bytes to a scratch
// .mjs module, as engine/runtime/pi_test.go does, and runs script under node
// with the module path substituted for %[1]q. It never loads a real Pi.
func runPiGateScript(t *testing.T, script string) string {
	t.Helper()
	if _, err := exec.LookPath("node"); err != nil {
		t.Skipf("node unavailable; skipping labdrian-gate.ts behavior test: %v", err)
	}
	dir := t.TempDir()
	gate := filepath.Join(dir, "extensions", "labdrian-gate.mjs")
	if err := os.MkdirAll(filepath.Dir(gate), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(gate, []byte(pipkg.GateExtensionSource()), 0o600); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(dir, "script.mjs")
	if err := os.WriteFile(scriptPath, []byte(fmt.Sprintf(script, gate)), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("node", scriptPath).CombinedOutput()
	if err != nil {
		t.Fatalf("node script failed: %v\n%s", err, out)
	}
	return string(out)
}

var piGuardTexts = []string{
	"gentle-ai-overlay shaper clearance record --stdin --root /r",
	"echo '{}' | ~/.claude/bin/gentle-ai-overlay shaper clearance record --stdin",
	`sh -c "gentle-ai-overlay shaper clearance record --stdin"`,
	"gentle-ai-overlay shaper   clearance\trecord --stdin",
	"gentle-ai-overlay shaper \\\n clearance \\\n record --stdin",
	"echo x > ~/.local/state/labdrian/shaper-clearance/p/g/a.json",
	"gentle-ai-overlay shaper assess --root /r --handoff h.json --goal g.json",
	"git checkout feat/shaper-clearance",
	"go test ./...",
	"",
}

// TestPiGateGuardMatchesTheGoGuard keeps the Pi tool_call guard's text
// matching identical to shaper.GuardMatches.
func TestPiGateGuardMatchesTheGoGuard(t *testing.T) {
	texts, _ := json.Marshal(piGuardTexts)
	out := runPiGateScript(t, `
const mod = await import(%[1]q);
const texts = `+string(texts)+`;
console.log(JSON.stringify(texts.map((t) => mod.matchesShaperClearanceGuard(t))));
`)
	var got []bool
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	for i, text := range piGuardTexts {
		if want := shaper.GuardMatches(text); got[i] != want {
			t.Errorf("Pi guard(%q) = %v, Go guard = %v", text, got[i], want)
		}
	}
}

// TestPiGateToolCallBlocksClearanceRecordingAndStoreWrites drives the
// default export with a fake pi and asserts the tool_call handler blocks bash
// calls naming the record entry point or the store path, and write/edit calls
// whose input.path is inside the store, mirroring shaper.RunGuardHook.
func TestPiGateToolCallBlocksClearanceRecordingAndStoreWrites(t *testing.T) {
	out := runPiGateScript(t, `
const mod = await import(%[1]q);
const handlers = {};
mod.default({ on(name, fn) { handlers[name] = fn; } });
const call = (toolName, input) => handlers.tool_call ? handlers.tool_call({ type: "tool_call", toolName, toolCallId: "1", input }, {}) : "no handler";
const results = {
  record: await call("bash", { command: "gentle-ai-overlay shaper clearance record --stdin" }),
  store: await call("bash", { command: "cat ~/.local/state/labdrian/shaper-clearance/p/g/x.json" }),
  other: await call("bash", { command: "ls" }),
  write: await call("write", { path: "/s/labdrian/shaper-clearance/p/g/x.json", content: "{}" }),
  edit: await call("edit", { path: "~/.local/state/labdrian/shaper-clearance/p/g/x.json", edits: [] }),
  writeElsewhere: await call("write", { path: "/r/notes.md", content: "labdrian/shaper-clearance" }),
  read: await call("read", { path: "/s/labdrian/shaper-clearance/x.json" }),
  noCommand: await call("bash", {}),
  beforeAgentStart: typeof handlers.before_agent_start,
};
console.log(JSON.stringify(results));
`)
	var got map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	for _, key := range []string{"record", "store", "write", "edit"} {
		var res struct {
			Block  bool   `json:"block"`
			Reason string `json:"reason"`
		}
		if err := json.Unmarshal(got[key], &res); err != nil || !res.Block {
			t.Fatalf("%s: result %s, want {block:true}", key, got[key])
		}
		for _, want := range []string{"speed bump", "same OS user", "not a signature"} {
			if !strings.Contains(res.Reason, want) {
				t.Errorf("%s: reason %q lacks %q", key, res.Reason, want)
			}
		}
	}
	for _, key := range []string{"other", "writeElsewhere", "read", "noCommand"} {
		if raw, ok := got[key]; ok && string(raw) != "null" {
			t.Errorf("%s: result %s, want undefined", key, raw)
		}
	}
	if string(got["beforeAgentStart"]) != `"function"` {
		t.Errorf("before_agent_start handler lost: %s", got["beforeAgentStart"])
	}
}
