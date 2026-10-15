package installer_test

// Doctor does not judge the registry: it asks the engine (`status --only registry`) and puts the
// engine's level on the engine's line. These tests hold what it asks, of which directory, what it
// does with each answer, and what it says when the engine cannot answer (the pins in
// registry_pins.golden hold what it says of each registry with the real engine; the world is
// registry_pins_world_test.go).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeEngine replaces the engine of the home of the world with a program that records its
// arguments, says what it is given on stdout and ends with the code it is given.
func (w *pinWorld) fakeEngine(says string, exit string) (log string) {
	w.t.Helper()
	dir := w.t.TempDir()
	log = filepath.Join(dir, "engine.log")
	script := "#!/bin/sh\n" +
		"echo \"$*\" >> \"$FAKE_ENGINE_LOG\"\n" +
		"printf '%s\\n' \"$FAKE_ENGINE_SAYS\"\n" +
		"exit \"$FAKE_ENGINE_EXIT\"\n"
	if err := os.WriteFile(filepath.Join(w.home, ".claude", "bin", "gentle-ai-overlay"), []byte(script), 0o755); err != nil {
		w.t.Fatal(err)
	}
	w.env = append(w.env, "FAKE_ENGINE_LOG="+log, "FAKE_ENGINE_SAYS="+says, "FAKE_ENGINE_EXIT="+exit)
	return log
}

// registryLines are the lines of a run of doctor that are the line of the check of the registry.
func registryLines(run pinRun) []string {
	var lines []string
	for _, line := range strings.Split(run.Stdout+run.Stderr, "\n") {
		if isRegistryCheckLine(line) {
			lines = append(lines, line)
		}
	}
	return lines
}

func TestDoctorAsksTheEngineAboutTheRegistryOfTheOverlayWhenItHasOneAndOfTheDirectoryItRunsInWhenNot(t *testing.T) {
	for _, tc := range []struct {
		name         string
		overlayHasIt bool
		wantDir      string
	}{
		{"the overlay has a registry", true, "$OVERLAY"},
		{"it has not", false, "$HOME/proj"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := registryWorld(t)
			if tc.overlayHasIt {
				w.write(w.overlay, ".atl/skill-registry.md", "# registry\n")
			}
			log := w.fakeEngine("[OK  ] registry: x — scoped block present", "0")

			run := w.runIn(w.project(), "doctor")

			want := "status --only registry --cwd " + tc.wantDir + "\n"
			if got := w.loggedCalls(log); got != pinText(want) {
				t.Errorf("the engine was asked %q, want %q", got, want)
			}
			if got := registryLines(run); len(got) != 1 || got[0] != "PASS  registry: x — scoped block present" {
				t.Errorf("registry lines = %q, want the engine's line with the level of its code", got)
			}
		})
	}
}

func TestDoctorPutsTheLevelOfTheEnginesCodeOnItsLine(t *testing.T) {
	for _, tc := range []struct {
		code      string
		wantLine  string
		wantExit  int
		wantFinal string
	}{
		{"0", "PASS  registry: r — fine", 0, "doctor: OK"},
		{"2", "WARN  registry: r — fine", 0, "doctor: OK"},
		{"1", "FAIL  registry: r — fine", 1, "doctor: FAIL"},
	} {
		t.Run("code "+tc.code, func(t *testing.T) {
			w := registryWorld(t)
			w.fakeEngine("[XXXX] registry: r — fine", tc.code)

			run := w.runIn(w.project(), "doctor")

			if got := registryLines(run); len(got) != 1 || got[0] != tc.wantLine {
				t.Errorf("registry lines = %q, want %q", got, tc.wantLine)
			}
			if run.Exit != tc.wantExit || !strings.Contains(run.Stdout+run.Stderr, tc.wantFinal) {
				t.Errorf("doctor = %s, want exit %d and %q", pinRunText(run), tc.wantExit, tc.wantFinal)
			}
		})
	}
}

