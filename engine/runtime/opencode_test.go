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
	"time"

	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
)

const basePromptFixture = "line 1\n## Skills to load before work\nline 2"

func TestOpenCodeInstallWritesPluginConfigAndRestartRequiredStatus(t *testing.T) {
	root := t.TempDir()
	adapter := engineRuntime.NewOpenCodeAdapter(root)

	install := adapter.Install()
	if install.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Install status = %q, want %q", install.Status, engineRuntime.CapabilityRestartRequired)
	}

	pluginPath := filepath.Join(root, "plugins", "labdrian-runtime-parity.js")
	plugin, err := os.ReadFile(pluginPath)
	if err != nil {
		t.Fatalf("plugin should be written at %s: %v", pluginPath, err)
	}
	if !strings.Contains(string(plugin), "tool.execute.before") {
		t.Fatalf("plugin should contain OpenCode tool.execute.before hook; got:\n%s", string(plugin))
	}
	repoSource, err := os.ReadFile("labdrian-runtime-parity-plugin.mjs")
	if err != nil {
		t.Fatalf("read repository plugin source: %v", err)
	}
	if string(plugin) != string(repoSource) {
		t.Fatal("installed plugin source should match repository plugin source exactly")
	}

	configPath := filepath.Join(root, "labdrian-runtime-parity.json")
	config, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("config should be written at %s: %v", configPath, err)
	}
	if !strings.Contains(string(config), engineRuntime.OpenCodePluginHash()) {
		t.Fatalf("config should contain plugin hash %q; got:\n%s", engineRuntime.OpenCodePluginHash(), string(config))
	}

	status := adapter.Status()
	if status.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Status after install = %q, want restart_required; message: %s", status.Status, status.Message)
	}
	if !strings.Contains(status.Message, "restart") {
		t.Errorf("restart-required status should explain restart, got %q", status.Message)
	}
}

func TestOpenCodeStatusSupportedWhenActiveMarkerMatchesHash(t *testing.T) {
	root := t.TempDir()
	adapter := engineRuntime.NewOpenCodeAdapter(root)
	if result := adapter.Install(); result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Install status = %q", result.Status)
	}
	activeMarker := filepath.Join(root, "labdrian-runtime-parity.active.json")
	promptHash := readPromptConfigHash(t, root)
	active := `{"active_version":"` + engineRuntime.OpenCodePluginVersion + `","active_hash":"` + engineRuntime.OpenCodePluginHash() + `","active_prompt_config_hash":"` + promptHash + `","plugin_path":"` + filepath.Join(root, "plugins", "labdrian-runtime-parity.js") + `","config_root":"` + root + `"}`
	if err := os.WriteFile(activeMarker, []byte(active), 0o644); err != nil {
		t.Fatalf("write active marker: %v", err)
	}

	status := adapter.Status()
	if status.Status != engineRuntime.CapabilitySupported {
		t.Fatalf("Status after active marker = %q, want supported; message: %s", status.Status, status.Message)
	}
	if !strings.Contains(status.Message, engineRuntime.OpenCodePluginHash()) {
		t.Errorf("supported status should include hash, got %q", status.Message)
	}
}

func TestOpenCodeInstallWritesPromptConfigFromMinimalismContract(t *testing.T) {
	root := t.TempDir()
	adapter := engineRuntime.NewOpenCodeAdapter(root)
	if result := adapter.Install(); result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}

	config := readOpenCodeConfig(t, root)
	promptConfig := config["prompt_config"].(map[string]any)
	if promptConfig["contract_path"] != "skills/_shared/minimalism-contract.md" {
		t.Fatalf("contract path = %#v", promptConfig["contract_path"])
	}
	if promptConfig["injection_point"] != "## Skills to load before work" {
		t.Fatalf("injection point = %#v", promptConfig["injection_point"])
	}
	assertStringSlice(t, promptConfig["included_phases"], []string{"sdd-tasks", "sdd-apply"})
	assertStringSlice(t, promptConfig["excluded_phases"], []string{"sdd-propose", "sdd-spec", "sdd-design", "sdd-verify", "sdd-archive"})
	if config["prompt_config_hash"] == "" {
		t.Fatal("config should include prompt_config_hash")
	}
	contracts := promptConfig["contracts"].([]any)
	// Assert every unconditional contract by name. A count-only bound cannot
	// tell "all four are present" from "three are, plus the optional one",
	// which is how anti-generic-design stayed unwired while this test passed.
	for _, want := range []string{
		"skills/_shared/minimalism-contract.md",
		"skills/_shared/skill-discovery-safety.md",
		"skills/_shared/review-projection-contract.md",
		"skills/_shared/anti-generic-design.md",
	} {
		if !hasPromptContract(contracts, want) {
			t.Fatalf("contracts missing %s: %#v", want, contracts)
		}
	}
	if !hasPromptContract(contracts, "skills/_shared/oo-quality-contract.md") {
		t.Fatalf("contracts missing OO quality contract: %#v", contracts)
	}
}

