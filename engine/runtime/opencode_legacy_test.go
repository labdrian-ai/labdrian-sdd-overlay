package runtime_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
)

// legacyOwnedPlugin is the 1.x plugin file this adapter installed at plugins/labdrian-runtime-parity.js
// before the v2 layout: the bytes of engine/runtime/labdrian-runtime-parity-plugin.mjs at 16825fd3.
var legacyOwnedPlugin = func() string {
	data, err := os.ReadFile(filepath.Join("testdata", "legacy-runtime-parity-plugin-1x.mjs"))
	if err != nil {
		panic(err)
	}
	return string(data)
}()

func legacyPluginPath(root string) string {
	return filepath.Join(root, "plugins", "labdrian-runtime-parity.js")
}

func writeLegacyPlugin(t *testing.T, root, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "plugins"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyPluginPath(root), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// The 1.x file this adapter wrote is removed by install, update and uninstall: OpenCode v2 would
// fail to load it beside the v2 plugin.
func TestOpenCodeLifecycleRemovesTheLegacyPluginItInstalled(t *testing.T) {
	for name, step := range map[string]func(engineRuntime.OpenCodeAdapter) core.LifecycleResult{
		"install":   engineRuntime.OpenCodeAdapter.Install,
		"update":    engineRuntime.OpenCodeAdapter.Update,
		"uninstall": engineRuntime.OpenCodeAdapter.Uninstall,
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeLegacyPlugin(t, root, legacyOwnedPlugin)
			adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{})

			if result := step(adapter); result.Status != core.CapabilityRestartRequired {
				t.Fatalf("%s = %#v", name, result)
			}
			if _, err := os.Stat(legacyPluginPath(root)); !os.IsNotExist(err) {
				t.Errorf("the legacy plugin is still there after %s: %v", name, err)
			}
		})
	}
}

// A file at the legacy path that this adapter did not write is left as it is, even when it quotes
// the 1.x export.
func TestOpenCodeLifecycleKeepsAForeignFileAtTheLegacyPath(t *testing.T) {
	const foreign = "// copied from export default async function labdrianRuntimeParityPlugin(\nexport default async function someoneElse() { return {}; }\n"
	for name, step := range map[string]func(engineRuntime.OpenCodeAdapter) core.LifecycleResult{
		"install":   engineRuntime.OpenCodeAdapter.Install,
		"uninstall": engineRuntime.OpenCodeAdapter.Uninstall,
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeLegacyPlugin(t, root, foreign)
			adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{})

			step(adapter)
			data, err := os.ReadFile(legacyPluginPath(root))
			if err != nil || string(data) != foreign {
				t.Errorf("after %s the foreign file holds %q, %v; want it unchanged", name, data, err)
			}
		})
	}
}

// A legacy path that cannot be cleaned (here a directory) does not hide that the v2 plugin was
// written and OpenCode needs a restart; the message names the path.
func TestOpenCodeInstallKeepsRestartRequiredWhenTheLegacyPathCannotBeCleaned(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(legacyPluginPath(root), 0o755); err != nil {
		t.Fatal(err)
	}

	result := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{}).Install()

	if result.Status != core.CapabilityRestartRequired || !strings.Contains(result.Message, legacyPluginPath(root)) {
		t.Errorf("Install() = %q %q, want restart_required naming %s", result.Status, result.Message, legacyPluginPath(root))
	}
	if _, err := os.Stat(openCodePluginPath(root)); err != nil {
		t.Errorf("the v2 plugin was not written: %v", err)
	}
}

// Status names a file left at the legacy path, ours or not, instead of reporting the plugin active.
func TestOpenCodeStatusReportsAFileLeftAtTheLegacyPath(t *testing.T) {
	root := t.TempDir()
	adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{})
	if result := adapter.Install(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}
	writeLegacyPlugin(t, root, "export default async function someoneElse() {}\n")

	result := adapter.Status()

	if result.Status != core.CapabilityPartial || !strings.Contains(result.Message, legacyPluginPath(root)) {
		t.Errorf("Status() = %q %q, want partial naming %s", result.Status, result.Message, legacyPluginPath(root))
	}
}

// Uninstall leaves a plugin directory that holds a file it did not write, with that file.
func TestOpenCodeUninstallKeepsAForeignFileInThePluginDirectory(t *testing.T) {
	root := t.TempDir()
	adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{})
	if result := adapter.Install(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}
	extra := filepath.Join(filepath.Dir(openCodePluginPath(root)), "extra.js")
	if err := os.WriteFile(extra, []byte("// someone else's\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	adapter.Uninstall()

	if data, err := os.ReadFile(extra); err != nil || string(data) != "// someone else's\n" {
		t.Errorf("the foreign file holds %q, %v; want it kept", data, err)
	}
	if _, err := os.Stat(openCodePluginPath(root)); !os.IsNotExist(err) {
		t.Errorf("index.js is still there: %v", err)
	}
}

// An empty plugin directory left without its index.js is removed by uninstall too.
func TestOpenCodeUninstallRemovesAnEmptyPluginDirectory(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Dir(openCodePluginPath(root))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	result := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{}).Uninstall()

	if result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Uninstall() = %#v", result)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("the empty plugin directory is still there: %v", err)
	}
}
