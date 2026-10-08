package piguard_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/piguard"
)

func skipOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the guard is a shell script")
	}
}

// The guard goes first on the PATH, so a `pi` looked up while it is installed is the guard and
// never the one of the machine, whatever else the PATH holds.
func TestInstallPutsAPiThatRefusesFirstOnThePath(t *testing.T) {
	skipOnWindows(t)
	t.Setenv("PATH", os.Getenv("PATH")) // restored when the test ends, whatever Close does

	guard, err := piguard.Install()
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Close()

	path, err := exec.LookPath("pi")
	if err != nil {
		t.Fatalf("LookPath(pi): %v", err)
	}
	if !strings.HasPrefix(path, os.TempDir()) {
		t.Errorf("pi resolves to %s, want the guard in a temporary directory", path)
	}
	if guard.Started() {
		t.Fatal("the guard reports a start before anything ran it")
	}
	out, err := exec.Command(path, "install", "/some/package").CombinedOutput()
	if err == nil {
		t.Fatal("the guard pi exited 0; it must refuse")
	}
	if !guard.Started() {
		t.Error("the guard does not report that it was started")
	}
	if !strings.Contains(guard.Report(), "install") || !strings.Contains(guard.Report(), "/some/package") {
		t.Errorf("Report() = %q, want the arguments the guard was started with (output %q)", guard.Report(), out)
	}
}

func TestCloseRestoresThePathAndRemovesTheGuard(t *testing.T) {
	skipOnWindows(t)
	before := os.Getenv("PATH")
	t.Setenv("PATH", before)

	guard, err := piguard.Install()
	if err != nil {
		t.Fatal(err)
	}
	guardPath, _ := exec.LookPath("pi")
	guard.Close()

	if got := os.Getenv("PATH"); got != before {
		t.Errorf("PATH after Close = %q, want %q", got, before)
	}
	if _, err := os.Stat(filepath.Dir(guardPath)); err == nil {
		t.Errorf("the guard directory %s is still there", filepath.Dir(guardPath))
	}
}

func TestAGuardThatWasNotStartedReportsNothing(t *testing.T) {
	skipOnWindows(t)
	t.Setenv("PATH", os.Getenv("PATH"))
	guard, err := piguard.Install()
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Close()
	if guard.Started() || guard.Report() != "" {
		t.Errorf("Started=%v Report=%q before any run", guard.Started(), guard.Report())
	}
}

func TestVerdictFailsTheRunThatStartedTheGuard(t *testing.T) {
	skipOnWindows(t)
	t.Setenv("PATH", os.Getenv("PATH"))
	guard, err := piguard.Install()
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Close()

	if message, ok := guard.Verdict(); !ok || message != "" {
		t.Fatalf("Verdict before any start = %q, %v; want empty and ok", message, ok)
	}
	path, _ := exec.LookPath("pi")
	_ = exec.Command(path, "remove", "/the/package").Run()
	message, ok := guard.Verdict()
	if ok {
		t.Fatal("Verdict is ok after the guard was started")
	}
	if !strings.Contains(message, "remove /the/package") {
		t.Errorf("message = %q, want what the guard was started with", message)
	}
}
