package runtime_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
)

const basePromptFixture = "line 1\n## Skills to load before work\nline 2"

func TestOpenCodeInstallWritesPluginConfigAndRestartRequiredStatus(t *testing.T) {
	root := t.TempDir()
	adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{})

	install := adapter.Install()
	if install.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install status = %q, want %q", install.Status, core.CapabilityRestartRequired)
	}

	pluginPath := openCodePluginPath(root)
	plugin, err := os.ReadFile(pluginPath)
	if err != nil {
		t.Fatalf("plugin should be written at %s: %v", pluginPath, err)
	}
	for _, want := range []string{
		`import { Plugin } from "@opencode/plugin"`,
		"export default Plugin.define(",
		`id: "labdrian-runtime-parity"`,
	} {
		if !strings.Contains(string(plugin), want) {
			t.Fatalf("plugin should be a v2 definition containing %q; got:\n%s", want, string(plugin))
		}
	}
	if strings.Contains(string(plugin), "tool.execute.before") {
		t.Fatalf("plugin must not export the 1.x tool.execute.before hook; got:\n%s", string(plugin))
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
	if status.Status != core.CapabilityRestartRequired {
		t.Fatalf("Status after install = %q, want restart_required; message: %s", status.Status, status.Message)
	}
	if !strings.Contains(status.Message, "restart") {
		t.Errorf("restart-required status should explain restart, got %q", status.Message)
	}
}

func TestOpenCodeStatusSupportedWhenActiveMarkerMatchesHash(t *testing.T) {
	root := t.TempDir()
	adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{})
	if result := adapter.Install(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install status = %q", result.Status)
	}
	activeMarker := filepath.Join(root, "labdrian-runtime-parity.active.json")
	promptHash := readPromptConfigHash(t, root)
	active := `{"active_version":"` + engineRuntime.OpenCodePluginVersion + `","active_hash":"` + engineRuntime.OpenCodePluginHash() + `","active_prompt_config_hash":"` + promptHash + `","plugin_path":"` + openCodePluginPath(root) + `","config_root":"` + root + `"}`
	if err := os.WriteFile(activeMarker, []byte(active), 0o644); err != nil {
		t.Fatalf("write active marker: %v", err)
	}

	status := adapter.Status()
	if status.Status != core.CapabilitySupported {
		t.Fatalf("Status after active marker = %q, want supported; message: %s", status.Status, status.Message)
	}
	if !strings.Contains(status.Message, engineRuntime.OpenCodePluginHash()) {
		t.Errorf("supported status should include hash, got %q", status.Message)
	}
}

func TestOpenCodeInstallWritesPromptConfigFromMinimalismContract(t *testing.T) {
	root := t.TempDir()
	adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{})
	if result := adapter.Install(); result.Status != core.CapabilityRestartRequired {
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
	assertStringSlice(t, promptConfig["included_phases"], []string{"shaper", "builder"})
	assertStringSlice(t, promptConfig["excluded_phases"], []string{"sdd-propose", "sdd-spec", "sdd-design", "sdd-verify", "sdd-archive", "reviewer", "verifier"})
	if config["prompt_config_hash"] == "" {
		t.Fatal("config should include prompt_config_hash")
	}
	contracts := promptConfig["contracts"].([]any)
	// Assert every unconditional contract by name. A count-only bound cannot
	// tell "both are present" from "one is, plus the optional one", which is
	// how anti-generic-design stayed unwired while this test passed.
	for _, want := range []string{
		"skills/_shared/minimalism-contract.md",
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

// The owner decided (2026-10-09, H26) that a contract the plugin cannot use aborts the install,
// the optional one too: nothing is written, and the message names the file and the parse error.
// Until then the optional contract was dropped and the install went on without it.
func TestOpenCodeInstallRefusesAnOOContractWithMalformedOrUnsupportedContextMetadata(t *testing.T) {
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
			root := t.TempDir()
			adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{OverlayDir: overlayRoot})

			result := adapter.Install()

			if result.Status != core.CapabilityPartial {
				t.Fatalf("Install() = %#v, want partial: a contract the plugin cannot use aborts the install", result)
			}
			for _, want := range []string{"prompt config could not be derived", "skills/_shared/oo-quality-contract.md: "} {
				if !strings.Contains(result.Message, want) {
					t.Errorf("message = %q, want it to say %q", result.Message, want)
				}
			}
			for _, written := range []string{
				openCodePluginPath(root),
				filepath.Join(root, "labdrian-runtime-parity.json"),
			} {
				if _, err := os.Stat(written); !os.IsNotExist(err) {
					t.Errorf("%s was written although the install aborted (stat err: %v)", written, err)
				}
			}
		})
	}
}

