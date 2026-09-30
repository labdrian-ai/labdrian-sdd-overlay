package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings"
)

func TestCheckApproveGuard(t *testing.T) {
	const bin = "/x/.claude/bin/gentle-ai-overlay"
	full := buildSettingsWithHooks(bin)
	c := checkApproveGuard(full, nil, "s.json", bin)
	if !c.ok || c.degraded {
		t.Errorf("guard in place: %+v", c)
	}
	// Even in place it is a speed bump, and the report says so.
	if !strings.Contains(c.note, "speed bump") || !strings.Contains(c.note, "not a security boundary") {
		t.Errorf("an installed guard must still say it is a speed bump: %+v", c)
	}
	if !strings.Contains(c.label, "skills approve") {
		t.Errorf("label %q does not name the guarded verb", c.label)
	}

	// A machine installed before the guard existed: nothing owned.
	hooks := full["hooks"].(map[string]interface{})
	var kept []interface{}
	for _, e := range hooks["PreToolUse"].([]interface{}) {
		if !strings.Contains(mustJSONString(t, e), settings.LabdrianApproveGuardIdentity) {
			kept = append(kept, e)
		}
	}
	hooks["PreToolUse"] = kept
	c = checkApproveGuard(full, nil, "s.json", bin)
	if !c.ok || !c.degraded {
		t.Fatalf("a missing guard must be WARN, not FAIL: %+v", c)
	}
	for _, want := range []string{remediationNote, `matcher="Bash"`, settings.ApproveGuardFileToolMatcher, "restart Claude Code", "speed bump"} {
		if !strings.Contains(c.note, want) {
			t.Errorf("note %q does not contain %q", c.note, want)
		}
	}

	// A drifted entry is reported as such, not as missing.
	drifted := buildSettingsWithHooks(bin)
	for _, e := range drifted["hooks"].(map[string]interface{})["PreToolUse"].([]interface{}) {
		if em := e.(map[string]interface{}); strings.Contains(mustJSONString(t, e), settings.LabdrianApproveGuardIdentity) && em["matcher"] == settings.ApproveGuardFileToolMatcher {
			em["matcher"] = "Write"
		}
	}
	if c := checkApproveGuard(drifted, nil, "s.json", bin); !c.ok || !c.degraded || !strings.Contains(c.note, "drifted") {
		t.Errorf("a drifted guard must be WARN naming the drift: %+v", c)
	}

	if c := checkApproveGuard(nil, nil, "s.json", bin); !c.ok || !c.degraded || !strings.Contains(c.note, remediationNote) {
		t.Errorf("absent settings: %+v", c)
	}
	if c := checkApproveGuard(nil, errors.New("boom"), "s.json", bin); c.ok {
		t.Errorf("unreadable settings must FAIL like the other hook checks: %+v", c)
	}

	// Hooks switched off globally: the entries exist and guard nothing.
	disabled := buildSettingsWithHooks(bin)
	disabled["disableAllHooks"] = true
	if c := checkApproveGuard(disabled, nil, "s.json", bin); !c.ok || !c.degraded || !strings.Contains(c.note, "disableAllHooks") {
		t.Errorf("globally disabled hooks must be WARN naming disableAllHooks: %+v", c)
	}
}

// TestStatusCore_ApproveGuardMissing_Degraded pins the statusCore wiring of the
// approve guard check: a machine that has not re-run install-hooks since the
// guard landed is WARN/degraded, never FAIL, and the line names the fix.
func TestStatusCore_ApproveGuardMissing_Degraded(t *testing.T) {
	homeDir, binaryPath := buildFakeHomeWithBinary(t)
	buildFakeContract(t, homeDir)
	settingsData := buildSettingsWithHooks(binaryPath)
	hooks := settingsData["hooks"].(map[string]interface{})
	var kept []interface{}
	for _, e := range hooks["PreToolUse"].([]interface{}) {
		if !strings.Contains(mustJSONString(t, e), settings.LabdrianApproveGuardIdentity) {
			kept = append(kept, e)
		}
	}
	hooks["PreToolUse"] = kept
	deps := statusDeps{
		stat:         os.Stat,
		readFile:     os.ReadFile,
		loadSettings: func(_ string) (map[string]interface{}, error) { return settingsData, nil },
		home:         func() string { return homeDir },
		cwd:          func() string { return "" },
	}
	var outBuf bytes.Buffer
	allOK, degraded := statusCore(&outBuf, deps)
	out := outBuf.String()
	if !allOK || !degraded {
		t.Errorf("statusCore = (allOK %v, degraded %v), want (true, true); output:\n%s", allOK, degraded, out)
	}
	if !strings.Contains(out, "[WARN] guard: skills approve") || !strings.Contains(out, remediationNote) {
		t.Errorf("expected a [WARN] approve guard line naming the fix; output:\n%s", out)
	}
	if strings.Contains(out, "[FAIL]") {
		t.Errorf("a missing approve guard must never emit [FAIL]; output:\n%s", out)
	}
}

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
