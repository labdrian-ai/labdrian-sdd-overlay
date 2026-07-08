package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	runtimepkg "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
)

// NOTE (Phase 4, PR-3): the injectDesignHookPair test-fixture workaround that
// previously lived here has been removed now that Merger.mergeHooks installs
// the real anti-generic-design pair — TestRunRuntimeCore_AllTargetsStatusAllowsCodexPartialWithoutFailing
// below exercises the real Install() path end-to-end.

func TestRunRuntimeCore_OpenCodeStatusReportsMissingPlugin(t *testing.T) {
	overlayRoot := writeMinimalismOverlayFixture(t)
	configRoot := t.TempDir()
	t.Setenv("LABDRIAN_OVERLAY_DIR", overlayRoot)

	var outBuf, errBuf bytes.Buffer
	exitCode := -1

	runRuntimeCore(
		[]string{"status", "--target", "opencode", "--config-root", configRoot},
		&outBuf,
		&errBuf,
		func(code int) { exitCode = code },
	)

	if exitCode != 1 {
		t.Fatalf("runtime status should exit 1 when plugin is absent, got %d\nstdout=%q\nstderr=%q", exitCode, outBuf.String(), errBuf.String())
	}
	if errBuf.Len() != 0 {
		t.Fatalf("runtime status should not produce stderr on normal missing-plugin status; got %q", errBuf.String())
	}
	if !strings.Contains(outBuf.String(), "OpenCode plugin not installed") {
		t.Fatalf("runtime status should report missing plugin; got %q", outBuf.String())
	}
}

func TestRunRuntimeCore_OpenCodeInstallWritesPluginAndConfigWithoutHOME(t *testing.T) {
	overlayRoot := writeMinimalismOverlayFixture(t)
	configRoot := t.TempDir()
	t.Setenv("LABDRIAN_OVERLAY_DIR", overlayRoot)

	var outBuf, errBuf bytes.Buffer
	exitCode := -1

	runRuntimeCore(
		[]string{"install", "--target", "opencode", "--config-root", configRoot},
		&outBuf,
		&errBuf,
		func(code int) { exitCode = code },
	)

	if exitCode != 0 {
		t.Fatalf("runtime install should succeed via restart_required path (exit 0), got %d\nstdout=%q\nstderr=%q", exitCode, outBuf.String(), errBuf.String())
	}

	pluginPath := filepath.Join(configRoot, "plugins", "labdrian-runtime-parity.js")
	pluginSource, err := os.ReadFile(pluginPath)
	if err != nil {
		t.Fatalf("plugin should be written at %s: %v", pluginPath, err)
	}
	if !strings.Contains(string(pluginSource), "tool.execute.before") {
		t.Fatalf("installed plugin should contain tool.execute.before hook; plugin=%q", string(pluginSource))
	}

	configPath := filepath.Join(configRoot, "labdrian-runtime-parity.json")
	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("config should be written at %s: %v", configPath, err)
	}

	var cfg map[string]any
	if err := json.Unmarshal(configData, &cfg); err != nil {
		t.Fatalf("installed config must be valid JSON: %v\n%s", err, string(configData))
	}
	if _, ok := cfg["installed_hash"].(string); !ok {
		t.Fatalf("config missing installed_hash: %#v", cfg)
	}
	if cfg["installed_hash"] != runtimepkg.OpenCodePluginHash() {
		t.Fatalf("installed_hash=%v, want %s", cfg["installed_hash"], runtimepkg.OpenCodePluginHash())
	}
	if cfg["installed_version"] != runtimepkg.OpenCodePluginVersion {
		t.Fatalf("installed_version=%v, want %s", cfg["installed_version"], runtimepkg.OpenCodePluginVersion)
	}
	if cfg["plugin_path"] != pluginPath {
		t.Fatalf("plugin_path=%v, want %s", cfg["plugin_path"], pluginPath)
	}

	promptConfig, ok := cfg["prompt_config"].(map[string]any)
	if !ok {
		t.Fatalf("config should include prompt_config object: %#v", cfg)
	}
	if promptConfig["contract_path"] != "skills/_shared/minimalism-contract.md" {
		t.Fatalf("prompt_config.contract_path = %#v", promptConfig["contract_path"])
	}
	if promptConfig["injection_point"] == "" {
		t.Fatalf("prompt_config.injection_point missing: %#v", promptConfig)
	}
	if !errBufEmpty(errBuf) {
		t.Fatalf("runtime install should not produce stderr; got %q", errBuf.String())
	}

	if !strings.Contains(outBuf.String(), "OpenCode plugin changed; restart OpenCode") {
		t.Fatalf("runtime install should return restart-required guidance; got %q", outBuf.String())
	}
}

