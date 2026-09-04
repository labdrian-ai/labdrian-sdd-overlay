package shelltest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	runtimepkg "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
)

// TestOverlayLongtermMemBinaryPathMatchesEngine pins the agreement between
// the TWO HALVES that between them decide whether an MCP entry the overlay
// itself wrote still looks like ours.
//
// The shell entrypoint deploys the binary at a path it builds from
// $STATE_DIR and hands that exact literal to `longterm-mem register
// --binary`, which writes it into each runtime's MCP config. The engine
// re-derives the same path with filepath.Join and rebuilds the entry from
// it to decide ownership, and ownership is an EXACT STRING fingerprint
// (LongtermMemAdapter.ownedClaudeFingerprint and friends). filepath.Join
// normalizes; shell string concatenation does not. So the two halves agreed
// only for a $STATE_DIR that was already lexically clean: with a trailing
// slash the shell wrote "<dir>//bin/longterm-mem" into ~/.claude.json while
// the engine looked for "<dir>/bin/longterm-mem", and reported the overlay's
// own entry as "partial -- entry without record (unmanaged)".
//
// A Go test is the right home for this precisely because the defect lives
// BETWEEN the halves: only here can the shell's answer be compared against
// the engine function that is the other half of the contract, rather than
// against a hand-written expectation that could drift with it.
func TestOverlayLongtermMemBinaryPathMatchesEngine(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skipf("bash is not available: %v", err)
	}

	overlay, err := filepath.Abs(filepath.Join("..", "..", "bin", "labdrian-overlay"))
	if err != nil {
		t.Fatalf("resolve overlay path: %v", err)
	}
	if _, err := os.Stat(overlay); err != nil {
		t.Fatalf("overlay entrypoint not found at %s: %v", overlay, err)
	}

	base := t.TempDir()
	cases := []struct {
		name     string
		stateDir string
	}{
		{"already clean", filepath.Join(base, "state")},
		{"trailing slash", filepath.Join(base, "state") + "/"},
		{"trailing slash dot", filepath.Join(base, "state") + "/."},
		{"double slash", base + "//state"},
		// Built by concatenation, never filepath.Join: Join would clean
		// these before bash ever saw them, which is the very asymmetry
		// under test.
		{"parent segment", base + "/elsewhere/../state"},
		{"redundant dot segment", base + "/./state"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command("bash", "-c", `source "$1"; printf '%s\n' "$LONGTERM_MEM_BINARY"`, "_", overlay)
			cmd.Env = append(os.Environ(), "STATE_DIR="+tc.stateDir)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("sourcing the overlay failed: %v\n%s", err, out)
			}
			got := strings.TrimSpace(string(out))
			want := runtimepkg.LongtermMemBinaryPathForStateDir(tc.stateDir)
			if got != want {
				t.Fatalf("shell and engine disagree on the longterm-mem binary path for STATE_DIR=%q:\n  shell:  %q\n  engine: %q\nownership is an exact string fingerprint, so this makes the overlay's own MCP entry read as unmanaged", tc.stateDir, got, want)
			}
		})
	}
}