func TestOpenCodeInstallDoesNotWritePluginWhenPromptConfigCannotBeDerived(t *testing.T) {
	root := t.TempDir()
	adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{OverlayDir: t.TempDir()})

	result := adapter.Install()
	if result.Status != core.CapabilityPartial || !strings.Contains(result.Message, "prompt config could not be derived") {
		t.Fatalf("Install() should fail before writing plugin when prompt config is unavailable, got %#v", result)
	}
	pluginPath := openCodePluginPath(root)
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
	adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{})
	if result := adapter.Install(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}

	if _, err := os.Stat(openCodePluginPath(root)); err != nil {
		t.Fatalf("plugin should be installed before uninstall: %v", err)
	}

	result := adapter.Uninstall()
	if result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Uninstall status = %q, want restart_required", result.Status)
	}
	for _, path := range []string{
		openCodePluginPath(root),
		filepath.Join(root, "labdrian-runtime-parity.json"),
		// An empty plugin directory would be loaded by OpenCode as a plugin with no index.js.
		filepath.Dir(openCodePluginPath(root)),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("%s should be removed, stat err: %v", path, err)
		}
	}
}

func TestOpenCodeStatusAfterUninstallHonorsActiveMarker(t *testing.T) {
	root := t.TempDir()
	adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{})
	if result := adapter.Install(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}
	activeMarker := filepath.Join(root, "labdrian-runtime-parity.active.json")
	writeMatchingActiveMarker(t, root)
	if result := adapter.Uninstall(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Uninstall() = %#v", result)
	}
	if result := adapter.Status(); result.Status != core.CapabilityRestartRequired || !strings.Contains(result.Message, "restart OpenCode to unload") {
		t.Fatalf("Status() after uninstall with active marker should require restart, got %#v", result)
	}
	if result := adapter.Uninstall(); result.Status != core.CapabilityRestartRequired || !strings.Contains(result.Message, activeMarker) {
		t.Fatalf("second uninstall should keep restart guidance, got %#v", result)
	}
	if _, err := os.Stat(activeMarker); err != nil {
		t.Fatalf("second uninstall must not remove active marker, stat err: %v", err)
	}

	if err := os.Remove(activeMarker); err != nil {
		t.Fatalf("manual post-restart marker cleanup: %v", err)
	}
	if result := adapter.Status(); result.Status != core.CapabilityUnsupported {
		t.Fatalf("Status() after manual marker cleanup should be unsupported, got %#v", result)
	}
}

func TestOpenCodeInstallUpdatePreservesLoadedMarkerUntilRestart(t *testing.T) {
	root := t.TempDir()
	adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{})
	if result := adapter.Install(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("initial Install() = %#v", result)
	}
	writeMatchingActiveMarker(t, root)
	activeMarker := filepath.Join(root, "labdrian-runtime-parity.active.json")
	oldTime := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(activeMarker, oldTime, oldTime); err != nil {
		t.Fatalf("age active marker: %v", err)
	}
	beforeRefresh := readActiveMarkerJSON(t, activeMarker)

	if result := adapter.Install(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("refresh install = %#v", result)
	}
	afterRefresh := readActiveMarkerJSON(t, activeMarker)
	if !sameJSONIdentity(beforeRefresh, afterRefresh) {
		t.Fatalf("refresh should preserve active marker identity; before=%#v after=%#v", beforeRefresh, afterRefresh)
	}
	if result := adapter.Status(); result.Status != core.CapabilityRestartRequired || !strings.Contains(result.Message, "restart") {
		t.Fatalf("stale active marker after refresh should require restart, got %#v", result)
	}
}

func TestOpenCodeStatusRejectsTamperedPromptConfig(t *testing.T) {
	root := t.TempDir()
	adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{})
	if result := adapter.Install(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}
	writeMatchingActiveMarker(t, root)
	if result := adapter.Status(); result.Status != core.CapabilitySupported {
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
			if result := adapter.Status(); result.Status != core.CapabilityRestartRequired || !strings.Contains(result.Message, "prompt_config") {
				t.Fatalf("tampered prompt config should require restart, got %#v", result)
			}
			if result := adapter.Install(); result.Status != core.CapabilityRestartRequired {
				t.Fatalf("reinstall after tamper = %#v", result)
			}
			writeMatchingActiveMarker(t, root)
		})
	}
}