func TestRunRuntimeCore_OpenCodeStatusRequiresRestartWhenPluginIsInstalled(t *testing.T) {
	overlayRoot := writeMinimalismOverlayFixture(t)
	configRoot := t.TempDir()
	t.Setenv("LABDRIAN_OVERLAY_DIR", overlayRoot)

	var installOut, installErr bytes.Buffer
	installExitCode := -1
	runRuntimeCore(
		[]string{"install", "--target", "opencode", "--config-root", configRoot},
		&installOut,
		&installErr,
		func(code int) { installExitCode = code },
	)
	if installExitCode != 0 {
		t.Fatalf("runtime install should exit 0 before status check, got %d\nstdout=%q\nstderr=%q", installExitCode, installOut.String(), installErr.String())
	}

	var outBuf, errBuf bytes.Buffer
	exitCode := -1
	runRuntimeCore(
		[]string{"status", "--target", "opencode", "--config-root", configRoot},
		&outBuf,
		&errBuf,
		func(code int) { exitCode = code },
	)

	if exitCode != 1 {
		t.Fatalf("runtime status should fail when OpenCode plugin is installed but inactive, got %d\nstdout=%q\nstderr=%q", exitCode, outBuf.String(), errBuf.String())
	}
	if errBuf.Len() != 0 {
		t.Fatalf("runtime status should not emit stderr for restart-required state, got %q", errBuf.String())
	}
	if !strings.Contains(outBuf.String(), "[opencode] status: restart_required") {
		t.Fatalf("runtime status should report restart_required before active marker exists, got %q", outBuf.String())
	}
	if !strings.Contains(outBuf.String(), "restart") {
		t.Fatalf("runtime status should provide restart guidance, got %q", outBuf.String())
	}

	activeMarker := filepath.Join(configRoot, "labdrian-runtime-parity.active.json")
	if _, err := os.Stat(activeMarker); !os.IsNotExist(err) {
		t.Fatalf("active marker should not exist before status is checked for activation, stat err: %v", err)
	}
}

func TestRunRuntimeCore_ClaudeUpdateAndUninstallPreserveLegacyBehavior(t *testing.T) {
	overlayRoot := writeMinimalismOverlayFixture(t)
	configRoot := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("LABDRIAN_OVERLAY_DIR", overlayRoot)

	var installOut, installErr bytes.Buffer
	installExitCode := -1
	runRuntimeCore(
		[]string{"install", "--target", "claude", "--config-root", configRoot},
		&installOut,
		&installErr,
		func(code int) { installExitCode = code },
	)
	if installExitCode != 0 {
		t.Fatalf("runtime install --target claude should succeed, got %d\nstdout=%q\nstderr=%q", installExitCode, installOut.String(), installErr.String())
	}

	var updateOut, updateErr bytes.Buffer
	updateExitCode := -1
	runRuntimeCore(
		[]string{"update", "--target", "claude", "--config-root", configRoot},
		&updateOut,
		&updateErr,
		func(code int) { updateExitCode = code },
	)
	if updateExitCode != 0 {
		t.Fatalf("runtime update --target claude should succeed, got %d\nstdout=%q\nstderr=%q", updateExitCode, updateOut.String(), updateErr.String())
	}
	if errBuf := updateErr.String(); errBuf != "" {
		t.Fatalf("runtime update --target claude should not emit stderr, got %q", errBuf)
	}
	if !strings.Contains(updateOut.String(), "[claude] update") {
		t.Fatalf("runtime update --target claude output should include claude update result, got %q", updateOut.String())
	}

	var uninstallOut, uninstallErr bytes.Buffer
	uninstallExitCode := -1
	if err := os.WriteFile(filepath.Join(configRoot, "settings.json"), []byte(`{"other":true}`), 0o644); err != nil {
		t.Fatalf("write pre-existing claude settings fixture: %v", err)
	}
	runRuntimeCore(
		[]string{"uninstall", "--target", "claude", "--config-root", configRoot},
		&uninstallOut,
		&uninstallErr,
		func(code int) { uninstallExitCode = code },
	)
	if uninstallExitCode != 0 {
		t.Fatalf("runtime uninstall --target claude should succeed, got %d\nstdout=%q\nstderr=%q", uninstallExitCode, uninstallOut.String(), uninstallErr.String())
	}
	raw, err := os.ReadFile(filepath.Join(configRoot, "settings.json"))
	if err != nil {
		t.Fatalf("expected claude settings to persist after uninstall uninstall: %v", err)
	}
	if !strings.Contains(string(raw), "\"other\"") {
		t.Fatalf("expected unrelated claude settings key to remain after uninstall, got %q", string(raw))
	}
}

