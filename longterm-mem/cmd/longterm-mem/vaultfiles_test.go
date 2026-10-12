package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeCorruptSidecar leaves a precedence sidecar that is not JSON in the vault at vaultRoot.
func writeCorruptSidecar(t *testing.T, vaultRoot string) {
	t.Helper()
	dir := filepath.Join(vaultRoot, ".raw")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".longterm-mem-manifest.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write the sidecar: %v", err)
	}
}

// The vault's file system adapter names no consumer, and the program asks it for none: a failure begins with
// what failed, and the command that reports it puts its own name in front, once (the owner's decision of
// 2026-10-12; the output used to say "longterm-mem: promote: promote: parse ..."). openVault is the one place
// the adapter is built.
func TestOpenVault_ReportsFailuresWithoutAConsumersName(t *testing.T) {
	vaultRoot := t.TempDir()
	writeCorruptSidecar(t, vaultRoot)

	_, err := openVault(vaultRoot).LoadPrecedence()
	if err == nil {
		t.Fatal("LoadPrecedence = nil error, want the parse failure")
	}
	if want := "parse " + filepath.Join(vaultRoot, ".raw", ".longterm-mem-manifest.json") + ": "; !strings.HasPrefix(err.Error(), want) {
		t.Errorf("error %q does not start with %q", err, want)
	}
}

// The failure reaches the operator through the command with the command's name once: promote reconcile over a
// sidecar that cannot be parsed prints it on stderr.
func TestCmdPromoteReconcile_ACorruptSidecarIsReportedUnderTheCommandsNameOnce(t *testing.T) {
	const address = "c-000701"
	vaultRoot := vaultWithUnrecordedPage(t, address)
	writeCorruptSidecar(t, vaultRoot)
	useVault(t, vaultRoot)

	stderr := captureStderr(t, func() {
		if exit := run([]string{"promote", "reconcile", "--project", "reconcile-project", address}); exit == exitOK {
			t.Errorf("promote reconcile exited %d over a sidecar it cannot parse, want a failure", exit)
		}
	})
	if want := "longterm-mem: promote reconcile: parse "; !strings.Contains(stderr, want) {
		t.Errorf("stderr = %q, want it to contain %q", stderr, want)
	}
	if strings.Contains(stderr, "promote: promote:") {
		t.Errorf("stderr = %q, want the command's name once", stderr)
	}
}
