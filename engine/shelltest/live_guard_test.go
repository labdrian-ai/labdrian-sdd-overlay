package shelltest

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/enginetest"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/testhome"
)

// TestMain runs the tests of this package in a home directory of their own. They run the overlay script, and the
// engine it builds, against repositories made with git; git reads the global configuration and the global ignore
// and attributes files it finds under HOME and the XDG roots, and the script derives the overlay and state
// directories from HOME and from the variables that override them (OVERLAY_DIR, STATE_DIR, CODEX_HOME). Without
// this the answers depend on how whoever runs the tests has configured git and on what they exported. The tests
// that need a sandboxed home of their own pass one to the script (HOME=...) as before; the go command is kept on
// the caches it had (testhome).
func TestMain(m *testing.M) {
	code := testhome.Run(m)
	enginetest.Cleanup()
	os.Exit(code)
}

// The tests run in the home TestMain made for them, not in the machine's.
func TestTheTestsRunInAHomeOfTheirOwn(t *testing.T) { testhome.Check(t) }

// The script fetches from origin (version, update, self-update, apply, sync-check --check-origin), and the
// repository these tests live in has an origin that is a network address. The run lets git use files and nothing
// else, so a test that runs such a verb there is refused before git connects, instead of reaching the network
// with the credentials of whoever runs the tests.
func TestGitRefusesAnyTransportButFiles(t *testing.T) {
	out, err := exec.Command("git", "ls-remote", "ssh://nowhere.invalid/x.git").CombinedOutput()
	if err == nil {
		t.Fatalf("git reached a remote over ssh in the tests of this package:\n%s", out)
	}
	if !strings.Contains(string(out), "transport 'ssh' not allowed") {
		t.Errorf("git failed for another reason than the transport being refused (is the network there?):\n%s", out)
	}
}
