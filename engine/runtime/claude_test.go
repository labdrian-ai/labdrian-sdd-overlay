package runtime_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings"
)

func TestClaudeInstallWritesLifecycleHooksAndReportsSupportedStatus(t *testing.T) {
	root := t.TempDir()
	adapter := engineRuntime.NewClaudeAdapter(root)

	if result := adapter.Install(); result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}

	// mergeHooks does not install the anti-generic-design pair yet (Phase 4 —
	// a later PR in the anti-generic-design-runtime-wiring chain). Inject it
	// by hand so this test can still verify the minimalism+safety install
	// path reports "supported" once HasSupportedClaudeLifecycleState also
	// requires the design pair.
	settingsPath := filepath.Join(root, "settings.json")
	hookCommand := filepath.Join(root, "bin", "gentle-ai-overlay")
	injectDesignHookPair(t, settingsPath, hookCommand)

	status := adapter.Status()
	if status.Status != engineRuntime.CapabilitySupported {
		t.Fatalf("Status() after install = %#v", status)
	}
	if !strings.Contains(status.Message, "installed and owned") {
		t.Fatalf("status message should report installed and owned state, got %q", status.Message)
	}

	rootSettings := parseClaudeSettingsFile(t, settingsPath)
	if !hasOwnedClaudeHook(rootSettings, "UserPromptSubmit", hookCommand, settings.LabdrianMinimalismIdentity) {
		t.Fatalf("settings after install should include minimalism UserPromptSubmit hook: %#v", rootSettings["hooks"])
	}
	if !hasOwnedClaudeHook(rootSettings, "UserPromptSubmit", hookCommand, settings.LabdrianSafetyIdentity) {
		t.Fatalf("settings after install should include safety UserPromptSubmit hook: %#v", rootSettings["hooks"])
	}
	if !hasOwnedClaudeHook(rootSettings, "PreToolUse", hookCommand, settings.LabdrianMinimalismIdentity) {
		t.Fatalf("settings after install should include minimalism PreToolUse hook: %#v", rootSettings["hooks"])
	}
	if !hasOwnedClaudeHook(rootSettings, "PreToolUse", hookCommand, settings.LabdrianSafetyIdentity) {
		t.Fatalf("settings after install should include safety PreToolUse hook: %#v", rootSettings["hooks"])
	}
}

func TestClaudeUpdateRefreshesLifecycleAndKeepsSupportedStatus(t *testing.T) {
	root := t.TempDir()
	adapter := engineRuntime.NewClaudeAdapter(root)

	if result := adapter.Install(); result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}

	if result := adapter.Update(); result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Update() = %#v", result)
	}

	// mergeHooks does not install the anti-generic-design pair yet (Phase 4 —
	// a later PR in the chain). Inject it by hand so this test can still
	// verify Update() keeps the (minimalism+safety+design) lifecycle state
	// "supported".
	settingsPath := filepath.Join(root, "settings.json")
	hookCommand := filepath.Join(root, "bin", "gentle-ai-overlay")
	injectDesignHookPair(t, settingsPath, hookCommand)

	status := adapter.Status()
	if status.Status != engineRuntime.CapabilitySupported {
		t.Fatalf("Status() after update should remain supported, got %#v", status)
	}
}

func TestClaudeUninstallRemovesOwnedHooksAndReturnsUnhealthyStatus(t *testing.T) {
	root := t.TempDir()
	adapter := engineRuntime.NewClaudeAdapter(root)
	if result := adapter.Install(); result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}

	if result := adapter.Uninstall(); result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Uninstall() = %#v", result)
	}

	status := adapter.Status()
	if status.Status != engineRuntime.CapabilityUnsupported && status.Status != engineRuntime.CapabilityPartial {
		t.Fatalf("Status() after uninstall = %#v", status)
	}

	settingsPath := filepath.Join(root, "settings.json")
	rootSettings := parseClaudeSettingsFile(t, settingsPath)
	hookCommand := filepath.Join(root, "bin", "gentle-ai-overlay")
	if hasAnyOwnedClaudeHooks(rootSettings, "UserPromptSubmit", hookCommand) || hasAnyOwnedClaudeHooks(rootSettings, "PreToolUse", hookCommand) {
		t.Fatalf("owned hooks should be removed after uninstall, got %#v", rootSettings["hooks"])
	}
}

