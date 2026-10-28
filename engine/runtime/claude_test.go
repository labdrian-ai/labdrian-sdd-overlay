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
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings/settingsfile"
)

// newClaudeAdapter is the Claude adapter over root with the real settings file adapter, so the
// tests below see the bytes the adapter leaves in a temporary directory.
func newClaudeAdapter(root string) engineRuntime.ClaudeAdapter {
	return engineRuntime.NewClaudeAdapter(root, settingsfile.Installer{})
}

func TestClaudeInstallWritesLifecycleHooksAndReportsSupportedStatus(t *testing.T) {
	root := t.TempDir()
	adapter := newClaudeAdapter(root)

	if result := adapter.Install(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}

	// settings.Document.Merge installs both pairs (minimalism, design) via the real
	// settings file adapter (Phase 4, PR-3 of the
	// anti-generic-design-runtime-wiring chain) — no test-fixture workaround
	// needed anymore.
	settingsPath := filepath.Join(root, "settings.json")
	hookCommand := filepath.Join(root, "bin", "gentle-ai-overlay")

	status := adapter.Status()
	if status.Status != core.CapabilitySupported {
		t.Fatalf("Status() after install = %#v", status)
	}
	if !strings.Contains(status.Message, "installed and owned") {
		t.Fatalf("status message should report installed and owned state, got %q", status.Message)
	}

	rootSettings := parseClaudeSettingsFile(t, settingsPath)
	if !hasOwnedClaudeHook(rootSettings, "UserPromptSubmit", hookCommand, settings.LabdrianMinimalismIdentity) {
		t.Fatalf("settings after install should include minimalism UserPromptSubmit hook: %#v", rootSettings["hooks"])
	}
	if !hasOwnedClaudeHook(rootSettings, "UserPromptSubmit", hookCommand, settings.LabdrianDesignIdentity) {
		t.Fatalf("settings after install should include design UserPromptSubmit hook: %#v", rootSettings["hooks"])
	}
	if !hasOwnedClaudeHook(rootSettings, "PreToolUse", hookCommand, settings.LabdrianMinimalismIdentity) {
		t.Fatalf("settings after install should include minimalism PreToolUse hook: %#v", rootSettings["hooks"])
	}
	if !hasOwnedClaudeHook(rootSettings, "PreToolUse", hookCommand, settings.LabdrianDesignIdentity) {
		t.Fatalf("settings after install should include design PreToolUse hook: %#v", rootSettings["hooks"])
	}
}

func TestClaudeUpdateRefreshesLifecycleAndKeepsSupportedStatus(t *testing.T) {
	root := t.TempDir()
	adapter := newClaudeAdapter(root)

	if result := adapter.Install(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}

	if result := adapter.Update(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Update() = %#v", result)
	}

	// settings.Document.Merge installs both pairs (minimalism, design) via the real
	// settings file adapter's Install path — Update() keeps the (minimalism+design)
	// lifecycle state "supported" without any test-fixture workaround.
	status := adapter.Status()
	if status.Status != core.CapabilitySupported {
		t.Fatalf("Status() after update should remain supported, got %#v", status)
	}
}

func TestClaudeUninstallRemovesOwnedHooksAndReturnsUnhealthyStatus(t *testing.T) {
	root := t.TempDir()
	adapter := newClaudeAdapter(root)
	if result := adapter.Install(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}

	if result := adapter.Uninstall(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Uninstall() = %#v", result)
	}

	status := adapter.Status()
	if status.Status != core.CapabilityUnsupported && status.Status != core.CapabilityPartial {
		t.Fatalf("Status() after uninstall = %#v", status)
	}

	settingsPath := filepath.Join(root, "settings.json")
	rootSettings := parseClaudeSettingsFile(t, settingsPath)
	hookCommand := filepath.Join(root, "bin", "gentle-ai-overlay")
	if hasAnyOwnedClaudeHooks(rootSettings, "UserPromptSubmit", hookCommand) || hasAnyOwnedClaudeHooks(rootSettings, "PreToolUse", hookCommand) {
		t.Fatalf("owned hooks should be removed after uninstall, got %#v", rootSettings["hooks"])
	}
}

