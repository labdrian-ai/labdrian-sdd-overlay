package settings_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings"
)

// shaperGuardEntries returns the PreToolUse entries carrying the shaper
// clearance guard identity, keyed by matcher.
func shaperGuardEntries(t *testing.T, root map[string]interface{}) map[string]map[string]interface{} {
	t.Helper()
	out := map[string]map[string]interface{}{}
	hooks, _ := root["hooks"].(map[string]interface{})
	entries, _ := hooks["PreToolUse"].([]interface{})
	for _, e := range entries {
		em, _ := e.(map[string]interface{})
		if em == nil || !entryContainsBinarySubstring(e, settings.LabdrianShaperGuardIdentity) {
			continue
		}
		matcher, _ := em["matcher"].(string)
		if _, dup := out[matcher]; dup {
			t.Errorf("duplicate shaper guard entry for matcher %q", matcher)
		}
		out[matcher] = em
	}
	return out
}

func denyRules(root map[string]interface{}) []string {
	perms, _ := root["permissions"].(map[string]interface{})
	raw, _ := perms["deny"].([]interface{})
	var out []string
	for _, r := range raw {
		if s, ok := r.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func countString(list []string, s string) int {
	n := 0
	for _, v := range list {
		if v == s {
			n++
		}
	}
	return n
}

func TestInstall_AddsShaperGuardHooksAndDenyRuleIdempotently(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"permissions":{"deny":["Bash(rm -rf /)"],"allow":["Bash(ls)"]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	m := buildMerger(t, path)
	for i := 0; i < 2; i++ {
		if err := m.Install(); err != nil {
			t.Fatalf("Install #%d: %v", i+1, err)
		}
	}
	root := parseJSON(t, path)
	entries := shaperGuardEntries(t, root)
	for _, matcher := range []string{"Bash", settings.ShaperGuardFileToolMatcher} {
		e, ok := entries[matcher]
		if !ok {
			t.Fatalf("no shaper guard entry for matcher %q; entries %v", matcher, entries)
		}
		if !entryContainsBinarySubstring(e, testHookCommand) {
			t.Errorf("guard entry %q does not run %s", matcher, testHookCommand)
		}
	}
	deny := denyRules(root)
	if countString(deny, settings.ShaperClearanceDenyRule) != 1 {
		t.Errorf("deny rules %v, want exactly one %q", deny, settings.ShaperClearanceDenyRule)
	}
	if countString(deny, "Bash(rm -rf /)") != 1 {
		t.Errorf("foreign deny rule lost: %v", deny)
	}
	if !settings.HasShaperClearanceGuard(root, testHookCommand) {
		t.Errorf("HasShaperClearanceGuard = false after Install")
	}

	if err := m.Uninstall(); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	root = parseJSON(t, path)
	if got := shaperGuardEntries(t, root); len(got) != 0 {
		t.Errorf("guard entries survive Uninstall: %v", got)
	}
	deny = denyRules(root)
	if countString(deny, settings.ShaperClearanceDenyRule) != 0 || countString(deny, "Bash(rm -rf /)") != 1 {
		t.Errorf("deny rules after Uninstall = %v", deny)
	}
	perms, _ := root["permissions"].(map[string]interface{})
	if _, ok := perms["allow"]; !ok {
		t.Errorf("foreign permissions.allow lost: %v", root["permissions"])
	}
}

func TestShaperClearanceDenyRuleIsTheDecidedBackstop(t *testing.T) {
	if settings.ShaperClearanceDenyRule != "Bash(*shaper clearance record*)" {
		t.Errorf("ShaperClearanceDenyRule = %q", settings.ShaperClearanceDenyRule)
	}
}

func TestHasShaperClearanceGuard_RequiresEveryPart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := buildMerger(t, path).Install(); err != nil {
		t.Fatal(err)
	}
	full := parseJSON(t, path)
	if !settings.HasSupportedClaudeLifecycleState(full, testHookCommand) {
		t.Fatalf("HasSupportedClaudeLifecycleState = false after a full Install")
	}

	noDeny := parseJSON(t, path)
	noDeny["permissions"] = map[string]interface{}{"deny": []interface{}{}}
	if settings.HasShaperClearanceGuard(noDeny, testHookCommand) || settings.HasSupportedClaudeLifecycleState(noDeny, testHookCommand) {
		t.Errorf("guard reported present without the deny rule")
	}

	noHook := parseJSON(t, path)
	hooks := noHook["hooks"].(map[string]interface{})
	var kept []interface{}
	for _, e := range hooks["PreToolUse"].([]interface{}) {
		if em := e.(map[string]interface{}); em["matcher"] == "Bash" && entryContainsBinarySubstring(e, settings.LabdrianShaperGuardIdentity) {
			continue
		}
		kept = append(kept, e)
	}
	hooks["PreToolUse"] = kept
	if settings.HasShaperClearanceGuard(noHook, testHookCommand) {
		t.Errorf("guard reported present without the Bash hook")
	}

	disabled := parseJSON(t, path)
	disabled["disableAllHooks"] = true
	if settings.HasShaperClearanceGuard(disabled, testHookCommand) {
		t.Errorf("guard reported present with disableAllHooks")
	}
}

// runGuardCommand runs the installed guard command string under sh with
// hookInput on stdin and returns its exit code.
func runGuardCommand(t *testing.T, command, hookInput string) (int, string) {
	t.Helper()
	cmd := exec.Command("sh", "-c", command)
	cmd.Stdin = strings.NewReader(hookInput)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return 0, string(out)
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode(), string(out)
	}
	t.Fatalf("run guard command: %v", err)
	return -1, ""
}

func TestShaperGuardCommand_MissingBinaryStillDeniesMarkersButAllowsOthers(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skipf("sh unavailable: %v", err)
	}
	missing := filepath.Join(t.TempDir(), "gentle-ai-overlay")
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := settings.NewMerger(path, missing).Install(); err != nil {
		t.Fatal(err)
	}
	entries := shaperGuardEntries(t, parseJSON(t, path))
	command := extractCommand(t, entries["Bash"])
	for _, tc := range []struct {
		input string
		want  int
	}{
		{`{"tool_name":"Bash","tool_input":{"command":"gentle-ai-overlay shaper clearance record --stdin"}}`, 2},
		{`{"tool_name":"Write","tool_input":{"file_path":"/s/labdrian/shaper-clearance/p/g/x.json"}}`, 2},
		{`{"tool_name":"Bash","tool_input":{"command":"ls"}}`, 0},
	} {
		if code, out := runGuardCommand(t, command, tc.input); code != tc.want {
			t.Errorf("missing binary, input %s: exit %d (%s), want %d", tc.input, code, out, tc.want)
		}
	}
}

func TestShaperGuardCommand_PresentBinaryExitCodeIsUnmasked(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skipf("sh unavailable: %v", err)
	}
	bin := filepath.Join(t.TempDir(), "gentle-ai-overlay")
	script := "#!/bin/sh\n[ \"$1 $2\" = \"shaper guard-hook\" ] || exit 9\ncat >/dev/null\nexit 2\n"
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := settings.NewMerger(path, bin).Install(); err != nil {
		t.Fatal(err)
	}
	command := extractCommand(t, shaperGuardEntries(t, parseJSON(t, path))["Bash"])
	if code, out := runGuardCommand(t, command, `{"tool_name":"Bash","tool_input":{"command":"ls"}}`); code != 2 {
		t.Errorf("present binary exit 2 was masked: got %d (%s)", code, out)
	}
}