func TestOpenCodeInstallSkipsOOContractWithMalformedOrUnsupportedContextMetadata(t *testing.T) {
	overlayRoot := t.TempDir()
	sharedDir := filepath.Join(overlayRoot, "skills", "_shared")
	if err := os.MkdirAll(sharedDir, 0o755); err != nil {
		t.Fatalf("mkdir shared dir: %v", err)
	}
	_, callerPath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(callerPath)))
	minimalism, err := os.ReadFile(filepath.Join(repoRoot, "skills", "_shared", "minimalism-contract.md"))
	if err != nil {
		t.Fatalf("read minimalism contract fixture: %v", err)
	}
	for _, tt := range []struct {
		name      string
		ooContent string
	}{
		{
			name: "malformed language context list",
			ooContent: `---
applies_to_phases: [sdd-apply]
excluded_phases: []
injection_point: "## Skills to load before work"
language_context: typescript
activation_context: [oo-domain-design]
---
# Broken OO Contract
`,
		},
		{
			name: "unsupported context operator",
			ooContent: `---
applies_to_phases: [sdd-apply]
excluded_phases: []
injection_point: "## Skills to load before work"
language_context: [typescript]
activation_context: [oo-domain-design]
context_operator: prompt_contains
---
# Broken OO Contract
`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(sharedDir, "minimalism-contract.md"), minimalism, 0o644); err != nil {
				t.Fatalf("write minimalism contract: %v", err)
			}
			if err := os.WriteFile(filepath.Join(sharedDir, "oo-quality-contract.md"), []byte(tt.ooContent), 0o644); err != nil {
				t.Fatalf("write OO contract: %v", err)
			}
			t.Setenv("LABDRIAN_OVERLAY_DIR", overlayRoot)
			root := t.TempDir()
			adapter := engineRuntime.NewOpenCodeAdapter(root)
			if result := adapter.Install(); result.Status != engineRuntime.CapabilityRestartRequired {
				t.Fatalf("Install() = %#v", result)
			}

			config := readOpenCodeConfig(t, root)
			contracts := config["prompt_config"].(map[string]any)["contracts"].([]any)
			if hasPromptContract(contracts, "skills/_shared/oo-quality-contract.md") {
				t.Fatalf("malformed or unsupported OO contract should be skipped, got %#v", contracts)
			}
		})
	}
}

func TestOpenCodeInstallDoesNotWritePluginWhenPromptConfigCannotBeDerived(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LABDRIAN_OVERLAY_DIR", t.TempDir())
	adapter := engineRuntime.NewOpenCodeAdapter(root)

	result := adapter.Install()
	if result.Status != engineRuntime.CapabilityPartial || !strings.Contains(result.Message, "prompt config could not be derived") {
		t.Fatalf("Install() should fail before writing plugin when prompt config is unavailable, got %#v", result)
	}
	pluginPath := filepath.Join(root, "plugins", "labdrian-runtime-parity.js")
	if _, err := os.Stat(pluginPath); !os.IsNotExist(err) {
		t.Fatalf("plugin must not be written without valid config, stat err: %v", err)
	}
	configPath := filepath.Join(root, "labdrian-runtime-parity.json")
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("config must not be written when prompt config derivation fails, stat err: %v", err)
	}
}

func TestOpenCodeUninstallRemovesPluginAndConfig(t *testing.T) {
	root := t.TempDir()
	adapter := engineRuntime.NewOpenCodeAdapter(root)
	if result := adapter.Install(); result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}

	result := adapter.Uninstall()
	if result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Uninstall status = %q, want restart_required", result.Status)
	}
	for _, path := range []string{
		filepath.Join(root, "plugins", "labdrian-runtime-parity.js"),
		filepath.Join(root, "labdrian-runtime-parity.json"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("%s should be removed, stat err: %v", path, err)
		}
	}
}

