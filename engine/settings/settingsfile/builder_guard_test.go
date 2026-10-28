package settingsfile_test

// Tests for the builder guard family: the one PreToolUse entry on the Agent tool that runs 'roles builder-guard', which
// denies launching the builder agent when its prompt cites no task the shaper designed (own-rdd T11d, S14 g). It is owned,
// repaired and removed the way the approve guard family is, and it shares the Agent matcher with the entries of gate-task
// (the minimalism and design families) without ever reading past their identity. Everything runs against temp files; the
// real ~/.claude/settings.json is never touched.

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
)

// builderGuardEntries returns every entry of the PreToolUse list that carries the identity of the family and the test
// binary, in file order.
func builderGuardEntries(root map[string]interface{}, bin string) []map[string]interface{} {
	var out []map[string]interface{}
	hooks, _ := root["hooks"].(map[string]interface{})
	entries, _ := hooks["PreToolUse"].([]interface{})
	for _, e := range entries {
		if em, _ := e.(map[string]interface{}); em != nil && entryContainsBinarySubstring(e, bin) && entryContainsBinarySubstring(e, settings.LabdrianBuilderGuardIdentity) {
			out = append(out, em)
		}
	}
	return out
}

// agentEntries returns every entry of the PreToolUse list whose matcher is Agent, in file order.
func agentEntries(root map[string]interface{}) []interface{} {
	var out []interface{}
	hooks, _ := root["hooks"].(map[string]interface{})
	entries, _ := hooks["PreToolUse"].([]interface{})
	for _, e := range entries {
		if em, _ := e.(map[string]interface{}); em != nil && em["matcher"] == "Agent" {
			out = append(out, e)
		}
	}
	return out
}

// builderGuardCommand is the command of the entry, which a test reads back from the file.
func builderGuardCommand(t *testing.T, entry map[string]interface{}) string {
	t.Helper()
	inner, _ := entry["hooks"].([]interface{})
	if len(inner) != 1 {
		t.Fatalf("the entry holds %d hooks, want 1: %v", len(inner), entry)
	}
	command, _ := inner[0].(map[string]interface{})["command"].(string)
	return command
}

func TestInstall_AddsTheBuilderGuard(t *testing.T) {
	path, _ := installFresh(t)
	root := parseJSON(t, path)

	got := builderGuardEntries(root, testHookCommand)

	if len(got) != 1 {
		t.Fatalf("builder guard entries = %d, want 1: %v", len(got), got)
	}
	if got[0]["matcher"] != "Agent" {
		t.Errorf("the entry matcher is %v, want Agent", got[0]["matcher"])
	}
	if want := "command -v " + testHookCommand + " >/dev/null 2>&1 && " + testHookCommand + " roles builder-guard || true"; builderGuardCommand(t, got[0]) != want {
		t.Errorf("command %q, want the guarded never-blocking form %q", builderGuardCommand(t, got[0]), want)
	}
	if !settings.HasBuilderGuard(root, testHookCommand) {
		t.Errorf("the guard is not reported in place after Install: missing %v", settings.MissingBuilderGuardParts(root, testHookCommand))
	}
}

func TestBuilderGuardEntriesAreExactlyWhatInstallWrites(t *testing.T) {
	path, _ := installFresh(t)
	installed := builderGuardEntries(parseJSON(t, path), testHookCommand)

	built := settings.BuilderGuardEntries(testHookCommand)

	if len(built) != 1 || len(built["PreToolUse"]) != 1 || len(installed) != 1 || mustJSON(t, built["PreToolUse"][0]) != mustJSON(t, installed[0]) {
		t.Errorf("BuilderGuardEntries = %v, Install wrote %v", built, installed)
	}
}

