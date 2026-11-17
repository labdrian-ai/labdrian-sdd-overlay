package runtime_test

import (
	"os"
	"strings"
	"testing"

	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
)

// The Claude adapter keeps the top-level "outputStyle" key of settings.json (T27 commit b, L57's
// ownership rules): install and update write "labdrian", uninstall removes the key only while it holds
// exactly that, a foreign value such as "Gentleman" is kept and announced, and status reports the key.

func foreignStyleNotice(path, value string) string {
	return path + ` holds "outputStyle": ` + value + `; it was left as it is, so the labdrian output style was not enabled (set it to "labdrian" or remove the key, then run install-hooks again)`
}

func TestClaudeInstallAndUpdateWriteTheOutputStyleKey(t *testing.T) {
	for name, act := range map[string]func(engineRuntime.ClaudeAdapter) core.LifecycleResult{
		"install": engineRuntime.ClaudeAdapter.Install,
		"update":  engineRuntime.ClaudeAdapter.Update,
	} {
		root := t.TempDir()
		result := act(newClaudeAdapter(root))
		if result.Status != core.CapabilityRestartRequired {
			t.Fatalf("%s = %#v", name, result)
		}
		if got := parseClaudeSettingsFile(t, settingsPathOf(root))["outputStyle"]; got != "labdrian" {
			t.Errorf(`%s: outputStyle = %v, want "labdrian"`, name, got)
		}
	}
}

func TestClaudeInstallAndUpdateKeepAForeignOutputStyleAndSayIt(t *testing.T) {
	for name, act := range map[string]func(engineRuntime.ClaudeAdapter) core.LifecycleResult{
		"install": engineRuntime.ClaudeAdapter.Install,
		"update":  engineRuntime.ClaudeAdapter.Update,
	} {
		root := t.TempDir()
		path := settingsPathOf(root)
		if err := os.WriteFile(path, []byte(`{"outputStyle":"Gentleman"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		result := act(newClaudeAdapter(root))
		if result.Status != core.CapabilityRestartRequired || !strings.Contains(result.Message, "; "+foreignStyleNotice(path, `"Gentleman"`)) {
			t.Errorf("%s = %v %q, want restart_required carrying the notice", name, result.Status, result.Message)
		}
		if got := parseClaudeSettingsFile(t, path)["outputStyle"]; got != "Gentleman" {
			t.Errorf("%s: outputStyle = %v, want the foreign value kept", name, got)
		}
	}
}

// A foreign agent and a foreign output style are two notices, the agent first.
func TestClaudeInstallSaysBothNoticesWhenBothKeysAreForeign(t *testing.T) {
	root := t.TempDir()
	path := settingsPathOf(root)
	if err := os.WriteFile(path, []byte(`{"agent":"someone-else","outputStyle":"Gentleman"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	want := "Claude lifecycle hooks installed; restart Claude to load hook changes; " + foreignAgentNotice(path, `"someone-else"`) + "; " + foreignStyleNotice(path, `"Gentleman"`)
	if got := newClaudeAdapter(root).Install().Message; got != want {
		t.Errorf("Install message = %q, want %q", got, want)
	}
}

func TestClaudeUninstallRemovesTheOutputStyleKeyOnlyWhenItIsLabdrian(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
		kept  bool
	}{{"owned", "labdrian", false}, {"foreign", "Gentleman", true}} {
		root, path := installedWith(t, func(r map[string]interface{}) { r["outputStyle"] = tc.value })
		if result := newClaudeAdapter(root).Uninstall(); result.Status != core.CapabilityRestartRequired {
			t.Fatalf("%s: Uninstall() = %#v", tc.name, result)
		}
		got, present := parseClaudeSettingsFile(t, path)["outputStyle"]
		if present != tc.kept || (tc.kept && got != tc.value) {
			t.Errorf("%s: outputStyle = %v, %v; want kept=%v with %q", tc.name, got, present, tc.kept, tc.value)
		}
	}
}

func TestClaudeStatusIsPartialWhenTheOutputStyleKeyIsMissing(t *testing.T) {
	root, path := installedWith(t, func(r map[string]interface{}) { delete(r, "outputStyle") })
	status := newClaudeAdapter(root).Status()
	want := `Claude settings "outputStyle" key is missing in ` + path + "; the labdrian output style is not enabled; run 'labdrian install-hooks'"
	if status.Status != core.CapabilityPartial || status.Message != want {
		t.Errorf("Status() = %v %q, want partial %q", status.Status, status.Message, want)
	}
}

func TestClaudeStatusIsPartialAndNamesAForeignOutputStyleValue(t *testing.T) {
	root, path := installedWith(t, func(r map[string]interface{}) { r["outputStyle"] = "Gentleman" })
	status := newClaudeAdapter(root).Status()
	want := `Claude settings "outputStyle" is "Gentleman" in ` + path + `, not "labdrian"; the labdrian output style is not enabled (set it to "labdrian" or remove the key, then run 'labdrian install-hooks')`
	if status.Status != core.CapabilityPartial || status.Message != want {
		t.Errorf("Status() = %v %q, want partial %q", status.Status, status.Message, want)
	}
}

// The agent key is reported first: a settings file with both wrong says the main agent, as it did.
func TestClaudeStatusReportsTheMainAgentBeforeTheOutputStyle(t *testing.T) {
	root, _ := installedWith(t, func(r map[string]interface{}) {
		delete(r, "agent")
		delete(r, "outputStyle")
	})
	if got := newClaudeAdapter(root).Status().Message; !strings.Contains(got, `"agent" key is missing`) {
		t.Errorf("Status message = %q, want the agent key reported first", got)
	}
}

func TestClaudeStatusIsSupportedWhenBothKeysAreOurs(t *testing.T) {
	root := t.TempDir()
	newClaudeAdapter(root).Install()
	if status := newClaudeAdapter(root).Status(); status.Status != core.CapabilitySupported {
		t.Errorf("Status() = %v %q, want supported", status.Status, status.Message)
	}
}