func TestOpenCodeStatusAfterUninstallHonorsActiveMarker(t *testing.T) {
	root := t.TempDir()
	adapter := engineRuntime.NewOpenCodeAdapter(root)
	if result := adapter.Install(); result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}
	activeMarker := filepath.Join(root, "labdrian-runtime-parity.active.json")
	writeMatchingActiveMarker(t, root)
	if result := adapter.Uninstall(); result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Uninstall() = %#v", result)
	}
	if result := adapter.Status(); result.Status != engineRuntime.CapabilityRestartRequired || !strings.Contains(result.Message, "restart OpenCode to unload") {
		t.Fatalf("Status() after uninstall with active marker should require restart, got %#v", result)
	}
	if result := adapter.Uninstall(); result.Status != engineRuntime.CapabilityRestartRequired || !strings.Contains(result.Message, activeMarker) {
		t.Fatalf("second uninstall should keep restart guidance, got %#v", result)
	}
	if _, err := os.Stat(activeMarker); err != nil {
		t.Fatalf("second uninstall must not remove active marker, stat err: %v", err)
	}

	if err := os.Remove(activeMarker); err != nil {
		t.Fatalf("manual post-restart marker cleanup: %v", err)
	}
	if result := adapter.Status(); result.Status != engineRuntime.CapabilityUnsupported {
		t.Fatalf("Status() after manual marker cleanup should be unsupported, got %#v", result)
	}
}

func TestOpenCodeInstallUpdatePreservesLoadedMarkerUntilRestart(t *testing.T) {
	root := t.TempDir()
	adapter := engineRuntime.NewOpenCodeAdapter(root)
	if result := adapter.Install(); result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("initial Install() = %#v", result)
	}
	writeMatchingActiveMarker(t, root)
	activeMarker := filepath.Join(root, "labdrian-runtime-parity.active.json")
	oldTime := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(activeMarker, oldTime, oldTime); err != nil {
		t.Fatalf("age active marker: %v", err)
	}
	beforeRefresh := readActiveMarkerJSON(t, activeMarker)

	if result := adapter.Install(); result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("refresh install = %#v", result)
	}
	afterRefresh := readActiveMarkerJSON(t, activeMarker)
	if !sameJSONIdentity(beforeRefresh, afterRefresh) {
		t.Fatalf("refresh should preserve active marker identity; before=%#v after=%#v", beforeRefresh, afterRefresh)
	}
	if result := adapter.Status(); result.Status != engineRuntime.CapabilityRestartRequired || !strings.Contains(result.Message, "restart") {
		t.Fatalf("stale active marker after refresh should require restart, got %#v", result)
	}
}

func TestOpenCodeStatusRejectsTamperedPromptConfig(t *testing.T) {
	root := t.TempDir()
	adapter := engineRuntime.NewOpenCodeAdapter(root)
	if result := adapter.Install(); result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}
	writeMatchingActiveMarker(t, root)
	if result := adapter.Status(); result.Status != engineRuntime.CapabilitySupported {
		t.Fatalf("baseline status should be supported, got %#v", result)
	}

	for _, tt := range []struct {
		name   string
		tamper func(promptConfig map[string]any)
	}{
		{
			name: "wrong included phases",
			tamper: func(promptConfig map[string]any) {
				promptConfig["included_phases"] = []any{"sdd-apply"}
			},
		},
		{
			name: "unsupported legacy context operator",
			tamper: func(promptConfig map[string]any) {
				promptConfig["context_operator"] = "prompt_contains"
			},
		},
		{
			name: "unsupported nested contract context operator",
			tamper: func(promptConfig map[string]any) {
				contract := mustFindPromptContract(t, promptConfig["contracts"].([]any), "skills/_shared/oo-quality-contract.md")
				contract["context_operator"] = "prompt_contains"
			},
		},
		{
			name: "present empty nested contract context operator",
			tamper: func(promptConfig map[string]any) {
				contract := mustFindPromptContract(t, promptConfig["contracts"].([]any), "skills/_shared/oo-quality-contract.md")
				contract["context_operator"] = ""
			},
		},
		{
			name: "present null nested contract context operator",
			tamper: func(promptConfig map[string]any) {
				contract := mustFindPromptContract(t, promptConfig["contracts"].([]any), "skills/_shared/oo-quality-contract.md")
				contract["context_operator"] = nil
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			configPath := filepath.Join(root, "labdrian-runtime-parity.json")
			config := readOpenCodeConfig(t, root)
			promptConfig := config["prompt_config"].(map[string]any)
			tt.tamper(promptConfig)
			writeJSONFile(t, configPath, config)
			if result := adapter.Status(); result.Status != engineRuntime.CapabilityRestartRequired || !strings.Contains(result.Message, "prompt_config") {
				t.Fatalf("tampered prompt config should require restart, got %#v", result)
			}
			if result := adapter.Install(); result.Status != engineRuntime.CapabilityRestartRequired {
				t.Fatalf("reinstall after tamper = %#v", result)
			}
			writeMatchingActiveMarker(t, root)
		})
	}
}

