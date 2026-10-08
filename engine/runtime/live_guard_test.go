package runtime_test

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
// once removed a freshly installed labdrian-pi package during `go test ./...`, because a test
// reached a real `pi`. Three nets sit under that now. The adapter reaches `pi` only through a
// CommandRunner and every test hands it a fake (the next test and the one after it keep that
// true); the PATH of the run holds a `pi` that refuses to run in front of any real one, and the
// run fails if anything started it; and HOME, STATE_DIR and OVERLAY_DIR are pinned. Individual
// tests may still override the environment with t.Setenv.
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

	code := m.Run()
	if message, ok := guard.Verdict(); !ok {
		fmt.Fprintln(os.Stderr, message)
		code = 1
	}
	guard.Close()
	os.RemoveAll(home)
	os.Exit(code)
}

// TestLiveGuard_IsolatesHomeAndPi proves the TestMain guard is in force for this package: the
// home is not a real Pi installation, and a `pi` looked up by name is the guard that refuses.
func TestLiveGuard_IsolatesHomeAndPi(t *testing.T) {
	real, _ := os.UserHomeDir()
	if _, err := os.Stat(filepath.Join(real, ".pi", "agent", "settings.json")); err == nil {
		t.Fatalf("HOME %q still points at a real Pi installation", real)
	}
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

// TestNoTestOfThisPackageStartsTheRealCLI reads the source of the tests: they drive the code under test with a fake CommandRunner,
// so a test that imports the process adapter, or runs `pi` itself, can reach the CLI of the
// machine with the PATH of the run. The scan is piguard's, shared with the other packages.
func TestNoTestOfThisPackageStartsTheRealCLI(t *testing.T) {
	piguard.CheckTestSources(t, ".")
}