func TestRunRuntimeCore_OpenCodeUpdateAndUninstallPreserveLegacyBehavior(t *testing.T) {
	overlayRoot := writeMinimalismOverlayFixture(t)
	configRoot := t.TempDir()
	t.Setenv("LABDRIAN_OVERLAY_DIR", overlayRoot)

	var installOut, installErr bytes.Buffer
	installExitCode := -1
	runRuntimeCore(
		[]string{"install", "--target", "opencode", "--config-root", configRoot},
		&installOut,
		&installErr,
		func(code int) { installExitCode = code },
	)
	if installExitCode != 0 {
		t.Fatalf("runtime install --target opencode should succeed, got %d\nstdout=%q\nstderr=%q", installExitCode, installOut.String(), installErr.String())
	}

	var updateOut, updateErr bytes.Buffer
	updateExitCode := -1
	runRuntimeCore(
		[]string{"update", "--target", "opencode", "--config-root", configRoot},
		&updateOut,
		&updateErr,
		func(code int) { updateExitCode = code },
	)
	if updateExitCode != 0 {
		t.Fatalf("runtime update --target opencode should succeed, got %d\nstdout=%q\nstderr=%q", updateExitCode, updateOut.String(), updateErr.String())
	}
	if errBuf := updateErr.String(); errBuf != "" {
		t.Fatalf("runtime update --target opencode should not emit stderr, got %q", errBuf)
	}
	if !strings.Contains(updateOut.String(), "[opencode] update") {
		t.Fatalf("runtime update --target opencode output should include opencode update result, got %q", updateOut.String())
	}

	var uninstallOut, uninstallErr bytes.Buffer
	uninstallExitCode := -1
	runRuntimeCore(
		[]string{"uninstall", "--target", "opencode", "--config-root", configRoot},
		&uninstallOut,
		&uninstallErr,
		func(code int) { uninstallExitCode = code },
	)
	if uninstallExitCode != 0 {
		t.Fatalf("runtime uninstall --target opencode should succeed, got %d\nstdout=%q\nstderr=%q", uninstallExitCode, uninstallOut.String(), uninstallErr.String())
	}
	if _, err := os.Stat(filepath.Join(configRoot, "plugins", "labdrian-runtime-parity.js")); !os.IsNotExist(err) {
		t.Fatalf("expected opencode plugin to be removed after uninstall, got stat %v", err)
	}
	if _, err := os.Stat(filepath.Join(configRoot, "labdrian-runtime-parity.json")); !os.IsNotExist(err) {
		t.Fatalf("expected opencode config to be removed after uninstall, got stat %v", err)
	}
}

