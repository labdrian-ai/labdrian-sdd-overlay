package runtime_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
)

// injectedTool and agentField name the tool the plugin injects into and the input field that
// names the agent the call launches.
const (
	injectedTool = "subagent"
	agentField   = "agent"
)

// openCodePluginPath is where the adapter installs the v2 plugin under an OpenCode config root.
func openCodePluginPath(root string) string {
	return filepath.Join(root, "plugins", "labdrian-runtime-parity", "index.js")
}

// stubPluginPackage is the stand-in for the two modules OpenCode v2 hands a plugin; node resolves
// them from a node_modules directory above the plugin.
var stubPluginPackage = map[string]string{
	"package.json": `{"name":"@opencode/plugin","type":"module","exports":{".":"./index.js","./promise/tool":"./tool.js"}}`,
	"index.js":     `export const Plugin = { define: (plugin) => plugin };`,
	"tool.js":      `export class Error extends globalThis.Error { constructor(fields) { super(fields.message); this.name = "Tool.Error"; this.fields = fields; } } export const CallID = {};`,
}

func requireNode(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("node"); err != nil {
		t.Skipf("node unavailable; skipping JS plugin behavior test: %v", err)
	}
}

func repositoryPluginSource(t *testing.T) []byte {
	t.Helper()
	_, callerPath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	source, err := os.ReadFile(filepath.Join(filepath.Dir(callerPath), "labdrian-runtime-parity-plugin.mjs"))
	if err != nil {
		t.Fatalf("read repository plugin source: %v", err)
	}
	return source
}

// writePluginFixture lays out a config root the way the adapter does, with the repository plugin
// at the v2 path and the stub @opencode/plugin package above it. It returns the root and the plugin path.
func writePluginFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	pluginPath := openCodePluginPath(root)
	if err := os.MkdirAll(filepath.Dir(pluginPath), 0o755); err != nil {
		t.Fatalf("mkdir plugin dir: %v", err)
	}
	if err := os.WriteFile(pluginPath, repositoryPluginSource(t), 0o644); err != nil {
		t.Fatalf("write plugin fixture: %v", err)
	}
	stub := filepath.Join(root, "node_modules", "@opencode", "plugin")
	if err := os.MkdirAll(stub, 0o755); err != nil {
		t.Fatalf("mkdir stub package: %v", err)
	}
	for name, content := range stubPluginPackage {
		if err := os.WriteFile(filepath.Join(stub, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write stub %s: %v", name, err)
		}
	}
	return root, pluginPath
}

// engineEnvironment is the variable that names the engine binary the plugin's hook bridge runs.
const engineEnvironment = "LABDRIAN_ENGINE_BINARY"

// runPlugin loads the plugin at pluginPath under node with no engine to run (the bridge then lets
// every call through), runs its setup against a stub context and then body. See runPluginWithEngine.
func runPlugin(t *testing.T, pluginPath, body string) []byte {
	t.Helper()
	return runPluginWithEngine(t, pluginPath, filepath.Join(t.TempDir(), "no-engine"), body)
}

// runPluginWithEngine loads the plugin at pluginPath under node, with the hook bridge pointed at
// the engine binary, runs its setup against a stub context and then body, which sees plugin, hooks
// (name to handler, tool and session hooks alike), beforeHook (the execute.before one), synthetic
// (what the plugin sent through ctx.session.synthetic) and sessionInfo (what ctx.session.get answers for
// a session id: an object, or an Error to throw). It returns what body printed.
func runPluginWithEngine(t *testing.T, pluginPath, engine, body string) []byte {
	t.Helper()
	return runPluginWithEnv(t, pluginPath, []string{engineEnvironment + "=" + engine}, body)
}

