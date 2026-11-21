package runtime_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
)

// The OpenCode adapter installs the memorycard plugin (S11, L84) beside the runtime-parity plugin, with
// a record and an active marker of its own, and takes back only what it wrote.

func memorycardPluginFile(root string) string {
	return filepath.Join(root, "plugins", "memorycard", "index.js")
}

func memorycardRecordFile(root string) string { return filepath.Join(root, "memorycard-opencode.json") }

func memorycardMarkerFile(root string) string {
	return filepath.Join(root, "memorycard-opencode.active.json")
}

// writeMemorycardActiveMarker writes the marker the memorycard plugin writes when OpenCode loads it.
func writeMemorycardActiveMarker(t *testing.T, root string) {
	t.Helper()
	writeJSONFile(t, memorycardMarkerFile(root), map[string]string{
		"active_version": engineRuntime.OpenCodeMemorycardPluginVersion,
		"active_hash":    engineRuntime.OpenCodeMemorycardPluginHash(),
		"plugin_path":    memorycardPluginFile(root),
		"config_root":    root,
	})
}

func readRecord(t *testing.T, root string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(memorycardRecordFile(root))
	if err != nil {
		t.Fatalf("the adapter wrote no memorycard record: %v", err)
	}
	var record map[string]any
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("memorycard record is not JSON: %v\n%s", err, data)
	}
	return record
}

func installedMemorycard(t *testing.T) (string, engineRuntime.OpenCodeAdapter, core.LifecycleResult) {
	t.Helper()
	root := t.TempDir()
	adapter, result := installIn(t, root, engineRuntime.OpenCodeOptions{OverlayDir: shippedOverlay(t), PathLookup: fakeLookup(map[string]string{"memorycard": "/opt/bin/memorycard"})})
	return root, adapter, result
}

func TestOpenCodeInstallWritesTheMemorycardPluginAndItsRecord(t *testing.T) {
	root, _, result := installedMemorycard(t)
	if result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}
	plugin, err := os.ReadFile(memorycardPluginFile(root))
	if err != nil {
		t.Fatalf("the memorycard plugin is not at %s: %v", memorycardPluginFile(root), err)
	}
	source, err := os.ReadFile("memorycard-opencode-plugin.mjs")
	if err != nil {
		t.Fatal(err)
	}
	if string(plugin) != string(source) {
		t.Error("the installed memorycard plugin is not the repository source")
	}
	record := readRecord(t, root)
	want := map[string]any{
		"installed_version":  engineRuntime.OpenCodeMemorycardPluginVersion,
		"installed_hash":     engineRuntime.OpenCodeMemorycardPluginHash(),
		"plugin_path":        memorycardPluginFile(root),
		"plugin_config_root": root,
		"memorycard_command": "/opt/bin/memorycard",
	}
	for key, value := range want {
		if record[key] != value {
			t.Errorf("record[%s] = %v, want %v", key, record[key], value)
		}
	}
	if !strings.Contains(result.Message, "memorycard plugin") || !strings.Contains(result.Message, engineRuntime.OpenCodeMemorycardPluginVersion) {
		t.Errorf("message = %q, want it to name the memorycard plugin and its version", result.Message)
	}
	if _, err := os.Stat(openCodePluginPath(root)); err != nil {
		t.Errorf("the runtime-parity plugin is gone: %v", err)
	}
}

func TestOpenCodeRecordsTheBareMemorycardNameWhenItIsNotOnThePath(t *testing.T) {
	root := t.TempDir()
	installIn(t, root, engineRuntime.OpenCodeOptions{OverlayDir: shippedOverlay(t), PathLookup: fakeLookup(nil)})
	if got := readRecord(t, root)["memorycard_command"]; got != "memorycard" {
		t.Errorf("memorycard_command = %v, want the bare name", got)
	}
	root = t.TempDir()
	installIn(t, root, engineRuntime.OpenCodeOptions{OverlayDir: shippedOverlay(t)})
	if got := readRecord(t, root)["memorycard_command"]; got != "memorycard" {
		t.Errorf("with no lookup, memorycard_command = %v, want the bare name", got)
	}
}

func TestOpenCodeStatusIsSupportedOnlyWhenBothPluginsAreActive(t *testing.T) {
	root, adapter, _ := installedMemorycard(t)
	writeParityActiveMarker(t, root)
	if status := adapter.Status(); status.Status != core.CapabilityRestartRequired || !strings.Contains(status.Message, "memorycard plugin") {
		t.Errorf("Status() = %#v, want a restart owed for the memorycard plugin, which has not loaded", status)
	}
	writeMemorycardActiveMarker(t, root)
	if status := adapter.Status(); status.Status != core.CapabilitySupported {
		t.Errorf("Status() = %#v, want supported with both markers", status)
	}
}

