package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/piguard"
)

// TestMain isolates every test in this package from the developer's live machine: the Pi adapter
// shells out to a real `pi` CLI, which once removed a freshly installed labdrian-pi package
// during `go test ./...`. The cores of the commands take their CommandRunner as a parameter and
// every test hands them a fake (noPi); under that, the PATH of the run holds a `pi` that refuses
// to run in front of any real one, and the run fails if anything started it, the built program
// that the golden tests start included. Individual tests may still override the environment with
// t.Setenv. HOME, STATE_DIR and OVERLAY_DIR are pinned under a temporary directory, and
// XDG_STATE_HOME and XDG_CONFIG_HOME under the temporary HOME too, so no test can write the real
// shaper clearance store or the real XDG config.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "engine-test-home-*")
	if err != nil {
		panic(err)
	}
	guard, err := piguard.Install()
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", home)
	os.Setenv("STATE_DIR", filepath.Join(home, ".labdrian-overlay"))
	os.Unsetenv("OVERLAY_DIR")
	os.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	os.Unsetenv("GENTLE_PI_AGENTS_CHILD")
	// os.Exit does not run deferred calls, so what the run made is removed before it.
	code := m.Run()
	if message, ok := guard.Verdict(); !ok {
		fmt.Fprintln(os.Stderr, message)
		code = 1
	}
	guard.Close()
	os.RemoveAll(home)
	removeEngineBinary()
	os.Exit(code)
}

// TestLiveGuard_PiOnThePathIsTheGuard proves the TestMain guard is in force: a `pi` looked up by
// name is the one that refuses to run, not a real one.
func TestLiveGuard_PiOnThePathIsTheGuard(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("the pi guard is a shell script")
	}
	path, err := exec.LookPath("pi")
	if err != nil {
		t.Fatalf("no pi guard on the PATH of the run: %v", err)
	}
	if !strings.HasPrefix(filepath.Base(filepath.Dir(path)), "pi-guard-") {
		t.Fatalf("pi resolves to %s on the PATH of the run, want the refusing guard", path)
	}
}

// TestNoTestOfThisPackageUsesTheProcessAdapter reads the source of the tests. They drive the code
// under test with a fake CommandRunner, so a test that imports the process adapter, or runs `pi`
// itself, can reach the CLI of the machine with the PATH of the run. The scan is piguard's,
// shared with the other packages.
func TestNoTestOfThisPackageUsesTheProcessAdapter(t *testing.T) {
	piguard.CheckTestSources(t, ".")
}