func TestOpenCodeStatusRejectsMalformedNestedContractContextMetadata(t *testing.T) {
	root := t.TempDir()
	adapter := engineRuntime.NewOpenCodeAdapter(root)
	if result := adapter.Install(); result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}
	writeMatchingActiveMarker(t, root)

	configPath := filepath.Join(root, "labdrian-runtime-parity.json")
	config := readOpenCodeConfig(t, root)
	contract := mustFindPromptContract(t, config["prompt_config"].(map[string]any)["contracts"].([]any), "skills/_shared/oo-quality-contract.md")
	contract["language_context"] = "typescript"
	writeJSONFile(t, configPath, config)

	result := adapter.Status()
	if result.Status != engineRuntime.CapabilityPartial {
		t.Fatalf("malformed nested context metadata should invalidate status, got %#v", result)
	}
	if !strings.Contains(result.Message, "config missing or invalid") {
		t.Fatalf("status should explain invalid config, got %#v", result)
	}
}

func TestOpenCodeAdapterDoesNotPolluteCallerWorkingDirectory(t *testing.T) {
	configRoot := t.TempDir()
	callerCWD := t.TempDir()
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	t.Setenv("LABDRIAN_OVERLAY_DIR", repoRoot)
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	if err := os.Chdir(callerCWD); err != nil {
		t.Fatalf("chdir caller cwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	adapter := engineRuntime.NewOpenCodeAdapter(configRoot)
	if result := adapter.Install(); result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}
	if _, err := os.Stat(filepath.Join(callerCWD, ".opencode")); !os.IsNotExist(err) {
		t.Fatalf("install should not create .opencode under caller cwd, stat err: %v", err)
	}
	if _, err := os.Stat(filepath.Join(configRoot, "plugins", "labdrian-runtime-parity.js")); err != nil {
		t.Fatalf("plugin should be installed under explicit config root: %v", err)
	}
}

func TestOpenCodeAdapterRejectsUnresolvedOrRelativeConfigRoot(t *testing.T) {
	for _, adapter := range []engineRuntime.OpenCodeAdapter{
		engineRuntime.NewOpenCodeAdapter(""),
		engineRuntime.NewOpenCodeAdapter(filepath.Join("relative", "opencode")),
	} {
		if result := adapter.Install(); result.Status != engineRuntime.CapabilityUnsupported || !strings.Contains(result.Message, "OpenCode config root") {
			t.Fatalf("Install() should reject unsafe root, got %#v", result)
		}
		if result := adapter.Status(); result.Status != engineRuntime.CapabilityUnsupported || !strings.Contains(result.Message, "OpenCode config root") {
			t.Fatalf("Status() should reject unsafe root, got %#v", result)
		}
	}
}

func TestDefaultOpenCodeConfigRootUsesXDGConfigHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	xdg := filepath.Join(t.TempDir(), "xdg-config")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	want := filepath.Join(xdg, "opencode")
	if got := engineRuntime.DefaultOpenCodeConfigRoot(); got != want {
		t.Fatalf("DefaultOpenCodeConfigRoot() = %q, want %q", got, want)
	}
}

func TestDefaultOpenCodeConfigRootIgnoresRelativeXDGConfigHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join("relative", "xdg"))
	want := filepath.Join(home, ".config", "opencode")
	if got := engineRuntime.DefaultOpenCodeConfigRoot(); got != want {
		t.Fatalf("DefaultOpenCodeConfigRoot() = %q, want %q", got, want)
	}
}