func TestClaudeDefaultRootUsesHOMEWhenRootNotProvided(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	adapter := engineRuntime.NewClaudeAdapter("")

	if result := adapter.Install(); result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}

	if _, err := os.Stat(filepath.Join(home, ".claude", "settings.json")); err != nil {
		t.Fatalf("expected settings at default root %q, stat: %v", filepath.Join(home, ".claude", "settings.json"), err)
	}
}

func TestClaudeExplicitRootIsolatedFromDefaultHOME(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	explicitRoot := filepath.Join(t.TempDir(), "explicit-claude-root")
	adapter := engineRuntime.NewClaudeAdapter(explicitRoot)

	if result := adapter.Install(); result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}

	if _, err := os.Stat(filepath.Join(explicitRoot, "settings.json")); err != nil {
		t.Fatalf("expected settings at explicit root %q, stat: %v", explicitRoot, err)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "settings.json")); !os.IsNotExist(err) {
		t.Fatalf("explicit root should avoid default root, stat err: %v", err)
	}
}

func TestClaudeStatusRequiresFullLifecycleState(t *testing.T) {
	root := t.TempDir()
	settingsPath := filepath.Join(root, "settings.json")
	hookCommand := filepath.Join(root, "bin", "gentle-ai-overlay")
	adapter := engineRuntime.NewClaudeAdapter(root)

	if result := adapter.Install(); result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}

	rootSettings := parseClaudeSettingsFile(t, settingsPath)
	hooks, ok := rootSettings["hooks"].(map[string]interface{})
	if !ok {
		t.Fatal("settings hooks should be a map")
	}
	for _, key := range []string{"UserPromptSubmit", "PreToolUse"} {
		rootSettings, _ = dropEntriesWithIdentity(rootSettings, key, hookCommand, settings.LabdrianSafetyIdentity)
		rootSettings["hooks"] = hooks
	}

	malformedPayload, err := json.Marshal(rootSettings)
	if err != nil {
		t.Fatalf("marshal partial settings: %v", err)
	}
	if err := os.WriteFile(settingsPath, malformedPayload, 0644); err != nil {
		t.Fatalf("write partial settings: %v", err)
	}

	status := adapter.Status()
	if status.Status != engineRuntime.CapabilityPartial {
		t.Fatalf("status should be partial when safety hooks are missing, got %#v", status)
	}
}

func TestClaudeStatusFailsWhenSettingsIsMalformed(t *testing.T) {
	root := t.TempDir()
	settingsPath := filepath.Join(root, "settings.json")
	if err := os.WriteFile(settingsPath, []byte("not json"), 0644); err != nil {
		t.Fatalf("write malformed settings fixture: %v", err)
	}

	adapter := engineRuntime.NewClaudeAdapter(root)
	status := adapter.Status()
	if status.Status != engineRuntime.CapabilityUnsupported {
		t.Fatalf("Status() with malformed settings should be unsupported, got %#v", status)
	}

	install := adapter.Install()
	if install.Status != engineRuntime.CapabilityPartial {
		t.Fatalf("Install() with malformed settings should be partial, got %#v", install)
	}

	reloaded, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read malformed settings after failed install: %v", err)
	}
	if string(reloaded) != "not json" {
		t.Fatalf("malformed settings must remain unchanged, got %q", string(reloaded))
	}
}

func parseClaudeSettingsFile(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings file: %v", err)
	}
	var root map[string]interface{}
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("decode settings file: %v\n%s", err, string(data))
	}
	return root
}

