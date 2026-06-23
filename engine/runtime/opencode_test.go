package runtime_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
)

func TestOpenCodeInstallWritesPluginConfigAndRestartRequiredStatus(t *testing.T) {
	// R-103: install writes the plugin bridge/config and reports restart_required until active.
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
		t.Fatal("installed plugin source should match the repository plugin source exactly")
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
	// R-103: status becomes supported only after the plugin load writes the active marker.
	root := t.TempDir()
	adapter := engineRuntime.NewOpenCodeAdapter(root)
	if result := adapter.Install(); result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Install status = %q", result.Status)
	}
	activeMarker := filepath.Join(root, "labdrian-runtime-parity.active.json")
	promptHash := readPromptConfigHash(t, root)
	active := `{"active_version":"` + engineRuntime.OpenCodePluginVersion + `","active_hash":"` + engineRuntime.OpenCodePluginHash() + `","active_prompt_config_hash":"` + promptHash + `","plugin_path":"` + filepath.Join(root, "plugins", "labdrian-runtime-parity.js") + `","config_root":"` + root + `"}`
	active = strings.ReplaceAll(active, `\"`, `"`)
	if err := os.WriteFile(activeMarker, []byte(active), 0o644); err != nil {
		t.Fatalf("write active marker: %v", err)
	}

	status := adapter.Status()
	if status.Status != engineRuntime.CapabilitySupported {
		t.Fatalf("Status after active marker = %q, want supported; message: %s", status.Status, status.Message)
	}
	if !strings.Contains(status.Message, engineRuntime.OpenCodePluginHash()) {
		t.Errorf("supported status should name active hash, got %q", status.Message)
	}
}

func TestOpenCodeInstallWritesPromptConfigFromMinimalismContract(t *testing.T) {
	// R-103: installed prompt_config is derived from minimalism-contract frontmatter, including phase arrays.
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
	// R-103: fail closed so OpenCode cannot auto-load a plugin before valid config exists.
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
	// R-105: uninstall removes OpenCode-specific runtime artifacts.
	root := t.TempDir()
	adapter := engineRuntime.NewOpenCodeAdapter(root)
	adapter.Install()

	result := adapter.Uninstall()
	if result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Uninstall status = %q, want restart_required", result.Status)
	}
	if !strings.Contains(result.Message, "restart OpenCode") {
		t.Fatalf("Uninstall should report restart/manual cleanup requirement, got %q", result.Message)
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
	// R-105: removed plugin files still require restart when OpenCode may have loaded the plugin.
	root := t.TempDir()
	adapter := engineRuntime.NewOpenCodeAdapter(root)
	if result := adapter.Install(); result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}
	activeMarker := filepath.Join(root, "labdrian-runtime-parity.active.json")
	promptHash := readPromptConfigHash(t, root)
	active := `{"active_version":"` + engineRuntime.OpenCodePluginVersion + `","active_hash":"` + engineRuntime.OpenCodePluginHash() + `","active_prompt_config_hash":"` + promptHash + `","plugin_path":"` + filepath.Join(root, "plugins", "labdrian-runtime-parity.js") + `","config_root":"` + root + `"}`
	active = strings.ReplaceAll(active, `\"`, `"`)
	if err := os.WriteFile(activeMarker, []byte(active), 0o644); err != nil {
		t.Fatalf("write active marker: %v", err)
	}
	if result := adapter.Uninstall(); result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Uninstall() = %#v", result)
	}
	if result := adapter.Status(); result.Status != engineRuntime.CapabilityRestartRequired || !strings.Contains(result.Message, "restart OpenCode to unload") {
		t.Fatalf("Status() after uninstall with active marker should require restart, got %#v", result)
	}
	if result := adapter.Uninstall(); result.Status != engineRuntime.CapabilityRestartRequired || !strings.Contains(result.Message, activeMarker) {
		t.Fatalf("second uninstall without explicit restart confirmation should keep restart marker guidance, got %#v", result)
	}
	if _, err := os.Stat(activeMarker); err != nil {
		t.Fatalf("second uninstall must not remove active marker without explicit cleanup, stat err: %v", err)
	}
	if result := adapter.Status(); result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Status() with remaining active marker should stay restart_required, got %#v", result)
	}

	if err := os.Remove(activeMarker); err != nil {
		t.Fatalf("manual post-restart marker cleanup: %v", err)
	}
	if result := adapter.Status(); result.Status != engineRuntime.CapabilityUnsupported {
		t.Fatalf("Status() after manual marker cleanup should be unsupported, got %#v", result)
	}

	if result := adapter.Install(); result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Install() after manual cleanup = %#v", result)
	}
	if err := os.WriteFile(activeMarker, []byte(active), 0o644); err != nil {
		t.Fatalf("rewrite active marker: %v", err)
	}
	if result := adapter.Rollback(); result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Rollback() should also require restart when plugin may be active, got %#v", result)
	}
	if result := adapter.Rollback(); result.Status != engineRuntime.CapabilityRestartRequired || !strings.Contains(result.Message, activeMarker) {
		t.Fatalf("second rollback without explicit cleanup should keep restart marker guidance, got %#v", result)
	}

	// Recreate no-plugin/no-marker state for final status assertion.
	if err := os.Remove(activeMarker); err != nil {
		if !os.IsNotExist(err) {
			t.Fatalf("remove active marker: %v", err)
		}
	}
	if result := adapter.Status(); result.Status != engineRuntime.CapabilityUnsupported {
		t.Fatalf("Status() without plugin or active marker should be unsupported, got %#v", result)
	}
}

