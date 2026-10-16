package installer_test

// The commands that take longterm-mem out of a home (`longterm-mem uninstall`, and `status` and
// `uninstall` without the build step), run through bin/labdrian-overlay against a sandbox home and the
// real longterm-mem module: what they leave recorded, what they remove, and that an uninstall which
// cannot unregister (no binary, a binary of another version, a hard failure, --purge) still converges.
// The sandbox and the helpers are in overlay_sandbox_test.go and longterm_install_test.go.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestStatusUninstall_SkipBuildStep proves R-014's second scenario
// positively: status and uninstall never invoke the longterm-mem build step
// (a status command that silently rebuilt would still exit 0 and pass a
// weaker "command succeeded" test, so this asserts byte-for-byte content
// and mtime invariance of a pre-placed binary instead). It also covers the
// binary-removal guard (10b.5, D4) end to end, including its dangerous
// negative case explicitly: uninstalling one target while another remains
// installed must leave the binary in place.
func TestStatusUninstall_SkipBuildStep(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	overlay := overlayScript(t)
	overlayDir := realOverlayDir(t)
	home := t.TempDir()
	goEnv := goToolchainEnv(t)

	newEnv := func(stateDir string) []string {
		return append([]string{
			"HOME=" + home,
			"OVERLAY_DIR=" + overlayDir,
			"STATE_DIR=" + stateDir,
			"PATH=" + os.Getenv("PATH"),
		}, goEnv...)
	}

	const dummyContent = "#!/bin/sh\necho dummy\n"

	placeDummyBinary := func(t *testing.T, stateDir string) (string, os.FileInfo) {
		t.Helper()
		binPath := filepath.Join(stateDir, "bin", "longterm-mem")
		if err := os.MkdirAll(filepath.Dir(binPath), 0o755); err != nil {
			t.Fatalf("mkdir bin dir: %v", err)
		}
		if err := os.WriteFile(binPath, []byte(dummyContent), 0o755); err != nil {
			t.Fatalf("write dummy binary: %v", err)
		}
		info, err := os.Stat(binPath)
		if err != nil {
			t.Fatalf("stat dummy binary: %v", err)
		}
		return binPath, info
	}

	seedInstalledTargets := func(t *testing.T, stateDir, content string) string {
		t.Helper()
		trackFile := filepath.Join(stateDir, "longterm-mem", "installed-targets")
		if err := os.MkdirAll(filepath.Dir(trackFile), 0o755); err != nil {
			t.Fatalf("mkdir track dir: %v", err)
		}
		if err := os.WriteFile(trackFile, []byte(content), 0o644); err != nil {
			t.Fatalf("seed track file: %v", err)
		}
		return trackFile
	}

	assertUnchanged := func(t *testing.T, binPath string, before os.FileInfo) {
		t.Helper()
		after, statErr := os.Stat(binPath)
		if statErr != nil {
			t.Fatalf("binary vanished: %v", statErr)
		}
		if !after.ModTime().Equal(before.ModTime()) || after.Size() != before.Size() {
			t.Fatalf("binary was rebuilt: mtime/size changed (before=%v/%d after=%v/%d)", before.ModTime(), before.Size(), after.ModTime(), after.Size())
		}
		content, err := os.ReadFile(binPath)
		if err != nil {
			t.Fatalf("read binary: %v", err)
		}
		if string(content) != dummyContent {
			t.Fatalf("binary content changed — it was rebuilt")
		}
	}

	t.Run("StatusNeverBuilds", func(t *testing.T) {
		stateDir := t.TempDir()
		binPath, before := placeDummyBinary(t, stateDir)

		out, err := runOverlay(t, overlay, newEnv(stateDir), "longterm-mem", "status")
		if err != nil {
			t.Fatalf("longterm-mem status failed: %v\noutput:\n%s", err, out)
		}

		assertUnchanged(t, binPath, before)
	})

	t.Run("UninstallSingleTargetSkipsBuildAndLeavesBinaryInPlace", func(t *testing.T) {
		stateDir := t.TempDir()
		binPath, before := placeDummyBinary(t, stateDir)
		// This is the dangerous case: opencode and codex are still tracked
		// installed. Uninstalling only claude must never remove a binary
		// those still-installed targets depend on.
		trackFile := seedInstalledTargets(t, stateDir, "claude\nopencode\ncodex\n")

		out, err := runOverlay(t, overlay, newEnv(stateDir), "longterm-mem", "uninstall", "--target", "claude")
		if err != nil {
			t.Fatalf("longterm-mem uninstall --target claude failed: %v\noutput:\n%s", err, out)
		}

		assertUnchanged(t, binPath, before)

		remaining, err := os.ReadFile(trackFile)
		if err != nil {
			t.Fatalf("read track file after uninstall: %v", err)
		}
		if strings.Contains(string(remaining), "claude") {
			t.Errorf("expected 'claude' removed from tracked installed targets, got: %q", remaining)
		}
		for _, want := range []string{"opencode", "codex"} {
			if !strings.Contains(string(remaining), want) {
				t.Errorf("expected %q to remain tracked installed, got: %q", want, remaining)
			}
		}
	})

	t.Run("UninstallLastTargetRemovesBinary", func(t *testing.T) {
		stateDir := t.TempDir()
		binPath, _ := placeDummyBinary(t, stateDir)
		// Only codex remains tracked — uninstalling it leaves ZERO
		// install-state targets, which must remove the binary (D4/10b.5).
		seedInstalledTargets(t, stateDir, "codex\n")

		out, err := runOverlay(t, overlay, newEnv(stateDir), "longterm-mem", "uninstall", "--target", "codex")
		if err != nil {
			t.Fatalf("longterm-mem uninstall --target codex failed: %v\noutput:\n%s", err, out)
		}

		if _, statErr := os.Stat(binPath); statErr == nil {
			t.Fatalf("expected binary removed once the last install-state target is gone, but it is still present\noutput:\n%s", out)
		} else if !os.IsNotExist(statErr) {
			t.Fatalf("unexpected stat error: %v", statErr)
		}
	})

	// An absent tracking file must FAIL CLOSED: it means "this entrypoint
	// does not know what is installed", never "nothing is installed", and
	// treating empty as safe-to-wipe destroys exactly what the guard
	// protects (review finding R3-uninstall-guard-fails-open).
	t.Run("UninstallWithNoTrackingFileLeavesBinaryInPlace", func(t *testing.T) {
		stateDir := t.TempDir()
		binPath, _ := placeDummyBinary(t, stateDir)
		// Deliberately no seedInstalledTargets: the file does not exist.
		out, err := runOverlay(t, overlay, newEnv(stateDir), "longterm-mem", "uninstall", "--target", "claude")
		if err != nil {
			t.Fatalf("longterm-mem uninstall --target claude failed: %v\noutput:\n%s", err, out)
		}

		if _, statErr := os.Stat(binPath); statErr != nil {
			t.Fatalf("the shared binary was removed although no tracking file existed to prove nothing else depends on it (stat err=%v)\noutput:\n%s", statErr, out)
		}
		if !strings.Contains(out, "--purge") {
			t.Errorf("the refusal should tell the operator that --purge is the explicit way to force removal; got:\n%s", out)
		}
	})
}

