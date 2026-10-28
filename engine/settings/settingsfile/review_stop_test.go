package settingsfile_test

// Tests for the review stop-hook family: the one Stop entry that runs 'review stop-hook', which blocks the stop of an
// agent whose branch has a commit no review covers (own-rdd T6a, S6, L18). It is owned, repaired and removed the way the
// approve guard family is, and the Stop entries of other programs are never read past their identity. Everything runs
// against temp files; the real ~/.claude/settings.json is never touched.

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

// reviewStopEntries returns every entry of the Stop list that carries the identity of the family and the test binary, in
// file order.
func reviewStopEntries(root map[string]interface{}, bin string) []map[string]interface{} {
	var out []map[string]interface{}
	hooks, _ := root["hooks"].(map[string]interface{})
	entries, _ := hooks["Stop"].([]interface{})
	for _, e := range entries {
		if em, _ := e.(map[string]interface{}); em != nil && entryContainsBinarySubstring(e, bin) && entryContainsBinarySubstring(e, settings.LabdrianReviewStopIdentity) {
			out = append(out, em)
		}
	}
	return out
}

// reviewStopCommand is the command of the entry, which a test reads back from the file.
func reviewStopCommand(t *testing.T, entry map[string]interface{}) string {
	t.Helper()
	inner, _ := entry["hooks"].([]interface{})
	if len(inner) != 1 {
		t.Fatalf("the entry holds %d hooks, want 1: %v", len(inner), entry)
	}
	command, _ := inner[0].(map[string]interface{})["command"].(string)
	return command
}

func TestInstall_AddsTheReviewStopHook(t *testing.T) {
	path, _ := installFresh(t)
	root := parseJSON(t, path)

	got := reviewStopEntries(root, testHookCommand)

	if len(got) != 1 {
		t.Fatalf("review stop-hook entries = %d, want 1: %v", len(got), got)
	}
	if _, has := got[0]["matcher"]; has {
		t.Errorf("the entry has a matcher %v, want none: Stop has none", got[0]["matcher"])
	}
	if want := "command -v " + testHookCommand + " >/dev/null 2>&1 && " + testHookCommand + " review stop-hook || true"; reviewStopCommand(t, got[0]) != want {
		t.Errorf("command %q, want the guarded never-blocking form %q", reviewStopCommand(t, got[0]), want)
	}
	if !settings.HasReviewStopHook(root, testHookCommand) {
		t.Errorf("the hook is not reported in place after Install: missing %v", settings.MissingReviewStopHookParts(root, testHookCommand))
	}
}

func TestReviewStopHookEntriesAreExactlyWhatInstallWrites(t *testing.T) {
	path, _ := installFresh(t)
	installed := reviewStopEntries(parseJSON(t, path), testHookCommand)

	built := settings.ReviewStopHookEntries(testHookCommand)

	if len(built) != 1 || len(built["Stop"]) != 1 || len(installed) != 1 || mustJSON(t, built["Stop"][0]) != mustJSON(t, installed[0]) {
		t.Errorf("ReviewStopHookEntries = %v, Install wrote %v", built, installed)
	}
}

// The command runs under sh as Claude Code runs it, and nothing it does can block the stop: a missing binary is a silent
// no-op, and a binary that fails with the exit code that blocks leaves the exit at 0 with its answer on stdout.
func TestReviewStopHookCommand_RunsUnderShWithoutEverBlocking(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skipf("sh unavailable: %v", err)
	}
	install := func(t *testing.T, bin string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "settings.json")
		if err := newSettingsInstaller(path, bin).Install(); err != nil {
			t.Fatal(err)
		}
		entries := reviewStopEntries(parseJSON(t, path), bin)
		if len(entries) != 1 {
			t.Fatalf("review stop-hook entries = %d, want 1", len(entries))
		}
		return reviewStopCommand(t, entries[0])
	}
	const stopInput = `{"hook_event_name":"Stop","session_id":"s","cwd":"/work","stop_hook_active":false}`

	t.Run("a missing binary is a silent no-op", func(t *testing.T) {
		command := install(t, filepath.Join(t.TempDir(), "gentle-ai-overlay"))
		if code, out := runGuardCommand(t, command, stopInput); code != 0 || out != "" {
			t.Errorf("missing binary: exit %d, output %q, want a silent exit 0", code, out)
		}
	})

	t.Run("the present binary's answer reaches Claude Code and its failures cannot block", func(t *testing.T) {
		bin := filepath.Join(t.TempDir(), "gentle-ai-overlay")
		// Checks it is run as 'review stop-hook', prints a marker, and fails with the exit code that blocks a stop.
		script := "#!/bin/sh\n[ \"$1 $2\" = \"review stop-hook\" ] || exit 9\ncat >/dev/null\nprintf '{\"marker\":true}\\n'\nexit 2\n"
		if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
		code, out := runGuardCommand(t, install(t, bin), stopInput)
		if code != 0 {
			t.Errorf("exit %d (%s): a hook that fails must not block the stop it serves", code, out)
		}
		if !strings.Contains(out, `{"marker":true}`) {
			t.Errorf("output %q lost the hook's answer", out)
		}
	})
}

