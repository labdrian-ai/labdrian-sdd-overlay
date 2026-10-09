package settingsfile_test

// Tests for the approve guard family: the two PreToolUse entries that run
// 'skills guard-hook', which denies the agent running `skills approve` and
// writing the approval record. The family is a speed bump, not a security
// boundary, and is owned, repaired, and removed the way the projection family
// is. Everything runs against temp files; the real ~/.claude/settings.json is
// never touched.

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// approveGuardEntry is one owned approve guard entry read back from settings.
type approveGuardEntry struct {
	matcher string
	command string
	raw     map[string]interface{}
}

// approveGuardEntries returns every entry carrying the approve guard identity
// and the test binary, in file order.
func approveGuardEntries(root map[string]interface{}) []approveGuardEntry {
	var out []approveGuardEntry
	hooks, _ := root["hooks"].(map[string]interface{})
	for _, key := range []string{"UserPromptSubmit", "PreToolUse", "SessionEnd", "Stop"} {
		entries, _ := hooks[key].([]interface{})
		for _, e := range entries {
			em, _ := e.(map[string]interface{})
			if em == nil || !entryContainsBinarySubstring(e, testHookCommand) || !entryContainsBinarySubstring(e, settings.LabdrianApproveGuardIdentity) {
				continue
			}
			matcher, _ := em["matcher"].(string)
			var command string
			if inner, _ := em["hooks"].([]interface{}); len(inner) == 1 {
				ih, _ := inner[0].(map[string]interface{})
				command, _ = ih["command"].(string)
			}
			out = append(out, approveGuardEntry{matcher: matcher, command: command, raw: em})
		}
	}
	return out
}

func TestInstall_AddsTheApproveGuardFamily(t *testing.T) {
	path, _ := installFresh(t)
	root := parseJSON(t, path)
	got := approveGuardEntries(root)
	if len(got) != 2 {
		t.Fatalf("approve guard entries = %d, want 2: %+v", len(got), got)
	}
	for i, wantMatcher := range []string{settings.ApproveGuardBashMatcher, settings.ApproveGuardFileToolMatcher} {
		e := got[i]
		if e.matcher != wantMatcher {
			t.Errorf("entry %d matcher = %q, want %q", i, e.matcher, wantMatcher)
		}
		if want := testHookCommand + " skills guard-hook"; !strings.Contains(e.command, want) {
			t.Errorf("entry %d command %q does not run %q", i, e.command, want)
		}
	}
	// Both are PreToolUse entries: the guard decides before a tool runs.
	pre := root["hooks"].(map[string]interface{})["PreToolUse"].([]interface{})
	n := 0
	for _, e := range pre {
		if entryContainsBinarySubstring(e, settings.LabdrianApproveGuardIdentity) {
			n++
		}
	}
	if n != 2 {
		t.Errorf("PreToolUse holds %d approve guard entries, want 2", n)
	}
	if !settings.HasApproveGuard(root, testHookCommand) || !settings.HasSupportedClaudeLifecycleState(root, testHookCommand) {
		t.Errorf("the family is not reported in place after Install: missing %v", settings.MissingApproveGuardParts(root, testHookCommand))
	}
}

func TestApproveGuardMatchersAreTheDocumentedOnes(t *testing.T) {
	if settings.ApproveGuardBashMatcher != "Bash" {
		t.Errorf("ApproveGuardBashMatcher = %q", settings.ApproveGuardBashMatcher)
	}
	if settings.ApproveGuardFileToolMatcher != "Write|Edit|MultiEdit|NotebookEdit" {
		t.Errorf("ApproveGuardFileToolMatcher = %q", settings.ApproveGuardFileToolMatcher)
	}
	// The hook decides on the tools listed in engine/skills; the matcher that
	// routes calls to it must list exactly those, or a tool the guard knows
	// would never reach it (or the hook would run for a tool it ignores).
	if want := strings.Join(skills.ApproveGuardFileTools(), "|"); settings.ApproveGuardFileToolMatcher != want {
		t.Errorf("ApproveGuardFileToolMatcher = %q, but the guard watches %q", settings.ApproveGuardFileToolMatcher, want)
	}
}