// TestInstall_UninstallRoundTripRemovesTheMcpEntry proves CRITICAL-1's fix
// end to end using the REAL longterm-mem binary, not the two-line dummy
// stub TestStatusUninstall_SkipBuildStep substitutes (that stub exits 0 for
// every invocation, which is exactly why a state-dir mismatch between the
// shell's register and unregister call sites was invisible to the suite).
// Install writes an ownership-tagged MCP entry into Claude Code's own
// configuration; uninstall — invoked through the exact bin/labdrian-overlay
// call site CRITICAL-1 found broken — must actually remove it, leaving the
// unrelated pre-existing entry untouched throughout.
func TestInstall_UninstallRoundTripRemovesTheMcpEntry(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	overlay := overlayScript(t)
	overlayDir := realOverlayDir(t)
	home := t.TempDir()
	// STATE_DIR is deliberately a directory OUTSIDE $HOME/.labdrian-overlay
	// — the exact condition that exposed CRITICAL-1: install and uninstall
	// must agree on where install-state.json lives (the module's own
	// default, ~/.labdrian-overlay/longterm-mem, D9) regardless of where
	// STATE_DIR (the shared binary's own location) happens to point.
	stateDir := t.TempDir()
	env := append([]string{
		"HOME=" + home,
		"OVERLAY_DIR=" + overlayDir,
		"STATE_DIR=" + stateDir,
		"PATH=" + os.Getenv("PATH"),
	}, goToolchainEnv(t)...)

	claudeConfig := filepath.Join(home, ".claude.json")
	const seedContent = `{"mcpServers":{"unrelated":{"type":"stdio","command":"/bin/true","args":[]}}}`
	if err := os.WriteFile(claudeConfig, []byte(seedContent), 0o600); err != nil {
		t.Fatalf("seed .claude.json: %v", err)
	}

	installOut, err := runOverlay(t, overlay, env, "longterm-mem", "install", "--target", "claude")
	if err != nil {
		t.Fatalf("longterm-mem install --target claude failed: %v\noutput:\n%s", err, installOut)
	}

	afterInstall, err := os.ReadFile(claudeConfig)
	if err != nil {
		t.Fatalf("read .claude.json after install: %v", err)
	}
	if !strings.Contains(string(afterInstall), `"longterm-mem"`) {
		t.Fatalf("expected an ownership-tagged longterm-mem MCP entry after install, got:\n%s", afterInstall)
	}
	if !strings.Contains(string(afterInstall), `"unrelated"`) {
		t.Fatalf("install must not disturb the unrelated pre-existing entry, got:\n%s", afterInstall)
	}

	uninstallOut, err := runOverlay(t, overlay, env, "longterm-mem", "uninstall", "--target", "claude")
	if err != nil {
		t.Fatalf("longterm-mem uninstall --target claude failed: %v\noutput:\n%s", err, uninstallOut)
	}

	afterUninstall, err := os.ReadFile(claudeConfig)
	if err != nil {
		t.Fatalf("read .claude.json after uninstall: %v", err)
	}
	if strings.Contains(string(afterUninstall), `"longterm-mem"`) {
		t.Fatalf("CRITICAL-1 regression: the longterm-mem MCP entry is still present after uninstall (register/unregister disagreed on install-state.json's location), got:\n%s", afterUninstall)
	}
	if !strings.Contains(string(afterUninstall), `"unrelated"`) {
		t.Fatalf("uninstall must not remove the unrelated pre-existing entry, got:\n%s", afterUninstall)
	}
}

