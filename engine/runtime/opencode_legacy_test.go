package runtime_test

import (
	"os"
	"path/filepath"
	"testing"

	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
)

// legacyOwnedPlugin is the opening of the 1.x plugin file this adapter installed at
// plugins/labdrian-runtime-parity.js before the v2 layout.
const legacyOwnedPlugin = "import { createHash } from \"node:crypto\";\nexport default async function labdrianRuntimeParityPlugin() {\n  return {};\n}\n"

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

// A file at the legacy path that this adapter did not write is left as it is.
func TestOpenCodeLifecycleKeepsAForeignFileAtTheLegacyPath(t *testing.T) {
	const foreign = "export default async function someoneElse() { return {}; }\n"
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
