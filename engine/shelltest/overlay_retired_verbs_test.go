package shelltest

import (
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

// TestRetiredPrespecVerbIsAnUnknownCommand runs the real wrapper: the prespec verbs were retired
// with the prespec-malandra skill (clarify-request calls none), so `prespec` must be refused as an
// unknown command, whatever form a route to it could have been written in. It runs with HOME and
// STATE_DIR in the test's scratch directory, never the developer's real state.
func TestRetiredPrespecVerbIsAnUnknownCommand(t *testing.T) {
	env, _ := targetsHermeticEnv(t)
	got := overlaytest.Run(t, overlayScript(t), env, "", "prespec", "rank")

	if got.Exit != 1 {
		t.Errorf("prespec rank exit = %d, want 1\nstdout: %s\nstderr: %s", got.Exit, got.Stdout, got.Stderr)
	}
	if !strings.Contains(got.Stderr, "Unknown command: prespec") {
		t.Errorf("prespec rank stderr = %q, want the unknown-command error", got.Stderr)
	}
	if got.Stdout != "" {
		t.Errorf("prespec rank stdout = %q, want nothing", got.Stdout)
	}
}