// runPluginWithEnv is runPluginWithEngine with the environment variables the bridge reads given whole.
func runPluginWithEnv(t *testing.T, pluginPath string, environment []string, body string) []byte {
	t.Helper()
	requireNode(t)
	script := fmt.Sprintf(`const plugin = (await import(%q)).default;
const hooks = new Map();
const synthetic = [];
const hook = async (name, handler) => { hooks.set(name, handler); return { dispose: async () => {} }; };
const sessionInfo = new Map();
const ctx = { tool: { hook }, session: { hook, synthetic: async (input) => { synthetic.push(input); }, get: async ({ sessionID }) => { const info = sessionInfo.get(sessionID) ?? { id: sessionID }; if (info instanceof Error) throw info; return info; } } };
await plugin.setup(ctx);
const beforeHook = hooks.get("execute.before");
%s`, "file://"+pluginPath, body)
	scriptPath := filepath.Join(t.TempDir(), "labdrian-runtime-parity-plugin-behavior.mjs")
	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		t.Fatalf("write node script: %v", err)
	}
	command := exec.Command("node", scriptPath)
	command.Env = append(os.Environ(), environment...)
	out, err := command.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("node exited with error: %v\n%s", err, ee.Stderr)
		}
		t.Fatalf("node exec failed: %v", err)
	}
	return out
}

func minimalismConfig(pluginPath, root string) map[string]any {
	return map[string]any{
		"installed_version":   engineRuntime.OpenCodePluginVersion,
		"installed_hash":      "plugin-hash",
		"prompt_config_hash":  "marker-hash",
		"plugin_path":         pluginPath,
		"plugin_config_root":  root,
		"plugin_config_scope": "global-opencode-config",
		"prompt_config": map[string]any{
			"contract_path":   "skills/_shared/minimalism-contract.md",
			"included_phases": []string{"sdd-tasks"},
			"excluded_phases": []string{"sdd-spec", "sdd-verify", "sdd-archive", "sdd-design", "sdd-propose"},
			"injection_point": "## Skills to load before work",
		},
	}
}

