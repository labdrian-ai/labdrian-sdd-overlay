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

// The vault's file system adapter names no consumer, and what an operator reads when it fails has always
// begun "promote: ": the composition root is where that is decided, and openVault is the one place it is.
func TestOpenVault_ReportsFailuresUnderThePromotePrefix(t *testing.T) {
	vaultRoot := t.TempDir()
	writeCorruptSidecar(t, vaultRoot)

	_, err := openVault(vaultRoot).LoadPrecedence()
	if err == nil {
		t.Fatal("LoadPrecedence = nil error, want the parse failure")
	}
	if want := "promote: parse " + filepath.Join(vaultRoot, ".raw", ".longterm-mem-manifest.json") + ": "; !strings.HasPrefix(err.Error(), want) {
		t.Errorf("error %q does not start with %q", err, want)
	}
}

// The prefix reaches the operator through the command, not only through the adapter: promote reconcile over
// a sidecar that cannot be parsed prints it on stderr.
func TestCmdPromoteReconcile_ACorruptSidecarIsReportedUnderThePromotePrefix(t *testing.T) {
	const address = "c-000701"
	vaultRoot := vaultWithUnrecordedPage(t, address)
	writeCorruptSidecar(t, vaultRoot)
	useVault(t, vaultRoot)

	stderr := captureStderr(t, func() {
		if exit := run([]string{"promote", "reconcile", "--project", "reconcile-project", address}); exit == exitOK {
			t.Errorf("promote reconcile exited %d over a sidecar it cannot parse, want a failure", exit)
		}
	})
	if want := "longterm-mem: promote reconcile: promote: parse "; !strings.Contains(stderr, want) {
		t.Errorf("stderr = %q, want it to contain %q", stderr, want)
	}
}

// Every command gets its vault files from openVault. A file of the command that built the adapter itself
// would report its failures without the prefix, and nothing at that site would say so.
func TestCommandsOpenTheVaultThroughOpenVault(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") || name == "vaultfiles.go" {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(src), "vaultfs.New(") {
			t.Errorf("%s calls vaultfs.New itself; commands open the vault through openVault", name)
		}
	}
}