// TestUninstall_HardFailureKeepsTrackingAndSharedBinary proves the other
// half of CRITICAL-1's fix: bin/labdrian-overlay inspects unregister's real
// exit code rather than swallowing it with `|| warn ... continuing`. Only
// exit 0 (removed) and exit 6 (unmanaged — an entry longterm-mem does not
// own) may clear this target's bash-level tracking; any other exit status
// must keep the target tracked and leave the shared binary in place, so the
// binary-removal guard can never fire for a target whose entry may still be
// present in its runtime config.
func TestUninstall_HardFailureKeepsTrackingAndSharedBinary(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	overlay := overlayScript(t)
	overlayDir := realOverlayDir(t)
	home := t.TempDir()
	stateDir := t.TempDir()
	env := append([]string{
		"HOME=" + home,
		"OVERLAY_DIR=" + overlayDir,
		"STATE_DIR=" + stateDir,
		"PATH=" + os.Getenv("PATH"),
	}, goToolchainEnv(t)...)

	claudeConfig := filepath.Join(home, ".claude.json")
	if err := os.WriteFile(claudeConfig, []byte(`{"mcpServers":{}}`), 0o600); err != nil {
		t.Fatalf("seed .claude.json: %v", err)
	}

	installOut, err := runOverlay(t, overlay, env, "longterm-mem", "install", "--target", "claude")
	if err != nil {
		t.Fatalf("install failed: %v\noutput:\n%s", err, installOut)
	}

	// install-state.json lives at the module's own default state dir
	// (~/.labdrian-overlay/longterm-mem, D9), independent of $STATE_DIR.
	// Corrupt it so the next unregister call hits a REAL hard failure
	// (LoadInstallState's JSON parse error, exit 1), not merely "no
	// record" (exit 6, which is the recoverable, tracking-clearing case
	// this test must NOT trigger).
	installStatePath := filepath.Join(home, ".labdrian-overlay", "longterm-mem", "install-state.json")
	if _, statErr := os.Stat(installStatePath); statErr != nil {
		t.Fatalf("expected install-state.json at %s after install: %v", installStatePath, statErr)
	}
	if err := os.WriteFile(installStatePath, []byte("{not valid json"), 0o600); err != nil {
		t.Fatalf("corrupt install-state.json: %v", err)
	}

	binPath := filepath.Join(stateDir, "bin", "longterm-mem")
	binBefore, statErr := os.Stat(binPath)
	if statErr != nil {
		t.Fatalf("expected binary at %s after install: %v", binPath, statErr)
	}

	uninstallOut, err := runOverlay(t, overlay, env, "longterm-mem", "uninstall", "--target", "claude")
	if err == nil {
		t.Fatalf("expected uninstall to report failure (corrupted install-state.json forces unregister exit 1), got success\noutput:\n%s", uninstallOut)
	}

	binAfter, statErr := os.Stat(binPath)
	if statErr != nil {
		t.Fatalf("shared binary removed after a failed uninstall: %v", statErr)
	}
	if !binAfter.ModTime().Equal(binBefore.ModTime()) {
		t.Errorf("shared binary was rebuilt/replaced after a failed uninstall")
	}

	trackFile := filepath.Join(stateDir, "longterm-mem", "installed-targets")
	tracked, err := os.ReadFile(trackFile)
	if err != nil {
		t.Fatalf("read tracking file: %v", err)
	}
	if !strings.Contains(string(tracked), "claude") {
		t.Errorf("expected claude to remain tracked as installed after a failed uninstall, got: %q", tracked)
	}
}