func TestOpenCodeLifecycleAliasesAndStatusFailureModes(t *testing.T) {
	root := t.TempDir()
	adapter := engineRuntime.NewOpenCodeAdapter(root)

	if adapter.Target() != engineRuntime.TargetOpenCode {
		t.Fatalf("Target() = %q, want opencode", adapter.Target())
	}
	if result := adapter.Apply(); result.Action != engineRuntime.ActionInstall || result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Apply() should map to install and require restart, got %#v", result)
	}
	if result := adapter.SyncCheck(); result.Action != engineRuntime.ActionSyncCheck || result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("SyncCheck() should report restart-required, got %#v", result)
	}
	if result := adapter.Update(); result.Action != engineRuntime.ActionUpdate || result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Update() should reinstall and require restart, got %#v", result)
	}
	if result := adapter.Rollback(); result.Action != engineRuntime.ActionRollback || result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Rollback() should remove plugin bridge, got %#v", result)
	}
	if result := adapter.Status(); result.Status != engineRuntime.CapabilityUnsupported {
		t.Fatalf("Status() after rollback = %q, want unsupported", result.Status)
	}

	if result := adapter.Install(); result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}
	pluginPath := filepath.Join(root, "plugins", "labdrian-runtime-parity.js")
	if err := os.WriteFile(pluginPath, []byte("changed plugin source"), 0o644); err != nil {
		t.Fatalf("modify plugin: %v", err)
	}
	if result := adapter.Status(); result.Status != engineRuntime.CapabilityRestartRequired || !strings.Contains(result.Message, "artifact changed") {
		t.Fatalf("Status() should report artifact hash mismatch restart requirement, got %#v", result)
	}
}

