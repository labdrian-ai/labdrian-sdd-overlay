package main

// The hook bridge of the OpenCode plugin (S10) against the real engine. The plugin runs under node
// with a stand-in for the two modules OpenCode v2 hands it; the engine is the binary built from this
// tree, in an isolated world with a workflow bound to the repository, and every answer is compared
// with what the engine says to Claude Code for the same call. engine/runtime/opencode_bridge_test.go
// holds the cases that need no workflow (what the bridge sends, the failures of the engine).

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// bridgePluginStub is the stand-in for @opencode/plugin: Plugin.define returns its definition and
// Tool.Error keeps the fields it was given.
var bridgePluginStub = map[string]string{
	"package.json": `{"name":"@opencode/plugin","type":"module","exports":{".":"./index.js","./promise/tool":"./tool.js"}}`,
	"index.js":     `export const Plugin = { define: (plugin) => plugin };`,
	"tool.js":      `export class ToolError extends Error { constructor(fields) { super(fields.message); this.name = "Tool.Error"; } } export const Tool = { Error: ToolError };`,
}

// bridgeConfigRoot lays out an OpenCode config root with the repository plugin and the stub above it,
// and returns the plugin path.
func bridgeConfigRoot(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("node"); err != nil {
		t.Skipf("node unavailable; skipping the plugin bridge test: %v", err)
	}
	source, err := os.ReadFile(filepath.Join("..", "runtime", "labdrian-runtime-parity-plugin.mjs"))
	if err != nil {
		t.Fatalf("read the repository plugin: %v", err)
	}
	root := t.TempDir()
	pluginPath := filepath.Join(root, "plugins", "labdrian-runtime-parity", "index.js")
	files := map[string]string{
		pluginPath: string(source),
		filepath.Join(root, "labdrian-runtime-parity.json"): `{"installed_version":"e2e","installed_hash":"e2e","prompt_config_hash":"e2e"}`,
	}
	for name, content := range bridgePluginStub {
		files[filepath.Join(root, "node_modules", "@opencode", "plugin", name)] = content
	}
	for path, content := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return pluginPath
}

// bridgeRun loads the plugin under node in the world, with the engine binary of the world, runs
// body (which sees hooks, a name to handler map, and synthetic, what the plugin sent through
// ctx.session.synthetic), and returns what body printed.
func bridgeRun(t *testing.T, w phase7World, pluginPath, body string) []byte {
	t.Helper()
	script := `const plugin = (await import(` + jsonString(t, "file://"+pluginPath) + `)).default;
const hooks = new Map();
const synthetic = [];
const hook = async (name, handler) => { hooks.set(name, handler); return { dispose: async () => {} }; };
await plugin.setup({ tool: { hook }, session: { hook, synthetic: async (input) => { synthetic.push(input); } } });
` + body
	scriptPath := filepath.Join(t.TempDir(), "bridge.mjs")
	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("node", scriptPath)
	command.Dir = w.repo
	command.Env = append(w.env(), "LABDRIAN_ENGINE_BINARY="+w.binary)
	out, err := command.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("node exited with error: %v\n%s", err, ee.Stderr)
		}
		t.Fatalf("node exec failed: %v", err)
	}
	return out
}

func jsonString(t *testing.T, value string) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestOpenCodeBridgeToolEventsAgainstTheEngine(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs engine processes")
	}
	binary := phase6BuildEngineBinary(t)
	pluginPath := bridgeConfigRoot(t)

	t.Run("PreToolUseDeniesWhatTheEngineDeniesForClaude", func(t *testing.T) {
		w := newPhase7World(t, binary)
		w.bindRunning("proj-1", "wf-1", "sdd")
		w.verb("proj-1", "wf-1", "pause")
		// The reference is the answer to Claude Code for the same edit, as its tool is named there.
		claude := w.toolHook(w.repo, "Edit", phase7EditInputs["Edit"])
		var want struct {
			Specific struct {
				Decision string `json:"permissionDecision"`
				Reason   string `json:"permissionDecisionReason"`
			} `json:"hookSpecificOutput"`
		}
		if err := json.Unmarshal([]byte(claude.stdout), &want); err != nil || want.Specific.Decision != "deny" || want.Specific.Reason == "" {
			t.Fatalf("the reference answer for Claude Code should be a denial, got %q (%v)", claude.stdout, err)
		}
		before := w.stateSnapshot()

		out := bridgeRun(t, w, pluginPath, `const outcome = {};
for (const [tool, input] of [["patch", { filePath: "/work/repo/main.go" }], ["shell", { command: "ls" }], ["read", { filePath: "/work/repo/main.go" }]]) {
  try { await hooks.get("execute.before")({ tool, sessionID: "session-1", id: "call-" + tool, input }); outcome[tool] = null; }
  catch (error) { outcome[tool] = { name: error.name, message: error.message }; }
}
console.log(JSON.stringify(outcome));`)
		var got map[string]*struct {
			Name    string `json:"name"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(out, &got); err != nil {
			t.Fatalf("decode result: %v\nraw=%q", err, out)
		}
		if got["patch"] == nil || got["patch"].Name != "Tool.Error" || got["patch"].Message != want.Specific.Reason {
			t.Errorf("patch on a paused workflow = %#v, want a Tool.Error with Claude Code's reason %q", got["patch"], want.Specific.Reason)
		}
		if got["shell"] != nil || got["read"] != nil {
			t.Errorf("the other tools of a paused workflow go through: shell=%#v read=%#v", got["shell"], got["read"])
		}
		if !equalSnapshots(before, w.stateSnapshot()) {
			t.Error("the gate changed the state directory")
		}
	})

	t.Run("PostToolUseRecordsTheEvidenceOfAMemoryRead", func(t *testing.T) {
		w := newPhase7World(t, binary)
		w.bindRunning("proj-1", "wf-1", "odd")
		out := bridgeRun(t, w, pluginPath, `const result = { text: "response-content-canary" };
const call = { tool: "longterm-mem_query", sessionID: "session-1", id: "toolu_1", input: { project: "proj-1", query: "q", engram_id: 7 } };
await hooks.get("execute.after")({ ...call, status: "completed", result });
console.log(JSON.stringify(result));`)
		if strings.TrimSpace(string(out)) != `{"text":"response-content-canary"}` {
			t.Errorf("an engine with nothing to warn about leaves the result as it was, got %q", out)
		}
		data := sourceRead(t, sourceSidecar(w))
		sourceRecord(t, data, "mcp__longterm-mem__query", "longterm-mem", "proj-1", "")
	})
}

// equalSnapshots says whether two state snapshots hold the same files with the same content.
func equalSnapshots(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for path, content := range a {
		if other, ok := b[path]; !ok || other != content {
			return false
		}
	}
	return true
}