func TestOpenCodeStatusSaysARestartIsOwedWhenTheMemorycardPluginRunningIsNotTheInstalledOne(t *testing.T) {
	root, adapter, _ := installedMemorycard(t)
	writeParityActiveMarker(t, root)
	writeJSONFile(t, memorycardMarkerFile(root), map[string]string{
		"active_version": "an-older-version",
		"active_hash":    engineRuntime.OpenCodeMemorycardPluginHash(),
		"plugin_path":    memorycardPluginFile(root),
		"config_root":    root,
	})
	if status := adapter.Status(); status.Status != core.CapabilityRestartRequired || !strings.Contains(status.Message, "memorycard plugin") {
		t.Errorf("Status() = %#v, want a restart owed for the memorycard plugin", status)
	}
}

func TestOpenCodeStatusReportsAMemorycardPluginThatIsMissingStaleOrChanged(t *testing.T) {
	cases := map[string]struct {
		damage func(t *testing.T, root string)
		status core.CapabilityStatus
		want   string
	}{
		"plugin missing": {func(t *testing.T, root string) { remove(t, memorycardPluginFile(root)) }, core.CapabilityPartial, "memorycard plugin not installed"},
		"record missing": {func(t *testing.T, root string) { remove(t, memorycardRecordFile(root)) }, core.CapabilityPartial, "memorycard plugin record"},
		"record stale": {func(t *testing.T, root string) {
			writeJSONFile(t, memorycardRecordFile(root), map[string]string{"installed_version": "an-older-version", "installed_hash": "x"})
		}, core.CapabilityPartial, "memorycard plugin record"},
		"plugin changed": {func(t *testing.T, root string) {
			if err := os.WriteFile(memorycardPluginFile(root), []byte("// labdrian-memorycard-plugin: edited\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}, core.CapabilityRestartRequired, "memorycard plugin artifact changed"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			root, adapter, _ := installedMemorycard(t)
			writeParityActiveMarker(t, root)
			writeMemorycardActiveMarker(t, root)
			c.damage(t, root)
			status := adapter.Status()
			if status.Status != c.status || !strings.Contains(status.Message, c.want) {
				t.Errorf("Status() = %#v, want %s naming %q", status, c.status, c.want)
			}
		})
	}
}

func remove(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
}

func TestOpenCodeUpdateRewritesAChangedMemorycardPluginAndTheProgramItRuns(t *testing.T) {
	root, _, _ := installedMemorycard(t)
	if err := os.WriteFile(memorycardPluginFile(root), []byte("// labdrian-memorycard-plugin: edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	moved := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{OverlayDir: shippedOverlay(t), PathLookup: fakeLookup(map[string]string{"memorycard": "/usr/local/bin/memorycard"})})
	if result := moved.Update(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Update() = %#v", result)
	}
	source, _ := os.ReadFile("memorycard-opencode-plugin.mjs")
	if got, _ := os.ReadFile(memorycardPluginFile(root)); string(got) != string(source) {
		t.Error("update did not put the repository source back")
	}
	if got := readRecord(t, root)["memorycard_command"]; got != "/usr/local/bin/memorycard" {
		t.Errorf("memorycard_command = %v, want the path found by the update", got)
	}
}

func TestOpenCodeUninstallRemovesTheMemorycardPluginAndRecordAndNothingElseInItsDirectory(t *testing.T) {
	root, adapter, _ := installedMemorycard(t)
	writeMemorycardActiveMarker(t, root)
	foreign := filepath.Join(root, "plugins", "memorycard", "notes.txt")
	if err := os.WriteFile(foreign, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := adapter.Uninstall()
	for _, gone := range []string{memorycardPluginFile(root), memorycardRecordFile(root)} {
		if _, err := os.Stat(gone); !os.IsNotExist(err) {
			t.Errorf("%s was not removed: %v", gone, err)
		}
	}
	if got, err := os.ReadFile(foreign); err != nil || string(got) != "mine" {
		t.Errorf("a file the adapter did not write in the plugin directory changed: %q, %v", got, err)
	}
	if _, err := os.Stat(memorycardMarkerFile(root)); err != nil {
		t.Errorf("the active marker went: it stays until OpenCode is restarted: %v", err)
	}
	if !strings.Contains(result.Message, memorycardMarkerFile(root)) {
		t.Errorf("message = %q, want it to name the memorycard active marker that remains", result.Message)
	}

	// With nothing foreign left in it, the directory goes too.
	root2, adapter2, _ := installedMemorycard(t)
	adapter2.Uninstall()
	if _, err := os.Stat(filepath.Dir(memorycardPluginFile(root2))); !os.IsNotExist(err) {
		t.Errorf("the empty memorycard plugin directory stayed: %v", err)
	}
}

func TestOpenCodeLeavesAMemorycardPluginItDidNotWriteAndSaysSo(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(memorycardPluginFile(root)), 0o755); err != nil {
		t.Fatal(err)
	}
	const foreign = "export default { id: 'memorycard', setup() {} };\n"
	if err := os.WriteFile(memorycardPluginFile(root), []byte(foreign), 0o644); err != nil {
		t.Fatal(err)
	}
	adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{OverlayDir: shippedOverlay(t)})

	install := adapter.Install()
	if install.Status != core.CapabilityPartial || !strings.Contains(install.Message, memorycardPluginFile(root)) || !strings.Contains(install.Message, "not written by this adapter") {
		t.Errorf("Install() = %#v, want partial naming the foreign file", install)
	}
	if got, _ := os.ReadFile(memorycardPluginFile(root)); string(got) != foreign {
		t.Errorf("the foreign plugin was overwritten: %q", got)
	}
	if _, err := os.Stat(memorycardRecordFile(root)); !os.IsNotExist(err) {
		t.Errorf("a record was written for a plugin the adapter did not install: %v", err)
	}
	if _, err := os.Stat(openCodePluginPath(root)); err != nil {
		t.Errorf("the runtime-parity plugin was not installed beside it: %v", err)
	}
	writeParityActiveMarker(t, root)
	if status := adapter.Status(); status.Status != core.CapabilityPartial || !strings.Contains(status.Message, "not written by this adapter") {
		t.Errorf("Status() = %#v, want partial", status)
	}
	uninstall := adapter.Uninstall()
	if got, err := os.ReadFile(memorycardPluginFile(root)); err != nil || string(got) != foreign {
		t.Errorf("uninstall removed or changed the foreign plugin: %q, %v", got, err)
	}
	if !strings.Contains(uninstall.Message, memorycardPluginFile(root)+" was not written by this adapter and was left in place") || strings.Contains(uninstall.Message, "memorycard plugin's active marker") {
		t.Errorf("Uninstall() = %q, want it to say the foreign file was left in place and name no memorycard marker, since none of ours was removed", uninstall.Message)
	}
}

// The mark is in the first line, as the plugin header says: a foreign file that quotes it further down is not ours.
func TestOpenCodeDoesNotTakeAForeignFileThatQuotesTheMarkInItsBodyForItsOwn(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(memorycardPluginFile(root)), 0o755); err != nil {
		t.Fatal(err)
	}
	foreign := "export default { id: 'memorycard' };\n// not " + "labdrian-memorycard-plugin" + ", but it quotes it\n"
	if err := os.WriteFile(memorycardPluginFile(root), []byte(foreign), 0o644); err != nil {
		t.Fatal(err)
	}
	adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{OverlayDir: shippedOverlay(t)})
	if install := adapter.Install(); install.Status != core.CapabilityPartial || !strings.Contains(install.Message, "not written by this adapter") {
		t.Errorf("Install() = %#v, want partial: the file is foreign", install)
	}
	adapter.Uninstall()
	if got, _ := os.ReadFile(memorycardPluginFile(root)); string(got) != foreign {
		t.Errorf("the foreign file was changed or removed: %q", got)
	}
}

func TestOpenCodeSaysTheMemorycardPluginIsAlreadyCurrentWhenItsBytesMatch(t *testing.T) {
	_, adapter, first := installedMemorycard(t)
	if !strings.Contains(first.Message, "memorycard plugin written to") {
		t.Fatalf("first install = %q, want the plugin written", first.Message)
	}
	second := adapter.Install()
	if !strings.Contains(second.Message, "memorycard plugin already current in") || strings.Contains(second.Message, "memorycard plugin written to") {
		t.Errorf("second install = %q, want the plugin already current", second.Message)
	}
}

// The memorycard checks come after the runtime-parity marker checks: a parity restart owed is said first.
func TestOpenCodeStatusSaysTheParityRestartBeforeAnythingAboutTheMemorycardPlugin(t *testing.T) {
	root, adapter, _ := installedMemorycard(t)
	remove(t, memorycardPluginFile(root))
	status := adapter.Status()
	if status.Status != core.CapabilityRestartRequired || !strings.Contains(status.Message, "restart required to load plugin version") || strings.Contains(status.Message, "memorycard") {
		t.Errorf("Status() = %#v, want the parity restart first", status)
	}
}
