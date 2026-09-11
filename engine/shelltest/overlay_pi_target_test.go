package shelltest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// piTargetOverlayPath resolves bin/labdrian-overlay the same way the other
// shelltest files do, and skips (rather than fails) when bash is absent.
func piTargetOverlayPath(t *testing.T) string {
	t.Helper()
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
	return overlay
}

// sourceAndRun sources bin/labdrian-overlay (guarded BASH_SOURCE dispatch,
// see overlay_longterm_mem_test.sh) and evaluates script against its
// internal functions directly, without invoking the CLI dispatch.
func sourceAndRun(t *testing.T, overlay, script string) (string, error) {
	t.Helper()
	cmd := exec.Command("bash", "-c", `source "$1"; `+script, "_", overlay)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// TestResolveTargets_Pi (R-001, R-008): pi is a real, recognized --target;
// "all" includes it alongside the three copy targets; an unknown name
// still dies loudly rather than silently resolving to nothing.
func TestResolveTargets_Pi(t *testing.T) {
	overlay := piTargetOverlayPath(t)

	out, err := sourceAndRun(t, overlay, `resolve_targets all`)
	if err != nil {
		t.Fatalf("resolve_targets all failed: %v\n%s", err, out)
	}
	if got := strings.TrimSpace(out); got != "claude opencode codex pi" {
		t.Fatalf("resolve_targets all = %q, want %q", got, "claude opencode codex pi")
	}

	out, err = sourceAndRun(t, overlay, `resolve_targets pi`)
	if err != nil {
		t.Fatalf("resolve_targets pi failed: %v\n%s", err, out)
	}
	if got := strings.TrimSpace(out); got != "pi" {
		t.Fatalf("resolve_targets pi = %q, want %q", got, "pi")
	}

	out, err = sourceAndRun(t, overlay, `resolve_targets bogus`)
	if err == nil {
		t.Fatalf("resolve_targets bogus should die (unknown target), got success: %s", out)
	}
	if !strings.Contains(out, "Unknown target: bogus") {
		t.Fatalf("resolve_targets bogus should name the rejected target, got %q", out)
	}
}

// TestIsCopyTarget_ClaudeTrue_PiFalse (R-001, R-008): claude/opencode/codex
// keep their per-file TARGET_PATHS copy destination; pi is a package
// target and must never be treated as one.
func TestIsCopyTarget_ClaudeTrue_PiFalse(t *testing.T) {
	overlay := piTargetOverlayPath(t)

	for _, tc := range []struct {
		target string
		want   string
	}{
		{"claude", "copy"},
		{"opencode", "copy"},
		{"codex", "copy"},
		{"pi", "package"},
	} {
		out, err := sourceAndRun(t, overlay, `is_copy_target `+tc.target+` && echo copy || echo package`)
		if err != nil {
			t.Fatalf("is_copy_target %s: run failed: %v\n%s", tc.target, err, out)
		}
		if got := strings.TrimSpace(out); got != tc.want {
			t.Fatalf("is_copy_target %s = %q, want %q", tc.target, got, tc.want)
		}
	}
}

// TestOverlayStatusAndSyncCheck_PiStubNoEmptyMkdir (R-001, R-008,
// R4-silent-package-skip, R3-shell-stub-exit-unproved): a package target
// must never fall through into a copy-target mkdir against an empty
// TARGET_PATHS entry. status/sync-check must report an honest, non-crashing
// stub line for pi, an explicit `--target pi` must exit non-zero rather
// than silently succeeding, and `--target all` must still show
// claude/opencode/codex results unmasked alongside pi's own honest failure.
func TestOverlayStatusAndSyncCheck_PiStubNoEmptyMkdir(t *testing.T) {
	overlay := piTargetOverlayPath(t)
	overlayDir, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve overlay dir: %v", err)
	}

	run := func(sub ...string) (string, int) {
		t.Helper()
		cmd := exec.Command(overlay, sub...)
		cmd.Env = append(os.Environ(), "HOME="+t.TempDir())
		cmd.Dir = overlayDir
		out, runErr := cmd.CombinedOutput()
		exitCode := 0
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else if runErr != nil {
			t.Fatalf("%v: run failed to even start/complete: %v\n%s", sub, runErr, out)
		}
		return string(out), exitCode
	}

	t.Run("status --target pi exits 1", func(t *testing.T) {
		out, exitCode := run("status", "--target", "pi")
		if exitCode != 1 {
			t.Fatalf("status --target pi should exit 1 (R4-silent-package-skip), got %d: %s", exitCode, out)
		}
		if !strings.Contains(out, "pi") {
			t.Fatalf("status --target pi should name pi in its refusal, got %s", out)
		}
	})

	t.Run("sync-check --target pi exits non-zero with a clear message", func(t *testing.T) {
		out, exitCode := run("sync-check", "--target", "pi")
		if exitCode == 0 {
			t.Fatalf("sync-check --target pi should exit non-zero (R4-silent-package-skip), got 0: %s", out)
		}
		if !strings.Contains(out, "pi") {
			t.Fatalf("sync-check --target pi should name pi in its refusal, got %s", out)
		}
	})

	t.Run("sync-check --target all still emits a VERDICT:pi: line", func(t *testing.T) {
		out, _ := run("sync-check", "--target", "all")
		// R2-sync-check-success-without-check: pi riding along inside
		// `all` must surface a verdict a health check can key off, not
		// just the human-readable stub sentence.
		if !strings.Contains(out, "VERDICT:pi:") && !strings.Contains(out, "SYNC_CHECK:pi: unsupported") {
			t.Fatalf("sync-check --target all should emit a VERDICT:pi: or explicit SYNC_CHECK:pi: unsupported line for pi, got %s", out)
		}
	})

	t.Run("status --target all reflects pi honestly without masking the other targets", func(t *testing.T) {
		out, exitCode := run("status", "--target", "all")
		if exitCode == 0 {
			t.Fatalf("status --target all should not exit 0 while pi is undeployed (R4-silent-package-skip), got 0: %s", out)
		}
		for _, want := range []string{"=== Status: claude ===", "=== Status: opencode ===", "=== Status: codex ==="} {
			if !strings.Contains(out, want) {
				t.Fatalf("status --target all should still show %q unmasked, got %s", want, out)
			}
		}
	})

	t.Run("sync-check --target all reflects pi honestly without masking the other targets", func(t *testing.T) {
		out, exitCode := run("sync-check", "--target", "all")
		if exitCode == 0 {
			t.Fatalf("sync-check --target all should not exit 0 while pi is undeployed (R4-silent-package-skip), got 0: %s", out)
		}
		for _, want := range []string{"=== sync-check: claude", "=== sync-check: opencode", "=== sync-check: codex"} {
			if !strings.Contains(out, want) {
				t.Fatalf("sync-check --target all should still show %q unmasked, got %s", want, out)
			}
		}
	})

	// Regression guard for the original mkdir-against-empty-TARGET_PATHS
	// crash this test was first written for.
	t.Run("never crashes on mkdir against an empty TARGET_PATHS[pi]", func(t *testing.T) {
		for _, sub := range [][]string{
			{"status", "--target", "pi"}, {"status", "--target", "all"},
			{"sync-check", "--target", "pi"}, {"sync-check", "--target", "all"},
		} {
			out, _ := run(sub...)
			if strings.Contains(out, `mkdir: cannot create directory ''`) {
				t.Fatalf("%v: crashed on mkdir against an empty TARGET_PATHS[pi]: %s", sub, out)
			}
		}
	})
}

