package settings_test

// Edge cases of the projection family's ownership rule and of settings files
// whose hooks are not shaped as Claude Code expects. Everything runs against
// temp files; the real ~/.claude/settings.json is never touched.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings"
)

// foreignCommand mentions the projection identity but runs another program.
const foreignCommand = "/opt/other/tool projection hook --event UserPromptSubmit"

func foreignMentioningTheIdentity() map[string]interface{} {
	return map[string]interface{}{"hooks": []interface{}{map[string]interface{}{"type": "command", "command": foreignCommand}}}
}

// writeSettingsDoc writes doc as a settings file in a fresh temp directory and
// returns its path.
func writeSettingsDoc(t *testing.T, doc map[string]interface{}) string {
	t.Helper()
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func readFileBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// --- ownership ----------------------------------------------------------------

// TestProjectionOwnershipNeedsTheBinaryPathAndTheIdentity: an entry that carries
// only the identity token, because it runs some other program with a similar
// verb, is foreign: install keeps it next to ours and uninstall leaves it alone.
func TestProjectionOwnershipNeedsTheBinaryPathAndTheIdentity(t *testing.T) {
	doc := map[string]interface{}{"hooks": map[string]interface{}{
		"UserPromptSubmit": []interface{}{foreignMentioningTheIdentity()},
	}}
	path := writeSettingsDoc(t, doc)
	m := buildMerger(t, path)

	if err := m.Install(); err != nil {
		t.Fatal(err)
	}
	ups := parseJSON(t, path)["hooks"].(map[string]interface{})["UserPromptSubmit"].([]interface{})
	if !containsEntry(t, ups, foreignMentioningTheIdentity()) {
		t.Errorf("Install lost or altered the foreign entry: %s", mustJSON(t, ups))
	}
	if err := m.Uninstall(); err != nil {
		t.Fatal(err)
	}
	ups = parseJSON(t, path)["hooks"].(map[string]interface{})["UserPromptSubmit"].([]interface{})
	if len(ups) != 1 || !containsEntry(t, ups, foreignMentioningTheIdentity()) {
		t.Errorf("after Uninstall, UserPromptSubmit = %s, want only the foreign entry", mustJSON(t, ups))
	}
}

func containsEntry(t *testing.T, entries []interface{}, want interface{}) bool {
	t.Helper()
	wantJSON := mustJSON(t, want)
	for _, e := range entries {
		if mustJSON(t, e) == wantJSON {
			return true
		}
	}
	return false
}

// TestProjectionOwnershipIsASubstringOfTheHookCommand pins a limit every hook
// family shares: an entry is ours when its command contains the binary path
// given to the merger and the family's identity token. A binary path so short
// that it occurs in a foreign command claims that command, so a caller must
// pass the real, full path (the merge-settings and uninstall-hooks verbs
// require --hook-command). The test exists so that changing the rule is a
// deliberate decision, not an accident.
func TestProjectionOwnershipIsASubstringOfTheHookCommand(t *testing.T) {
	doc := map[string]interface{}{"hooks": map[string]interface{}{
		"UserPromptSubmit": []interface{}{foreignMentioningTheIdentity()},
	}}
	path := writeSettingsDoc(t, doc)

	// "o" occurs in "/opt/other/tool", so the foreign entry is treated as ours.
	if parts := settings.MissingProjectionHookParts(parseJSON(t, path), "o"); !strings.Contains(strings.Join(parts, "|"), "drifted") {
		t.Fatalf("a one-letter hook command did not claim the foreign entry: %v", parts)
	}
	if err := settings.NewMerger(path, "o").Uninstall(); err != nil {
		t.Fatal(err)
	}
	if hooks, _ := parseJSON(t, path)["hooks"].(map[string]interface{}); hooks["UserPromptSubmit"] != nil {
		t.Errorf("Uninstall with a one-letter hook command kept the entry it claims: %s", mustJSON(t, hooks))
	}
}

// TestAnEmptyHookCommandClaimsNothing: with no binary path there is nothing to
// tell our entries from anyone else's, so no entry is ours. Install and
// Uninstall refuse, and leave the file byte for byte as it was; the status
// helpers report the family missing rather than adopting a foreign entry.
func TestAnEmptyHookCommandClaimsNothing(t *testing.T) {
	doc := map[string]interface{}{"hooks": map[string]interface{}{
		"UserPromptSubmit": []interface{}{foreignMentioningTheIdentity()},
		"PreToolUse":       []interface{}{foreignMentioningTheIdentity()},
	}}

	t.Run("status", func(t *testing.T) {
		parts := settings.MissingProjectionHookParts(doc, "")
		if len(parts) != 3 {
			t.Errorf("parts = %v, want the three entries reported missing", parts)
		}
		if strings.Contains(strings.Join(parts, "|"), "drifted") {
			t.Errorf("parts = %v: an empty hook command adopted a foreign entry as drifted", parts)
		}
		if settings.HasProjectionHooks(doc, "") {
			t.Error("HasProjectionHooks = true for an empty hook command")
		}
	})

	for name, act := range map[string]func(*settings.Merger) error{
		"Install":   func(m *settings.Merger) error { return m.Install() },
		"Uninstall": func(m *settings.Merger) error { return m.Uninstall() },
	} {
		t.Run(name, func(t *testing.T) {
			path := writeSettingsDoc(t, doc)
			before := readFileBytes(t, path)
			err := act(settings.NewMerger(path, ""))
			if err == nil {
				t.Fatalf("%s with an empty hook command returned nil, want an error", name)
			}
			if !strings.Contains(err.Error(), "hook command") {
				t.Errorf("error %q does not name the hook command", err)
			}
			if after := readFileBytes(t, path); !bytes.Equal(after, before) {
				t.Errorf("%s changed the file:\n%s\nwas\n%s", name, after, before)
			}
			if _, statErr := os.Stat(path + ".bak"); statErr == nil {
				t.Errorf("%s wrote a backup although it changed nothing", name)
			}
		})
	}
}

// --- malformed settings ---------------------------------------------------------

// malformedHooks are settings roots whose hooks are not shaped as Claude Code
// expects, next to a foreign top-level key that must always survive.
func malformedHooks() map[string]map[string]interface{} {
	events := func(ups, pre interface{}) map[string]interface{} {
		return map[string]interface{}{"model": "opus", "hooks": map[string]interface{}{
			"Notification": []interface{}{foreignMentioningTheIdentity()}, "UserPromptSubmit": ups, "PreToolUse": pre,
		}}
	}
	return map[string]map[string]interface{}{
		"hooks is an array":            {"model": "opus", "hooks": []interface{}{"x"}},
		"hooks is a string":            {"model": "opus", "hooks": "x"},
		"hooks is a number":            {"model": "opus", "hooks": float64(1)},
		"hooks is null":                {"model": "opus", "hooks": nil},
		"events are strings":           events("x", "y"),
		"events are objects":           events(map[string]interface{}{}, map[string]interface{}{"matcher": "Bash"}),
		"events are null":              events(nil, nil),
		"entries are not objects":      events([]interface{}{"x", float64(1), nil}, []interface{}{[]interface{}{}, true}),
		"entries have no inner hooks":  events([]interface{}{map[string]interface{}{"matcher": "Bash"}}, []interface{}{map[string]interface{}{"hooks": "x"}}),
		"inner hooks are not objects":  events([]interface{}{map[string]interface{}{"hooks": []interface{}{"x", float64(2)}}}, []interface{}{map[string]interface{}{"hooks": []interface{}{nil}}}),
		"inner commands are not texts": events([]interface{}{map[string]interface{}{"hooks": []interface{}{map[string]interface{}{"command": float64(3)}}}}, []interface{}{}),
	}
}

// TestMissingProjectionHookPartsToleratesMalformedSettings: a settings file
// whose hooks are not shaped as expected is reported as lacking the whole
// family, the same as a file with no hooks at all. Status reads the file; it
// never panics and never mistakes garbage for a hook.
func TestMissingProjectionHookPartsToleratesMalformedSettings(t *testing.T) {
	want := settings.MissingProjectionHookParts(map[string]interface{}{}, testHookCommand)
	if len(want) != 3 {
		t.Fatalf("test bug: empty settings report %v, want the three entries", want)
	}
	for name, root := range malformedHooks() {
		t.Run(name, func(t *testing.T) {
			if got := settings.MissingProjectionHookParts(root, testHookCommand); !reflect.DeepEqual(got, want) {
				t.Errorf("parts = %v, want %v", got, want)
			}
			if settings.HasProjectionHooks(root, testHookCommand) {
				t.Error("HasProjectionHooks = true")
			}
		})
	}
}

// TestInstallOverMalformedHooksReplacesOnlyTheUnusableValue: install completes
// the family even where hooks or an event are not shaped as expected. Like every
// other family, it replaces just the value that cannot be a hook list (Claude
// Code cannot use it either), keeps every other key and event, and leaves the
// original bytes in settings.json.bak. Uninstall finds nothing of ours in such a
// file and leaves it byte for byte alone.
func TestInstallOverMalformedHooksReplacesOnlyTheUnusableValue(t *testing.T) {
	for name, root := range malformedHooks() {
		t.Run(name, func(t *testing.T) {
			path := writeSettingsDoc(t, root)
			original := readFileBytes(t, path)
			m := buildMerger(t, path)

			if err := m.Uninstall(); err != nil {
				t.Fatalf("Uninstall: %v", err)
			}
			if after := readFileBytes(t, path); !bytes.Equal(after, original) {
				t.Errorf("Uninstall changed a file with nothing of ours in it:\n%s", after)
			}
			if _, err := os.Stat(path + ".bak"); err == nil {
				t.Errorf("Uninstall rewrote a file with nothing of ours in it (it left a backup)")
			}

			if err := m.Install(); err != nil {
				t.Fatalf("Install: %v", err)
			}
			after := parseJSON(t, path)
			if !settings.HasProjectionHooks(after, testHookCommand) {
				t.Errorf("family incomplete after Install: %v", settings.MissingProjectionHookParts(after, testHookCommand))
			}
			if after["model"] != "opus" {
				t.Errorf("foreign top-level key lost: %v", after)
			}
			if !bytes.Equal(readFileBytes(t, path+".bak"), original) {
				t.Errorf("settings.json.bak does not hold the original bytes")
			}
			// A well-formed sibling event is exactly as it was.
			if notification, ok := root["hooks"].(map[string]interface{}); ok && notification["Notification"] != nil {
				got := after["hooks"].(map[string]interface{})["Notification"]
				if mustJSON(t, got) != mustJSON(t, notification["Notification"]) {
					t.Errorf("Notification changed: %s", mustJSON(t, got))
				}
			}
		})
	}
}
