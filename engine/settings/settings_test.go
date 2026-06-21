// Package settings_test exercises the merge-settings and uninstall-hooks logic
// using fixture-based temp dirs. Tests NEVER touch the live ~/.claude/settings.json.
package settings_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings"
)

// hookCommand is the unique identifier used in tests to represent the deployed
// binary path. It is NOT the real ~/.claude/bin path — tests are isolated.
const testHookCommand = "/home/testuser/.claude/bin/gentle-ai-overlay"

// buildMerger creates a Merger pointed at the given settings path with the
// test binary path. This is the test's single entry point into the package.
func buildMerger(t *testing.T, settingsPath string) *settings.Merger { //nolint:unparam
	t.Helper()
	return settings.NewMerger(settingsPath, testHookCommand)
}

// parseJSON is a test helper that parses a JSON file and returns the raw map.
func parseJSON(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("parseJSON: read %s: %v", path, err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("parseJSON: unmarshal %s: %v", path, err)
	}
	return out
}

// containsHookWithCommand returns true if the JSON object at top-level key
// "hooks" → hookKey → slice contains an entry whose "command" == cmd.
func containsHookWithCommand(root map[string]interface{}, hookKey, cmd string) bool {
	hooks, ok := root["hooks"].(map[string]interface{})
	if !ok {
		return false
	}
	entries, ok := hooks[hookKey].([]interface{})
	if !ok {
		return false
	}
	for _, e := range entries {
		if m, ok := e.(map[string]interface{}); ok {
			if m["command"] == cmd {
				return true
			}
		}
	}
	return false
}

// countHookEntriesWithCommand counts how many entries under hooks[hookKey] have
// the given command — used to verify idempotency.
func countHookEntriesWithCommand(root map[string]interface{}, hookKey, cmd string) int {
	hooks, ok := root["hooks"].(map[string]interface{})
	if !ok {
		return 0
	}
	entries, ok := hooks[hookKey].([]interface{})
	if !ok {
		return 0
	}
	n := 0
	for _, e := range entries {
		if m, ok := e.(map[string]interface{}); ok {
			if m["command"] == cmd {
				n++
			}
		}
	}
	return n
}

// --- TC-SET-1: absent file → create with both hooks ---

func TestMerge_AbsentFile_CreatesWithBothHooks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	m := buildMerger(t, path)

	if err := m.Install(); err != nil {
		t.Fatalf("Install on absent file: %v", err)
	}

	root := parseJSON(t, path)
	if !containsHookWithCommand(root, "UserPromptSubmit", testHookCommand) {
		t.Errorf("absent file: UserPromptSubmit hook not found in %v", root)
	}
	if !containsHookWithCommand(root, "PreToolUse", testHookCommand) {
		t.Errorf("absent file: PreToolUse hook not found in %v", root)
	}
}

// --- TC-SET-2: preserve existing keys ---

func TestMerge_ExistingKeys_Preserved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	// Write a settings.json with arbitrary existing keys.
	existing := map[string]interface{}{
		"env":        map[string]string{"MY_VAR": "hello"},
		"mcpServers": map[string]interface{}{"my-server": map[string]interface{}{"url": "http://localhost"}},
		"theme":      "dark",
	}
	data, _ := json.Marshal(existing)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	m := buildMerger(t, path)
	if err := m.Install(); err != nil {
		t.Fatalf("Install with existing keys: %v", err)
	}

	root := parseJSON(t, path)
	// All original keys must still be present.
	if root["theme"] != "dark" {
		t.Errorf("'theme' key should be preserved; got %v", root["theme"])
	}
	envMap, ok := root["env"].(map[string]interface{})
	if !ok || envMap["MY_VAR"] != "hello" {
		t.Errorf("env.MY_VAR should be preserved; got %v", root["env"])
	}
	// Hooks must also be present.
	if !containsHookWithCommand(root, "UserPromptSubmit", testHookCommand) {
		t.Errorf("UserPromptSubmit hook not found after merge")
	}
}

// --- TC-SET-3: idempotent — running twice produces no duplicates ---

func TestMerge_Idempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	m := buildMerger(t, path)

	if err := m.Install(); err != nil {
		t.Fatalf("first Install: %v", err)
	}
	if err := m.Install(); err != nil {
		t.Fatalf("second Install: %v", err)
	}

	root := parseJSON(t, path)
	if n := countHookEntriesWithCommand(root, "UserPromptSubmit", testHookCommand); n != 1 {
		t.Errorf("UserPromptSubmit: expected exactly 1 entry, got %d", n)
	}
	if n := countHookEntriesWithCommand(root, "PreToolUse", testHookCommand); n != 1 {
		t.Errorf("PreToolUse: expected exactly 1 entry, got %d", n)
	}
}

// --- TC-SET-4: malformed existing settings.json → error, original untouched ---

