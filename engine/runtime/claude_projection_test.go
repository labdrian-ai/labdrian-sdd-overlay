package runtime_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings"
)

// projectionCommands returns the command strings of every projection entry
// under key, for the adapter's hook command.
func projectionCommands(root map[string]interface{}, key, hookCommand string) []string {
	var out []string
	hooks, _ := root["hooks"].(map[string]interface{})
	entries, _ := hooks[key].([]interface{})
	for _, e := range entries {
		if !hasHookEntryForIdentity(e, hookCommand, settings.LabdrianProjectionIdentity) {
			continue
		}
		em, _ := e.(map[string]interface{})
		inner, _ := em["hooks"].([]interface{})
		for _, ih := range inner {
			m, _ := ih.(map[string]interface{})
			if cmd, ok := m["command"].(string); ok {
				out = append(out, cmd)
			}
		}
	}
	return out
}

func writeClaudeSettings(t *testing.T, path string, root map[string]interface{}) {
	t.Helper()
	data, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestClaudeInstallWritesTheProjectionHookFamily(t *testing.T) {
	root := t.TempDir()
	adapter := engineRuntime.NewClaudeAdapter(root)
	hookCommand := filepath.Join(root, "bin", "gentle-ai-overlay")

	if result := adapter.Install(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}
	got := parseClaudeSettingsFile(t, filepath.Join(root, "settings.json"))

	ups := projectionCommands(got, "UserPromptSubmit", hookCommand)
	if len(ups) != 1 || !strings.Contains(ups[0], hookCommand+" projection hook --event UserPromptSubmit") {
		t.Errorf("UserPromptSubmit projection commands = %v", ups)
	}
	pre := projectionCommands(got, "PreToolUse", hookCommand)
	if len(pre) != 2 {
		t.Fatalf("PreToolUse projection commands = %v, want 2", pre)
	}
	for _, c := range pre {
		if !strings.Contains(c, hookCommand+" projection hook --event PreToolUse") {
			t.Errorf("PreToolUse projection command %q", c)
		}
	}
	if status := adapter.Status(); status.Status != core.CapabilitySupported {
		t.Errorf("Status() after install = %#v", status)
	}
}

func TestClaudeStatusIsPartialUntilInstallHooksIsRerunForTheProjectionFamily(t *testing.T) {
	root := t.TempDir()
	settingsPath := filepath.Join(root, "settings.json")
	hookCommand := filepath.Join(root, "bin", "gentle-ai-overlay")
	adapter := engineRuntime.NewClaudeAdapter(root)
	if result := adapter.Install(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}

	// An install made before the projection family existed.
	older := parseClaudeSettingsFile(t, settingsPath)
	for _, key := range []string{"UserPromptSubmit", "PreToolUse"} {
		older, _ = dropEntriesWithIdentity(older, key, hookCommand, settings.LabdrianProjectionIdentity)
	}
	writeClaudeSettings(t, settingsPath, older)

	status := adapter.Status()
	if status.Status != core.CapabilityPartial {
		t.Fatalf("status without the projection family = %#v, want partial", status)
	}
	if !strings.Contains(status.Message, "labdrian install-hooks") {
		t.Errorf("partial message must name the fix: %q", status.Message)
	}

	if result := adapter.Install(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("re-run Install() = %#v", result)
	}
	if status := adapter.Status(); status.Status != core.CapabilitySupported {
		t.Errorf("status after re-running install = %#v, want supported", status)
	}
}

func TestClaudeStatusIsPartialWhenAProjectionEntryDrifted(t *testing.T) {
	root := t.TempDir()
	settingsPath := filepath.Join(root, "settings.json")
	adapter := engineRuntime.NewClaudeAdapter(root)
	if result := adapter.Install(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}
	drifted := parseClaudeSettingsFile(t, settingsPath)
	for _, e := range drifted["hooks"].(map[string]interface{})["PreToolUse"].([]interface{}) {
		if em := e.(map[string]interface{}); em["matcher"] == settings.ProjectionMemoryQueryMatcher {
			em["matcher"] = "mcp__longterm-mem__query" // narrower than the gate
		}
	}
	writeClaudeSettings(t, settingsPath, drifted)

	if status := adapter.Status(); status.Status != core.CapabilityPartial {
		t.Fatalf("status with a narrowed matcher = %#v, want partial", status)
	}
	if result := adapter.Update(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Update() = %#v", result)
	}
	if status := adapter.Status(); status.Status != core.CapabilitySupported {
		t.Errorf("status after Update = %#v, want supported", status)
	}
}

func TestClaudeUninstallRemovesTheProjectionFamilyAndKeepsForeignEntries(t *testing.T) {
	root := t.TempDir()
	settingsPath := filepath.Join(root, "settings.json")
	hookCommand := filepath.Join(root, "bin", "gentle-ai-overlay")
	adapter := engineRuntime.NewClaudeAdapter(root)
	foreign := map[string]interface{}{"matcher": settings.ProjectionEditToolMatcher, "hooks": []interface{}{map[string]interface{}{"type": "command", "command": "/opt/other/edit-guard"}}}
	writeClaudeSettings(t, settingsPath, map[string]interface{}{"hooks": map[string]interface{}{"PreToolUse": []interface{}{foreign}}})

	if result := adapter.Install(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}
	if result := adapter.Uninstall(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Uninstall() = %#v", result)
	}
	after := parseClaudeSettingsFile(t, settingsPath)
	for _, key := range []string{"UserPromptSubmit", "PreToolUse"} {
		if cmds := projectionCommands(after, key, hookCommand); len(cmds) != 0 {
			t.Errorf("%s projection entries survive Uninstall: %v", key, cmds)
		}
	}
	entries, _ := after["hooks"].(map[string]interface{})["PreToolUse"].([]interface{})
	if len(entries) != 1 {
		t.Fatalf("PreToolUse after Uninstall = %v, want only the foreign entry", entries)
	}
	if a, b := mustMarshal(t, entries[0]), mustMarshal(t, foreign); a != b {
		t.Errorf("foreign entry changed: %s vs %s", a, b)
	}
}

func mustMarshal(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