func TestLabdrianRuntimeParityPluginMutatesPrompt(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skipf("node unavailable; skipping JS plugin behavior test: %v", err)
	}

	_, callerPath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	rootPluginPath := filepath.Join(filepath.Dir(callerPath), "labdrian-runtime-parity-plugin.mjs")
	source, err := os.ReadFile(rootPluginPath)
	if err != nil {
		t.Fatalf("read repository plugin source: %v", err)
	}

	pluginRoot := t.TempDir()
	pluginDir := filepath.Join(pluginRoot, "plugins")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("mkdir plugin dir: %v", err)
	}
	pluginPath := filepath.Join(pluginDir, "labdrian-runtime-parity-plugin.mjs")
	if err := os.WriteFile(pluginPath, source, 0o644); err != nil {
		t.Fatalf("write test plugin fixture: %v", err)
	}

	configPath := filepath.Join(pluginRoot, "labdrian-runtime-parity.json")
	config := map[string]any{
		"installed_version":   engineRuntime.OpenCodePluginVersion,
		"installed_hash":      "plugin-hash",
		"prompt_config_hash":  "marker-hash",
		"plugin_path":         pluginPath,
		"plugin_config_root":  pluginRoot,
		"plugin_config_scope": "global-opencode-config",
		"prompt_config": map[string]any{
			"contract_path":   "skills/_shared/minimalism-contract.md",
			"included_phases": []string{"sdd-tasks"},
			"excluded_phases": []string{"sdd-spec", "sdd-verify", "sdd-archive", "sdd-design", "sdd-propose"},
			"injection_point": "## Skills to load before work",
		},
	}
	configJSON, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(configPath, configJSON, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	pluginURL := "file://" + pluginPath
	script := fmt.Sprintf(`import path from "node:path";
const runtimePlugin = await import(%q).then((m) => m.default);
const hooks = await runtimePlugin();

if (typeof hooks["tool.execute.before"] !== "function") {
  throw new Error("tool.execute.before hook missing from runtime plugin default export");
}

const base = %q;
	const withContract = base + "\nskills/_shared/minimalism-contract.md";
const output = {
  included: { args: { prompt: base, subagent_type: "sdd-tasks" } },
  excluded: { args: { prompt: withContract, subagent_type: "sdd-spec" } },
  unchanged: { args: { prompt: base, subagent_type: "sdd-apply" } },
};

hooks["tool.execute.before"]({tool: "task"}, output.included);
hooks["tool.execute.before"]({tool: "task"}, output.excluded);
hooks["tool.execute.before"]({tool: "task"}, output.unchanged);

import { readFile } from "node:fs/promises";
const marker = JSON.parse(await readFile(path.join(%q, "labdrian-runtime-parity.active.json"), "utf8"));

console.log(JSON.stringify({
  included: output.included.args.prompt,
  excluded: output.excluded.args.prompt,
  unchanged: output.unchanged.args.prompt,
  active_marker: marker,
}));
`, pluginURL, basePromptFixture, pluginRoot)

	scriptPath := filepath.Join(t.TempDir(), "labdrian-runtime-parity-plugin-behavior.mjs")
	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		t.Fatalf("write node script: %v", err)
	}

	cmd := exec.Command("node", scriptPath)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("node exited with error: %v\n%s", err, ee.Stderr)
		}
		t.Fatalf("node exec failed: %v", err)
	}

	var got struct {
		Included     string         `json:"included"`
		Excluded     string         `json:"excluded"`
		Unchanged    string         `json:"unchanged"`
		ActiveMarker map[string]any `json:"active_marker"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode result: %v\nraw=%q", err, string(out))
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

	if got.ActiveMarker["active_version"] != engineRuntime.OpenCodePluginVersion {
		t.Fatalf("active marker should include installed version, got %#v", got.ActiveMarker)
	}
	if got.ActiveMarker["active_prompt_config_hash"] != config["prompt_config_hash"] {
		t.Fatalf("active marker should include prompt config hash, got %#v", got.ActiveMarker)
	}
	if got.ActiveMarker["plugin_path"] != pluginPath {
		t.Fatalf("active marker plugin_path mismatch: got %#v", got.ActiveMarker)
	}
}

func TestLabdrianRuntimeParityPluginEvaluatesContextAwareContracts(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skipf("node unavailable; skipping JS plugin behavior test: %v", err)
	}

	_, callerPath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	source, err := os.ReadFile(filepath.Join(filepath.Dir(callerPath), "labdrian-runtime-parity-plugin.mjs"))
	if err != nil {
		t.Fatalf("read repository plugin source: %v", err)
	}
	pluginRoot := t.TempDir()
	pluginDir := filepath.Join(pluginRoot, "plugins")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("mkdir plugin dir: %v", err)
	}
	pluginPath := filepath.Join(pluginDir, "labdrian-runtime-parity-plugin.mjs")
	if err := os.WriteFile(pluginPath, source, 0o644); err != nil {
		t.Fatalf("write test plugin fixture: %v", err)
	}
	config := map[string]any{
		"installed_version":   engineRuntime.OpenCodePluginVersion,
		"installed_hash":      engineRuntime.OpenCodePluginHash(),
		"prompt_config_hash":  "marker-hash",
		"plugin_path":         pluginPath,
		"plugin_config_root":  pluginRoot,
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
	}
	writeJSONFile(t, filepath.Join(pluginRoot, "labdrian-runtime-parity.json"), config)
	pluginURL := "file://" + pluginPath
	script := fmt.Sprintf(`const runtimePlugin = await import(%q).then((m) => m.default);
const hooks = await runtimePlugin();
const base = %q;
		const outputs = {
		  staticTamperOnly: { args: { prompt: base, subagent_type: "sdd-apply" } },
		  matching: { args: { prompt: base, subagent_type: "sdd-apply", work_context: { trusted: true, languages: ["typescript"], activations: ["oo-domain-design"], work_kinds: ["application-code"] } } },
		  missingContext: { args: { prompt: base + " TypeScript NestJS SOLID", subagent_type: "sdd-apply" } },
		  nonDomain: { args: { prompt: base, subagent_type: "sdd-apply", work_context: { trusted: true, languages: ["go"], activations: ["non-domain"], work_kinds: ["application-code"] } } },
		  docsWork: { args: { prompt: base, subagent_type: "sdd-apply", work_context: { trusted: true, languages: ["typescript"], activations: ["oo-domain-design"], work_kinds: ["docs"] } } },
		  configWork: { args: { prompt: base, subagent_type: "sdd-apply", work_context: { trusted: true, languages: ["typescript"], activations: ["oo-domain-design"], work_kinds: ["config"] } } },
		  generatedArtifactWork: { args: { prompt: base, subagent_type: "sdd-apply", work_context: { trusted: true, languages: ["typescript"], activations: ["oo-domain-design"], work_kinds: ["generated-artifact"] } } },
		  missingWorkKinds: { args: { prompt: base, subagent_type: "sdd-apply", work_context: { trusted: true, languages: ["typescript"], activations: ["oo-domain-design"] } } },
		  emptyWorkKinds: { args: { prompt: base, subagent_type: "sdd-apply", work_context: { trusted: true, languages: ["typescript"], activations: ["oo-domain-design"], work_kinds: [] } } },
		  malformedWorkKinds: { args: { prompt: base, subagent_type: "sdd-apply", work_context: { trusted: true, languages: ["typescript"], activations: ["oo-domain-design"], work_kinds: "application-code" } } },
		};
for (const output of Object.values(outputs)) {
  hooks["tool.execute.before"]({ tool: "task" }, output);
}
console.log(JSON.stringify(Object.fromEntries(Object.entries(outputs).map(([name, output]) => [name, output.args.prompt]))));`, pluginURL, basePromptFixture)

	scriptPath := filepath.Join(t.TempDir(), "labdrian-runtime-parity-context-contracts.mjs")
	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		t.Fatalf("write node script: %v", err)
	}
	out, err := exec.Command("node", scriptPath).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("node exited with error: %v\n%s", err, ee.Stderr)
		}
		t.Fatalf("node exec failed: %v", err)
	}
	var got struct {
		StaticTamperOnly      string `json:"staticTamperOnly"`
		Matching              string `json:"matching"`
		MissingContext        string `json:"missingContext"`
		NonDomain             string `json:"nonDomain"`
		DocsWork              string `json:"docsWork"`
		ConfigWork            string `json:"configWork"`
		GeneratedArtifactWork string `json:"generatedArtifactWork"`
		MissingWorkKinds      string `json:"missingWorkKinds"`
		EmptyWorkKinds        string `json:"emptyWorkKinds"`
		MalformedWorkKinds    string `json:"malformedWorkKinds"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode result: %v\nraw=%q", err, string(out))
	}
	if strings.Contains(got.StaticTamperOnly, "skills/_shared/oo-quality-contract.md") {
		t.Fatalf("static config work_context must not inject OO contract through plugin hook path: %q", got.StaticTamperOnly)
	}
	if !strings.Contains(got.Matching, "skills/_shared/oo-quality-contract.md") {
		t.Fatalf("trusted matching work context should inject OO contract: %q", got.Matching)
	}
	if strings.Contains(got.MissingContext, "skills/_shared/oo-quality-contract.md") {
		t.Fatalf("prompt text alone must not inject OO contract: %q", got.MissingContext)
	}
	if strings.Contains(got.NonDomain, "skills/_shared/oo-quality-contract.md") {
		t.Fatalf("non-domain context must not inject OO contract: %q", got.NonDomain)
	}
	if strings.Contains(got.DocsWork, "skills/_shared/oo-quality-contract.md") {
		t.Fatalf("docs work must pass through without OO contract injection: %q", got.DocsWork)
	}
	if strings.Contains(got.ConfigWork, "skills/_shared/oo-quality-contract.md") {
		t.Fatalf("config work must pass through without OO contract injection: %q", got.ConfigWork)
	}
	if strings.Contains(got.GeneratedArtifactWork, "skills/_shared/oo-quality-contract.md") {
		t.Fatalf("generated-artifact work must pass through without OO contract injection: %q", got.GeneratedArtifactWork)
	}
	if strings.Contains(got.MissingWorkKinds, "skills/_shared/oo-quality-contract.md") {
		t.Fatalf("missing work_kinds must not inject OO contract: %q", got.MissingWorkKinds)
	}
	if strings.Contains(got.EmptyWorkKinds, "skills/_shared/oo-quality-contract.md") {
		t.Fatalf("empty work_kinds must not inject OO contract: %q", got.EmptyWorkKinds)
	}
	if strings.Contains(got.MalformedWorkKinds, "skills/_shared/oo-quality-contract.md") {
		t.Fatalf("malformed work_kinds must not inject OO contract: %q", got.MalformedWorkKinds)
	}
	if got.MissingContext == got.Matching {
		t.Fatalf("missing per-invocation context should pass through instead of using static config")
	}
}

