package runtime_test

import (
	"path/filepath"
	"strings"
	"testing"

	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings"
)

// approveGuardCommands returns the command strings of every skills approve guard
// entry under PreToolUse, keyed by matcher, for the adapter's hook command.
func approveGuardCommands(root map[string]interface{}, hookCommand string) map[string]string {
	out := map[string]string{}
	hooks, _ := root["hooks"].(map[string]interface{})
	entries, _ := hooks["PreToolUse"].([]interface{})
	for _, e := range entries {
		if !hasHookEntryForIdentity(e, hookCommand, settings.LabdrianApproveGuardIdentity) {
			continue
		}
		em, _ := e.(map[string]interface{})
		matcher, _ := em["matcher"].(string)
		inner, _ := em["hooks"].([]interface{})
		ih, _ := inner[0].(map[string]interface{})
		out[matcher], _ = ih["command"].(string)
	}
	return out
}

func TestClaudeInstallWritesTheApproveGuard(t *testing.T) {
	root := t.TempDir()
	adapter := engineRuntime.NewClaudeAdapter(root)
	hookCommand := filepath.Join(root, "bin", "gentle-ai-overlay")

	if result := adapter.Install(); result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}
	got := approveGuardCommands(parseClaudeSettingsFile(t, filepath.Join(root, "settings.json")), hookCommand)
	if len(got) != 2 {
		t.Fatalf("approve guard entries = %v, want one for Bash and one for the file tools", got)
	}
	for _, matcher := range []string{settings.ApproveGuardBashMatcher, settings.ApproveGuardFileToolMatcher} {
		if c := got[matcher]; !strings.Contains(c, hookCommand+" skills guard-hook") {
			t.Errorf("matcher %q command = %q, want it to run 'skills guard-hook'", matcher, c)
		}
	}
	if status := adapter.Status(); status.Status != engineRuntime.CapabilitySupported {
		t.Errorf("Status() after install = %#v", status)
	}
}

func TestClaudeStatusIsPartialUntilInstallHooksIsRerunForTheApproveGuard(t *testing.T) {
	root := t.TempDir()
	settingsPath := filepath.Join(root, "settings.json")
	hookCommand := filepath.Join(root, "bin", "gentle-ai-overlay")
	adapter := engineRuntime.NewClaudeAdapter(root)
	if result := adapter.Install(); result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}

	// An install made before the approve guard existed.
	older, _ := dropEntriesWithIdentity(parseClaudeSettingsFile(t, settingsPath), "PreToolUse", hookCommand, settings.LabdrianApproveGuardIdentity)
	writeClaudeSettings(t, settingsPath, older)

	status := adapter.Status()
	if status.Status != engineRuntime.CapabilityPartial {
		t.Fatalf("status without the approve guard = %#v, want partial", status)
	}
	if !strings.Contains(status.Message, "labdrian install-hooks") {
		t.Errorf("partial message must name the fix: %q", status.Message)
	}

	if result := adapter.Install(); result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("re-run Install() = %#v", result)
	}
	if status := adapter.Status(); status.Status != engineRuntime.CapabilitySupported {
		t.Errorf("status after re-running install = %#v, want supported", status)
	}
}

func TestClaudeStatusIsPartialWhenAnApproveGuardEntryDrifted(t *testing.T) {
	root := t.TempDir()
	settingsPath := filepath.Join(root, "settings.json")
	adapter := engineRuntime.NewClaudeAdapter(root)
	if result := adapter.Install(); result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}
	drifted := parseClaudeSettingsFile(t, settingsPath)
	for _, e := range drifted["hooks"].(map[string]interface{})["PreToolUse"].([]interface{}) {
		if em := e.(map[string]interface{}); em["matcher"] == settings.ApproveGuardFileToolMatcher &&
			strings.Contains(mustMarshal(t, e), settings.LabdrianApproveGuardIdentity) {
			em["matcher"] = "Write" // narrower than the guard
		}
	}
	writeClaudeSettings(t, settingsPath, drifted)

	if status := adapter.Status(); status.Status != engineRuntime.CapabilityPartial {
		t.Fatalf("status with a narrowed matcher = %#v, want partial", status)
	}
	if result := adapter.Update(); result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Update() = %#v", result)
	}
	if status := adapter.Status(); status.Status != engineRuntime.CapabilitySupported {
		t.Errorf("status after Update = %#v, want supported", status)
	}
}

func TestClaudeUninstallRemovesTheApproveGuardAndKeepsForeignEntries(t *testing.T) {
	root := t.TempDir()
	settingsPath := filepath.Join(root, "settings.json")
	hookCommand := filepath.Join(root, "bin", "gentle-ai-overlay")
	adapter := engineRuntime.NewClaudeAdapter(root)
	foreign := map[string]interface{}{"matcher": settings.ApproveGuardBashMatcher, "hooks": []interface{}{map[string]interface{}{"type": "command", "command": "/opt/other/bash-guard"}}}
	writeClaudeSettings(t, settingsPath, map[string]interface{}{"hooks": map[string]interface{}{"PreToolUse": []interface{}{foreign}}})

	if result := adapter.Install(); result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}
	if result := adapter.Uninstall(); result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Uninstall() = %#v", result)
	}
	after := parseClaudeSettingsFile(t, settingsPath)
	if got := approveGuardCommands(after, hookCommand); len(got) != 0 {
		t.Errorf("approve guard entries survive Uninstall: %v", got)
	}
	entries, _ := after["hooks"].(map[string]interface{})["PreToolUse"].([]interface{})
	if len(entries) != 1 || mustMarshal(t, entries[0]) != mustMarshal(t, foreign) {
		t.Errorf("PreToolUse after Uninstall = %v, want only the foreign entry", entries)
	}
}