func TestDoctorSaysTheRegistryWasNotCheckedWhenTheEngineCannotAnswer(t *testing.T) {
	notChecked := func(binary string) string {
		return "WARN  registry not checked: the engine at " + binary + " did not answer 'status --only registry' (is it older than this overlay?). Rebuild it with 'labdrian doctor --fix'."
	}
	// An engine that ran and ended with a code it does not give for a report (the code of a crash, of a
	// kill, of a program that could not be run, or one of its own) is a fault of the engine, not a
	// registry that was left unchecked for want of one: doctor says FAIL, with or without a line printed
	// before it (that line may be half an answer), and its own exit says so. Only 0, 2 and 1 are what the
	// engine says of a registry. An engine that is not there, or that does not know the question, is a
	// WARN, as before.
	endedWith := func(binary, code string) string {
		return "FAIL  registry not checked: the engine at " + binary + " ended with code " + code + " on 'status --only registry'; only 0 (ok), 2 (a warning) and 1 (a failure) are answers. Rebuild it with 'labdrian doctor --fix'."
	}
	const binary = "$HOME/.claude/bin/gentle-ai-overlay"
	const aLine = "[FAIL] registry: r — present but EMPTY"
	for _, tc := range []struct {
		name  string
		says  string
		exit  string
		setup func(w *pinWorld)
		want  string
	}{
		{"an engine that crashes after it printed a line (SIGSEGV)", aLine, "139", nil, endedWith(binary, "139")},
		{"an engine that aborts after it printed a line (SIGABRT)", aLine, "134", nil, endedWith(binary, "134")},
		{"an engine that is killed after it printed a line (SIGKILL)", aLine, "137", nil, endedWith(binary, "137")},
		{"an engine that ends with a code of its own after a line", aLine, "3", nil, endedWith(binary, "3")},
		{"an engine that ends with the highest code after a line", aLine, "255", nil, endedWith(binary, "255")},
		{"an engine that crashes before it printed anything", "", "139", nil, endedWith(binary, "139")},
		{"an engine that cannot be run", "", "126", nil, endedWith(binary, "126")},
		{"an engine that crashes in the middle of its whole report", "[OK  ] binary: b\n[FAIL] hook: h", "134", nil, endedWith(binary, "134")},
		{"an engine that ignores the question and prints its whole report", "[OK  ] binary: b\n[FAIL] hook: h\n[OK  ] registry: r", "1", nil, notChecked(binary)},
		{"an engine that refuses the question", "error: status: unknown", "1", nil, notChecked(binary)},
		{"an engine that says nothing", "", "1", nil, notChecked(binary)},
		{"no engine at all", "", "0", func(w *pinWorld) {
			if err := os.Remove(filepath.Join(w.home, ".claude", "bin", "gentle-ai-overlay")); err != nil {
				t.Fatal(err)
			}
		}, "WARN  registry not checked: the engine judges it and its binary is not at " + binary + ". Rebuild it with 'labdrian doctor --fix'."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := registryWorld(t)
			w.fakeEngine(tc.says, tc.exit)
			if tc.setup != nil {
				tc.setup(w)
			}

			run := w.runIn(w.project(), "doctor")

			var got []string
			for _, line := range registryLines(run) {
				if strings.Contains(line, "registry not checked") {
					got = append(got, line)
				}
			}
			if len(got) != 1 || got[0] != tc.want {
				t.Errorf("lines that say the registry was not checked = %q, want %q", got, tc.want)
			}
			wantExit, wantSummary := 0, "doctor: OK"
			if strings.HasPrefix(tc.want, "FAIL") {
				wantExit, wantSummary = 1, "doctor: FAIL"
			}
			if run.Exit != wantExit || !strings.Contains(run.Stdout+run.Stderr, wantSummary) {
				t.Errorf("doctor = %s, want exit %d and %q: a registry not checked because the engine is not there or does not know the question is a warning; one the engine ran and failed to answer is a failure", pinRunText(run), wantExit, wantSummary)
			}
		})
	}
}