func TestApproveGuardIdentityIsItsOwn(t *testing.T) {
	// Each family is told apart by binary path plus its own token, so no token
	// may be a piece of another.
	ids := []string{
		settings.LabdrianApproveGuardIdentity, settings.LabdrianProjectionIdentity, settings.LabdrianShaperGuardIdentity,
		settings.LabdrianReviewReceiptIdentity, settings.LabdrianSyncTriggerIdentity, settings.LabdrianMinimalismIdentity,
		settings.LabdrianDesignIdentity,
	}
	for i, a := range ids {
		for j, b := range ids {
			if i != j && strings.Contains(a, b) {
				t.Errorf("identity %q contains identity %q", a, b)
			}
		}
	}
}

// The command decides by JSON output and exit 0, never by exit code 2, and a
// missing binary is a no-op.
//
// MISSING-BINARY DECISION (unlike the shaper clearance guard, which falls back
// to a text match and still denies): with the binary gone this guard allows
// everything. A fallback could only match the raw hook JSON for "skills
// approve", and that phrase is in the content of documentation, tests, and help
// text the agent legitimately writes, so a fail-closed fallback would deny
// ordinary edits whenever the binary is missing. And the entry points cannot
// approve without the binary: the labdrian wrapper stops when
// ~/.claude/bin/gentle-ai-overlay is not executable, which is the very path the
// hook runs.
func TestApproveGuardCommandNeverBlocksAndSkipsAMissingBinary(t *testing.T) {
	path, _ := installFresh(t)
	for _, e := range approveGuardEntries(parseJSON(t, path)) {
		want := "command -v " + testHookCommand + " >/dev/null 2>&1 && " + testHookCommand + " skills guard-hook || true"
		if e.command != want {
			t.Errorf("command %q, want the guarded never-blocking form %q", e.command, want)
		}
	}
}

func TestApproveGuardCommand_RunsUnderShWithoutEverBlocking(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skipf("sh unavailable: %v", err)
	}
	const approveCall = `{"tool_name":"Bash","tool_input":{"command":"labdrian skills approve --id x --approver y"}}`
	install := func(t *testing.T, bin string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "settings.json")
		if err := newSettingsInstaller(path, bin).Install(); err != nil {
			t.Fatal(err)
		}
		for _, e := range approveGuardEntriesFor(parseJSON(t, path), bin) {
			if e.matcher == settings.ApproveGuardBashMatcher {
				return e.command
			}
		}
		t.Fatal("no Bash approve guard entry")
		return ""
	}

	t.Run("a missing binary is a silent no-op even for an approve call", func(t *testing.T) {
		command := install(t, filepath.Join(t.TempDir(), "gentle-ai-overlay"))
		if code, out := runGuardCommand(t, command, approveCall); code != 0 || out != "" {
			t.Errorf("missing binary: exit %d, output %q, want a silent exit 0", code, out)
		}
	})

	t.Run("the present binary's answer reaches Claude Code and its failures cannot block", func(t *testing.T) {
		bin := filepath.Join(t.TempDir(), "gentle-ai-overlay")
		// Checks it is run as 'skills guard-hook', prints a marker, and fails with
		// the exit code that blocks a call.
		script := "#!/bin/sh\n[ \"$1 $2\" = \"skills guard-hook\" ] || exit 9\ncat >/dev/null\nprintf '{\"marker\":true}\\n'\nexit 2\n"
		if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
		code, out := runGuardCommand(t, install(t, bin), approveCall)
		if code != 0 {
			t.Errorf("exit %d (%s): a hook that fails must not block the call it serves", code, out)
		}
		if !strings.Contains(out, `{"marker":true}`) {
			t.Errorf("output %q lost the hook's answer", out)
		}
	})
}

