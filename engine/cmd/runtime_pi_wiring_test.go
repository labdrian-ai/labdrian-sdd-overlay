package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// The wiring of the Pi runtime in the composition root: the CommandRunner the command is given
// is the one the adapter runs `pi` through, and the settings read from the environment reach the
// adapter as options, never by the adapter reading them.

// piOverlayWorld is an overlay the Pi package can be built from, a state directory to build it
// in, and a home with no Pi settings, all under temporary directories and named by the
// environment the way the program is configured.
func piOverlayWorld(t *testing.T) (stateDir string) {
	t.Helper()
	overlayRoot, _ := pipkgFixtureOverlay(t)
	stateDir = t.TempDir()
	t.Setenv("OVERLAY_DIR", overlayRoot)
	t.Setenv("STATE_DIR", stateDir)
	t.Setenv("HOME", t.TempDir())
	return stateDir
}

func TestRuntimeInstallPiRunsPiThroughTheInjectedRunner(t *testing.T) {
	stateDir := piOverlayWorld(t)
	commands := &scriptedPiCommands{}

	var out, errOut bytes.Buffer
	code := -1
	runRuntimeCore(commands, noGit(), []string{"install", "--target", "pi"}, &out, &errOut, func(c int) { code = c })

	if code != 0 {
		t.Fatalf("install --target pi exited %d\nstdout=%q\nstderr=%q", code, out.String(), errOut.String())
	}
	pkg := filepath.Join(stateDir, "pi", "labdrian-pi")
	if len(commands.runs) < 1 || strings.Join(commands.runs[0][1:], " ") != "install "+pkg {
		t.Fatalf("the runner was asked to run %v, want `pi install %s` first", commands.runs, pkg)
	}
	if !strings.Contains(out.String(), "ran `pi install "+pkg+"`") {
		t.Errorf("stdout = %q, want it to say the install ran", out.String())
	}
}

func TestRuntimeInstallPiWithoutTheCLIKeepsTheBuildAndTheHint(t *testing.T) {
	stateDir := piOverlayWorld(t)
	commands := &scriptedPiCommands{missing: true}

	var out, errOut bytes.Buffer
	code := -1
	runRuntimeCore(commands, noGit(), []string{"install", "--target", "pi"}, &out, &errOut, func(c int) { code = c })

	pkg := filepath.Join(stateDir, "pi", "labdrian-pi")
	if want := "run: pi install " + pkg; !strings.Contains(out.String(), want) {
		t.Errorf("stdout = %q, want the hint %q", out.String(), want)
	}
	if len(commands.runs) != 0 {
		t.Errorf("a command ran although the CLI was not found: %v", commands.runs)
	}
	if code != 1 {
		t.Errorf("exit = %d, want 1 for a partial install", code)
	}
}

func TestRuntimeInstallPiHonoursTheSkipVariableThroughTheConfig(t *testing.T) {
	piOverlayWorld(t)
	t.Setenv("LABDRIAN_PI_SKIP_SUBAGENTS", "1")
	commands := &scriptedPiCommands{}

	var out, errOut bytes.Buffer
	runRuntimeCore(commands, noGit(), []string{"install", "--target", "pi"}, &out, &errOut, func(int) {})

	for _, run := range commands.runs {
		if strings.Contains(strings.Join(run, " "), "pi-subagents") {
			t.Fatalf("the extension was installed although the variable asked to skip it: %v", commands.runs)
		}
	}
	if !strings.Contains(out.String(), "Pi Subagents extension check skipped (LABDRIAN_PI_SKIP_SUBAGENTS=1)") {
		t.Errorf("stdout = %q, want the skip told", out.String())
	}
}

func TestRuntimeCoreWithoutACommandRunnerRefusesBeforeActing(t *testing.T) {
	piOverlayWorld(t)
	var out, errOut bytes.Buffer
	code := -1
	runRuntimeCore(nil, noGit(), []string{"install", "--target", "pi"}, &out, &errOut, func(c int) { code = c })
	if code != 1 || !strings.Contains(errOut.String(), "no command runner") {
		t.Fatalf("exit=%d stderr=%q, want a refusal naming the missing command runner", code, errOut.String())
	}
	if out.Len() != 0 {
		t.Errorf("stdout = %q, want nothing done", out.String())
	}
}
