package installer_test

// What bin/labdrian-overlay asks the engine about the record of the installed targets, and what it
// does with the answers (Phase 9, B3). The record is the engine's (`installed-targets`); the script
// is left with the order of things: unregister a runtime, then release it; assess what the uninstall
// comes to, then remove the registration record, then retire the binary. These tests run the script
// against an engine that only records the calls it is given and ends with the code the test hands it,
// so what the script sends is held to the flags the command takes, and what it does when the engine
// says no (or does not know the command) is held too. What the engine does with the record is
// engine/runtime/ownership's and engine/cmd's; what the script says and writes around it is pinned by
// longterm_pins_*_test.go.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// recordingEngineScript is an engine that writes its arguments, one call a line, in FAKE_ENGINE_LOG and
// ends with the code the test gave it for the installed-targets verb, or with 0.
const recordingEngineScript = `#!/bin/sh
echo "$*" >> "$FAKE_ENGINE_LOG"
case "$1 $2" in
  "installed-targets record") exit "${FAKE_RECORD_EXIT:-0}" ;;
  "installed-targets release") exit "${FAKE_RELEASE_EXIT:-0}" ;;
  "installed-targets assess") exit "${FAKE_ASSESS_EXIT:-0}" ;;
  "installed-targets retire") exit "${FAKE_RETIRE_EXIT:-0}" ;;
esac
exit 0
`

// recordingEngineWorld is a world whose engine is the recording one; it returns the world and the log
// the engine writes.
func recordingEngineWorld(t *testing.T) (*pinWorld, string) {
	t.Helper()
	w := longtermWorld(t)
	engine := filepath.Join(w.home, ".claude", "bin", "gentle-ai-overlay")
	if err := os.WriteFile(engine, []byte(recordingEngineScript), 0o755); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(t.TempDir(), "engine.log")
	w.setEnv("FAKE_ENGINE_LOG", log)
	return w, log
}

// state is the state directory as the script names it in what it hands the engine.
const state = "$HOME/.labdrian-overlay"

func lines(l ...string) string { return strings.Join(l, "\n") + "\n" }

func TestEngineCalls_InstallRecordsATargetAfterTheEngineHasReportedItsStatus(t *testing.T) {
	w, log := recordingEngineWorld(t)
	run := w.runLongterm("longterm-mem", "install", "--target", "claude")
	if run.Exit != 0 {
		t.Fatalf("exit %d\n%s%s", run.Exit, run.Stdout, run.Stderr)
	}
	want := lines(
		"runtime install --component longterm-mem --target claude --state-dir "+state,
		"installed-targets record --state-dir "+state+" --target claude",
	)
	if got := w.loggedCalls(log); got != want {
		t.Errorf("the engine was called\n%swant\n%s", got, want)
	}
}

func TestEngineCalls_InstallRecordsNothingForATargetRegisterDidNotRegister(t *testing.T) {
	w, log := recordingEngineWorld(t)
	w.setEnv("FAKE_REGISTER_EXIT", "6")
	w.runLongterm("longterm-mem", "install", "--target", "claude")
	want := lines("runtime install --component longterm-mem --target claude --state-dir " + state)
	if got := w.loggedCalls(log); got != want {
		t.Errorf("the engine was called\n%swant\n%s", got, want)
	}
}