// approveGuardEntriesFor is approveGuardEntries for a settings file written
// with another binary path.
func approveGuardEntriesFor(root map[string]interface{}, bin string) []approveGuardEntry {
	var out []approveGuardEntry
	hooks, _ := root["hooks"].(map[string]interface{})
	entries, _ := hooks["PreToolUse"].([]interface{})
	for _, e := range entries {
		em, _ := e.(map[string]interface{})
		if em == nil || !entryContainsBinarySubstring(e, bin) || !entryContainsBinarySubstring(e, settings.LabdrianApproveGuardIdentity) {
			continue
		}
		matcher, _ := em["matcher"].(string)
		inner, _ := em["hooks"].([]interface{})
		ih, _ := inner[0].(map[string]interface{})
		command, _ := ih["command"].(string)
		out = append(out, approveGuardEntry{matcher: matcher, command: command, raw: em})
	}
	return out
}

func TestInstall_ApproveGuardFamilyIsIdempotent(t *testing.T) {
	path, m := installFresh(t)
	first := readFileBytes(t, path)
	if err := m.Install(); err != nil {
		t.Fatal(err)
	}
	if second := readFileBytes(t, path); !bytes.Equal(first, second) {
		t.Errorf("second Install changed the file")
	}
	if n := len(approveGuardEntries(parseJSON(t, path))); n != 2 {
		t.Errorf("approve guard entries after two installs = %d, want 2", n)
	}
}

// approveGuardForeign holds entries that share our event key, our binary path,
// and even our matchers, but carry no approve guard identity, next to entries
// that mention the identity while running another program.
func approveGuardForeign() (string, []interface{}) {
	pre := []interface{}{
		map[string]interface{}{"matcher": "Bash", "hooks": []interface{}{map[string]interface{}{"type": "command", "command": "/opt/other/bash-guard", "timeout": float64(3)}}},
		map[string]interface{}{"matcher": settings.ApproveGuardFileToolMatcher, "hooks": []interface{}{map[string]interface{}{"type": "command", "command": "/opt/other/edit-guard"}}},
		map[string]interface{}{"matcher": "Bash", "hooks": []interface{}{map[string]interface{}{"type": "command", "command": testHookCommand + " other verb"}}},
		map[string]interface{}{"matcher": "Bash", "hooks": []interface{}{map[string]interface{}{"type": "command", "command": "/opt/other/tool skills guard-hook"}}},
	}
	doc := map[string]interface{}{
		"model": "opus",
		"hooks": map[string]interface{}{"PreToolUse": pre, "Notification": []interface{}{map[string]interface{}{"hooks": []interface{}{}}}},
	}
	b, _ := json.Marshal(doc)
	return string(b), pre
}

func TestApproveGuardFamilyPreservesForeignEntriesByteForByte(t *testing.T) {
	doc, pre := approveGuardForeign()
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	m := installerFor(t, path)
	check := func(stage string) {
		t.Helper()
		root := parseJSON(t, path)
		have := map[string]bool{}
		for _, e := range root["hooks"].(map[string]interface{})["PreToolUse"].([]interface{}) {
			have[mustJSON(t, e)] = true
		}
		for _, f := range pre {
			if !have[mustJSON(t, f)] {
				t.Errorf("%s: foreign PreToolUse entry lost or altered: %s", stage, mustJSON(t, f))
			}
		}
		if root["model"] != "opus" || root["hooks"].(map[string]interface{})["Notification"] == nil {
			t.Errorf("%s: foreign keys lost: %v", stage, root)
		}
	}
	if err := m.Install(); err != nil {
		t.Fatal(err)
	}
	check("after Install")
	if n := len(approveGuardEntries(parseJSON(t, path))); n != 2 {
		t.Errorf("approve guard entries = %d, want 2 next to same-matcher foreign entries", n)
	}
	if err := m.Uninstall(); err != nil {
		t.Fatal(err)
	}
	check("after Uninstall")
	if n := len(approveGuardEntries(parseJSON(t, path))); n != 0 {
		t.Errorf("approve guard entries survive Uninstall: %d", n)
	}
}

