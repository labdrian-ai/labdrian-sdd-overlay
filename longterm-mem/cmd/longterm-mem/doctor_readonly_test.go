package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestResolveProjectFlagReadOnly_LeavesTheIdentityLedgerUntouched: doctor
// documents itself as running "read-only diagnostic checks", but it
// resolved its project through the same path every writing command uses,
// which records the repository's derived names in its identity ledger. A
// diagnostic that mutates the thing it is diagnosing cannot be used to
// inspect a suspect repository without changing it first.
func TestResolveProjectFlagReadOnly_LeavesTheIdentityLedgerUntouched(t *testing.T) {
	repo := declaredRepo(t, "ledger-probe")
	t.Chdir(repo)
	ledger := filepath.Join(repo, ".git", "longterm-mem", "identity-ledger.json")

	if _, err := os.Stat(ledger); err == nil {
		t.Fatalf("fixture already has a ledger at %s; the probe proves nothing", ledger)
	}

	var project string
	captureStderr(t, func() { project, _ = resolveProjectFlagReadOnly("doctor", "") })
	if project == "" {
		t.Fatalf("resolveProjectFlagReadOnly resolved no project inside a repository")
	}

	if _, err := os.Stat(ledger); !os.IsNotExist(err) {
		t.Fatalf("read-only resolution wrote the identity ledger at %s (stat err = %v)", ledger, err)
	}
}

// TestResolveProjectFlag_StillRecordsForWritingCommands is the other half:
// the read-only variant must be a NEW path, not a silent change to the one
// every writing command depends on. If this goes green while the test above
// does too, the ledger still works where it is supposed to.
func TestResolveProjectFlag_StillRecordsForWritingCommands(t *testing.T) {
	repo := declaredRepo(t, "ledger-probe")
	t.Chdir(repo)
	ledger := filepath.Join(repo, ".git", "longterm-mem", "identity-ledger.json")

	captureStderr(t, func() { resolveProjectFlag("sync", "") })

	if _, err := os.Stat(ledger); err != nil {
		t.Fatalf("a writing command's resolution no longer records the ledger at %s: %v", ledger, err)
	}
}