func TestLabdrianRuntimeParityPluginSkipsMalformedLegacyContextMetadata(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skipf("node unavailable; skipping JS plugin behavior test: %v", err)
	}

	_, callerPath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	source, err := os.ReadFile(filepath.Join(filepath.Dir(callerPath), "labdrian-runtime-parity-plugin.mjs"))
	if err != nil {
		t.Fatalf("read repository plugin source: %v", err)
	}

	for _, tt := range []struct {
		name         string
		promptConfig map[string]any
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
			pluginRoot := t.TempDir()
			pluginDir := filepath.Join(pluginRoot, "plugins")
			if err := os.MkdirAll(pluginDir, 0o755); err != nil {
				t.Fatalf("mkdir plugin dir: %v", err)
			}
			pluginPath := filepath.Join(pluginDir, "labdrian-runtime-parity-plugin.mjs")
			if err := os.WriteFile(pluginPath, source, 0o644); err != nil {
				t.Fatalf("write test plugin fixture: %v", err)
			}
			writeJSONFile(t, filepath.Join(pluginRoot, "labdrian-runtime-parity.json"), map[string]any{
				"installed_version":   engineRuntime.OpenCodePluginVersion,
				"installed_hash":      engineRuntime.OpenCodePluginHash(),
				"prompt_config_hash":  "marker-hash",
				"plugin_path":         pluginPath,
				"plugin_config_root":  pluginRoot,
				"plugin_config_scope": "global-opencode-config",
				"prompt_config":       tt.promptConfig,
			})
			pluginURL := "file://" + pluginPath
			script := fmt.Sprintf(`const runtimePlugin = await import(%q).then((m) => m.default);
const hooks = await runtimePlugin();
const output = { args: { prompt: %q, subagent_type: "sdd-apply", work_context: { trusted: true, languages: ["typescript"], activations: ["oo-domain-design"], work_kinds: ["application-code"] } } };
hooks["tool.execute.before"]({ tool: "task" }, output);
console.log(output.args.prompt);`, pluginURL, basePromptFixture)
			scriptPath := filepath.Join(t.TempDir(), "labdrian-runtime-parity-legacy-context.mjs")
			if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
				t.Fatalf("write node script: %v", err)
			}
			out, err := exec.Command("node", scriptPath).Output()
			if err != nil {
				if ee, ok := err.(*exec.ExitError); ok {
					t.Fatalf("node exited with error: %v\n%s", err, ee.Stderr)
				}
				t.Fatalf("node exec failed: %v", err)
			}
			got := strings.TrimSpace(string(out))
			injected := strings.Contains(got, "skills/_shared/oo-quality-contract.md")
			wantInjected := tt.name == "legacy matching context injects"
			if injected != wantInjected {
				t.Fatalf("legacy prompt config injection mismatch: got=%v want=%v prompt=%q", injected, wantInjected, got)
			}
		})
	}
}