func TestUninstall_RemovesTheApproveGuardFamilyAndOnlyIt(t *testing.T) {
	path, m := installFresh(t)
	if err := m.Uninstall(); err != nil {
		t.Fatal(err)
	}
	after := parseJSON(t, path)
	if n := len(approveGuardEntries(after)); n != 0 {
		t.Errorf("approve guard entries left: %d", n)
	}
	// With every family gone the event keys are removed, not left empty.
	if hooks, _ := after["hooks"].(map[string]interface{}); hooks["PreToolUse"] != nil {
		t.Errorf("PreToolUse left behind: %s", mustJSON(t, hooks["PreToolUse"]))
	}

	// Removing just this family's entries by hand leaves the others in place, so
	// the identity does not reach into a sibling family.
	path, _ = installFresh(t)
	root := parseJSON(t, path)
	hooks := root["hooks"].(map[string]interface{})
	var kept []interface{}
	for _, e := range hooks["PreToolUse"].([]interface{}) {
		if !entryContainsBinarySubstring(e, settings.LabdrianApproveGuardIdentity) {
			kept = append(kept, e)
		}
	}
	if len(kept) != len(hooks["PreToolUse"].([]interface{}))-2 {
		t.Fatalf("test bug: expected to drop exactly the two approve guard entries")
	}
	if !settings.HasProjectionHooks(root, testHookCommand) || !settings.HasShaperClearanceGuard(root, testHookCommand) {
		t.Fatalf("test bug: the sibling families must be in place before the check")
	}
	hooks["PreToolUse"] = kept
	if !settings.HasProjectionHooks(root, testHookCommand) || !settings.HasShaperClearanceGuard(root, testHookCommand) {
		t.Errorf("dropping the approve guard entries disturbed a sibling family")
	}
	if settings.HasApproveGuard(root, testHookCommand) {
		t.Errorf("HasApproveGuard = true without its entries")
	}
}

func TestHasApproveGuard_ReportsMissingDriftedAndDisabledParts(t *testing.T) {
	path, _ := installFresh(t)
	full := parseJSON(t, path)
	if parts := settings.MissingApproveGuardParts(full, testHookCommand); len(parts) != 0 {
		t.Fatalf("missing parts after Install = %v", parts)
	}

	mutate := func(fn func(root, hooks map[string]interface{})) map[string]interface{} {
		root := parseJSON(t, path)
		fn(root, root["hooks"].(map[string]interface{}))
		return root
	}
	dropMatcher := func(hooks map[string]interface{}, matcher string) {
		var kept []interface{}
		for _, e := range hooks["PreToolUse"].([]interface{}) {
			em := e.(map[string]interface{})
			if entryContainsBinarySubstring(e, settings.LabdrianApproveGuardIdentity) && em["matcher"] == matcher {
				continue
			}
			kept = append(kept, e)
		}
		hooks["PreToolUse"] = kept
	}
	ourEntries := func(hooks map[string]interface{}) []map[string]interface{} {
		var out []map[string]interface{}
		for _, e := range hooks["PreToolUse"].([]interface{}) {
			if entryContainsBinarySubstring(e, settings.LabdrianApproveGuardIdentity) {
				out = append(out, e.(map[string]interface{}))
			}
		}
		return out
	}
	cases := []struct {
		name string
		root map[string]interface{}
		want string
	}{
		{"bash entry removed", mutate(func(_, h map[string]interface{}) { dropMatcher(h, "Bash") }), `matcher="Bash"`},
		{"file tool entry removed", mutate(func(_, h map[string]interface{}) { dropMatcher(h, settings.ApproveGuardFileToolMatcher) }), settings.ApproveGuardFileToolMatcher},
		{"file tool matcher narrowed", mutate(func(_, h map[string]interface{}) {
			for _, e := range ourEntries(h) {
				if e["matcher"] == settings.ApproveGuardFileToolMatcher {
					e["matcher"] = "Write|Edit"
				}
			}
		}), "drifted"},
		{"command edited", mutate(func(_, h map[string]interface{}) {
			ih := ourEntries(h)[0]["hooks"].([]interface{})[0].(map[string]interface{})
			ih["command"] = ih["command"].(string) + " --extra"
		}), "drifted"},
		{"command without the never-blocking guard", mutate(func(_, h map[string]interface{}) {
			ih := ourEntries(h)[0]["hooks"].([]interface{})[0].(map[string]interface{})
			ih["command"] = testHookCommand + " skills guard-hook"
		}), "drifted"},
		{"an entry duplicated", mutate(func(_, h map[string]interface{}) {
			h["PreToolUse"] = append(h["PreToolUse"].([]interface{}), ourEntries(h)[0])
		}), "drifted"},
		{"hooks disabled", mutate(func(root, _ map[string]interface{}) { root["disableAllHooks"] = true }), "disableAllHooks"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if settings.HasApproveGuard(c.root, testHookCommand) {
				t.Error("HasApproveGuard = true")
			}
			if settings.HasSupportedClaudeLifecycleState(c.root, testHookCommand) {
				t.Error("lifecycle state reported supported")
			}
			if joined := strings.Join(settings.MissingApproveGuardParts(c.root, testHookCommand), "|"); !strings.Contains(joined, c.want) {
				t.Errorf("missing parts %q do not name %q", joined, c.want)
			}
		})
	}
	if settings.HasApproveGuard(map[string]interface{}{}, testHookCommand) {
		t.Errorf("empty settings reported as having the approve guard")
	}
}