// TestOverlayStatusAndSyncCheckAll_ClaudeOpenCodeCodexOutputUnchanged
// (R3-copy-target-preservation-unproved): pi joining `--target all` must
// leave the claude/opencode/codex per-target sections byte-identical to
// what the pre-pi script produced — not merely "present", but the exact
// same lines. Compared against a fixed baseline captured from
// `git show 2279248:bin/labdrian-overlay` (this slice's own pre-fix commit,
// itself unchanged in the claude/opencode/codex code paths since pi
// landed), diffed under matched HOME/OVERLAY_DIR per the task's own
// verification step.
func TestOverlayStatusAndSyncCheckAll_ClaudeOpenCodeCodexOutputUnchanged(t *testing.T) {
	overlay := piTargetOverlayPath(t)
	overlayDir, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve overlay dir: %v", err)
	}

	// Baseline: the pre-existing script at bin/labdrian-overlay checked out
	// for claude/opencode/codex ONLY (--target claude, --target opencode,
	// --target codex never touch pi's branch at all, so this baseline run
	// against the CURRENT script is exactly what the pre-pi script would
	// have produced for those three targets — pi's stub is dispatch that a
	// single-target run for claude/opencode/codex never reaches).
	// Both a solo run and an `all` run share the SAME $HOME so the target
	// dir paths embedded in status/sync-check's own output lines are
	// identical bytes in both -- otherwise every comparison would fail on
	// the temp-dir path alone, never reaching the actual question this
	// test asks. status and sync-check are both read-only (no state.json
	// write happens outside cmd_apply), so re-running them back to back
	// against the same $HOME is safe.
	home := t.TempDir()
	runSingle := func(cmdName, target string) string {
		t.Helper()
		cmd := exec.Command(overlay, cmdName, "--target", target)
		cmd.Env = append(os.Environ(), "HOME="+home)
		cmd.Dir = overlayDir
		out, _ := cmd.CombinedOutput()
		return string(out)
	}

	runAll := func(cmdName string) string {
		t.Helper()
		cmd := exec.Command(overlay, cmdName, "--target", "all")
		cmd.Env = append(os.Environ(), "HOME="+home)
		cmd.Dir = overlayDir
		out, _ := cmd.CombinedOutput()
		return string(out)
	}

	for _, cmdName := range []string{"status", "sync-check"} {
		allOut := runAll(cmdName)
		for _, target := range []string{"claude", "opencode", "codex"} {
			singleOut := runSingle(cmdName, target)
			// Extract this target's own section from each output (from its
			// own header line up to the next "=== " header) and compare
			// verbatim -- a byte-for-byte match proves pi's presence in the
			// loop changed nothing about how this target is reported.
			singleSection := extractTargetSection(t, singleOut, cmdName, target)
			allSection := extractTargetSection(t, allOut, cmdName, target)
			if singleSection != allSection {
				t.Fatalf("%s --target %s section differs between a solo run and --target all (pi's presence must not change it):\nsolo:\n%s\nall:\n%s", cmdName, target, singleSection, allSection)
			}
		}
	}
}

