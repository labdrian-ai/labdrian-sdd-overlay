package overlaycli

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/tui/internal/testhome"
)

// Every test of this binary runs in a home of its own: the backend script and
// git it drives would otherwise read the state directory, the deployment
// targets and the git configuration of whoever runs the tests.
func TestMain(m *testing.M) { os.Exit(testhome.Run(m)) }

func TestTheTestsRunInAHomeOfTheirOwn(t *testing.T) { testhome.Check(t) }

// writeStubBackend installs a fake bin/labdrian-overlay under root that
// records its invocation (cwd + args) to recorderPath, prints stdout, and
// exits with exitCode -- used to prove the adapter's contract with the process
// (the arguments, the working directory, the output fed through even on a
// nonzero exit) without touching a real git repo or the real backend script.
func writeStubBackend(t *testing.T, root, recorderPath, stdout string, exitCode int) {
	t.Helper()
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	script := "#!/bin/sh\n" +
		"printf '%s|%s\\n' \"$PWD\" \"$*\" >> " + strconv.Quote(recorderPath) + "\n" +
		"cat <<'STUBEOF'\n" + stdout + "\nSTUBEOF\n" +
		"exit " + strconv.Itoa(exitCode) + "\n"
	if err := os.WriteFile(filepath.Join(binDir, "labdrian-overlay"), []byte(script), 0o755); err != nil {
		t.Fatalf("write stub backend: %v", err)
	}
}