func TestMerge_MalformedSettings_ErrorAndUntouched(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	original := []byte("THIS IS NOT JSON {{{")
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}

	m := buildMerger(t, path)
	err := m.Install()
	if err == nil {
		t.Fatal("Install on malformed JSON: expected error, got nil")
	}

	// Original file must be untouched.
	got, _ := os.ReadFile(path)
	if string(got) != string(original) {
		t.Errorf("malformed JSON: original file should be untouched; got %q", string(got))
	}
}

// --- TC-SET-5: atomic write — backup created before overwrite ---

func TestMerge_AtomicWrite_BackupCreated(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	bakPath := path + ".bak"

	// Write an initial valid settings.json.
	initial := []byte(`{"existing":true}`)
	if err := os.WriteFile(path, initial, 0644); err != nil {
		t.Fatal(err)
	}

	m := buildMerger(t, path)
	if err := m.Install(); err != nil {
		t.Fatalf("Install: %v", err)
	}

	// Backup must exist and contain the original content.
	bak, err := os.ReadFile(bakPath)
	if err != nil {
		t.Fatalf("backup not created at %s: %v", bakPath, err)
	}
	if string(bak) != string(initial) {
		t.Errorf("backup content mismatch: got %q want %q", string(bak), string(initial))
	}
}

// --- TC-SET-6: uninstall removes our entries, leaves rest intact ---

func TestUninstall_RemovesOurHooks_LeavesRest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	m := buildMerger(t, path)

	// Install first to have something to uninstall.
	initial := map[string]interface{}{
		"theme": "dark",
		"hooks": map[string]interface{}{
			"UserPromptSubmit": []interface{}{
				map[string]interface{}{"command": "other-tool", "hooks": []interface{}{}},
			},
		},
	}
	data, _ := json.Marshal(initial)
	os.WriteFile(path, data, 0644)

	if err := m.Install(); err != nil {
		t.Fatalf("Install: %v", err)
	}
	// Sanity: our hooks are present.
	if !containsHookWithCommand(parseJSON(t, path), "UserPromptSubmit", testHookCommand) {
		t.Fatal("hook should be present after Install")
	}

	if err := m.Uninstall(); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}

	root := parseJSON(t, path)
	// Our hooks must be gone.
	if containsHookWithCommand(root, "UserPromptSubmit", testHookCommand) {
		t.Error("UserPromptSubmit hook should be removed after Uninstall")
	}
	if containsHookWithCommand(root, "PreToolUse", testHookCommand) {
		t.Error("PreToolUse hook should be removed after Uninstall")
	}
	// Other hooks and keys must remain.
	if root["theme"] != "dark" {
		t.Errorf("'theme' key should survive Uninstall; got %v", root["theme"])
	}
	if !containsHookWithCommand(root, "UserPromptSubmit", "other-tool") {
		t.Errorf("'other-tool' hook should survive Uninstall; got %v", root["hooks"])
	}
}

// --- TC-SET-7: uninstall is idempotent ---

func TestUninstall_Idempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	m := buildMerger(t, path)

	if err := m.Install(); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if err := m.Uninstall(); err != nil {
		t.Fatalf("first Uninstall: %v", err)
	}
	if err := m.Uninstall(); err != nil {
		t.Fatalf("second Uninstall: %v", err)
	}
	// No panic, no error, no duplicate removal.
}

// --- TC-SET-8: absent file on Uninstall is a no-op (not an error) ---

func TestUninstall_AbsentFile_NoOp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.json")
	m := buildMerger(t, path)

	if err := m.Uninstall(); err != nil {
		t.Fatalf("Uninstall on absent file: expected no error, got %v", err)
	}
}

// --- TC-SET-9: existing other hooks under same key are preserved after merge ---

func TestMerge_ExistingHooksUnderSameKey_Preserved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	// Pre-existing settings with other hooks under the same keys we add to.
	initial := map[string]interface{}{
		"hooks": map[string]interface{}{
			"UserPromptSubmit": []interface{}{
				map[string]interface{}{"command": "other-prompt-tool"},
			},
			"PreToolUse": []interface{}{
				map[string]interface{}{"command": "other-pre-tool"},
			},
		},
	}
	data, _ := json.Marshal(initial)
	os.WriteFile(path, data, 0644)

	m := buildMerger(t, path)
	if err := m.Install(); err != nil {
		t.Fatalf("Install: %v", err)
	}

	root := parseJSON(t, path)
	// Our hooks present.
	if !containsHookWithCommand(root, "UserPromptSubmit", testHookCommand) {
		t.Error("our UserPromptSubmit hook not found")
	}
	// Other hooks still present.
	if !containsHookWithCommand(root, "UserPromptSubmit", "other-prompt-tool") {
		t.Error("other-prompt-tool hook should be preserved")
	}
	if !containsHookWithCommand(root, "PreToolUse", "other-pre-tool") {
		t.Error("other-pre-tool hook should be preserved")
	}
}

// --- TC-SET-10: merged JSON is valid (parseable) before rename ---

func TestMerge_OutputIsValidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	m := buildMerger(t, path)

	if err := m.Install(); err != nil {
		t.Fatalf("Install: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		t.Errorf("output is not valid JSON: %v\n%s", err, string(data))
	}
}