func TestClaudeDefaultRootIsUnderTheConfigHomeWhenNoRootIsGiven(t *testing.T) {
	home := t.TempDir()
	adapter, err := shippedRegistry(t).New(core.TargetClaude, core.Config{Home: home})
	if err != nil {
		t.Fatal(err)
	}

	if result := adapter.Install(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}

	if _, err := os.Stat(filepath.Join(home, ".claude", "settings.json")); err != nil {
		t.Fatalf("expected settings at default root %q, stat: %v", filepath.Join(home, ".claude", "settings.json"), err)
	}
}

func TestClaudeExplicitRootIsolatedFromDefaultHOME(t *testing.T) {
	home := t.TempDir()
	explicitRoot := filepath.Join(t.TempDir(), "explicit-claude-root")
	adapter, err := shippedRegistry(t).New(core.TargetClaude, core.Config{Home: home, ConfigRoot: explicitRoot})
	if err != nil {
		t.Fatal(err)
	}

	if result := adapter.Install(); result.Status != core.CapabilityRestartRequired {
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
	adapter := newClaudeAdapter(root)

	if result := adapter.Install(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}

	rootSettings := parseClaudeSettingsFile(t, settingsPath)
	hooks, ok := rootSettings["hooks"].(map[string]interface{})
	if !ok {
		t.Fatal("settings hooks should be a map")
	}
	for _, key := range []string{"UserPromptSubmit", "PreToolUse"} {
		rootSettings, _ = dropEntriesWithIdentity(rootSettings, key, hookCommand, settings.LabdrianDesignIdentity)
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
	if status.Status != core.CapabilityPartial {
		t.Fatalf("status should be partial when design hooks are missing, got %#v", status)
	}
}

// TestClaudeStatusPartialMessageNamesRemediationCommands asserts the partial
// message names the exact upgrade step (post-#291): a machine missing any
// owned family — including the SessionEnd sync-trigger family — must be told
// to run 'labdrian uninstall-hooks' then 'labdrian install-hooks', not just
// "not fully owned/installed".
func TestClaudeStatusPartialMessageNamesRemediationCommands(t *testing.T) {
	root := t.TempDir()
	adapter := newClaudeAdapter(root)

	// Fresh root: no settings.json at all is CapabilityUnsupported, not
	// CapabilityPartial, so seed a two-family (no SessionEnd) fixture
	// directly — the same "pre-upgrade machine" shape the SessionEnd
	// requirement's status scenario covers.
	settingsPath := filepath.Join(root, "settings.json")
	hookCommand := filepath.Join(root, "bin", "gentle-ai-overlay")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := (settingsfile.Installer{}).Install(settingsPath, hookCommand); err != nil {
		t.Fatalf("seed Install: %v", err)
	}

	seeded := parseClaudeSettingsFile(t, settingsPath)
	seeded, _ = dropEntriesWithIdentity(seeded, "SessionEnd", hookCommand, settings.LabdrianSyncTriggerIdentity)
	data, err := json.Marshal(seeded)
	if err != nil {
		t.Fatalf("marshal seeded settings: %v", err)
	}
	if err := os.WriteFile(settingsPath, data, 0644); err != nil {
		t.Fatalf("write seeded settings: %v", err)
	}

	status := adapter.Status()
	if status.Status != core.CapabilityPartial {
		t.Fatalf("status should be partial when the SessionEnd family is missing, got %#v", status)
	}
	if !strings.Contains(status.Message, "labdrian uninstall-hooks") || !strings.Contains(status.Message, "labdrian install-hooks") {
		t.Errorf("partial message must name both remediation commands; got %q", status.Message)
	}
}

func TestClaudeStatusFailsWhenSettingsIsMalformed(t *testing.T) {
	root := t.TempDir()
	settingsPath := filepath.Join(root, "settings.json")
	if err := os.WriteFile(settingsPath, []byte("not json"), 0644); err != nil {
		t.Fatalf("write malformed settings fixture: %v", err)
	}

	adapter := newClaudeAdapter(root)
	status := adapter.Status()
	if status.Status != core.CapabilityUnsupported {
		t.Fatalf("Status() with malformed settings should be unsupported, got %#v", status)
	}

	install := adapter.Install()
	if install.Status != core.CapabilityPartial {
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

// The review stop-hook (own-rdd T6a, S6) is the Stop entry that asks the agent to review the next commit no review
// covers. The adapter owns it as it owns every family: install writes it, status is partial until install-hooks is
// run again on a machine that installed before it, update repairs a drift, and uninstall takes it off and leaves the
// Stop entries of other programs.

// reviewStopEntries returns the entries under Stop that are the review stop-hook's, for the adapter's hook command.
func reviewStopEntries(root map[string]interface{}, hookCommand string) []interface{} {
	hooks, _ := root["hooks"].(map[string]interface{})
	entries, _ := hooks["Stop"].([]interface{})
	var ours []interface{}
	for _, e := range entries {
		if hasHookEntryForIdentity(e, hookCommand, settings.LabdrianReviewStopIdentity) {
			ours = append(ours, e)
		}
	}
	return ours
}

func TestClaudeInstallWritesTheReviewStopHook(t *testing.T) {
	root := t.TempDir()
	adapter := newClaudeAdapter(root)
	hookCommand := filepath.Join(root, "bin", "gentle-ai-overlay")

	if result := adapter.Install(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}

	ours := reviewStopEntries(parseClaudeSettingsFile(t, filepath.Join(root, "settings.json")), hookCommand)
	if len(ours) != 1 {
		t.Fatalf("review stop-hook entries under Stop = %v, want exactly one", ours)
	}
	if _, hasMatcher := ours[0].(map[string]interface{})["matcher"]; hasMatcher {
		t.Errorf("the Stop entry has a matcher, and Stop has none: %v", ours[0])
	}
	if got := mustMarshal(t, ours[0]); !strings.Contains(got, hookCommand+" review stop-hook") {
		t.Errorf("the Stop entry = %s, want it to run 'review stop-hook'", got)
	}
	if status := adapter.Status(); status.Status != core.CapabilitySupported {
		t.Errorf("Status() after install = %#v", status)
	}
}

func TestClaudeStatusIsPartialUntilInstallHooksIsRerunForTheReviewStopHook(t *testing.T) {
	root := t.TempDir()
	settingsPath := filepath.Join(root, "settings.json")
	hookCommand := filepath.Join(root, "bin", "gentle-ai-overlay")
	adapter := newClaudeAdapter(root)
	if result := adapter.Install(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}

	// An install made before the review stop-hook existed.
	older, _ := dropEntriesWithIdentity(parseClaudeSettingsFile(t, settingsPath), "Stop", hookCommand, settings.LabdrianReviewStopIdentity)
	writeClaudeSettings(t, settingsPath, older)

	status := adapter.Status()
	if status.Status != core.CapabilityPartial {
		t.Fatalf("status without the review stop-hook = %#v, want partial", status)
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
	if ours := reviewStopEntries(parseClaudeSettingsFile(t, settingsPath), hookCommand); len(ours) != 1 {
		t.Errorf("review stop-hook entries after re-running install = %v, want exactly one", ours)
	}
}

func TestClaudeStatusIsPartialWhenTheReviewStopHookEntryDrifted(t *testing.T) {
	root := t.TempDir()
	settingsPath := filepath.Join(root, "settings.json")
	hookCommand := filepath.Join(root, "bin", "gentle-ai-overlay")
	adapter := newClaudeAdapter(root)
	if result := adapter.Install(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}
	drifted := parseClaudeSettingsFile(t, settingsPath)
	for _, e := range reviewStopEntries(drifted, hookCommand) {
		inner := e.(map[string]interface{})["hooks"].([]interface{})[0].(map[string]interface{})
		inner["command"] = inner["command"].(string) + " # edited"
	}
	writeClaudeSettings(t, settingsPath, drifted)

	if status := adapter.Status(); status.Status != core.CapabilityPartial {
		t.Fatalf("status with an edited review stop-hook = %#v, want partial", status)
	}
	if result := adapter.Update(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Update() = %#v", result)
	}
	if status := adapter.Status(); status.Status != core.CapabilitySupported {
		t.Errorf("status after Update = %#v, want supported", status)
	}
	if ours := reviewStopEntries(parseClaudeSettingsFile(t, settingsPath), hookCommand); len(ours) != 1 {
		t.Errorf("review stop-hook entries after Update = %v, want exactly one", ours)
	}
}

func TestClaudeUninstallRemovesTheReviewStopHookAndKeepsForeignStopEntries(t *testing.T) {
	root := t.TempDir()
	settingsPath := filepath.Join(root, "settings.json")
	hookCommand := filepath.Join(root, "bin", "gentle-ai-overlay")
	adapter := newClaudeAdapter(root)
	foreign := map[string]interface{}{"hooks": []interface{}{map[string]interface{}{"type": "command", "command": "/opt/other/stop-hook"}}}
	writeClaudeSettings(t, settingsPath, map[string]interface{}{"hooks": map[string]interface{}{"Stop": []interface{}{foreign}}})

	if result := adapter.Install(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}
	if result := adapter.Uninstall(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Uninstall() = %#v", result)
	}
	after := parseClaudeSettingsFile(t, settingsPath)
	if ours := reviewStopEntries(after, hookCommand); len(ours) != 0 {
		t.Errorf("review stop-hook entries survive Uninstall: %v", ours)
	}
	entries, _ := after["hooks"].(map[string]interface{})["Stop"].([]interface{})
	if len(entries) != 1 || mustMarshal(t, entries[0]) != mustMarshal(t, foreign) {
		t.Errorf("Stop after Uninstall = %v, want only the foreign entry", entries)
	}
}

// The builder guard (own-rdd T11d, S14 g) is the PreToolUse entry on the Agent tool that denies a builder launch whose
// task the shaper did not design. The adapter owns it as it owns every family: install writes it, status is partial
// until install-hooks is run again on a machine that installed before it, update repairs a drift, and uninstall takes it
// off and leaves the entries of other programs on the same matcher.

// builderGuardEntries returns the entries under PreToolUse that are the builder guard's, for the adapter's hook command.
func builderGuardEntries(root map[string]interface{}, hookCommand string) []interface{} {
	hooks, _ := root["hooks"].(map[string]interface{})
	entries, _ := hooks["PreToolUse"].([]interface{})
	var ours []interface{}
	for _, e := range entries {
		if hasHookEntryForIdentity(e, hookCommand, settings.LabdrianBuilderGuardIdentity) {
			ours = append(ours, e)
		}
	}
	return ours
}

func TestClaudeInstallWritesTheBuilderGuard(t *testing.T) {
	root := t.TempDir()
	adapter := newClaudeAdapter(root)
	hookCommand := filepath.Join(root, "bin", "gentle-ai-overlay")

	if result := adapter.Install(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}

	ours := builderGuardEntries(parseClaudeSettingsFile(t, filepath.Join(root, "settings.json")), hookCommand)
	if len(ours) != 1 {
		t.Fatalf("builder guard entries under PreToolUse = %v, want exactly one", ours)
	}
	if matcher := ours[0].(map[string]interface{})["matcher"]; matcher != "Agent" {
		t.Errorf("the PreToolUse entry matcher is %v, want Agent", matcher)
	}
	if got := mustMarshal(t, ours[0]); !strings.Contains(got, hookCommand+" roles builder-guard") {
		t.Errorf("the PreToolUse entry = %s, want it to run 'roles builder-guard'", got)
	}
	if status := adapter.Status(); status.Status != core.CapabilitySupported {
		t.Errorf("Status() after install = %#v", status)
	}
}

func TestClaudeStatusIsPartialUntilInstallHooksIsRerunForTheBuilderGuard(t *testing.T) {
	root := t.TempDir()
	settingsPath := filepath.Join(root, "settings.json")
	hookCommand := filepath.Join(root, "bin", "gentle-ai-overlay")
	adapter := newClaudeAdapter(root)
	if result := adapter.Install(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}

	// An install made before the builder guard existed.
	older, _ := dropEntriesWithIdentity(parseClaudeSettingsFile(t, settingsPath), "PreToolUse", hookCommand, settings.LabdrianBuilderGuardIdentity)
	writeClaudeSettings(t, settingsPath, older)

	status := adapter.Status()
	if status.Status != core.CapabilityPartial {
		t.Fatalf("status without the builder guard = %#v, want partial", status)
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
	if ours := builderGuardEntries(parseClaudeSettingsFile(t, settingsPath), hookCommand); len(ours) != 1 {
		t.Errorf("builder guard entries after re-running install = %v, want exactly one", ours)
	}
}

func TestClaudeStatusIsPartialWhenTheBuilderGuardEntryDrifted(t *testing.T) {
	root := t.TempDir()
	settingsPath := filepath.Join(root, "settings.json")
	hookCommand := filepath.Join(root, "bin", "gentle-ai-overlay")
	adapter := newClaudeAdapter(root)
	if result := adapter.Install(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}
	drifted := parseClaudeSettingsFile(t, settingsPath)
	for _, e := range builderGuardEntries(drifted, hookCommand) {
		inner := e.(map[string]interface{})["hooks"].([]interface{})[0].(map[string]interface{})
		inner["command"] = inner["command"].(string) + " # edited"
	}
	writeClaudeSettings(t, settingsPath, drifted)

	if status := adapter.Status(); status.Status != core.CapabilityPartial {
		t.Fatalf("status with an edited builder guard = %#v, want partial", status)
	}
	if result := adapter.Update(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Update() = %#v", result)
	}
	if status := adapter.Status(); status.Status != core.CapabilitySupported {
		t.Errorf("status after Update = %#v, want supported", status)
	}
	if ours := builderGuardEntries(parseClaudeSettingsFile(t, settingsPath), hookCommand); len(ours) != 1 {
		t.Errorf("builder guard entries after Update = %v, want exactly one", ours)
	}
}

func TestClaudeUninstallRemovesTheBuilderGuardAndKeepsForeignAgentEntries(t *testing.T) {
	root := t.TempDir()
	settingsPath := filepath.Join(root, "settings.json")
	hookCommand := filepath.Join(root, "bin", "gentle-ai-overlay")
	adapter := newClaudeAdapter(root)
	foreign := map[string]interface{}{"matcher": "Agent", "hooks": []interface{}{map[string]interface{}{"type": "command", "command": "/opt/other/agent-hook"}}}
	writeClaudeSettings(t, settingsPath, map[string]interface{}{"hooks": map[string]interface{}{"PreToolUse": []interface{}{foreign}}})

	if result := adapter.Install(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}
	if result := adapter.Uninstall(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Uninstall() = %#v", result)
	}
	after := parseClaudeSettingsFile(t, settingsPath)
	if ours := builderGuardEntries(after, hookCommand); len(ours) != 0 {
		t.Errorf("builder guard entries survive Uninstall: %v", ours)
	}
	entries, _ := after["hooks"].(map[string]interface{})["PreToolUse"].([]interface{})
	if len(entries) != 1 || mustMarshal(t, entries[0]) != mustMarshal(t, foreign) {
		t.Errorf("PreToolUse after Uninstall = %v, want only the foreign entry", entries)
	}
}

// NOTE (Phase 4, PR-3): the injectDesignHookPair test-fixture workaround that
// previously lived here has been removed now that the settings merge installs
// the real anti-generic-design pair — tests exercise the real Install()/
// Update() path end-to-end (see TestClaudeInstallWritesLifecycleHooksAndReportsSupportedStatus,
// TestClaudeUpdateRefreshesLifecycleAndKeepsSupportedStatus above).