func TestOpenCodeInstallUpdatePreservesLoadedMarkerUntilRestart(t *testing.T) {
	// R-103/R-105: install/update must not erase active-marker evidence while OpenCode may still be running.
	for _, tt := range []struct {
		name         string
		refresh      func(engineRuntime.OpenCodeAdapter) engineRuntime.LifecycleResult
		remove       func(engineRuntime.OpenCodeAdapter) engineRuntime.LifecycleResult
		removeAction string
	}{
		{name: "install then rollback", refresh: func(a engineRuntime.OpenCodeAdapter) engineRuntime.LifecycleResult { return a.Install() }, remove: func(a engineRuntime.OpenCodeAdapter) engineRuntime.LifecycleResult { return a.Rollback() }, removeAction: "rollback"},
		{name: "update then uninstall", refresh: func(a engineRuntime.OpenCodeAdapter) engineRuntime.LifecycleResult { return a.Update() }, remove: func(a engineRuntime.OpenCodeAdapter) engineRuntime.LifecycleResult { return a.Uninstall() }, removeAction: "uninstall"},
	} {
		t.Run(tt.name, func(t *testing.T) {
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

			if result := tt.refresh(adapter); result.Status != engineRuntime.CapabilityRestartRequired {
				t.Fatalf("refresh result = %#v", result)
			}
			afterRefresh := readActiveMarkerJSON(t, activeMarker)
			if !sameJSONIdentity(beforeRefresh, afterRefresh) {
				t.Fatalf("refresh must preserve active marker JSON identity; before=%#v after=%#v", beforeRefresh, afterRefresh)
			}
			if result := adapter.Status(); result.Status != engineRuntime.CapabilityRestartRequired || !strings.Contains(result.Message, "restart") {
				t.Fatalf("stale active marker after refresh should require restart, got %#v", result)
			}

			if result := tt.remove(adapter); result.Status != engineRuntime.CapabilityRestartRequired {
				t.Fatalf("%s result = %#v", tt.removeAction, result)
			}
			afterRemove := readActiveMarkerJSON(t, activeMarker)
			if !sameJSONIdentity(beforeRefresh, afterRemove) {
				t.Fatalf("%s must preserve active marker JSON identity; before=%#v after=%#v", tt.removeAction, beforeRefresh, afterRemove)
			}
			if result := adapter.Status(); result.Status != engineRuntime.CapabilityRestartRequired || !strings.Contains(result.Message, "restart OpenCode to unload") {
				t.Fatalf("status after %s should preserve restart visibility, got %#v", tt.removeAction, result)
			}
		})
	}
}