func TestEngineCalls_UninstallReleasesEachTargetThenAsksWhatItComesToThenRetires(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		env  map[string]string
		want string
	}{
		{"an uninstall that finds nothing depending on the binary", []string{"--target", "claude"}, nil, lines(
			"installed-targets release --state-dir "+state+" --target claude",
			"installed-targets assess --state-dir "+state,
			"runtime uninstall --component longterm-mem --state-dir "+state,
			"installed-targets retire --state-dir "+state+" --binary "+state+"/bin/longterm-mem",
		)},
		{"purge is handed on to both", []string{"--target", "claude", "--purge"}, nil, lines(
			"installed-targets release --state-dir "+state+" --target claude",
			"installed-targets assess --state-dir "+state+" --purge",
			"runtime uninstall --component longterm-mem --state-dir "+state,
			"installed-targets retire --state-dir "+state+" --binary "+state+"/bin/longterm-mem --purge",
		)},
		{"a target that was not unregistered is told to the engine, which keeps everything", []string{"--target", "claude"},
			map[string]string{"FAKE_UNREGISTER_EXIT": "1", "FAKE_ASSESS_EXIT": "3"}, lines(
				"installed-targets assess --state-dir " + state + " --unregister-failed",
			)},
		{"purge through a failed unregister", []string{"--target", "claude", "--purge"},
			map[string]string{"FAKE_UNREGISTER_EXIT": "1"}, lines(
				"installed-targets assess --state-dir "+state+" --purge --unregister-failed",
				"runtime uninstall --component longterm-mem --state-dir "+state,
				"installed-targets retire --state-dir "+state+" --binary "+state+"/bin/longterm-mem --purge",
			)},
		{"a target that is kept (an unmanaged entry) is not released", []string{"--target", "claude"},
			map[string]string{"FAKE_UNREGISTER_EXIT": "6", "FAKE_ASSESS_EXIT": "3"}, lines(
				"installed-targets assess --state-dir " + state,
			)},
		{"a version-skewed unregister is released all the same", []string{"--target", "claude"},
			map[string]string{"FAKE_UNREGISTER_EXIT": "2"}, lines(
				"installed-targets release --state-dir "+state+" --target claude",
				"installed-targets assess --state-dir "+state,
				"runtime uninstall --component longterm-mem --state-dir "+state,
				"installed-targets retire --state-dir "+state+" --binary "+state+"/bin/longterm-mem",
			)},
		{"the engine keeps the registration record and the binary while a target is installed", []string{"--target", "claude"},
			map[string]string{"FAKE_ASSESS_EXIT": "3"}, lines(
				"installed-targets release --state-dir "+state+" --target claude",
				"installed-targets assess --state-dir "+state,
			)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, log := recordingEngineWorld(t)
			w.placeBinary()
			for k, v := range tc.env {
				w.setEnv(k, v)
			}
			w.runLongterm(append([]string{"longterm-mem", "uninstall"}, tc.args...)...)
			if got := w.loggedCalls(log); got != tc.want {
				t.Errorf("the engine was called\n%swant\n%s", got, tc.want)
			}
		})
	}
}

func TestEngineCalls_UninstallWithoutAUsableBinaryReleasesEveryTargetWithoutUnregistering(t *testing.T) {
	w, log := recordingEngineWorld(t)
	w.runLongterm("longterm-mem", "uninstall", "--target", "all")
	want := lines(
		"installed-targets release --state-dir "+state+" --target claude",
		"installed-targets release --state-dir "+state+" --target opencode",
		"installed-targets release --state-dir "+state+" --target codex",
		"installed-targets release --state-dir "+state+" --target pi",
		"installed-targets assess --state-dir "+state,
		"runtime uninstall --component longterm-mem --state-dir "+state,
		"installed-targets retire --state-dir "+state+" --binary "+state+"/bin/longterm-mem",
	)
	if got := w.loggedCalls(log); got != want {
		t.Errorf("the engine was called\n%swant\n%s", got, want)
	}
	if calls := w.loggedCalls(w.env0("FAKE_LM_LOG")); calls != pinText("") {
		t.Errorf("the longterm-mem was called without a binary to call: %s", calls)
	}
}

func TestEngineCalls_ARunEndsAtOnceWhenTheEngineRefusesToRecordOrToRelease(t *testing.T) {
	t.Run("record", func(t *testing.T) {
		w, log := recordingEngineWorld(t)
		w.setEnv("FAKE_RECORD_EXIT", "1")
		run := w.runLongterm("longterm-mem", "install", "--target", "all")
		if run.Exit != 1 {
			t.Errorf("exit %d, want 1", run.Exit)
		}
		want := lines(
			"runtime install --component longterm-mem --target claude --state-dir "+state,
			"installed-targets record --state-dir "+state+" --target claude",
		)
		if got := w.loggedCalls(log); got != want {
			t.Errorf("the engine was called\n%swant\n%s: the run goes on to no other target", got, want)
		}
	})
	t.Run("release", func(t *testing.T) {
		w, log := recordingEngineWorld(t)
		w.placeBinary()
		w.setEnv("FAKE_RELEASE_EXIT", "1")
		run := w.runLongterm("longterm-mem", "uninstall", "--target", "all")
		if run.Exit != 1 {
			t.Errorf("exit %d, want 1", run.Exit)
		}
		want := lines("installed-targets release --state-dir " + state + " --target claude")
		if got := w.loggedCalls(log); got != want {
			t.Errorf("the engine was called\n%swant\n%s: nothing else is unregistered, assessed or removed", got, want)
		}
	})
}

func TestEngineCalls_ARetireThatFailsEndsTheRunWithExit1(t *testing.T) {
	w, _ := recordingEngineWorld(t)
	w.placeBinary()
	w.setEnv("FAKE_RETIRE_EXIT", "1")
	if run := w.runLongterm("longterm-mem", "uninstall", "--target", "claude"); run.Exit != 1 {
		t.Errorf("exit %d, want 1", run.Exit)
	}
}

