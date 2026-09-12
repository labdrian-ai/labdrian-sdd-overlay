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
		cmd.Env = append(os.Environ(), "HOME="+home, "STATE_DIR="+stateDir, "OVERLAY_DIR="+overlayDir,
			// these tests build directly from the checkout (no apply, so no main checkout); pin the deploy ref to HEAD
			"LABDRIAN_PI_DEPLOY_REF=HEAD")
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	// Before any build: status honestly reports "not built" and exits
	// non-zero; sync-check emits VERDICT:pi:NOT_BUILT and exits non-zero
	// too -- neither state is healthy, so neither may exit 0
	// (R4-silent-package-skip). The R-007 --no-extensions/--no-skills
	// disclosure is always present in status, independent of build state.
	statusBefore, statusBeforeErr := runHelper("pipkg_status_and_report")
	if statusBeforeErr == nil {
		t.Fatalf("status before build should exit non-zero (not built), got success: %s", statusBefore)
	}
	if !strings.Contains(statusBefore, "not built") {
		t.Fatalf("status before build should report 'not built', got: %s", statusBefore)
	}
	if !strings.Contains(statusBefore, "--no-extensions") || !strings.Contains(statusBefore, "--no-skills") {
		t.Fatalf("status should always disclose --no-extensions/--no-skills, got: %s", statusBefore)
	}
	if !strings.Contains(statusBefore, "(short aliases: -ne and -ns)") {
		t.Fatalf("disclosure must name the -ne/-ns aliases pi --help documents, got: %s", statusBefore)
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

	// Right after build (W-01): pipkg_status_and_report now delegates to
	// `engine runtime status --target pi`, which proves three owned
	// entries (built+in-sync, listed in ~/.pi/agent/settings.json,
	// longterm-mem MCP-registered) -- not just the bare drift check the
	// old bash-only implementation ran. Nothing was installed or
	// registered yet, so it stays honestly "partial" and exits non-zero,
	// naming the still-missing proof.
	statusAfter, statusAfterErr := runHelper("pipkg_status_and_report")
	if statusAfterErr == nil {
		t.Fatalf("status right after build (before install/register) should exit non-zero (partial), got success: %s", statusAfter)
	}
	if !strings.Contains(statusAfter, "partial") {
		t.Fatalf("status right after build should report partial, got: %s", statusAfter)
	}
	if !strings.Contains(statusAfter, "--no-extensions") || !strings.Contains(statusAfter, "--no-skills") {
		t.Fatalf("status right after build should still disclose --no-extensions/--no-skills (via the delegated adapter message), got: %s", statusAfter)
	}
	// sync-check still reports no drift and exits 0 (unchanged -- W-01
	// scoped the delegation to status only, not sync-check).
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
	if !strings.Contains(statusDrift, "package.json") {
		t.Fatalf("status after tampering should report drift detected, got: %s", statusDrift)
	}
}

// TestPipkgStatusAndReport_DelegatesHonestPartialThenSupported (W-01):
// pipkg_status_and_report must delegate to `engine runtime status --target
// pi` once the package is built, rather than the old bash-only "no
// drift = partial" heuristic. This proves the full honest transition with
// a freshly built engine binary: built-but-unregistered stays "partial"
// (naming the still-missing register step), and only reports "supported"
// once the package is also listed in ~/.pi/agent/settings.json and has a
// longterm-mem MCP entry in mcp.json (the same three owned entries
// PiAdapter.Status() proves -- see TestPiAdapter_StatusTriangulatesAllThreeOwnedEntries
// in engine/runtime/pi_test.go for the unit-level equivalent).
func TestPipkgStatusAndReport_DelegatesHonestPartialThenSupported(t *testing.T) {
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
	runHelper := func(script string) (string, error) {
		t.Helper()
		cmd := exec.Command("bash", "-c", `source "$1"; `+script, "_", overlay)
		cmd.Dir = overlayDir
		cmd.Env = append(os.Environ(), "HOME="+home, "STATE_DIR="+stateDir, "OVERLAY_DIR="+overlayDir,
			// these tests build directly from the checkout (no apply, so no main checkout); pin the deploy ref to HEAD
			"LABDRIAN_PI_DEPLOY_REF=HEAD")
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	buildOut, buildErr := runHelper("pipkg_build_and_report")
	if buildErr != nil {
		t.Fatalf("build should succeed: %v\n%s", buildErr, buildOut)
	}
	destDir := filepath.Join(stateDir, "pi", "labdrian-pi")

	// Built but never registered: partial, naming the missing register step.
	statusPartial, statusPartialErr := runHelper("pipkg_status_and_report")
	if statusPartialErr == nil {
		t.Fatalf("status before register should exit non-zero (partial), got success: %s", statusPartial)
	}
	if !strings.Contains(statusPartial, "partial") {
		t.Fatalf("status before register should report partial, got: %s", statusPartial)
	}
	if !strings.Contains(statusPartial, "longterm-mem register --target pi") {
		t.Fatalf("status before register should name the missing register command, got: %s", statusPartial)
	}

	// Simulate `pi install <destDir>` (writes ~/.pi/agent/settings.json)
	// and `longterm-mem register --target pi` (writes destDir/mcp.json)
	// without depending on either real binary -- this test's own scope is
	// the bash delegation, already proven end-to-end at the unit level.
	settingsPath := filepath.Join(home, ".pi", "agent", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		t.Fatalf("mkdir settings.json dir: %v", err)
	}
	settingsJSON := `{"packages": ["` + destDir + `"]}`
	if err := os.WriteFile(settingsPath, []byte(settingsJSON), 0644); err != nil {
		t.Fatalf("write settings.json: %v", err)
	}
	mcpJSON := `{"mcpServers": {"longterm-mem": {"type": "stdio", "command": "/opt/labdrian-overlay/bin/longterm-mem", "args": ["mcp"]}}}`
	if err := os.WriteFile(filepath.Join(destDir, "mcp.json"), []byte(mcpJSON), 0644); err != nil {
		t.Fatalf("write mcp.json: %v", err)
	}

	// Now every owned entry is proven: supported.
	statusSupported, statusSupportedErr := runHelper("pipkg_status_and_report")
	if statusSupportedErr != nil {
		t.Fatalf("status after listing+registration should exit 0 (supported): %v\n%s", statusSupportedErr, statusSupported)
	}
	if !strings.Contains(statusSupported, "supported") {
		t.Fatalf("status after listing+registration should report supported, got: %s", statusSupported)
	}
}