func hasHookEntryForIdentity(entry interface{}, hookCommand, identity string) bool {
	em, ok := entry.(map[string]interface{})
	if !ok {
		return false
	}
	innerHooks, ok := em["hooks"].([]interface{})
	if !ok {
		return false
	}
	for _, inner := range innerHooks {
		innerMap, ok := inner.(map[string]interface{})
		if !ok {
			continue
		}
		cmd, ok := innerMap["command"].(string)
		if !ok {
			continue
		}
		if strings.Contains(cmd, hookCommand) && strings.Contains(cmd, identity) {
			return true
		}
	}
	return false
}

func hasOwnedClaudeHook(root map[string]interface{}, key, hookCommand, identity string) bool {
	hooks, ok := root["hooks"].(map[string]interface{})
	if !ok {
		return false
	}
	entries, ok := hooks[key].([]interface{})
	if !ok {
		return false
	}
	for _, entry := range entries {
		if hasHookEntryForIdentity(entry, hookCommand, identity) {
			return true
		}
	}
	return false
}

func hasAnyOwnedClaudeHooks(root map[string]interface{}, key, hookCommand string) bool {
	hooks, ok := root["hooks"].(map[string]interface{})
	if !ok {
		return false
	}
	entries, ok := hooks[key].([]interface{})
	if !ok {
		return false
	}
	for _, entry := range entries {
		em := entry.(map[string]interface{})
		if _, hasType := em["type"]; hasType {
			continue
		}
		innerHooks, _ := em["hooks"].([]interface{})
		for _, inner := range innerHooks {
			innerMap, ok := inner.(map[string]interface{})
			if !ok {
				continue
			}
			cmd, ok := innerMap["command"].(string)
			if ok && strings.Contains(cmd, hookCommand) {
				return true
			}
		}
	}
	return false
}

func dropEntriesWithIdentity(root map[string]interface{}, key, hookCommand, identity string) (map[string]interface{}, error) {
	hooks, ok := root["hooks"].(map[string]interface{})
	if !ok {
		return root, nil
	}
	entries, ok := hooks[key].([]interface{})
	if !ok {
		return root, nil
	}
	kept := make([]interface{}, 0, len(entries))
	for _, entry := range entries {
		if !hasHookEntryForIdentity(entry, hookCommand, identity) {
			kept = append(kept, entry)
		}
	}
	hooks[key] = kept
	root["hooks"] = hooks
	return root, nil
}

// injectDesignHookPair patches an already-installed settings.json to add the
// anti-generic-design UserPromptSubmit/PreToolUse hook pair by hand.
// Merger.mergeHooks does not install this pair yet (Phase 4 of the
// anti-generic-design-runtime-wiring chain, a later PR) — tests that only
// care about OTHER lifecycle-state behavior use this helper to keep Claude
// "supported" without depending on that future work.
func injectDesignHookPair(t *testing.T, settingsPath, hookCommand string) {
	t.Helper()
	root := parseClaudeSettingsFile(t, settingsPath)
	hooks, ok := root["hooks"].(map[string]interface{})
	if !ok {
		t.Fatalf("injectDesignHookPair: settings has no hooks map")
	}
	command := hookCommand + " " + settings.LabdrianDesignIdentity
	hooks["UserPromptSubmit"] = append(hooks["UserPromptSubmit"].([]interface{}), map[string]interface{}{
		"hooks": []interface{}{map[string]interface{}{"type": "command", "command": command}},
	})
	hooks["PreToolUse"] = append(hooks["PreToolUse"].([]interface{}), map[string]interface{}{
		"matcher": "Agent",
		"hooks":   []interface{}{map[string]interface{}{"type": "command", "command": command}},
	})
	root["hooks"] = hooks
	patched, err := json.Marshal(root)
	if err != nil {
		t.Fatalf("injectDesignHookPair: marshal: %v", err)
	}
	if err := os.WriteFile(settingsPath, patched, 0644); err != nil {
		t.Fatalf("injectDesignHookPair: write %s: %v", settingsPath, err)
	}
}