// reviewStopForeign holds Stop entries of other programs: two that have nothing to do with ours, one that runs our binary
// with another verb, and one that mentions the identity while running another program.
func reviewStopForeign() []interface{} {
	stop := func(command string) interface{} {
		return map[string]interface{}{"hooks": []interface{}{map[string]interface{}{"type": "command", "command": command, "timeout": float64(5)}}}
	}
	return []interface{}{
		stop("moshi-hook stop"),
		stop("gentle-ai review-stop"),
		stop(testHookCommand + " other verb"),
		stop("/opt/other/tool review stop-hook"),
	}
}

func TestReviewStopHookFamilyPreservesForeignStopEntriesByteForByte(t *testing.T) {
	foreign := reviewStopForeign()
	path := writeSettingsDoc(t, map[string]interface{}{
		"model": "opus",
		"hooks": map[string]interface{}{"Stop": foreign, "Notification": []interface{}{map[string]interface{}{"hooks": []interface{}{}}}},
	})
	m := installerFor(t, path)
	check := func(stage string, wantOurs int) {
		t.Helper()
		root := parseJSON(t, path)
		have := map[string]bool{}
		for _, e := range root["hooks"].(map[string]interface{})["Stop"].([]interface{}) {
			have[mustJSON(t, e)] = true
		}
		for _, f := range foreign {
			if !have[mustJSON(t, f)] {
				t.Errorf("%s: foreign Stop entry lost or altered: %s", stage, mustJSON(t, f))
			}
		}
		if root["model"] != "opus" || root["hooks"].(map[string]interface{})["Notification"] == nil {
			t.Errorf("%s: foreign keys lost: %v", stage, root)
		}
		if n := len(reviewStopEntries(root, testHookCommand)); n != wantOurs {
			t.Errorf("%s: review stop-hook entries = %d, want %d", stage, n, wantOurs)
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
}

// With every family gone the event keys are removed, not left empty, and the Stop key is one of them.
func TestUninstall_RemovesTheReviewStopHookAndTheStopKeyItLeavesEmpty(t *testing.T) {
	path, m := installFresh(t)

	if err := m.Uninstall(); err != nil {
		t.Fatal(err)
	}

	hooks, _ := parseJSON(t, path)["hooks"].(map[string]interface{})
	if hooks["Stop"] != nil {
		t.Errorf("the Stop key is left behind: %s", mustJSON(t, hooks["Stop"]))
	}
}

// An install made before the hook existed gains it when Install runs again, and every other entry stays as it was.
func TestInstall_AddsTheReviewStopHookToAnOlderInstall(t *testing.T) {
	path, m := installFresh(t)
	root := parseJSON(t, path)
	delete(root["hooks"].(map[string]interface{}), "Stop")
	b, _ := json.Marshal(root)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if settings.HasReviewStopHook(parseJSON(t, path), testHookCommand) {
		t.Fatal("an install without the hook reported it in place")
	}
	before := parseJSON(t, path)

	if err := m.Install(); err != nil {
		t.Fatal(err)
	}

	after := parseJSON(t, path)
	if !settings.HasReviewStopHook(after, testHookCommand) {
		t.Fatalf("re-running Install did not put the hook in place: missing %v", settings.MissingReviewStopHookParts(after, testHookCommand))
	}
	delete(after["hooks"].(map[string]interface{}), "Stop")
	if !reflect.DeepEqual(before, after) {
		t.Errorf("Install changed something else than the Stop key:\nbefore: %s\n after: %s", mustJSON(t, before), mustJSON(t, after))
	}
}

// withReviewStopFamily adds exactly the entry Install writes for the hook to a hand-built root and returns root.
func withReviewStopFamily(root map[string]interface{}, hookCommand string) map[string]interface{} {
	hooks := root["hooks"].(map[string]interface{})
	existing, _ := hooks["Stop"].([]interface{})
	hooks["Stop"] = append(existing, settings.ReviewStopHookEntries(hookCommand)["Stop"]...)
	return root
}
