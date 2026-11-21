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

func writeSettingsMap(t *testing.T, path string, root map[string]interface{}) {
	t.Helper()
	data, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func settingsPathOf(root string) string { return filepath.Join(root, "settings.json") }

func foreignAgentNotice(path, value string) string {
	return path + ` holds "agent": ` + value + `; it was left as it is, so the foreman main session was not enabled (set it to "foreman" or remove the key, then run install-hooks again)`
}

func installedWith(t *testing.T, edit func(root map[string]interface{})) (string, string) {
	t.Helper()
	root := t.TempDir()
	if result := newClaudeAdapter(root).Install(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}
	rootSettings := parseClaudeSettingsFile(t, settingsPathOf(root))
	edit(rootSettings)
	writeSettingsMap(t, settingsPathOf(root), rootSettings)
	return root, settingsPathOf(root)
}

func TestClaudeStatusIsPartialWhenTheMainAgentKeyIsMissing(t *testing.T) {
	root, path := installedWith(t, func(r map[string]interface{}) { delete(r, "agent") })
	status := newClaudeAdapter(root).Status()
	want := `Claude settings "agent" key is missing in ` + path + "; the foreman main session is not enabled; run 'labdrian install-hooks'"
	if status.Status != core.CapabilityPartial || status.Message != want {
		t.Errorf("Status() = %v %q, want partial %q", status.Status, status.Message, want)
	}
}

func TestClaudeStatusIsPartialAndNamesAForeignMainAgentValue(t *testing.T) {
	root, path := installedWith(t, func(r map[string]interface{}) { r["agent"] = "someone-else" })
	status := newClaudeAdapter(root).Status()
	want := `Claude settings "agent" is "someone-else" in ` + path + `, not "foreman"; the foreman main session is not enabled (set it to "foreman" or remove the key, then run 'labdrian install-hooks')`
	if status.Status != core.CapabilityPartial || status.Message != want {
		t.Errorf("Status() = %v %q, want partial %q", status.Status, status.Message, want)
	}
}

func TestClaudeInstallAndUpdateCarryTheNoticeOfAForeignMainAgentValue(t *testing.T) {
	for name, act := range map[string]func(engineRuntime.ClaudeAdapter) core.LifecycleResult{
		"install": engineRuntime.ClaudeAdapter.Install,
		"update":  engineRuntime.ClaudeAdapter.Update,
	} {
		root := t.TempDir()
		path := settingsPathOf(root)
		if err := os.WriteFile(path, []byte(`{"agent":"someone-else"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		result := act(newClaudeAdapter(root))
		if result.Status != core.CapabilityRestartRequired || !strings.Contains(result.Message, "; "+foreignAgentNotice(path, `"someone-else"`)) {
			t.Errorf("%s = %v %q, want restart_required carrying the notice", name, result.Status, result.Message)
		}
	}
}

func TestClaudeInstallSaysNothingExtraWhenTheForemanIsSet(t *testing.T) {
	root := t.TempDir()
	result := newClaudeAdapter(root).Install()
	if result.Message != "Claude lifecycle hooks installed; restart Claude to load hook changes" {
		t.Errorf("Install message = %q", result.Message)
	}
}