func TestOpenCodePluginSourceAssertions(t *testing.T) {
	// R-001/R-103: the plugin mutates only scoped task prompts and avoids unsafe typing.
	pluginPath := "labdrian-runtime-parity-plugin.mjs"
	plugin, err := os.ReadFile(pluginPath)
	if err != nil {
		t.Fatalf("read plugin source: %v", err)
	}
	source := string(plugin)

	for _, want := range []string{
		"export default async function labdrianRuntimeParityPlugin",
		"export function mutatePrompt",
		"normalizePromptConfig",
		"prompt_config",
		"return {",
		"tool.execute.before",
		`input.tool !== "task"`,
		"output.args.prompt =",
		"writeFile(",
		"activeMarkerPath",
		"labdrian-runtime-parity.json",
		"config_root",
		"active_hash",
		"createHash",
		"fileURLToPath(import.meta.url)",
		"installed_version",
		"active_prompt_config_hash",
		"isToolExecuteInput",
		"isToolExecuteOutput",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("plugin source should contain %q", want)
		}
	}
	if strings.Contains(source, ": any") || strings.Contains(source, " as any") {
		t.Error("plugin source must not use any")
	}
	if strings.Contains(source, "output.args =") {
		t.Error("plugin must mutate output.args.prompt in place, not replace output.args")
	}
	for _, forbiddenPhase := range []string{`"sdd-tasks"`, `"sdd-apply"`, `"sdd-propose"`, `"sdd-spec"`, `"sdd-design"`, `"sdd-verify"`, `"sdd-archive"`} {
		if strings.Contains(source, forbiddenPhase) {
			t.Errorf("plugin source must read scoped phase names from installed config, found hardcoded %s", forbiddenPhase)
		}
	}
	for _, forbidden := range []string{"export const hooks", "export async function ToolExecuteBefore", "@opencode-ai/plugin"} {
		if strings.Contains(source, forbidden) {
			t.Errorf("plugin source must use OpenCode plugin function shape, found forbidden %q", forbidden)
		}
	}
}

func TestOpenCodeLifecycleAliasesAndStatusFailureModes(t *testing.T) {
	// R-103/R-105: lifecycle aliases use real install/status/cleanup semantics and report degraded states honestly.
	root := t.TempDir()
	adapter := engineRuntime.NewOpenCodeAdapter(root)

	if adapter.Target() != engineRuntime.TargetOpenCode {
		t.Fatalf("Target() = %q, want opencode", adapter.Target())
	}
	if result := adapter.Apply(); result.Action != engineRuntime.ActionInstall || result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Apply() should install and require restart, got %#v", result)
	}
	if result := adapter.SyncCheck(); result.Action != engineRuntime.ActionSyncCheck || result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("SyncCheck() should report restart-required installed plugin, got %#v", result)
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

	configPath := filepath.Join(root, "labdrian-runtime-parity.json")
	if err := os.WriteFile(configPath, []byte(`{"plugin_path":"x","installed_hash":""}`), 0o644); err != nil {
		t.Fatalf("write malformed config: %v", err)
	}
	if result := adapter.Status(); result.Status != engineRuntime.CapabilityPartial || !strings.Contains(result.Message, "config missing or invalid") {
		t.Fatalf("Status() should report invalid config as partial, got %#v", result)
	}
}

func TestOpenCodeStatusTreatsMalformedActiveMarkerAsRestartRequired(t *testing.T) {
	// R-103/R-105: corrupt active marker still means a process may have loaded the removed plugin.
	root := t.TempDir()
	adapter := engineRuntime.NewOpenCodeAdapter(root)
	if result := adapter.Install(); result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}
	activeMarker := filepath.Join(root, "labdrian-runtime-parity.active.json")
	if err := os.WriteFile(activeMarker, []byte("{not-json"), 0o644); err != nil {
		t.Fatalf("write corrupt marker: %v", err)
	}
	if result := adapter.Uninstall(); result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Uninstall() = %#v", result)
	}
	if result := adapter.Status(); result.Status != engineRuntime.CapabilityRestartRequired || !strings.Contains(result.Message, activeMarker) || !strings.Contains(result.Message, "manual cleanup") {
		t.Fatalf("corrupt remaining active marker should require restart/manual cleanup, got %#v", result)
	}
}