// TestUninstall_MissingBinaryStillConverges: an uninstall that cannot run
// the binary at all must still finish. Keeping the target tracked would
// wedge the run permanently -- every retry reproduces the same state, and
// the only escape (--purge) is the one that destroys the shared binary
// while leaving the entries it could have removed. A state no operator
// action can resolve must not be treated as a recoverable failure.
func TestUninstall_MissingBinaryStillConverges(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	overlay := overlayScript(t)
	overlayDir := realOverlayDir(t)
	home := t.TempDir()
	stateDir := t.TempDir()
	env := append([]string{
		"HOME=" + home,
		"OVERLAY_DIR=" + overlayDir,
		"STATE_DIR=" + stateDir,
		"PATH=" + os.Getenv("PATH"),
	}, goToolchainEnv(t)...)

	claudeConfig := filepath.Join(home, ".claude.json")
	if err := os.WriteFile(claudeConfig, []byte(`{"mcpServers":{}}`), 0o600); err != nil {
		t.Fatalf("seed .claude.json: %v", err)
	}

	installOut, err := runOverlay(t, overlay, env, "longterm-mem", "install", "--target", "claude")
	if err != nil {
		t.Fatalf("install failed: %v\noutput:\n%s", err, installOut)
	}

	// The binary is gone by the time uninstall runs -- a wiped bin
	// directory, a partially restored backup, an interrupted upgrade.
	binPath := filepath.Join(stateDir, "bin", "longterm-mem")
	if err := os.Remove(binPath); err != nil {
		t.Fatalf("remove binary: %v", err)
	}

	uninstallOut, err := runOverlay(t, overlay, env, "longterm-mem", "uninstall", "--target", "claude")
	if err != nil {
		t.Fatalf("uninstall did not converge with the binary missing: %v\noutput:\n%s", err, uninstallOut)
	}
	if !strings.Contains(uninstallOut, "by hand") {
		t.Errorf("uninstall did not tell the operator the MCP entries were left behind:\n%s", uninstallOut)
	}

	trackFile := filepath.Join(stateDir, "longterm-mem", "installed-targets")
	tracked, err := os.ReadFile(trackFile)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read tracking file: %v", err)
	}
	if strings.Contains(string(tracked), "claude") {
		t.Errorf("claude stayed tracked after a converging uninstall, so the run can never finish; got: %q", tracked)
	}
}

