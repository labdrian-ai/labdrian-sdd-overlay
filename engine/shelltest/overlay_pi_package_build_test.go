package shelltest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestPipkgHelpers_BuildStatusSyncCheck (R-002, R-003, R-010, pi-package-
// build slice 2.5): exercises the real pipkg_build_and_report /
// pipkg_status_and_report / pipkg_sync_check_and_report bash helpers
// end-to-end against a real built engine binary, without running cmd_apply
// itself (which mutates git state) -- these are the same functions
// cmd_apply/cmd_status/cmd_sync_check dispatch to for --target pi.
func TestPipkgHelpers_BuildStatusSyncCheck(t *testing.T) {
	overlay := piTargetOverlayPath(t)
	overlayDir, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve overlay dir: %v", err)
	}

	home := t.TempDir()
	engineDir := filepath.Join(home, ".claude", "bin")
	if err := os.MkdirAll(engineDir, 0755); err != nil {
		t.Fatalf("mkdir engine dir: %v", err)
	}
	enginePath := filepath.Join(engineDir, "gentle-ai-overlay")
	build := exec.Command("go", "build", "-o", enginePath, "./cmd")
	build.Dir = filepath.Join(overlayDir, "engine")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build engine binary: %v\n%s", err, out)
	}

	stateDir := filepath.Join(home, "state")
	// runHelper runs the named bash helper and returns its combined output
	// alongside its exit status. Callers assert on both explicitly --
	// these helpers now return honest non-zero exits for "not built" and
	// "drift" (R4-silent-package-skip), so a blanket t.Fatalf on any
	// non-zero exit would defeat the very thing this test proves.
	runHelper := func(script string) (string, error) {
		t.Helper()
		cmd := exec.Command("bash", "-c", `source "$1"; `+script, "_", overlay)
		cmd.Dir = overlayDir
		cmd.Env = append(os.Environ(), "HOME="+home, "STATE_DIR="+stateDir, "OVERLAY_DIR="+overlayDir)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	// Before any build: status honestly reports "not built" and exits
	// non-zero; sync-check emits VERDICT:pi:NOT_BUILT and exits non-zero
	// too -- neither state is healthy, so neither may exit 0
	// (R4-silent-package-skip).
	statusBefore, statusBeforeErr := runHelper("pipkg_status_and_report")
	if statusBeforeErr == nil {
		t.Fatalf("status before build should exit non-zero (not built), got success: %s", statusBefore)
	}
	if !strings.Contains(statusBefore, "not built") {
		t.Fatalf("status before build should report 'not built', got: %s", statusBefore)
	}
	syncBefore, syncBeforeErr := runHelper("pipkg_sync_check_and_report")
	if syncBeforeErr == nil {
		t.Fatalf("sync-check before build should exit non-zero (not built), got success: %s", syncBefore)
	}
	if !strings.Contains(syncBefore, "VERDICT:pi:NOT_BUILT") {
		t.Fatalf("sync-check before build should emit VERDICT:pi:NOT_BUILT, got: %s", syncBefore)
	}

	// Build: package.json must exist afterward, and the install hint is
	// printed; a successful build exits 0.
	buildOut, buildErr := runHelper("pipkg_build_and_report")
	if buildErr != nil {
		t.Fatalf("build should succeed: %v\n%s", buildErr, buildOut)
	}
	if !strings.Contains(buildOut, "install hint: pi install") {
		t.Fatalf("build should print the install hint, got: %s", buildOut)
	}
	destPkg := filepath.Join(stateDir, "pi", "labdrian-pi", "package.json")
	if _, statErr := os.Stat(destPkg); statErr != nil {
		t.Fatalf("expected package.json to be built at %s: %v", destPkg, statErr)
	}

	// Right after build: status reports built, no drift, and exits 0;
	// sync-check reports no drift, emits VERDICT:pi:IN_SYNC, and exits 0.
	statusAfter, statusAfterErr := runHelper("pipkg_status_and_report")
	if statusAfterErr != nil {
		t.Fatalf("status right after build should exit 0: %v\n%s", statusAfterErr, statusAfter)
	}
	if !strings.Contains(statusAfter, "no drift") {
		t.Fatalf("status right after build should report no drift, got: %s", statusAfter)
	}
	syncAfter, syncAfterErr := runHelper("pipkg_sync_check_and_report")
	if syncAfterErr != nil {
		t.Fatalf("sync-check right after build should exit 0: %v\n%s", syncAfterErr, syncAfter)
	}
	if !strings.Contains(syncAfter, "SYNC_CHECK:pi: no drift") || !strings.Contains(syncAfter, "VERDICT:pi:IN_SYNC") {
		t.Fatalf("sync-check right after build should report no drift and VERDICT:pi:IN_SYNC, got: %q", syncAfter)
	}

	// Edit one built file directly: drift must now be reported (R-003),
	// with VERDICT:pi:DRIFT and a non-zero exit.
	if err := os.WriteFile(destPkg, []byte(`{"name":"tampered"}`), 0644); err != nil {
		t.Fatalf("tamper with built package.json: %v", err)
	}
	syncDrift, syncDriftErr := runHelper("pipkg_sync_check_and_report")
	if syncDriftErr == nil {
		t.Fatalf("sync-check after tampering should exit non-zero (drift), got success: %s", syncDrift)
	}
	if !strings.Contains(syncDrift, "SYNC_CHECK:pi: drift") || !strings.Contains(syncDrift, "package.json") || !strings.Contains(syncDrift, "VERDICT:pi:DRIFT") {
		t.Fatalf("sync-check after tampering must report drift naming package.json and VERDICT:pi:DRIFT, got: %s", syncDrift)
	}
	statusDrift, statusDriftErr := runHelper("pipkg_status_and_report")
	if statusDriftErr == nil {
		t.Fatalf("status after tampering should exit non-zero (drift), got success: %s", statusDrift)
	}
	if !strings.Contains(statusDrift, "drift detected") {
		t.Fatalf("status after tampering should report drift detected, got: %s", statusDrift)
	}
}