func TestOpenCodeStatusRejectsSpoofedOrStaleActiveMarker(t *testing.T) {
	// R-103: active marker must match installed version and plugin path, not just exist.
	root := t.TempDir()
	adapter := engineRuntime.NewOpenCodeAdapter(root)
	if result := adapter.Install(); result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}
	activeMarker := filepath.Join(root, "labdrian-runtime-parity.active.json")
	writeMarker := func(version, activeHash, promptConfigHash, pluginPath, configRoot string) {
		t.Helper()
		payload := `{"active_version":"` + version + `","active_hash":"` + activeHash + `","active_prompt_config_hash":"` + promptConfigHash + `","plugin_path":"` + pluginPath + `","config_root":"` + configRoot + `"}`
		payload = strings.ReplaceAll(payload, `\"`, `"`)
		if err := os.WriteFile(activeMarker, []byte(payload), 0o644); err != nil {
			t.Fatalf("write active marker: %v", err)
		}
	}

	promptHash := readPromptConfigHash(t, root)
	writeMarker("stale-version", engineRuntime.OpenCodePluginHash(), promptHash, filepath.Join(root, "plugins", "labdrian-runtime-parity.js"), root)
	if result := adapter.Status(); result.Status != engineRuntime.CapabilityRestartRequired || !strings.Contains(result.Message, "version mismatch") {
		t.Fatalf("stale version marker should require restart, got %#v", result)
	}

	writeMarker(engineRuntime.OpenCodePluginVersion, engineRuntime.OpenCodePluginHash(), promptHash, filepath.Join(root, "plugins", "other.js"), root)
	if result := adapter.Status(); result.Status != engineRuntime.CapabilityRestartRequired || !strings.Contains(result.Message, "path mismatch") {
		t.Fatalf("spoofed path marker should require restart, got %#v", result)
	}

	writeMarker(engineRuntime.OpenCodePluginVersion, engineRuntime.OpenCodePluginHash(), promptHash, filepath.Join(root, "plugins", "labdrian-runtime-parity.js"), filepath.Join(root, "other-config"))
	if result := adapter.Status(); result.Status != engineRuntime.CapabilityRestartRequired || !strings.Contains(result.Message, "config root mismatch") {
		t.Fatalf("spoofed config root marker should require restart, got %#v", result)
	}

	writeMarker(engineRuntime.OpenCodePluginVersion, "not-current-hash", promptHash, filepath.Join(root, "plugins", "labdrian-runtime-parity.js"), root)
	if result := adapter.Status(); result.Status != engineRuntime.CapabilityRestartRequired || !strings.Contains(result.Message, "hash mismatch") {
		t.Fatalf("spoofed hash marker should require restart, got %#v", result)
	}

	writeMarker(engineRuntime.OpenCodePluginVersion, engineRuntime.OpenCodePluginHash(), "not-current-prompt-config", filepath.Join(root, "plugins", "labdrian-runtime-parity.js"), root)
	if result := adapter.Status(); result.Status != engineRuntime.CapabilityRestartRequired || !strings.Contains(result.Message, "prompt config") {
		t.Fatalf("stale active prompt config marker should require restart, got %#v", result)
	}
}

func TestOpenCodeStatusRejectsTamperedPromptConfig(t *testing.T) {
	// R-103: supported status is bound to the frontmatter-derived prompt_config identity.
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
		t.Fatalf("wrong included phase array should require update/restart, got %#v", result)
	}

	config = readOpenCodeConfig(t, root)
	promptConfig = config["prompt_config"].(map[string]any)
	promptConfig["included_phases"] = []any{}
	writeJSONFile(t, configPath, config)
	if result := adapter.Status(); result.Status != engineRuntime.CapabilityRestartRequired || !strings.Contains(result.Message, "prompt_config") {
		t.Fatalf("empty included phase array should require update/restart, got %#v", result)
	}
}