func TestInstall_RepairsADriftedApproveGuardWithoutDuplicates(t *testing.T) {
	path, m := installFresh(t)
	root := parseJSON(t, path)
	hooks := root["hooks"].(map[string]interface{})
	pre := hooks["PreToolUse"].([]interface{})
	var dup interface{}
	for _, e := range pre {
		em := e.(map[string]interface{})
		if !entryContainsBinarySubstring(e, settings.LabdrianApproveGuardIdentity) {
			continue
		}
		if em["matcher"] == settings.ApproveGuardFileToolMatcher {
			em["matcher"] = "Write" // narrower than the guard
		} else {
			dup = e
		}
	}
	hooks["PreToolUse"] = append(pre, dup) // a duplicated Bash entry is drift too
	b, _ := json.Marshal(root)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := m.Install(); err != nil {
		t.Fatal(err)
	}
	after := parseJSON(t, path)
	if !settings.HasApproveGuard(after, testHookCommand) {
		t.Fatalf("still drifted: %v", settings.MissingApproveGuardParts(after, testHookCommand))
	}
	if n := len(approveGuardEntries(after)); n != 2 {
		t.Errorf("approve guard entries after repair = %d, want 2", n)
	}
}

func TestInstall_AddsTheApproveGuardToAnOlderInstall(t *testing.T) {
	path, m := installFresh(t)
	root := parseJSON(t, path)
	hooks := root["hooks"].(map[string]interface{})
	var kept []interface{}
	for _, e := range hooks["PreToolUse"].([]interface{}) {
		if !entryContainsBinarySubstring(e, settings.LabdrianApproveGuardIdentity) {
			kept = append(kept, e)
		}
	}
	hooks["PreToolUse"] = kept
	b, _ := json.Marshal(root)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if settings.HasSupportedClaudeLifecycleState(parseJSON(t, path), testHookCommand) {
		t.Fatal("an install without the approve guard reported supported")
	}
	before := parseJSON(t, path)
	if err := m.Install(); err != nil {
		t.Fatal(err)
	}
	after := parseJSON(t, path)
	if !settings.HasSupportedClaudeLifecycleState(after, testHookCommand) {
		t.Fatalf("re-running Install did not complete the state")
	}
	have := map[string]bool{}
	for _, e := range after["hooks"].(map[string]interface{})["PreToolUse"].([]interface{}) {
		have[mustJSON(t, e)] = true
	}
	for _, e := range before["hooks"].(map[string]interface{})["PreToolUse"].([]interface{}) {
		if !have[mustJSON(t, e)] {
			t.Errorf("older PreToolUse entry changed: %s", mustJSON(t, e))
		}
	}
	if reflect.DeepEqual(before, after) {
		t.Errorf("Install changed nothing")
	}
}