// TestUninstall_VersionSkewStillConverges pins the other half of the
// convergence rule the resilience lens asked for. A binary that runs but
// rejects the invocation (exit 2 — a subcommand or flag it does not know,
// i.e. version skew) is in the same class as a missing one: every re-run
// reproduces it exactly, so blocking on it would leave the shared binary
// and the engine record stranded with --purge, the action that orphans
// the entries, as the only escape. It must converge, and it must say what
// was left behind.
//
// Only the missing-binary half had an automated test; this is the branch
// that was proved by hand.
func TestUninstall_VersionSkewStillConverges(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	overlay := overlayScript(t)
	overlayDir := realOverlayDir(t)
	home := t.TempDir()
	stateDir := t.TempDir()
	env := append([]string{
		"HOME=" + home,
		"OVERLAY_DIR=" + overlayDir,
		"STATE_DIR=" + stateDir,
		"PATH=" + os.Getenv("PATH"),
	}, goToolchainEnv(t)...)

	claudeConfig := filepath.Join(home, ".claude.json")
	if err := os.WriteFile(claudeConfig, []byte(`{"mcpServers":{}}`), 0o600); err != nil {
		t.Fatalf("seed .claude.json: %v", err)
	}

	installOut, err := runOverlay(t, overlay, env, "longterm-mem", "install", "--target", "claude")
	if err != nil {
		t.Fatalf("install failed: %v\noutput:\n%s", err, installOut)
	}

	// Replace the installed binary with one that runs and refuses the
	// invocation, the way a genuinely older build would.
	// Unlink first: the just-executed binary is still mapped, so writing
	// over it in place fails with ETXTBSY.
	binPath := filepath.Join(stateDir, "bin", "longterm-mem")
	if err := os.Remove(binPath); err != nil {
		t.Fatalf("remove installed binary: %v", err)
	}
	if err := os.WriteFile(binPath, []byte("#!/bin/sh\necho 'unknown subcommand: unregister' >&2\nexit 2\n"), 0o755); err != nil {
		t.Fatalf("install version-skewed binary: %v", err)
	}

	uninstallOut, err := runOverlay(t, overlay, env, "longterm-mem", "uninstall", "--target", "claude")
	if err != nil {
		t.Fatalf("uninstall did not converge against a version-skewed binary: %v\noutput:\n%s", err, uninstallOut)
	}
	if !strings.Contains(uninstallOut, "version skew") {
		t.Errorf("uninstall did not name the version skew it hit:\n%s", uninstallOut)
	}
	if !strings.Contains(uninstallOut, "by hand") {
		t.Errorf("uninstall did not tell the operator the entry was left behind:\n%s", uninstallOut)
	}

	trackFile := filepath.Join(stateDir, "longterm-mem", "installed-targets")
	tracked, err := os.ReadFile(trackFile)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read tracking file: %v", err)
	}
	if strings.Contains(string(tracked), "claude") {
		t.Errorf("claude stayed tracked, so the run can never finish; got: %q", tracked)
	}
}