func readOpenCodeConfig(t *testing.T, root string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "labdrian-runtime-parity.json"))
	if err != nil {
		t.Fatalf("read OpenCode config: %v", err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("decode OpenCode config: %v\n%s", err, string(data))
	}
	return config
}

func readPromptConfigHash(t *testing.T, root string) string {
	t.Helper()
	config := readOpenCodeConfig(t, root)
	hash, _ := config["prompt_config_hash"].(string)
	if hash == "" {
		t.Fatalf("prompt_config_hash missing in %#v", config)
	}
	return hash
}

func writeMatchingActiveMarker(t *testing.T, root string) {
	t.Helper()
	promptHash := readPromptConfigHash(t, root)
	active := `{"active_version":"` + engineRuntime.OpenCodePluginVersion + `","active_hash":"` + engineRuntime.OpenCodePluginHash() + `","active_prompt_config_hash":"` + promptHash + `","plugin_path":"` + filepath.Join(root, "plugins", "labdrian-runtime-parity.js") + `","config_root":"` + root + `"}`
	if err := os.WriteFile(filepath.Join(root, "labdrian-runtime-parity.active.json"), []byte(active), 0o644); err != nil {
		t.Fatalf("write active marker: %v", err)
	}
}

func writeJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("marshal %s: %v", path, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readActiveMarkerJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read active marker: %v", err)
	}
	var marker map[string]any
	if err := json.Unmarshal(data, &marker); err != nil {
		t.Fatalf("decode active marker: %v\n%s", err, string(data))
	}
	return marker
}

func sameJSONIdentity(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for key, av := range a {
		if b[key] != av {
			return false
		}
	}
	return true
}

func assertStringSlice(t *testing.T, got any, want []string) {
	t.Helper()
	items, ok := got.([]any)
	if !ok {
		t.Fatalf("got %#v, want JSON array", got)
	}
	if len(items) != len(want) {
		t.Fatalf("got %#v, want %#v", items, want)
	}
	for i, wantItem := range want {
		if items[i] != wantItem {
			t.Fatalf("got %#v, want %#v", items, want)
		}
	}
}

func hasPromptContract(contracts []any, path string) bool {
	for _, raw := range contracts {
		contract, ok := raw.(map[string]any)
		if ok && contract["contract_path"] == path {
			return true
		}
	}
	return false
}

func mustFindPromptContract(t *testing.T, contracts []any, path string) map[string]any {
	t.Helper()
	for _, raw := range contracts {
		contract, ok := raw.(map[string]any)
		if ok && contract["contract_path"] == path {
			return contract
		}
	}
	t.Fatalf("contracts missing %s: %#v", path, contracts)
	return nil
}