func TestOpenCodeStatusRejectsMutuallyStalePromptConfigHash(t *testing.T) {
	// R-103: config and marker agreeing with each other is insufficient when both are stale vs current frontmatter.
	root := t.TempDir()
	adapter := engineRuntime.NewOpenCodeAdapter(root)
	if result := adapter.Install(); result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}
	configPath := filepath.Join(root, "labdrian-runtime-parity.json")
	config := readOpenCodeConfig(t, root)
	config["prompt_config_hash"] = "mutually-stale-prompt-config-hash"
	writeJSONFile(t, configPath, config)
	active := `{"active_version":"` + engineRuntime.OpenCodePluginVersion + `","active_hash":"` + engineRuntime.OpenCodePluginHash() + `","active_prompt_config_hash":"mutually-stale-prompt-config-hash","plugin_path":"` + filepath.Join(root, "plugins", "labdrian-runtime-parity.js") + `","config_root":"` + root + `"}`
	active = strings.ReplaceAll(active, `\"`, `"`)
	if err := os.WriteFile(filepath.Join(root, "labdrian-runtime-parity.active.json"), []byte(active), 0o644); err != nil {
		t.Fatalf("write stale active marker: %v", err)
	}
	if result := adapter.Status(); result.Status == engineRuntime.CapabilitySupported || !strings.Contains(result.Message, "prompt_config") {
		t.Fatalf("mutually stale prompt_config_hash must not be supported, got %#v", result)
	}
}

func TestOpenCodeStatusRejectsStaleInstalledConfigEvenWithMatchingMarker(t *testing.T) {
	// R-103: mutable config/marker agreement is insufficient; installed config must match current plugin version/hash.
	root := t.TempDir()
	adapter := engineRuntime.NewOpenCodeAdapter(root)
	if result := adapter.Install(); result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}
	configPath := filepath.Join(root, "labdrian-runtime-parity.json")
	config := `{"plugin_path":"` + filepath.Join(root, "plugins", "labdrian-runtime-parity.js") + `","installed_hash":"not-current","installed_version":"stale-version","activation_marker":"` + filepath.Join(root, "labdrian-runtime-parity.active.json") + `","plugin_config_root":"` + root + `","plugin_config_scope":"global-opencode-config"}`
	config = strings.ReplaceAll(config, `\"`, `"`)
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatalf("write stale config: %v", err)
	}
	active := `{"active_version":"stale-version","active_hash":"not-current","active_prompt_config_hash":"not-current","plugin_path":"` + filepath.Join(root, "plugins", "labdrian-runtime-parity.js") + `","config_root":"` + root + `"}`
	active = strings.ReplaceAll(active, `\"`, `"`)
	if err := os.WriteFile(filepath.Join(root, "labdrian-runtime-parity.active.json"), []byte(active), 0o644); err != nil {
		t.Fatalf("write stale active marker: %v", err)
	}
	if result := adapter.Status(); result.Status != engineRuntime.CapabilityPartial || !strings.Contains(result.Message, "not current") {
		t.Fatalf("stale mutually-agreeing config/marker should not be supported, got %#v", result)
	}
}

func TestOpenCodeAdapterRejectsUnresolvedOrRelativeConfigRoot(t *testing.T) {
	// R-103: unresolved config roots must not fall back to caller cwd writes.
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
	// R-103: default OpenCode config root honors XDG_CONFIG_HOME when present.
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
	// R-103: relative XDG_CONFIG_HOME must not create repo-relative OpenCode roots.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join("relative", "xdg"))
	want := filepath.Join(home, ".config", "opencode")
	if got := engineRuntime.DefaultOpenCodeConfigRoot(); got != want {
		t.Fatalf("DefaultOpenCodeConfigRoot() = %q, want %q", got, want)
	}
}