// TestUninstall_PurgeForcesRemovalWithAnotherTargetStillTracked pins
// SUGGESTION-4's first half: --purge is the documented rollback procedure
// (design.md:60) and the one path that can orphan an MCP entry. Uninstalling
// only claude with --purge must still force removal of the shared binary
// and the engine-owned registration record even though opencode remains
// installed and tracked — exactly the state in which --purge orphans an
// entry on purpose. opencode's own MCP entry is left untouched (its target
// was never named, so unregister never ran for it), but its bash-level
// tracking is wiped along with claude's by the same rm -f, so a later
// `uninstall --target opencode` would find nothing to do even though
// opencode's runtime config still carries the entry.
func TestUninstall_PurgeForcesRemovalWithAnotherTargetStillTracked(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	overlay := overlayScript(t)
	overlayDir := realOverlayDir(t)
	home := t.TempDir()
	stateDir := t.TempDir()
	env := append([]string{
		"HOME=" + home,
		"OVERLAY_DIR=" + overlayDir,
		"STATE_DIR=" + stateDir,
		"PATH=" + os.Getenv("PATH"),
	}, goToolchainEnv(t)...)

	claudeConfig := filepath.Join(home, ".claude.json")
	if err := os.WriteFile(claudeConfig, []byte(`{"mcpServers":{}}`), 0o600); err != nil {
		t.Fatalf("seed .claude.json: %v", err)
	}
	opencodeConfigDir := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(opencodeConfigDir, 0o755); err != nil {
		t.Fatalf("mkdir opencode config dir: %v", err)
	}
	opencodeConfig := filepath.Join(opencodeConfigDir, "opencode.json")
	if err := os.WriteFile(opencodeConfig, []byte(`{"mcp":{}}`), 0o600); err != nil {
		t.Fatalf("seed opencode.json: %v", err)
	}

	// Two real installs, both real registrations: register cannot create a
	// runtime's config file from nothing (it only edits an existing one),
	// so opencode.json above must exist before this call for opencode to
	// end up with a real, ownership-tagged entry.
	for _, target := range []string{"claude", "opencode"} {
		out, err := runOverlay(t, overlay, env, "longterm-mem", "install", "--target", target)
		if err != nil {
			t.Fatalf("install --target %s failed: %v\noutput:\n%s", target, err, out)
		}
	}

	opencodeBefore, err := os.ReadFile(opencodeConfig)
	if err != nil {
		t.Fatalf("read opencode.json after install: %v", err)
	}
	if !strings.Contains(string(opencodeBefore), `"longterm-mem"`) {
		t.Fatalf("expected opencode.json to carry the longterm-mem entry after install, got: %q", opencodeBefore)
	}

	trackFile := filepath.Join(stateDir, "longterm-mem", "installed-targets")
	trackedBefore, err := os.ReadFile(trackFile)
	if err != nil {
		t.Fatalf("read tracking file after install: %v", err)
	}
	for _, want := range []string{"claude", "opencode"} {
		if !strings.Contains(string(trackedBefore), want) {
			t.Fatalf("expected %q tracked as installed before uninstall, got: %q", want, trackedBefore)
		}
	}

	binPath := filepath.Join(stateDir, "bin", "longterm-mem")
	regPath := filepath.Join(stateDir, "longterm-mem-registration.json")
	if _, statErr := os.Stat(regPath); statErr != nil {
		t.Fatalf("expected engine registration record at %s after install: %v", regPath, statErr)
	}

	uninstallOut, err := runOverlay(t, overlay, env, "longterm-mem", "uninstall", "--target", "claude", "--purge")
	if err != nil {
		t.Fatalf("uninstall --target claude --purge failed: %v\noutput:\n%s", err, uninstallOut)
	}

	if _, statErr := os.Stat(binPath); !os.IsNotExist(statErr) {
		t.Errorf("expected --purge to remove the shared binary even though opencode remains installed; stat err=%v", statErr)
	}
	if _, statErr := os.Stat(regPath); !os.IsNotExist(statErr) {
		t.Errorf("expected --purge to remove the engine registration record even though opencode remains installed; stat err=%v", statErr)
	}
	if _, statErr := os.Stat(trackFile); !os.IsNotExist(statErr) {
		t.Errorf("expected --purge to clear the bash-level tracking file entirely (rm -f), orphaning opencode's tracking too; stat err=%v", statErr)
	}

	opencodeAfter, err := os.ReadFile(opencodeConfig)
	if err != nil {
		t.Fatalf("read opencode.json after purge: %v", err)
	}
	if !strings.Contains(string(opencodeAfter), `"longterm-mem"`) {
		t.Errorf("expected opencode's own MCP entry to be left behind (orphaned) by --target claude --purge, got: %q", opencodeAfter)
	}

	claudeAfter, err := os.ReadFile(claudeConfig)
	if err != nil {
		t.Fatalf("read .claude.json after purge: %v", err)
	}
	if strings.Contains(string(claudeAfter), `"longterm-mem"`) {
		t.Errorf("expected claude's own entry actually unregistered by --target claude --purge, got: %q", claudeAfter)
	}
}