func TestOpenCodeStatusRejectsMalformedNestedContractContextMetadata(t *testing.T) {
	root := t.TempDir()
	adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{})
	if result := adapter.Install(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}
	writeMatchingActiveMarker(t, root)

	configPath := filepath.Join(root, "labdrian-runtime-parity.json")
	config := readOpenCodeConfig(t, root)
	contract := mustFindPromptContract(t, config["prompt_config"].(map[string]any)["contracts"].([]any), "skills/_shared/oo-quality-contract.md")
	contract["language_context"] = "typescript"
	writeJSONFile(t, configPath, config)

	result := adapter.Status()
	if result.Status != core.CapabilityPartial {
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
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	if err := os.Chdir(callerCWD); err != nil {
		t.Fatalf("chdir caller cwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	adapter := engineRuntime.NewOpenCodeAdapter(configRoot, engineRuntime.OpenCodeOptions{OverlayDir: repoRoot})
	if result := adapter.Install(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}
	if _, err := os.Stat(filepath.Join(callerCWD, ".opencode")); !os.IsNotExist(err) {
		t.Fatalf("install should not create .opencode under caller cwd, stat err: %v", err)
	}
	if _, err := os.Stat(openCodePluginPath(configRoot)); err != nil {
		t.Fatalf("plugin should be installed under explicit config root: %v", err)
	}
}

func TestOpenCodeAdapterRejectsUnresolvedOrRelativeConfigRoot(t *testing.T) {
	for _, adapter := range []engineRuntime.OpenCodeAdapter{
		engineRuntime.NewOpenCodeAdapter("", engineRuntime.OpenCodeOptions{}),
		engineRuntime.NewOpenCodeAdapter(filepath.Join("relative", "opencode"), engineRuntime.OpenCodeOptions{}),
	} {
		if result := adapter.Install(); result.Status != core.CapabilityUnsupported || !strings.Contains(result.Message, "OpenCode config root") {
			t.Fatalf("Install() should reject unsafe root, got %#v", result)
		}
		if result := adapter.Status(); result.Status != core.CapabilityUnsupported || !strings.Contains(result.Message, "OpenCode config root") {
			t.Fatalf("Status() should reject unsafe root, got %#v", result)
		}
	}
}

func TestOpenCodeLifecycleAliasesAndStatusFailureModes(t *testing.T) {
	root := t.TempDir()
	adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{})

	if adapter.Target() != core.TargetOpenCode {
		t.Fatalf("Target() = %q, want opencode", adapter.Target())
	}
	if result := adapter.Apply(); result.Action != core.ActionInstall || result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Apply() should map to install and require restart, got %#v", result)
	}
	if result := adapter.SyncCheck(); result.Action != core.ActionSyncCheck || result.Status != core.CapabilityRestartRequired {
		t.Fatalf("SyncCheck() should report restart-required, got %#v", result)
	}
	if result := adapter.Update(); result.Action != core.ActionUpdate || result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Update() should reinstall and require restart, got %#v", result)
	}
	if result := adapter.Rollback(); result.Action != core.ActionRollback || result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Rollback() should remove plugin bridge, got %#v", result)
	}
	if result := adapter.Status(); result.Status != core.CapabilityUnsupported {
		t.Fatalf("Status() after rollback = %q, want unsupported", result.Status)
	}

	if result := adapter.Install(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}
	pluginPath := openCodePluginPath(root)
	if err := os.WriteFile(pluginPath, []byte("changed plugin source"), 0o644); err != nil {
		t.Fatalf("modify plugin: %v", err)
	}
	if result := adapter.Status(); result.Status != core.CapabilityRestartRequired || !strings.Contains(result.Message, "artifact changed") {
		t.Fatalf("Status() should report artifact hash mismatch restart requirement, got %#v", result)
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
	active := `{"active_version":"` + engineRuntime.OpenCodePluginVersion + `","active_hash":"` + engineRuntime.OpenCodePluginHash() + `","active_prompt_config_hash":"` + promptHash + `","plugin_path":"` + openCodePluginPath(root) + `","config_root":"` + root + `"}`
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