func TestOpenCodePluginBehaviorFixture(t *testing.T) {
	// R-001/R-103: execute the installed OpenCode plugin artifact with Node to prove hook behavior and marker write.
	root := t.TempDir()
	adapter := engineRuntime.NewOpenCodeAdapter(root)
	if result := adapter.Install(); result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"type":"module"}`), 0o644); err != nil {
		t.Fatalf("write package.json for installed .js ESM fixture: %v", err)
	}
	pluginPath := filepath.Join(root, "plugins", "labdrian-runtime-parity.js")
	if _, err := os.Stat(pluginPath); err != nil {
		t.Fatalf("installed plugin artifact should exist at %s: %v", pluginPath, err)
	}
	customConfigPath := filepath.Join(root, "labdrian-runtime-parity.json")
	customConfig := map[string]any{
		"installed_version":  engineRuntime.OpenCodePluginVersion,
		"prompt_config_hash": "custom-prompt-hash",
		"prompt_config": map[string]any{
			"contract_path":   "custom/contracts/minimal.md",
			"included_phases": []string{"custom-include"},
			"excluded_phases": []string{"custom-exclude"},
			"injection_point": "## Custom Skills Header",
		},
	}
	customConfigData, err := json.Marshal(customConfig)
	if err != nil {
		t.Fatalf("marshal custom config: %v", err)
	}
	if err := os.WriteFile(customConfigPath, customConfigData, 0o644); err != nil {
		t.Fatalf("write custom plugin config: %v", err)
	}
	scriptPath := filepath.Join(root, "run-plugin-test.mjs")
	script := `