// An empty hook command claims nothing: there is no binary path to tell our
// entries from a foreign one that mentions the identity.
func TestApproveGuardWithAnEmptyHookCommandClaimsNothing(t *testing.T) {
	doc := map[string]interface{}{"hooks": map[string]interface{}{
		"PreToolUse": []interface{}{map[string]interface{}{"matcher": "Bash", "hooks": []interface{}{map[string]interface{}{"type": "command", "command": "/opt/other/tool skills guard-hook"}}}},
	}}
	parts := settings.MissingApproveGuardParts(doc, "")
	if len(parts) != 2 || strings.Contains(strings.Join(parts, "|"), "drifted") {
		t.Errorf("parts = %v, want the two entries missing and none adopted as drifted", parts)
	}
	if settings.HasApproveGuard(doc, "") {
		t.Error("HasApproveGuard = true for an empty hook command")
	}
	path := writeSettingsDoc(t, doc)
	before := readFileBytes(t, path)
	for name, act := range map[string]func(*settingsInstaller) error{
		"Install":   func(m *settingsInstaller) error { return m.Install() },
		"Uninstall": func(m *settingsInstaller) error { return m.Uninstall() },
	} {
		if err := act(newSettingsInstaller(path, "")); err == nil {
			t.Errorf("%s with an empty hook command returned nil, want an error", name)
		}
		if !bytes.Equal(readFileBytes(t, path), before) {
			t.Errorf("%s changed the file", name)
		}
	}
}

func TestMissingApproveGuardPartsToleratesMalformedSettings(t *testing.T) {
	want := settings.MissingApproveGuardParts(map[string]interface{}{}, testHookCommand)
	if len(want) != 2 {
		t.Fatalf("test bug: empty settings report %v, want the two entries", want)
	}
	for name, root := range malformedHooks() {
		t.Run(name, func(t *testing.T) {
			if got := settings.MissingApproveGuardParts(root, testHookCommand); !reflect.DeepEqual(got, want) {
				t.Errorf("parts = %v, want %v", got, want)
			}
		})
	}
}

func TestApproveGuardHookEntriesAreExactlyWhatInstallWrites(t *testing.T) {
	path, _ := installFresh(t)
	installed := approveGuardEntries(parseJSON(t, path))
	built := settings.ApproveGuardHookEntries(testHookCommand)
	if len(built) != 1 || len(built["PreToolUse"]) != len(installed) {
		t.Fatalf("ApproveGuardHookEntries = %v, want the %d PreToolUse entries Install writes", built, len(installed))
	}
	for i, e := range installed {
		if mustJSON(t, built["PreToolUse"][i]) != mustJSON(t, e.raw) {
			t.Errorf("entry %d: built %s, installed %s", i, mustJSON(t, built["PreToolUse"][i]), mustJSON(t, e.raw))
		}
	}
}

// withApproveGuardFamily adds exactly the approve guard entries Install writes
// to a hand-built root and returns root.
func withApproveGuardFamily(root map[string]interface{}, hookCommand string) map[string]interface{} {
	hooks := root["hooks"].(map[string]interface{})
	existing, _ := hooks["PreToolUse"].([]interface{})
	hooks["PreToolUse"] = append(existing, settings.ApproveGuardHookEntries(hookCommand)["PreToolUse"]...)
	return root
}