func TestRunRuntimeCore_RejectsUnknownRuntimeFlags(t *testing.T) {
	overlayRoot := writeMinimalismOverlayFixture(t)
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("LABDRIAN_OVERLAY_DIR", overlayRoot)

	var outBuf, errBuf bytes.Buffer
	exitCode := -1

	runRuntimeCore(
		[]string{"status", "--config-rooot", "/tmp/ignored-root"},
		&outBuf,
		&errBuf,
		func(code int) { exitCode = code },
	)

	if exitCode != 1 {
		t.Fatalf("runtime status with typo flag should exit 1, got %d\nstdout=%q\nstderr=%q", exitCode, outBuf.String(), errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "unknown flag") {
		t.Fatalf("runtime status typo should report unknown flag, got %q", errBuf.String())
	}

	if _, err := os.Stat(filepath.Join(userHome, ".config", "opencode", "plugins", "labdrian-runtime-parity.js")); !os.IsNotExist(err) {
		t.Fatalf("default config root should not be used after parse failure; unexpected file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(userHome, ".config", "opencode", "labdrian-runtime-parity.json")); !os.IsNotExist(err) {
		t.Fatalf("default config root should not be used after parse failure; unexpected file: %v", err)
	}
}

func TestRunRuntimeCore_ReportsNonOpenCodeTargetAsLifecycleResult(t *testing.T) {
	overlayRoot := writeMinimalismOverlayFixture(t)
	configRoot := t.TempDir()
	t.Setenv("LABDRIAN_OVERLAY_DIR", overlayRoot)
	home := t.TempDir()
	t.Setenv("HOME", home)

	var outBuf, errBuf bytes.Buffer
	exitCode := -1

	runRuntimeCore(
		[]string{"status", "--target", "claude", "--config-root", configRoot},
		&outBuf,
		&errBuf,
		func(code int) { exitCode = code },
	)

	if exitCode != 1 {
		t.Fatalf("runtime status with unsupported target should exit 1, got %d\nstdout=%q\nstderr=%q", exitCode, outBuf.String(), errBuf.String())
	}
	if errBuf.Len() != 0 {
		t.Fatalf("runtime status with unsupported target should not print argument errors, got %q", errBuf.String())
	}
	if !strings.Contains(outBuf.String(), "[claude] status: unsupported") {
		t.Fatalf("runtime status should report unsupported Claude lifecycle, got %q", outBuf.String())
	}
}

func TestRunRuntimeCore_AllTargetsRunsCodexLifecycleTogether(t *testing.T) {
	overlayRoot := writeMinimalismOverlayFixture(t)
	configRoot := t.TempDir()
	t.Setenv("LABDRIAN_OVERLAY_DIR", overlayRoot)
	home := t.TempDir()
	t.Setenv("HOME", home)

	var outBuf, errBuf bytes.Buffer
	exitCode := -1

	runRuntimeCore(
		[]string{"install", "--target", "all", "--config-root", configRoot},
		&outBuf,
		&errBuf,
		func(code int) { exitCode = code },
	)

	if exitCode != 0 {
		t.Fatalf("runtime install --target all should succeed with all targets now real, got %d\nstdout=%q\nstderr=%q", exitCode, outBuf.String(), errBuf.String())
	}
	if errBuf.Len() != 0 {
		t.Fatalf("runtime install --target all should not print parse errors, got %q", errBuf.String())
	}

	out := outBuf.String()
	for _, want := range []string{"[claude] install", "[opencode] install", "[codex] install"} {
		if !strings.Contains(out, want) {
			t.Fatalf("runtime install --target all output missing %q: %q", want, out)
		}
	}
	if _, err := os.Stat(filepath.Join(configRoot, "settings.json")); err != nil {
		t.Fatalf("expected claude settings at config root %q: %v", filepath.Join(configRoot, "settings.json"), err)
	}
	if _, err := os.Stat(filepath.Join(configRoot, "plugins", "labdrian-runtime-parity.js")); err != nil {
		t.Fatalf("expected opencode plugin at config root %q: %v", filepath.Join(configRoot, "plugins", "labdrian-runtime-parity.js"), err)
	}
	if _, err := os.Stat(filepath.Join(configRoot, "labdrian-runtime-lifecycle.json")); err != nil {
		t.Fatalf("expected codex manifest at config root %q: %v", filepath.Join(configRoot, "labdrian-runtime-lifecycle.json"), err)
	}
}

func TestRunRuntimeCore_ClaudeDefaultsToHOMEWhenConfigRootNotProvided(t *testing.T) {
	overlayRoot := writeMinimalismOverlayFixture(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LABDRIAN_OVERLAY_DIR", overlayRoot)

	var outBuf, errBuf bytes.Buffer
	exitCode := -1

	runRuntimeCore(
		[]string{"install", "--target", "claude"},
		&outBuf,
		&errBuf,
		func(code int) { exitCode = code },
	)

	if exitCode != 0 {
		t.Fatalf("runtime install --target claude should succeed with default root, got %d\nstdout=%q\nstderr=%q", exitCode, outBuf.String(), errBuf.String())
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "settings.json")); err != nil {
		t.Fatalf("expected Claude settings in default HOME root, got %v", err)
	}
}

func TestRunRuntimeCore_ClaudeExplicitConfigRootIsolatedFromHOME(t *testing.T) {
	overlayRoot := writeMinimalismOverlayFixture(t)
	home := t.TempDir()
	explicitRoot := filepath.Join(t.TempDir(), "explicit")
	t.Setenv("HOME", home)
	t.Setenv("LABDRIAN_OVERLAY_DIR", overlayRoot)

	var outBuf, errBuf bytes.Buffer
	exitCode := -1

	runRuntimeCore(
		[]string{"install", "--target", "claude", "--config-root", explicitRoot},
		&outBuf,
		&errBuf,
		func(code int) { exitCode = code },
	)

	if exitCode != 0 {
		t.Fatalf("runtime install --target claude with explicit root should succeed, got %d\nstdout=%q\nstderr=%q", exitCode, outBuf.String(), errBuf.String())
	}
	if _, err := os.Stat(filepath.Join(explicitRoot, "settings.json")); err != nil {
		t.Fatalf("expected Claude settings in explicit root %q: %v", filepath.Join(explicitRoot, "settings.json"), err)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "settings.json")); !os.IsNotExist(err) {
		t.Fatalf("explicit root should avoid default HOME root settings, got stat=%v", err)
	}
}

func TestRunRuntimeCore_CodexInstallUsesCODEXHomeOrConfigRoot(t *testing.T) {
	overlayRoot := writeMinimalismOverlayFixture(t)
	codeHome := t.TempDir()
	overrideRoot := filepath.Join(t.TempDir(), "explicit-codex")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", codeHome)
	t.Setenv("LABDRIAN_OVERLAY_DIR", overlayRoot)

	var outBuf, errBuf bytes.Buffer
	exitCode := -1
	runRuntimeCore(
		[]string{"install", "--target", "codex"},
		&outBuf,
		&errBuf,
		func(code int) { exitCode = code },
	)
	if exitCode != 0 {
		t.Fatalf("runtime install --target codex with CODEX_HOME should succeed, got %d\nstdout=%q\nstderr=%q", exitCode, outBuf.String(), errBuf.String())
	}
	if _, err := os.Stat(filepath.Join(codeHome, "labdrian-runtime-lifecycle.json")); err != nil {
		t.Fatalf("expected codex manifest in CODEX_HOME %q: %v", filepath.Join(codeHome, "labdrian-runtime-lifecycle.json"), err)
	}

	t.Setenv("CODEX_HOME", "")

	var overrideOut, overrideErr bytes.Buffer
	overrideExit := -1
	runRuntimeCore(
		[]string{"install", "--target", "codex", "--config-root", overrideRoot},
		&overrideOut,
		&overrideErr,
		func(code int) { overrideExit = code },
	)
	if overrideExit != 0 {
		t.Fatalf("runtime install --target codex with --config-root should succeed, got %d\nstdout=%q\nstderr=%q", overrideExit, overrideOut.String(), overrideErr.String())
	}
	if _, err := os.Stat(filepath.Join(overrideRoot, "labdrian-runtime-lifecycle.json")); err != nil {
		t.Fatalf("expected codex manifest in explicit --config-root %q: %v", filepath.Join(overrideRoot, "labdrian-runtime-lifecycle.json"), err)
	}

	if errBuf.Len()+overrideErr.Len() != 0 {
		t.Fatalf("runtime install --target codex should not write stderr, got default=%q override=%q", errBuf.String(), overrideErr.String())
	}

	if !strings.Contains(outBuf.String(), "[codex] install") {
		t.Fatalf("runtime install output should include codex section, got %q", outBuf.String())
	}
	if !strings.Contains(overrideOut.String(), "[codex] install") {
		t.Fatalf("runtime install output with explicit root should include codex section, got %q", overrideOut.String())
	}
}

func TestRunRuntimeCore_CodexStatusWithoutManifestIsPartial(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CODEX_HOME", root)
	t.Setenv("HOME", t.TempDir())
	var outBuf, errBuf bytes.Buffer
	exitCode := -1

	runRuntimeCore(
		[]string{"status", "--target", "codex"},
		&outBuf,
		&errBuf,
		func(code int) { exitCode = code },
	)

	if exitCode != 1 {
		t.Fatalf("runtime status --target codex without manifest should exit 1, got %d\nstdout=%q\nstderr=%q", exitCode, outBuf.String(), errBuf.String())
	}
	if errBuf.Len() != 0 {
		t.Fatalf("runtime status --target codex should not emit stderr, got %q", errBuf.String())
	}
	if !strings.Contains(outBuf.String(), "[codex] status: partial") {
		t.Fatalf("runtime status --target codex should report partial, got %q", outBuf.String())
	}
}

func TestRunRuntimeCore_AllTargetsStatusAllowsCodexPartialWithoutFailing(t *testing.T) {
	overlayRoot := writeMinimalismOverlayFixture(t)
	configRoot := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LABDRIAN_OVERLAY_DIR", overlayRoot)

	var installOut, installErr bytes.Buffer
	installExit := -1
	runRuntimeCore(
		[]string{"install", "--target", "all", "--config-root", configRoot},
		&installOut,
		&installErr,
		func(code int) { installExit = code },
	)
	if installExit != 0 {
		t.Fatalf("runtime install --target all should succeed before status check, got %d\nout=%q\nerr=%q", installExit, installOut.String(), installErr.String())
	}

	// mergeHooks installs the anti-generic-design pair via the real
	// Merger.Install() path above (Phase 4, PR-3) — Claude's status is
	// "supported" without any test-fixture workaround, keeping this test
	// isolated to the codex partial-status exemption it is actually verifying.

	codeHome := configRoot
	configPath := filepath.Join(codeHome, "labdrian-runtime-parity.json")
	if err := writeOpenCodeActiveMarkerFromConfig(t, codeHome, configPath); err != nil {
		t.Fatalf("write matching OpenCode active marker: %v", err)
	}

	if err := os.Remove(filepath.Join(configRoot, "labdrian-runtime-lifecycle.json")); err != nil {
		t.Fatalf("remove codex manifest: %v", err)
	}

	var outBuf, errBuf bytes.Buffer
	exitCode := -1
	runRuntimeCore(
		[]string{"status", "--target", "all", "--config-root", configRoot},
		&outBuf,
		&errBuf,
		func(code int) { exitCode = code },
	)

	if exitCode != 0 {
		t.Fatalf("runtime status --target all should exit 0 when codex is partial and other targets remain supported, got %d\nout=%q\nerr=%q", exitCode, outBuf.String(), errBuf.String())
	}
	if !strings.Contains(outBuf.String(), "[codex] status: partial") {
		t.Fatalf("runtime status --target all should include codex partial state, got %q", outBuf.String())
	}
	if !strings.Contains(outBuf.String(), "[claude] status: supported") {
		t.Fatalf("runtime status --target all should show supported Claude status, got %q", outBuf.String())
	}
	if !strings.Contains(outBuf.String(), "[opencode] status: supported") {
		t.Fatalf("runtime status --target all should show supported OpenCode status, got %q", outBuf.String())
	}

	if errBuf.Len() != 0 {
		t.Fatalf("runtime status --target all with codex partial warning should not emit stderr, got %q", errBuf.String())
	}
}

func TestRunRuntimeCore_AllTargetsStatusFailsWhenClaudeOrOpenCodeFails(t *testing.T) {
	overlayRoot := writeMinimalismOverlayFixture(t)
	codeHome := filepath.Join(t.TempDir(), "codex-failing-home")
	if err := os.MkdirAll(codeHome, 0o755); err != nil {
		t.Fatalf("create codex home: %v", err)
	}
	if err := writeCodexManifest(t, codeHome); err != nil {
		t.Fatalf("write codex manifest: %v", err)
	}

	t.Setenv("HOME", t.TempDir())
	t.Setenv("CODEX_HOME", codeHome)
	t.Setenv("LABDRIAN_OVERLAY_DIR", overlayRoot)

	var outBuf, errBuf bytes.Buffer
	exitCode := -1
	runRuntimeCore(
		[]string{"status", "--target", "all"},
		&outBuf,
		&errBuf,
		func(code int) { exitCode = code },
	)

	if exitCode != 1 {
		t.Fatalf("runtime status --target all should fail when Claude/OpenCode status fails, got %d\nout=%q\nerr=%q", exitCode, outBuf.String(), errBuf.String())
	}
	if !strings.Contains(outBuf.String(), "[claude] status: ") {
		t.Fatalf("runtime status output should include Claude result, got %q", outBuf.String())
	}
	if !strings.Contains(outBuf.String(), "[opencode] status: ") {
		t.Fatalf("runtime status output should include OpenCode result, got %q", outBuf.String())
	}
	if !strings.Contains(outBuf.String(), "[codex] status") {
		t.Fatalf("runtime status output should include Codex result, got %q", outBuf.String())
	}
	if errBuf.Len() != 0 {
		t.Fatalf("runtime status --target all failures should remain argument-level clean, got %q", errBuf.String())
	}
}

func writeMinimalismOverlayFixture(t *testing.T) string {
	t.Helper()
	overlayRoot := t.TempDir()
	contractDir := filepath.Join(overlayRoot, "skills", "_shared")
	if err := os.MkdirAll(contractDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contractDir, "minimalism-contract.md"), []byte(testContractContent), 0o644); err != nil {
		t.Fatal(err)
	}
	return overlayRoot
}