// TestUninstall_PurgeForcesRemovalThroughHardUnregisterFailure pins
// SUGGESTION-4's second half, the one that matters most: without --purge, a
// hard unregister failure (exit 1) keeps the target tracked and leaves the
// shared binary and engine registration record untouched
// (TestUninstall_HardFailureKeepsTrackingAndSharedBinary). --purge forces
// cleanup through that exact failure — the state in which it orphans
// entries on purpose — and still surfaces the failure on the process exit
// code, so an operator reaching for it is told cleanup happened AND that
// something was not cleanly unregistered.
func TestUninstall_PurgeForcesRemovalThroughHardUnregisterFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	overlay := overlayScript(t)
	overlayDir := realOverlayDir(t)
	home := t.TempDir()
	stateDir := t.TempDir()
	env := append([]string{
		"HOME=" + home,
		"OVERLAY_DIR=" + overlayDir,
		"STATE_DIR=" + stateDir,
		"PATH=" + os.Getenv("PATH"),
	}, goToolchainEnv(t)...)

	claudeConfig := filepath.Join(home, ".claude.json")
	if err := os.WriteFile(claudeConfig, []byte(`{"mcpServers":{}}`), 0o600); err != nil {
		t.Fatalf("seed .claude.json: %v", err)
	}

	installOut, err := runOverlay(t, overlay, env, "longterm-mem", "install", "--target", "claude")
	if err != nil {
		t.Fatalf("install failed: %v\noutput:\n%s", err, installOut)
	}

	// Corrupt install-state.json so the next unregister call hits a REAL
	// hard failure (LoadInstallState's JSON parse error, exit 1) — the same
	// technique TestUninstall_HardFailureKeepsTrackingAndSharedBinary uses.
	installStatePath := filepath.Join(home, ".labdrian-overlay", "longterm-mem", "install-state.json")
	if _, statErr := os.Stat(installStatePath); statErr != nil {
		t.Fatalf("expected install-state.json at %s after install: %v", installStatePath, statErr)
	}
	if err := os.WriteFile(installStatePath, []byte("{not valid json"), 0o600); err != nil {
		t.Fatalf("corrupt install-state.json: %v", err)
	}

	binPath := filepath.Join(stateDir, "bin", "longterm-mem")
	regPath := filepath.Join(stateDir, "longterm-mem-registration.json")
	if _, statErr := os.Stat(regPath); statErr != nil {
		t.Fatalf("expected engine registration record at %s after install: %v", regPath, statErr)
	}

	uninstallOut, err := runOverlay(t, overlay, env, "longterm-mem", "uninstall", "--target", "claude", "--purge")
	if err == nil {
		t.Fatalf("expected uninstall --purge to still surface the hard unregister failure on the exit code, got success\noutput:\n%s", uninstallOut)
	}

	if _, statErr := os.Stat(binPath); !os.IsNotExist(statErr) {
		t.Errorf("expected --purge to force-remove the shared binary despite the hard unregister failure; stat err=%v", statErr)
	}
	if _, statErr := os.Stat(regPath); !os.IsNotExist(statErr) {
		t.Errorf("expected --purge to force-remove the engine registration record despite the hard unregister failure; stat err=%v", statErr)
	}

	trackFile := filepath.Join(stateDir, "longterm-mem", "installed-targets")
	if _, statErr := os.Stat(trackFile); !os.IsNotExist(statErr) {
		t.Errorf("expected --purge to clear the bash-level tracking file despite the hard unregister failure; stat err=%v", statErr)
	}

	claudeAfter, err := os.ReadFile(claudeConfig)
	if err != nil {
		t.Fatalf("read .claude.json after purge: %v", err)
	}
	if !strings.Contains(string(claudeAfter), `"longterm-mem"`) {
		t.Errorf("expected claude's own entry to remain (unregister genuinely failed, --purge only forces the shared cleanup), got: %q", claudeAfter)
	}
}
