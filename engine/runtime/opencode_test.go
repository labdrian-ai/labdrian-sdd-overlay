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

	configPath := filepath.Join(root, "labdrian-runtime-parity.json")
	config := readOpenCodeConfig(t, root)
	promptConfig := config["prompt_config"].(map[string]any)
	promptConfig["included_phases"] = []any{"sdd-apply"}
	writeJSONFile(t, configPath, config)
	if result := adapter.Status(); result.Status != engineRuntime.CapabilityRestartRequired || !strings.Contains(result.Message, "prompt_config") {
		t.Fatalf("wrong included phase array should require restart, got %#v", result)
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
		"installed_version":   "2026-06-22-runtime-parity-3",
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

	if got.ActiveMarker["active_version"] != "2026-06-22-runtime-parity-3" {
		t.Fatalf("active marker should include installed version, got %#v", got.ActiveMarker)
	}
	if got.ActiveMarker["active_prompt_config_hash"] != config["prompt_config_hash"] {
		t.Fatalf("active marker should include prompt config hash, got %#v", got.ActiveMarker)
	}
	if got.ActiveMarker["plugin_path"] != pluginPath {
		t.Fatalf("active marker plugin_path mismatch: got %#v", got.ActiveMarker)
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
