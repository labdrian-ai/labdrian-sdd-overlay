// Package shelltest carries no production code. It exists so the shell-level
// tests for bin/labdrian-overlay run in CI without a workflow change: the
// existing "Engine Tests" job already runs `go test ./...` from engine/, and
// these tests drive the script, and a bash harness, next to it.
//
// The longterm-mem harness is a plain bash script, runnable on its own
// (`engine/shelltest/overlay_longterm_mem_test.sh`), so a failure can be
// reproduced and iterated on without Go in the loop. The dispatch of the
// engine's commands (overlay_dispatch_test.go) is a table in Go, one row for
// each command. The world the tests run the script in (where it is, an
// environment, a repository, a fake engine, a run that keeps the streams
// apart) is engine/overlaytest's.
package shelltest

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/enginetest"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

// overlayScript is bin/labdrian-overlay for a test that runs it; the test is skipped on a machine with
// no bash.
func overlayScript(t *testing.T) string {
	t.Helper()
	overlaytest.RequireBash(t)
	return overlaytest.Script(t)
}

func TestOverlayLongtermMemShellHarness(t *testing.T) {
	overlaytest.RequireBash(t)

	harness, err := filepath.Abs("overlay_longterm_mem_test.sh")
	if err != nil {
		t.Fatalf("resolve harness path: %v", err)
	}
	if _, err := os.Stat(harness); err != nil {
		t.Fatalf("harness not found at %s: %v", harness, err)
	}

	cmd := exec.Command("bash", harness)
	// The cases that read back or seed the record of the installed targets need a real engine, which
	// owns it (engine installed-targets); the test builds one from this checkout and hands it over.
	cmd.Env = append(os.Environ(), "LONGTERM_TEST_ENGINE="+enginetest.Binary(t))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("shell harness failed: %v\n%s", err, output)
	}
	t.Logf("shell harness output:\n%s", output)
}
