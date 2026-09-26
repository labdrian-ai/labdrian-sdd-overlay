package shelltest

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestOverlayRolesDispatchShellHarness runs the bash harness that proves
// `labdrian roles <verb>` forwards to the engine binary verbatim.
func TestOverlayRolesDispatchShellHarness(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skipf("bash is not available: %v", err)
	}
	harness, err := filepath.Abs("overlay_roles_dispatch_test.sh")
	if err != nil {
		t.Fatalf("resolve harness path: %v", err)
	}
	if _, err := os.Stat(harness); err != nil {
		t.Fatalf("harness not found at %s: %v", harness, err)
	}
	output, err := exec.Command("bash", harness).CombinedOutput()
	if err != nil {
		t.Fatalf("shell harness failed: %v\n%s", err, output)
	}
	t.Logf("shell harness output:\n%s", output)
}