func TestOpenCodePluginIsAV2DefinitionThatRegistersItsHooks(t *testing.T) {
	requireNode(t)
	root, pluginPath := writePluginFixture(t)
	writeJSONFile(t, filepath.Join(root, "labdrian-runtime-parity.json"), minimalismConfig(pluginPath, root))

	out := runPlugin(t, pluginPath, `console.log(JSON.stringify({
  id: plugin.id,
  setupIsFunction: typeof plugin.setup === "function",
  hooks: [...hooks.keys()].sort(),
}));`)
	var got struct {
		ID              string   `json:"id"`
		SetupIsFunction bool     `json:"setupIsFunction"`
		Hooks           []string `json:"hooks"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode result: %v\nraw=%q", err, string(out))
	}
	if got.ID != "labdrian-runtime-parity" || !got.SetupIsFunction {
		t.Fatalf("plugin should be Plugin.define({id, setup}) with id labdrian-runtime-parity, got %#v", got)
	}
	if strings.Join(got.Hooks, ",") != "context,execute.after,execute.before,experimental.idle,prompt" {
		t.Fatalf("setup should register exactly the hooks of the bridge, got %v", got.Hooks)
	}
}

func TestOpenCodePluginWritesTheActiveMarkerAtTheConfigRoot(t *testing.T) {
	requireNode(t)
	root, pluginPath := writePluginFixture(t)
	config := minimalismConfig(pluginPath, root)
	writeJSONFile(t, filepath.Join(root, "labdrian-runtime-parity.json"), config)

	runPlugin(t, pluginPath, `console.log("loaded");`)

	marker := readActiveMarkerJSON(t, filepath.Join(root, "labdrian-runtime-parity.active.json"))
	if marker["active_version"] != engineRuntime.OpenCodePluginVersion {
		t.Fatalf("active marker should include installed version, got %#v", marker)
	}
	if marker["active_prompt_config_hash"] != config["prompt_config_hash"] {
		t.Fatalf("active marker should include prompt config hash, got %#v", marker)
	}
	if marker["plugin_path"] != pluginPath {
		t.Fatalf("active marker plugin_path mismatch: got %#v", marker)
	}
	if marker["config_root"] != root {
		t.Fatalf("active marker config_root should be the config root %q, got %#v", root, marker)
	}
}

func TestOpenCodePluginInjectsAndStripsTheContract(t *testing.T) {
	requireNode(t)
	root, pluginPath := writePluginFixture(t)
	writeJSONFile(t, filepath.Join(root, "labdrian-runtime-parity.json"), minimalismConfig(pluginPath, root))

	script := fmt.Sprintf(`const base = %q;
const withContract = base + "\nskills/_shared/minimalism-contract.md";
const calls = {
  included: { tool: %q, input: { prompt: base, %s: "sdd-tasks" } },
  excluded: { tool: %q, input: { prompt: withContract, %s: "sdd-spec" } },
  unchanged: { tool: %q, input: { prompt: base, %s: "sdd-apply" } },
};
for (const call of Object.values(calls)) await beforeHook(call);
console.log(JSON.stringify(Object.fromEntries(Object.entries(calls).map(([name, call]) => [name, call.input.prompt]))));`,
		basePromptFixture, injectedTool, agentField, injectedTool, agentField, injectedTool, agentField)
	var got struct {
		Included  string `json:"included"`
		Excluded  string `json:"excluded"`
		Unchanged string `json:"unchanged"`
	}
	if err := json.Unmarshal(runPlugin(t, pluginPath, script), &got); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if !strings.Contains(got.Included, "skills/_shared/minimalism-contract.md") {
		t.Fatalf("included phase should inject minimalism contract: %q", got.Included)
	}
	if strings.Contains(got.Excluded, "skills/_shared/minimalism-contract.md") {
		t.Fatalf("excluded phase should strip minimalism contract: %q", got.Excluded)
	}
	if got.Unchanged != basePromptFixture {
		t.Fatalf("non-targeted phase should return original prompt: %q", got.Unchanged)
	}
}

// An absolute contract path, which is what the adapter writes, is injected as the line it is, once,
// and stripped by that exact line.
func TestOpenCodePluginInjectsAnAbsoluteContractPathOnceAndStripsIt(t *testing.T) {
	requireNode(t)
	root, pluginPath := writePluginFixture(t)
	absolute := "/overlay/skills/_shared/minimalism-contract.md"
	config := minimalismConfig(pluginPath, root)
	config["prompt_config"].(map[string]any)["contract_path"] = absolute
	writeJSONFile(t, filepath.Join(root, "labdrian-runtime-parity.json"), config)

	script := fmt.Sprintf(`const base = %q;
const calls = {
  once: { tool: %q, input: { prompt: base, %s: "sdd-tasks" } },
  twice: { tool: %q, input: { prompt: base, %s: "sdd-tasks" } },
};
for (const call of Object.values(calls)) await beforeHook(call);
await beforeHook(calls.twice);
const stripped = { tool: %q, input: { prompt: calls.once.input.prompt, %s: "sdd-spec" } };
await beforeHook(stripped);
console.log(JSON.stringify({ once: calls.once.input.prompt, twice: calls.twice.input.prompt, stripped: stripped.input.prompt }));`,
		basePromptFixture, injectedTool, agentField, injectedTool, agentField, injectedTool, agentField)
	var got struct {
		Once     string `json:"once"`
		Twice    string `json:"twice"`
		Stripped string `json:"stripped"`
	}
	if err := json.Unmarshal(runPlugin(t, pluginPath, script), &got); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if strings.Count(got.Once, absolute) != 1 {
		t.Fatalf("included phase should inject the absolute path once: %q", got.Once)
	}
	if got.Twice != got.Once {
		t.Fatalf("a second injection should change nothing:\n%q\n%q", got.Twice, got.Once)
	}
	if strings.Contains(got.Stripped, absolute) {
		t.Fatalf("excluded phase should strip the absolute path: %q", got.Stripped)
	}
}

func TestOpenCodePluginInjectsOnlyOnTheSubagentTool(t *testing.T) {
	requireNode(t)
	root, pluginPath := writePluginFixture(t)
	writeJSONFile(t, filepath.Join(root, "labdrian-runtime-parity.json"), minimalismConfig(pluginPath, root))

	script := fmt.Sprintf(`const base = %q;
const calls = {
  subagent: { tool: "subagent", input: { prompt: base, agent: "sdd-tasks" } },
  legacyTask: { tool: "task", input: { prompt: base, subagent_type: "sdd-tasks" } },
  legacyFieldOnSubagent: { tool: "subagent", input: { prompt: base, subagent_type: "sdd-tasks" } },
  shell: { tool: "shell", input: { prompt: base, agent: "sdd-tasks" } },
  noAgent: { tool: "subagent", input: { prompt: base } },
  nonStringAgent: { tool: "subagent", input: { prompt: base, agent: { name: "sdd-tasks" } } },
  noPrompt: { tool: "subagent", input: { agent: "sdd-tasks" } },
  noInput: { tool: "subagent" },
};
for (const call of Object.values(calls)) await beforeHook(call);
console.log(JSON.stringify({
  subagent: calls.subagent.input.prompt,
  legacyTask: calls.legacyTask.input.prompt,
  legacyFieldOnSubagent: calls.legacyFieldOnSubagent.input.prompt,
  shell: calls.shell.input.prompt,
  noAgent: calls.noAgent.input.prompt,
  nonStringAgent: calls.nonStringAgent.input.prompt,
  noPrompt: calls.noPrompt.input.prompt ?? null,
  noInput: calls.noInput.input ?? null,
}));`, basePromptFixture)
	var got map[string]any
	if err := json.Unmarshal(runPlugin(t, pluginPath, script), &got); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	const contract = "skills/_shared/minimalism-contract.md"
	if prompt, _ := got["subagent"].(string); !strings.Contains(prompt, contract) {
		t.Fatalf("a subagent launch should get the contract in input.prompt: %q", prompt)
	}
	for _, name := range []string{"legacyTask", "legacyFieldOnSubagent", "shell", "noAgent", "nonStringAgent"} {
		if got[name] != basePromptFixture {
			t.Fatalf("%s must be left untouched, got %q", name, got[name])
		}
	}
	if got["noPrompt"] != nil || got["noInput"] != nil {
		t.Fatalf("a subagent call with no prompt or no input must stay as it was, got %#v", got)
	}
}

func TestOpenCodePluginEvaluatesContextAwareContracts(t *testing.T) {
	requireNode(t)
	root, pluginPath := writePluginFixture(t)
	writeJSONFile(t, filepath.Join(root, "labdrian-runtime-parity.json"), map[string]any{
		"installed_version":   engineRuntime.OpenCodePluginVersion,
		"installed_hash":      engineRuntime.OpenCodePluginHash(),
		"prompt_config_hash":  "marker-hash",
		"plugin_path":         pluginPath,
		"plugin_config_root":  root,
		"plugin_config_scope": "global-opencode-config",
		"work_context": map[string]any{
			"trusted":     true,
			"languages":   []string{"typescript"},
			"activations": []string{"oo-domain-design"},
			"work_kinds":  []string{"application-code"},
		},
		"prompt_config": map[string]any{
			"contracts": []map[string]any{{
				"contract_path":      "skills/_shared/oo-quality-contract.md",
				"included_phases":    []string{"sdd-apply"},
				"excluded_phases":    []string{},
				"injection_point":    "## Skills to load before work",
				"language_context":   []string{"typescript"},
				"activation_context": []string{"oo-domain-design"},
			}},
		},
	})
	trusted := func(kinds string) string {
		return `work_context: { trusted: true, languages: ["typescript"], activations: ["oo-domain-design"], work_kinds: ` + kinds + ` }`
	}
	script := fmt.Sprintf(`const base = %q;
const input = (prompt, extra) => ({ tool: %q, input: { prompt, %s: "sdd-apply", ...extra } });
const calls = {
  staticTamperOnly: input(base, {}),
  matching: input(base, { %s }),
  missingContext: input(base + " TypeScript NestJS SOLID", {}),
  nonDomain: input(base, { work_context: { trusted: true, languages: ["go"], activations: ["non-domain"], work_kinds: ["application-code"] } }),
  docsWork: input(base, { %s }),
  configWork: input(base, { %s }),
  generatedArtifactWork: input(base, { %s }),
  missingWorkKinds: input(base, { work_context: { trusted: true, languages: ["typescript"], activations: ["oo-domain-design"] } }),
  emptyWorkKinds: input(base, { %s }),
  malformedWorkKinds: input(base, { work_context: { trusted: true, languages: ["typescript"], activations: ["oo-domain-design"], work_kinds: "application-code" } }),
};
for (const call of Object.values(calls)) await beforeHook(call);
console.log(JSON.stringify(Object.fromEntries(Object.entries(calls).map(([name, call]) => [name, call.input.prompt]))));`,
		basePromptFixture, injectedTool, agentField,
		trusted(`["application-code"]`), trusted(`["docs"]`), trusted(`["config"]`), trusted(`["generated-artifact"]`), trusted(`[]`))
	var got map[string]string
	if err := json.Unmarshal(runPlugin(t, pluginPath, script), &got); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	const contract = "skills/_shared/oo-quality-contract.md"
	if !strings.Contains(got["matching"], contract) {
		t.Fatalf("trusted matching work context should inject OO contract: %q", got["matching"])
	}
	for _, name := range []string{"staticTamperOnly", "missingContext", "nonDomain", "docsWork", "configWork", "generatedArtifactWork", "missingWorkKinds", "emptyWorkKinds", "malformedWorkKinds"} {
		if strings.Contains(got[name], contract) {
			t.Fatalf("%s must pass through without OO contract injection: %q", name, got[name])
		}
	}
	if got["missingContext"] == got["matching"] {
		t.Fatalf("missing per-invocation context should pass through instead of using static config")
	}
}

func TestOpenCodePluginSkipsMalformedLegacyContextMetadata(t *testing.T) {
	requireNode(t)
	for _, tt := range []struct {
		name         string
		promptConfig map[string]any
		wantInjected bool
	}{
		{
			name: "legacy matching context injects",
			promptConfig: map[string]any{
				"contract_path":      "skills/_shared/oo-quality-contract.md",
				"included_phases":    []string{"sdd-apply"},
				"excluded_phases":    []string{},
				"injection_point":    "## Skills to load before work",
				"language_context":   []string{"typescript"},
				"activation_context": []string{"oo-domain-design"},
			},
			wantInjected: true,
		},
		{
			name: "legacy unsupported context operator passes through",
			promptConfig: map[string]any{
				"contract_path":      "skills/_shared/oo-quality-contract.md",
				"included_phases":    []string{"sdd-apply"},
				"excluded_phases":    []string{},
				"injection_point":    "## Skills to load before work",
				"language_context":   []string{"typescript"},
				"activation_context": []string{"oo-domain-design"},
				"context_operator":   "prompt_contains",
			},
		},
		{
			name: "legacy malformed language context passes through",
			promptConfig: map[string]any{
				"contract_path":      "skills/_shared/oo-quality-contract.md",
				"included_phases":    []string{"sdd-apply"},
				"excluded_phases":    []string{},
				"injection_point":    "## Skills to load before work",
				"language_context":   "typescript",
				"activation_context": []string{"oo-domain-design"},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root, pluginPath := writePluginFixture(t)
			writeJSONFile(t, filepath.Join(root, "labdrian-runtime-parity.json"), map[string]any{
				"installed_version":   engineRuntime.OpenCodePluginVersion,
				"installed_hash":      engineRuntime.OpenCodePluginHash(),
				"prompt_config_hash":  "marker-hash",
				"plugin_path":         pluginPath,
				"plugin_config_root":  root,
				"plugin_config_scope": "global-opencode-config",
				"prompt_config":       tt.promptConfig,
			})
			script := fmt.Sprintf(`const call = { tool: %q, input: { prompt: %q, %s: "sdd-apply", work_context: { trusted: true, languages: ["typescript"], activations: ["oo-domain-design"], work_kinds: ["application-code"] } } };
await beforeHook(call);
console.log(call.input.prompt);`, injectedTool, basePromptFixture, agentField)
			got := strings.TrimSpace(string(runPlugin(t, pluginPath, script)))
			if injected := strings.Contains(got, "skills/_shared/oo-quality-contract.md"); injected != tt.wantInjected {
				t.Fatalf("legacy prompt config injection mismatch: got=%v want=%v prompt=%q", injected, tt.wantInjected, got)
			}
		})
	}
}
