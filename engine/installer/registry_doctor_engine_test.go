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

// registryLines are the lines of a run of doctor that speak of the registry.
func registryLines(run pinRun) []string {
	var lines []string
	for _, line := range strings.Split(run.Stdout+run.Stderr, "\n") {
		if strings.Contains(strings.ToLower(line), "registry") {
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
	const binary = "$HOME/.claude/bin/gentle-ai-overlay"
	for _, tc := range []struct {
		name  string
		says  string
		exit  string
		setup func(w *pinWorld)
		want  string
	}{
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
			if run.Exit != 0 {
				t.Errorf("doctor = %s, want exit 0: a registry that was not checked is a warning, not a failure", pinRunText(run))
			}
		})
	}
}