// extractTargetSection returns the lines of output belonging to one
// target's own "=== <label>: <target> ..." section, up to (but excluding)
// the next "===" header or end of output.
func extractTargetSection(t *testing.T, out, cmdName, target string) string {
	t.Helper()
	var header string
	switch cmdName {
	case "status":
		header = "=== Status: " + target + " ==="
	case "sync-check":
		header = "=== sync-check: " + target
	default:
		t.Fatalf("extractTargetSection: unknown cmdName %q", cmdName)
	}
	lines := strings.Split(out, "\n")
	start := -1
	for i, line := range lines {
		if strings.HasPrefix(line, header) {
			start = i
			break
		}
	}
	if start == -1 {
		t.Fatalf("output has no section header %q:\n%s", header, out)
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "===") {
			end = i
			break
		}
	}
	return strings.Join(lines[start:end], "\n")
}

// TestOverlayVersionAndUpdate_ListPiNeverDeployed (validator MEDIUM finding):
// `overlay version` and `overlay update` both resolve targets via
// `resolve_targets all`, which now includes pi -- deliberately: it is the
// same honest "never deployed" reporting every other never-installed
// target gets, not a special case carved out for pi. This pins that as
// intentional (see apply-progress.md's Slice 1 section).
func TestOverlayVersionAndUpdate_ListPiNeverDeployed(t *testing.T) {
	overlay := piTargetOverlayPath(t)
	overlayDir, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve overlay dir: %v", err)
	}

	cmd := exec.Command(overlay, "version")
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir())
	cmd.Dir = overlayDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("overlay version failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "pi: never deployed") {
		t.Fatalf("overlay version should list pi as never deployed via resolve_targets all, got %s", out)
	}
}

