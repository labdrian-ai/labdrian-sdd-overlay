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

// TestOverlayStatusAndSyncCheck_PiStubNoEmptyMkdir (R-001, R-008): a
// package target must never fall through into a copy-target mkdir against
// an empty TARGET_PATHS entry. status/sync-check must report an honest,
// non-crashing stub line for pi, and `--target all` must still show
// claude/opencode/codex results unmasked alongside it.
func TestOverlayStatusAndSyncCheck_PiStubNoEmptyMkdir(t *testing.T) {
	overlay := piTargetOverlayPath(t)
	overlayDir, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve overlay dir: %v", err)
	}

	for _, sub := range [][]string{
		{"status", "--target", "pi"},
		{"status", "--target", "all"},
		{"sync-check", "--target", "pi"},
		{"sync-check", "--target", "all"},
	} {
		cmd := exec.Command(overlay, sub...)
		cmd.Env = append(os.Environ(), "HOME="+t.TempDir())
		cmd.Dir = overlayDir
		out, _ := cmd.CombinedOutput() // non-copy targets may report non-zero honestly; only shape matters here
		text := string(out)
		if strings.Contains(text, `mkdir: cannot create directory ''`) {
			t.Fatalf("%v: crashed on mkdir against an empty TARGET_PATHS[pi]: %s", sub, text)
		}
		if !strings.Contains(text, "pi") {
			t.Fatalf("%v: output has no honest line naming pi: %s", sub, text)
		}
	}
}
