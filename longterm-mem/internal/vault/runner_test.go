package vault

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeFixtureScript creates an executable POSIX shell script at
// filepath.Join(dir, name) with body and returns its path.
func writeFixtureScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write fixture script %s: %v", path, err)
	}
	return path
}

func TestRunner_RefusesOutsideVaultRoot(t *testing.T) {
	vaultRoot := t.TempDir()
	outsideDir := t.TempDir()
	marker := filepath.Join(outsideDir, "ran.marker")
	script := writeFixtureScript(t, outsideDir, "outside.sh", "#!/bin/sh\n: > '"+marker+"'\n")

	runner := &Runner{Root: vaultRoot}
	_, _, _, err := runner.Run(context.Background(), script)

	if err == nil {
		t.Fatal("expected Run to refuse a script outside the vault root, got nil error")
	}
	if !strings.Contains(err.Error(), "outside") {
		t.Errorf("expected the refusal error to mention the vault-root boundary, got: %v", err)
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Error("expected no subprocess to have run — marker file should not exist")
	}
}

func TestRunner_RunInterpreted_RefusesOutsideVaultRoot(t *testing.T) {
	vaultRoot := t.TempDir()
	outsideDir := t.TempDir()
	marker := filepath.Join(outsideDir, "ran.marker")
	script := writeFixtureScript(t, outsideDir, "outside.py", "import sys\nopen('"+marker+"', 'w').close()\nsys.exit(0)\n")

	runner := &Runner{Root: vaultRoot}
	_, _, _, err := runner.RunInterpreted(context.Background(), "python3", script)

	if err == nil {
		t.Fatal("expected RunInterpreted to refuse a script outside the vault root, got nil error")
	}
	if !strings.Contains(err.Error(), "outside") {
		t.Errorf("expected the refusal error to mention the vault-root boundary, got: %v", err)
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Error("expected no subprocess to have run — marker file should not exist")
	}
}

func TestRunner_ArgvOnly_NoShellMetacharacters(t *testing.T) {
	vaultRoot := t.TempDir()
	writeFixtureScript(t, vaultRoot, "spy.sh", "#!/bin/sh\nprintf '%s' \"$1\"\n")

	runner := &Runner{Root: vaultRoot}
	malicious := "; rm -rf /"
	stdout, _, exitCode, err := runner.Run(context.Background(), "spy.sh", malicious)

	if err != nil {
		t.Fatalf("Run returned an unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if got := string(stdout); got != malicious {
		t.Errorf("expected the argument to arrive as one literal argv element %q, got %q", malicious, got)
	}
}

func TestRunner_TimeoutSurfacesExitAndStderr(t *testing.T) {
	vaultRoot := t.TempDir()
	writeFixtureScript(t, vaultRoot, "slow.sh", "#!/bin/sh\necho 'starting long operation' >&2\nsleep 5\n")

	runner := &Runner{Root: vaultRoot}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, stderr, exitCode, err := runner.Run(ctx, "slow.sh")
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Run returned an unexpected error on timeout: %v", err)
	}
	if exitCode == 0 {
		t.Fatal("expected a non-zero synthetic exit code on timeout, got 0")
	}
	if !strings.Contains(string(stderr), "starting long operation") {
		t.Errorf("expected captured stderr to contain the pre-timeout output, got %q", string(stderr))
	}
	if elapsed >= 5*time.Second {
		t.Fatalf("Run did not respect the context timeout — took %v", elapsed)
	}
}