// The guard's entry is appended last to the installed families, so it sits last on the PreToolUse list of a fresh
// install, and the Agent matcher holds the entries of gate-task (minimalism, design) and the guard, each once.
func TestInstall_PutsTheBuilderGuardLastBesideTheGateEntriesOfTheAgentMatcher(t *testing.T) {
	path, _ := installFresh(t)
	root := parseJSON(t, path)

	entries := root["hooks"].(map[string]interface{})["PreToolUse"].([]interface{})
	last, _ := entries[len(entries)-1].(map[string]interface{})
	if last == nil || !entryContainsBinarySubstring(last, settings.LabdrianBuilderGuardIdentity) {
		t.Errorf("the last PreToolUse entry is %s, want the builder guard's", mustJSON(t, entries[len(entries)-1]))
	}
	agents := agentEntries(root)
	if len(agents) != 3 {
		t.Fatalf("Agent entries = %d, want 3 (minimalism, design, builder guard): %s", len(agents), mustJSON(t, agents))
	}
	for _, identity := range []string{settings.LabdrianMinimalismIdentity, settings.LabdrianDesignIdentity, settings.LabdrianBuilderGuardIdentity} {
		n := 0
		for _, e := range agents {
			if entryContainsBinarySubstring(e, identity) {
				n++
			}
		}
		if n != 1 {
			t.Errorf("Agent entries that carry %q = %d, want 1", identity, n)
		}
	}
}

// The command runs under sh as Claude Code runs it, and nothing it does can block the call: a missing binary is a silent
// no-op that lets the launch through, and a binary that fails with the exit code that blocks leaves the exit at 0 with its
// answer on stdout.
func TestBuilderGuardCommand_RunsUnderShWithoutEverBlocking(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skipf("sh unavailable: %v", err)
	}
	install := func(t *testing.T, bin string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "settings.json")
		if err := newSettingsInstaller(path, bin).Install(); err != nil {
			t.Fatal(err)
		}
		entries := builderGuardEntries(parseJSON(t, path), bin)
		if len(entries) != 1 {
			t.Fatalf("builder guard entries = %d, want 1", len(entries))
		}
		return builderGuardCommand(t, entries[0])
	}
	const agentInput = `{"hook_event_name":"PreToolUse","tool_name":"Agent","cwd":"/work","tool_input":{"subagent_type":"builder","prompt":"x"}}`

	t.Run("a missing binary is a silent no-op", func(t *testing.T) {
		command := install(t, filepath.Join(t.TempDir(), "gentle-ai-overlay"))
		if code, out := runGuardCommand(t, command, agentInput); code != 0 || out != "" {
			t.Errorf("missing binary: exit %d, output %q, want a silent exit 0", code, out)
		}
	})

	t.Run("the present binary's answer reaches Claude Code and its failures cannot block", func(t *testing.T) {
		bin := filepath.Join(t.TempDir(), "gentle-ai-overlay")
		// Checks it is run as 'roles builder-guard', prints a marker, and fails with the exit code that blocks a call.
		script := "#!/bin/sh\n[ \"$1 $2\" = \"roles builder-guard\" ] || exit 9\ncat >/dev/null\nprintf '{\"marker\":true}\\n'\nexit 2\n"
		if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
		code, out := runGuardCommand(t, install(t, bin), agentInput)
		if code != 0 {
			t.Errorf("exit %d (%s): a hook that fails must not block the call it serves", code, out)
		}
		if !strings.Contains(out, `{"marker":true}`) {
			t.Errorf("output %q lost the hook's answer", out)
		}
	})
}

// builderGuardForeign holds PreToolUse entries of other programs on the Agent matcher: one with nothing to do with ours,
// one that runs our binary with another verb, and one that mentions the identity while running another program.
func builderGuardForeign() []interface{} {
	agent := func(command string) interface{} {
		return map[string]interface{}{"matcher": "Agent", "hooks": []interface{}{map[string]interface{}{"type": "command", "command": command, "timeout": float64(5)}}}
	}
	return []interface{}{
		agent("other-tool agent-hook"),
		agent(testHookCommand + " other verb"),
		agent("/opt/other/tool roles builder-guard"),
	}
}

