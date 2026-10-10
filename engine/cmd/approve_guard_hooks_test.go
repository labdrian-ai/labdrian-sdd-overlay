package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings"
)

// merge-settings installs the guard and uninstall-hooks removes it, through the
// same verbs install-hooks and uninstall-hooks call.
func TestRunMergeSettingsAndUninstallHooks_ManageTheApproveGuard(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	const hookCmd = "/test/.claude/bin/gentle-ai-overlay"
	args := []string{"--settings", path, "--hook-command", hookCmd}
	count := func() int {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var root map[string]interface{}
		if err := json.Unmarshal(data, &root); err != nil {
			t.Fatal(err)
		}
		hooks, _ := root["hooks"].(map[string]interface{})
		entries, _ := hooks["PreToolUse"].([]interface{})
		n := 0
		for _, e := range entries {
			if s := mustJSONString(t, e); strings.Contains(s, hookCmd+" "+settings.LabdrianApproveGuardIdentity) {
				n++
			}
		}
		return n
	}

	runMergeSettings(args)
	if n := count(); n != 2 {
		t.Fatalf("merge-settings wrote %d approve guard entries, want 2 (Bash and the file tools)", n)
	}
	runMergeSettings(args)
	if n := count(); n != 2 {
		t.Errorf("a second merge-settings left %d approve guard entries, want 2", n)
	}
	runUninstallHooks(args)
	if n := count(); n != 0 {
		t.Errorf("uninstall-hooks left %d approve guard entries", n)
	}
}

// mustJSONString is the JSON of v as text, for a test that looks for a word in an entry of a
// settings object without caring how the entry is laid out. This test is its only user.
func mustJSONString(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