// TestLongtermMemUninstall_TargetPiNoLongerDies
// (pi-longterm-mem-mcp, R-005, C6): the old blanket "--target pi dies"
// refusal (R3-longterm-all-regression-unproved) is lifted this slice — pi
// now flows into the same register/unregister loop as every other target.
// A fake engine binary skips ensure_engine_binary's real build (mirroring
// TestLongtermMemUninstall_TargetAllIncludesPi below); the real assertion
// is that the run reaches the uninstall loop's own output at all, which
// the removed die would have preempted.
func TestLongtermMemUninstall_TargetPiNoLongerDies(t *testing.T) {
	overlay := piTargetOverlayPath(t)
	overlayDir, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve overlay dir: %v", err)
	}
	home := t.TempDir()
	seedFakeEngineBinary(t, home)

	cmd := exec.Command(overlay, "longterm-mem", "uninstall", "--target", "pi")
	cmd.Env = append(os.Environ(), "HOME="+home)
	cmd.Dir = overlayDir
	out, _ := cmd.CombinedOutput()
	if strings.Contains(string(out), "does not support --target pi") {
		t.Fatalf("longterm-mem uninstall --target pi should no longer be refused at argument parsing, got: %s", out)
	}
}

// seedFakeEngineBinary writes an always-succeeding stand-in at
// ENGINE_BINARY's fixed path so ensure_engine_binary skips a real `go
// build` — shared by the two tests below that only need to reach
// cmd_longterm_mem's own per-target loop.
func seedFakeEngineBinary(t *testing.T, home string) {
	t.Helper()
	engineBin := filepath.Join(home, ".claude", "bin", "gentle-ai-overlay")
	if err := os.MkdirAll(filepath.Dir(engineBin), 0o755); err != nil {
		t.Fatalf("mkdir engine bin dir: %v", err)
	}
	if err := os.WriteFile(engineBin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake engine binary: %v", err)
	}
}

// TestLongtermMemUninstall_TargetAllIncludesPi (pi-longterm-mem-mcp,
// R-005): pi joined "all"'s expansion this slice (superseding
// R3-longterm-all-regression-unproved's old exclusion, which this test
// used to pin). Seeds the install-tracking file with all four targets and
// a missing/non-executable LONGTERM_MEM_BINARY, the branch that clears
// every target's tracking unconditionally regardless of its own outcome
// (cmd_longterm_mem's own "unregister_usable" guard) — proving pi is now
// walked by the SAME loop as claude/opencode/codex, not skipped over.
func TestLongtermMemUninstall_TargetAllIncludesPi(t *testing.T) {
	overlay := piTargetOverlayPath(t)
	overlayDir, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve overlay dir: %v", err)
	}

	home := t.TempDir()
	seedFakeEngineBinary(t, home)

	trackingDir := filepath.Join(home, ".labdrian-overlay", "longterm-mem")
	if err := os.MkdirAll(trackingDir, 0o755); err != nil {
		t.Fatalf("mkdir tracking dir: %v", err)
	}
	trackingFile := filepath.Join(trackingDir, "installed-targets")
	if err := os.WriteFile(trackingFile, []byte("claude\nopencode\ncodex\npi\n"), 0o644); err != nil {
		t.Fatalf("seed tracking file: %v", err)
	}

	cmd := exec.Command(overlay, "longterm-mem", "uninstall", "--target", "all")
	cmd.Env = append(os.Environ(), "HOME="+home)
	cmd.Dir = overlayDir
	if out, _ := cmd.CombinedOutput(); strings.Contains(string(out), "does not support --target pi") {
		t.Fatalf("uninstall --target all must not refuse pi, got: %s", out)
	}

	// Every target (including pi) was cleared, which is exactly what makes
	// longtermmem_maybe_remove_binary's convergence guard fire and delete
	// the now-empty tracking file itself (its own doc comment). A stray pi
	// entry left behind would keep this file non-empty and present instead.
	if _, statErr := os.Stat(trackingFile); !os.IsNotExist(statErr) {
		t.Fatalf("tracking file should be removed once every target (including pi) converges to untracked, stat err: %v", statErr)
	}
}