func writeOpenCodeActiveMarkerFromConfig(t *testing.T, root, configPath string) error {
	t.Helper()

	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read OpenCode config: %w", err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("decode OpenCode config: %w", err)
	}
	promptConfigHash, ok := config["prompt_config_hash"].(string)
	if !ok || promptConfigHash == "" {
		return fmt.Errorf("prompt_config_hash missing in %q", configPath)
	}

	marker := map[string]string{
		"active_version":            runtimepkg.OpenCodePluginVersion,
		"active_hash":               runtimepkg.OpenCodePluginHash(),
		"active_prompt_config_hash": promptConfigHash,
		"plugin_path":               filepath.Join(root, "plugins", "labdrian-runtime-parity.js"),
		"config_root":               root,
	}
	markerPath := filepath.Join(root, "labdrian-runtime-parity.active.json")
	if err := os.MkdirAll(filepath.Dir(markerPath), 0o755); err != nil {
		return fmt.Errorf("create marker root: %w", err)
	}
	encoded, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal marker: %w", err)
	}
	if err := os.WriteFile(markerPath, encoded, 0o644); err != nil {
		return fmt.Errorf("write marker: %w", err)
	}
	return nil
}

func writeCodexManifest(t *testing.T, root string) error {
	t.Helper()
	payload, err := json.Marshal(map[string]string{
		"managed_by":        "labdrian-sdd-overlay",
		"installed_version": "2026-06-22-runtime-parity-3",
		"config_root":       root,
	})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, "labdrian-runtime-lifecycle.json"), payload, 0o644)
}

func errBufEmpty(buf bytes.Buffer) bool {
	return buf.Len() == 0
}
