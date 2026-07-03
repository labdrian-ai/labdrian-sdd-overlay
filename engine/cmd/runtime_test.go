package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	runtimepkg "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
)

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

func TestRunRuntimeCore_AllTargets_AdvisoryCodexWhenOtherTargetsSucceed(t *testing.T) {
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
		t.Fatalf("runtime install --target all should succeed with codex advisory, got %d\nstdout=%q\nstderr=%q", exitCode, outBuf.String(), errBuf.String())
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
	if !strings.Contains(out, "(advisory)") {
		t.Fatalf("runtime install --target all should include advisory for codex: %q", out)
	}

	if _, err := os.Stat(filepath.Join(configRoot, "settings.json")); err != nil {
		t.Fatalf("expected claude settings at config root %q: %v", filepath.Join(configRoot, "settings.json"), err)
	}
	if _, err := os.Stat(filepath.Join(configRoot, "plugins", "labdrian-runtime-parity.js")); err != nil {
		t.Fatalf("expected opencode plugin at config root %q: %v", filepath.Join(configRoot, "plugins", "labdrian-runtime-parity.js"), err)
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

func errBufEmpty(buf bytes.Buffer) bool {
	return buf.Len() == 0
}