func TestBuilderGuardFamilyPreservesForeignAgentEntriesByteForByte(t *testing.T) {
	foreign := builderGuardForeign()
	path := writeSettingsDoc(t, map[string]interface{}{
		"model": "opus",
		"hooks": map[string]interface{}{"PreToolUse": foreign, "Notification": []interface{}{map[string]interface{}{"hooks": []interface{}{}}}},
	})
	m := installerFor(t, path)
	check := func(stage string, wantOurs int) {
		t.Helper()
		root := parseJSON(t, path)
		have := map[string]bool{}
		for _, e := range root["hooks"].(map[string]interface{})["PreToolUse"].([]interface{}) {
			have[mustJSON(t, e)] = true
		}
		for _, f := range foreign {
			if !have[mustJSON(t, f)] {
				t.Errorf("%s: foreign Agent entry lost or altered: %s", stage, mustJSON(t, f))
			}
		}
		if root["model"] != "opus" || root["hooks"].(map[string]interface{})["Notification"] == nil {
			t.Errorf("%s: foreign keys lost: %v", stage, root)
		}
		if n := len(builderGuardEntries(root, testHookCommand)); n != wantOurs {
			t.Errorf("%s: builder guard entries = %d, want %d", stage, n, wantOurs)
		}
	}

	if err := m.Install(); err != nil {
		t.Fatal(err)
	}
	check("after Install", 1)
	first := readFileBytes(t, path)
	if err := m.Install(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, readFileBytes(t, path)) {
		t.Error("a second Install changed the file")
	}
	if err := m.Uninstall(); err != nil {
		t.Fatal(err)
	}
	check("after Uninstall", 0)
	if agents := agentEntries(parseJSON(t, path)); len(agents) != len(foreign) {
		t.Errorf("Agent entries after Uninstall = %d, want only the %d foreign ones: %s", len(agents), len(foreign), mustJSON(t, agents))
	}
}

// An install made before the guard existed gains it when Install runs again, with the entries of gate-task on the same
// matcher and every other entry exactly as they were.
func TestInstall_AddsTheBuilderGuardToAnOlderInstall(t *testing.T) {
	path, m := installFresh(t)
	root := parseJSON(t, path)
	hooks := root["hooks"].(map[string]interface{})
	var kept []interface{}
	for _, e := range hooks["PreToolUse"].([]interface{}) {
		if !entryContainsBinarySubstring(e, settings.LabdrianBuilderGuardIdentity) {
			kept = append(kept, e)
		}
	}
	hooks["PreToolUse"] = kept
	b, _ := json.Marshal(root)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if settings.HasBuilderGuard(parseJSON(t, path), testHookCommand) || settings.HasSupportedClaudeLifecycleState(parseJSON(t, path), testHookCommand) {
		t.Fatal("an install without the guard reported it, or the whole lifecycle, in place")
	}
	before := parseJSON(t, path)

	if err := m.Install(); err != nil {
		t.Fatal(err)
	}

	after := parseJSON(t, path)
	if !settings.HasBuilderGuard(after, testHookCommand) || !settings.HasSupportedClaudeLifecycleState(after, testHookCommand) {
		t.Fatalf("re-running Install did not put the guard in place: missing %v", settings.MissingBuilderGuardParts(after, testHookCommand))
	}
	for _, e := range agentEntries(before) {
		found := false
		for _, a := range agentEntries(after) {
			found = found || reflect.DeepEqual(e, a)
		}
		if !found {
			t.Errorf("the Agent entry %s of the older install changed", mustJSON(t, e))
		}
	}
	afterHooks := after["hooks"].(map[string]interface{})
	var withoutGuard []interface{}
	for _, e := range afterHooks["PreToolUse"].([]interface{}) {
		if !entryContainsBinarySubstring(e, settings.LabdrianBuilderGuardIdentity) {
			withoutGuard = append(withoutGuard, e)
		}
	}
	afterHooks["PreToolUse"] = withoutGuard
	if !reflect.DeepEqual(before, after) {
		t.Errorf("Install changed something else than the guard's entry:\nbefore: %s\n after: %s", mustJSON(t, before), mustJSON(t, after))
	}
}

// withBuilderGuardFamily adds exactly the entry Install writes for the guard to a hand-built root and returns root.
func withBuilderGuardFamily(root map[string]interface{}, hookCommand string) map[string]interface{} {
	hooks := root["hooks"].(map[string]interface{})
	existing, _ := hooks["PreToolUse"].([]interface{})
	hooks["PreToolUse"] = append(existing, settings.BuilderGuardEntries(hookCommand)["PreToolUse"]...)
	return root
}
