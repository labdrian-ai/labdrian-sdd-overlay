package installer_test

// What the script does with gadu-generate and status-hooks, and what its help says about them: the
// wrapper forwards gadu-generate to the engine binary with an explicit OVERLAY_DIR and its arguments
// (--check included), status-hooks does not build the engine and exits non-zero when the binary is
// missing, and the usage text exposes both.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

// TestGaduGenerate_ForwardsThroughWrapper validates that wrapper dispatch forwards
// gadu-generate to the engine binary with an explicit OVERLAY_DIR and preserves
// args (including --check).
func TestGaduGenerate_ForwardsThroughWrapper(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	// Missing engine must fail loudly first.
	overlay := overlaytest.Script(t)
	home := t.TempDir()
	overlayDir := t.TempDir()
	env := overlaytest.Env(home, overlayDir)

	if out, err := overlaytest.Combined(t, overlay, env, "", "gadu-generate", "--check"); err == nil {
		t.Fatalf("expected gadu-generate without binary to fail, but exit was 0\noutput:\n%s", out)
	} else if !strings.Contains(out, "engine binary not found") {
		t.Fatalf("expected missing-engine guidance, got: %s", out)
	}

	// Fake binary logs invocation details.
	logPath := filepath.Join(home, "gadu-wrapper.log")
	overlaytest.StubEngine(t, home, "#!/usr/bin/env bash\n"+
		"set -euo pipefail\n"+
		"printf 'OVERLAY_DIR=%s\\n' \"$OVERLAY_DIR\" >"+logPath+"\n"+
		"printf 'ARGC:%s\\n' \"$#\" >>"+logPath+"\n"+
		"for arg in \"$@\"; do printf 'ARG:%s\\n' \"$arg\" >>"+logPath+"; done\n")

	out, err := overlaytest.Combined(t, overlay, env, "", "gadu-generate", "--check", "--future", "value with spaces")
	if err != nil {
		t.Fatalf("gadu-generate with fake engine failed: %v\noutput:\n%s", err, out)
	}

	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake engine log: %v", err)
	}
	gotLog := strings.TrimSpace(string(logged))
	wantPrefix := "OVERLAY_DIR=" + overlayDir
	if !strings.Contains(gotLog, wantPrefix) {
		t.Errorf("expected logged OVERLAY_DIR %q, got %q", wantPrefix, gotLog)
	}
	for _, want := range []string{
		"ARGC:4",
		"ARG:gadu-generate",
		"ARG:--check",
		"ARG:--future",
		"ARG:value with spaces",
	} {
		if !strings.Contains(gotLog, want) {
			t.Errorf("expected fake engine log to contain %q, got %q", want, gotLog)
		}
	}
}

// TestStatusHooks_IsReadOnlyAndFailLoudOnMissingBinary validates that status-hooks
// no longer builds the engine and still exits non-zero when missing.
func TestStatusHooks_IsReadOnlyAndFailLoudOnMissingBinary(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}

	overlay := overlaytest.Script(t)
	home := t.TempDir()
	overlayDir := t.TempDir()
	env := overlaytest.Env(home, overlayDir)

	out, err := overlaytest.Combined(t, overlay, env, "", "status-hooks")
	if err == nil {
		t.Fatalf("expected status-hooks without binary to fail, but exit was 0\noutput:\n%s", out)
	}
	if !strings.Contains(out, "run 'labdrian install-hooks'") {
		t.Fatalf("expected install-hooks guidance, got: %s", out)
	}

	if _, err := os.Stat(filepath.Join(home, ".claude", "bin", "gentle-ai-overlay")); err == nil {
		t.Fatalf("status-hooks should not write or build binary at %s", filepath.Join(home, ".claude", "bin", "gentle-ai-overlay"))
	}
}

// TestOverlay_HelpContainsGaduGenerateAndStatusHooksGuidance asserts the updated
// inline usage text exposes the new command and read-only status-hooks contract.
func TestOverlay_HelpContainsGaduGenerateAndStatusHooksGuidance(t *testing.T) {
	overlay := overlaytest.Script(t)
	home := t.TempDir()
	output, err := overlaytest.Combined(t, overlay, overlaytest.Env(home, t.TempDir()), "", "--help")
	if err != nil {
		t.Fatalf("--help should exit 0: %v\noutput:\n%s", err, output)
	}

	if !strings.Contains(output, "gadu-generate [--check]") {
		t.Fatalf("expected gadu-generate command in help output, got: %s", output)
	}
	if !strings.Contains(output, "status-hooks") {
		t.Fatalf("expected status-hooks line in help output, got: %s", output)
	}
	if !strings.Contains(output, "read-only") {
		t.Fatalf("expected read-only status-hooks guidance in help output, got: %s", output)
	}
}