func TestEngineCalls_AnEngineThatDoesNotAnswerAssessKeepsEverythingAndFailsTheRun(t *testing.T) {
	// An engine older than the command ends with its own "unknown subcommand" and exit 1; any code but 0
	// and 3 is the engine not answering, and the registration record and the binary stay.
	for _, exit := range []string{"1", "2", "127"} {
		t.Run("exit "+exit, func(t *testing.T) {
			w, log := recordingEngineWorld(t)
			w.placeBinary()
			w.setEnv("FAKE_ASSESS_EXIT", exit)
			run := w.runLongterm("longterm-mem", "uninstall", "--target", "claude")
			if run.Exit != 1 {
				t.Errorf("exit %d, want 1", run.Exit)
			}
			if want := "WARN: longterm-mem: the engine could not say whether any runtime still depends on the shared binary (exit " + exit + " of 'installed-targets assess'); leaving the registration record and the shared binary untouched. Rebuild the engine with 'labdrian doctor --fix' and re-run uninstall.\n"; run.Stderr != want {
				t.Errorf("stderr = %q, want %q", run.Stderr, want)
			}
			wantCalls := lines(
				"installed-targets release --state-dir "+state+" --target claude",
				"installed-targets assess --state-dir "+state,
			)
			if got := w.loggedCalls(log); got != wantCalls {
				t.Errorf("the engine was called\n%swant\n%s", got, wantCalls)
			}
			if _, err := os.Stat(w.sharedBinary()); err != nil {
				t.Errorf("the shared binary was removed: %v", err)
			}
		})
	}
}

// The script calls the verbs of the command with the flags each takes and no other. The list is the one
// the engine holds in its usage (engine/cmd/installed_targets.go): a flag the script sends that the
// command does not take would be a usage error at the first uninstall.
func TestEngineCalls_TheScriptSendsOnlyTheVerbsAndFlagsTheCommandTakes(t *testing.T) {
	script, err := os.ReadFile(overlayScript(t))
	if err != nil {
		t.Fatal(err)
	}
	taken := map[string]map[string]bool{
		"record":  {"--target": true},
		"release": {"--target": true},
		"assess":  {"--purge": true, "--unregister-failed": true},
		"retire":  {"--binary": true, "--purge": true},
	}
	call := regexp.MustCompile(`longtermmem_targets (\w+)([^|;\n]*)`)
	flag := regexp.MustCompile(`--[a-z-]+`)
	seen := map[string]bool{}
	for _, m := range call.FindAllStringSubmatch(string(script), -1) {
		verb, rest := m[1], m[2]
		allowed, ok := taken[verb]
		if !ok {
			t.Errorf("the script calls the verb %q, which the command does not take: %s", verb, m[0])
			continue
		}
		seen[verb] = true
		for _, f := range flag.FindAllString(rest, -1) {
			if !allowed[f] {
				t.Errorf("the script sends %s to %s, which does not take it: %s", f, verb, m[0])
			}
		}
	}
	for verb := range taken {
		if !seen[verb] {
			t.Errorf("the script never calls %s", verb)
		}
	}
	// The helper hands the state directory over once, in front of the arguments.
	if !strings.Contains(string(script), `"$ENGINE_BINARY" installed-targets "$verb" --state-dir "$STATE_DIR" "$@"`) {
		t.Error("the helper does not call the engine with the state directory in front of the arguments")
	}
}

// Nothing of the old record is left in the script: no temporary file for a rewrite, no filter, no lock
// made of a directory, no age test for an abandoned one.
func TestEngineCalls_NoBashIsLeftThatKeepsTheRecord(t *testing.T) {
	script, err := os.ReadFile(overlayScript(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{
		"LONGTERM_MEM_INSTALLED_TARGETS", "LONGTERM_MEM_TARGETS_LOCK", "LONGTERM_MEM_TARGETS_LOCK_STALE_MINUTES",
		"longtermmem_installed_targets_", "longtermmem_targets_lock_", "longtermmem_maybe_remove_binary",
		"longtermmem_target_name_is_wellformed", ".installed-targets.XXXXXX", "grep -v -x -F", "tr '\\000' '\\001'",
		"-mmin", "tracking_known", "tracking_readable",
	} {
		if strings.Contains(string(script), gone) {
			t.Errorf("the script still has %q, which belonged to the record it used to keep", gone)
		}
	}
}