import { pathToFileURL } from "node:url";
import { readFile } from "node:fs/promises";
const pluginPath = process.argv[2];
const mod = await import(pathToFileURL(pluginPath));
const hooks = await mod.default();
const run = async (tool, subagent_type, prompt) => {
  const output = { args: { subagent_type, prompt } };
  await hooks["tool.execute.before"]({ tool }, output);
  return output.args.prompt;
};
const taskPrompt = await run("task", "custom-include", "Do tasks");
const applyPrompt = await run("task", "sdd-apply", "Do apply");
const existingHeaderPrompt = await run("task", "custom-include", "Do tasks\n## Custom Skills Header\n- existing");
const strippedPrompt = await run("task", "custom-exclude", taskPrompt);
const ignoredToolPrompt = await run("bash", "custom-include", "Do tasks");
const generalPrompt = await run("task", "general", "Do general");
const marker = JSON.parse(await readFile(new URL("../labdrian-runtime-parity.active.json", pathToFileURL(pluginPath))));
console.log(JSON.stringify({ taskPrompt, applyPrompt, existingHeaderPrompt, strippedPrompt, ignoredToolPrompt, generalPrompt, marker }));
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		t.Fatalf("write node script: %v", err)
	}
	out, err := exec.Command("node", scriptPath, pluginPath).CombinedOutput()
	if err != nil {
		t.Fatalf("node plugin behavior test failed (Node is required for this repo): %v\n%s", err, string(out))
	}
	var result struct {
		TaskPrompt        string `json:"taskPrompt"`
		ApplyPrompt       string `json:"applyPrompt"`
		ExistingHeader    string `json:"existingHeaderPrompt"`
		StrippedPrompt    string `json:"strippedPrompt"`
		IgnoredToolPrompt string `json:"ignoredToolPrompt"`
		GeneralPrompt     string `json:"generalPrompt"`
		Marker            struct {
			ActiveVersion string `json:"active_version"`
			ActiveHash    string `json:"active_hash"`
			PromptHash    string `json:"active_prompt_config_hash"`
			PluginPath    string `json:"plugin_path"`
			ConfigRoot    string `json:"config_root"`
		} `json:"marker"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("decode node plugin output: %v\n%s", err, string(out))
	}
	if !strings.Contains(result.TaskPrompt, "custom/contracts/minimal.md") {
		t.Fatalf("plugin should inject configured contract for configured include phase, got %#v", result)
	}
	if strings.Contains(result.ApplyPrompt, "custom/contracts/minimal.md") {
		t.Fatalf("plugin must not inject hardcoded sdd-apply when config excludes it, got %#v", result)
	}
	if strings.Contains(result.StrippedPrompt, "custom/contracts/minimal.md") {
		t.Fatalf("plugin should strip contract for excluded phases in place, got %#v", result)
	}
	if strings.Count(result.ExistingHeader, "## Custom Skills Header") != 1 || !strings.Contains(result.ExistingHeader, "## Custom Skills Header\ncustom/contracts/minimal.md\n- existing") {
		t.Fatalf("plugin should inject under existing header without duplicating it, got %q", result.ExistingHeader)
	}
	if result.IgnoredToolPrompt != "Do tasks" || result.GeneralPrompt != "Do general" {
		t.Fatalf("plugin should ignore non-task tools and unrelated phases, got %#v", result)
	}
	if result.Marker.ActiveVersion != engineRuntime.OpenCodePluginVersion || result.Marker.ActiveHash != engineRuntime.OpenCodePluginHash() || result.Marker.PromptHash != "custom-prompt-hash" || result.Marker.PluginPath != pluginPath || result.Marker.ConfigRoot != root {
		t.Fatalf("plugin should write active marker identity, got %#v", result.Marker)
	}
}

func TestOpenCodePromptMutationParityWithGoHelpers(t *testing.T) {
	// R-001/R-103: JS and Go share generic exact-line prompt semantics even though runtimes differ.
	root := t.TempDir()
	pluginPath, err := filepath.Abs("labdrian-runtime-parity-plugin.mjs")
	if err != nil {
		t.Fatalf("resolve plugin path: %v", err)
	}
	scriptPath := filepath.Join(root, "prompt-parity.mjs")
	script := `
import { pathToFileURL } from "node:url";
const mod = await import(pathToFileURL(process.argv[2]));
const config = { prompt_config: { contract_path: "contract.md", included_phases: ["include"], excluded_phases: ["exclude"], injection_point: "## Header" } };
const cases = [
  mod.mutatePrompt("Do work", "include", config),
  mod.mutatePrompt("Do work\n## Header\n- existing", "include", config),
  mod.mutatePrompt("Do work\n## Header\ncontract.md\n- existing", "exclude", config),
  mod.mutatePrompt("Do work", "other", config),
];
console.log(JSON.stringify(cases));
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		t.Fatalf("write parity script: %v", err)
	}
	out, err := exec.Command("node", scriptPath, pluginPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run JS parity script: %v\n%s", err, string(out))
	}
	var got []string
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode JS parity output: %v\n%s", err, string(out))
	}
	phases := engineRuntime.ContractPhases{AppliesTo: []string{"include"}, Excluded: []string{"exclude"}, InjectionPoint: "## Header"}
	want := []string{}
	for _, tc := range []struct{ prompt, phase string }{
		{"Do work", "include"},
		{"Do work\n## Header\n- existing", "include"},
		{"Do work\n## Header\ncontract.md\n- existing", "exclude"},
		{"Do work", "other"},
	} {
		mutated, _ := engineRuntime.MutatePrompt(tc.prompt, tc.phase, "contract.md", phases)
		want = append(want, mutated)
	}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("JS prompt mutation = %#v, want Go helper output %#v", got, want)
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
	active = strings.ReplaceAll(active, `\"`, `"`)
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

func TestOpenCodeAdapterDoesNotPolluteCallerWorkingDirectory(t *testing.T) {
	// R-103/R-105: OpenCode lifecycle writes only to the explicit config root, never caller cwd.
	configRoot := t.TempDir()
	callerCWD := t.TempDir()
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	t.Setenv("LABDRIAN_OVERLAY_DIR", repoRoot)
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
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
		t.Fatalf("OpenCode install should not create .opencode under caller cwd, stat err: %v", err)
	}
	if _, err := os.Stat(filepath.Join(configRoot, "plugins", "labdrian-runtime-parity.js")); err != nil {
		t.Fatalf("OpenCode plugin should be installed under explicit config root: %v", err)
	}
}
