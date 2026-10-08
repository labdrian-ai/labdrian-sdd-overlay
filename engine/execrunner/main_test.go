package execrunner

import (
	"os"
	"os/exec"
	"testing"
)

// TestMain keeps every test of this package away from the programs of the machine it runs on: an
// earlier version of the Pi tests ran a real `pi remove` against the setup of the person who ran
// `go test`. The PATH of the run is an empty temporary directory, so a test that forgets to name
// the directory of its fake binary finds nothing to start. A test that needs a binary puts a
// fake one in its own directory and names it with t.Setenv("PATH", dir).
func TestMain(m *testing.M) {
	empty, err := os.MkdirTemp("", "execrunner-empty-path-*")
	if err != nil {
		panic(err)
	}
	os.Setenv("PATH", empty)
	code := m.Run()
	os.RemoveAll(empty)
	os.Exit(code)
}

// TestTheRunPathHoldsNoRealProgram proves the guard of TestMain is in force.
func TestTheRunPathHoldsNoRealProgram(t *testing.T) {
	for _, name := range []string{"pi", "claude", "codex", "opencode", "sh", "git"} {
		if path, err := exec.LookPath(name); err == nil {
			t.Errorf("%q resolves to %s on the PATH of the test run; the tests of this package must see none", name, path)
		}
	}
}
