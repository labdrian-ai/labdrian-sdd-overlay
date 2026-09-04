package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// newDegradedEngramDB builds an Engram database that engram.Open can only
// reach through its immutable=1 fallback, and returns its path. It mirrors
// TestCmdStatus_ReportsDegradedEngramConnection's fixture exactly: a WAL
// database with no -wal/-shm on disk, inside a directory nobody may write,
// so the primary mode=ro connection cannot create the shared-memory index.
func newDegradedEngramDB(t *testing.T) string {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("running as root ignores directory permissions; the degraded fallback cannot be forced")
	}

	engramDir := t.TempDir()
	schema, err := os.ReadFile(filepath.Join("..", "..", "internal", "engram", "testdata", "schema.sql"))
	if err != nil {
		t.Fatalf("read engram schema fixture: %v", err)
	}
	dbPath := filepath.Join(engramDir, "engram.db")
	setup, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open fixture setup connection: %v", err)
	}
	if _, err := setup.Exec("PRAGMA journal_mode=WAL"); err != nil {
		t.Fatalf("set fixture journal_mode=WAL: %v", err)
	}
	if _, err := setup.Exec(string(schema)); err != nil {
		t.Fatalf("apply schema fixture: %v", err)
	}
	if err := setup.Close(); err != nil {
		t.Fatalf("close fixture setup connection: %v", err)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		_ = os.Remove(dbPath + suffix)
	}
	if err := os.Chmod(engramDir, 0o555); err != nil {
		t.Fatalf("chmod %s 0o555: %v", engramDir, err)
	}
	t.Cleanup(func() { _ = os.Chmod(engramDir, 0o755) })
	return dbPath
}

// The immutable=1 fallback serves a snapshot frozen at connect time for as
// long as the process holds the connection. `query` declares that -- it
// emits the engram_degraded_snapshot diagnostic -- and until now nothing
// else did, although the SAME store drives the two commands that WRITE
// from it: `sync` promotes every eligible observation and propagates
// relation status, and `promote` writes one page.
//
// A frozen corpus is worse on the write side than on the read side, not
// better: a stale query returns stale answers, while a stale sync writes
// pages that claim to be the current state of a corpus it can no longer
// see, and then records the sync as done. The operator must be able to
// tell a live corpus from a frozen one on EVERY surface, which is the
// whole point of Store.Degraded existing at all.
func TestCmdSync_DeclaresADegradedEngramSnapshot(t *testing.T) {
	dbPath := newDegradedEngramDB(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LONGTERM_MEM_VAULT", t.TempDir())
	t.Setenv("LONGTERM_MEM_ENGRAM_DB", dbPath)

	stderr := captureStderr(t, func() {
		captureStdout(t, func() { run([]string{"sync", "--project", "degraded-write-project"}) })
	})

	if !strings.Contains(stderr, "snapshot") {
		t.Fatalf("sync wrote from a degraded (immutable=1) Engram connection without declaring it; stderr:\n%s", stderr)
	}
}

// The twin surface: `promote` writes one page off the same store, through
// the same Open, and was equally silent about it. It is asserted even
// though this fixture's promotion then fails, because the declaration is
// about the CONNECTION, not about the outcome -- a caller must learn the
// corpus was frozen whether the write landed or not.
func TestCmdPromote_DeclaresADegradedEngramSnapshot(t *testing.T) {
	dbPath := newDegradedEngramDB(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LONGTERM_MEM_VAULT", t.TempDir())
	t.Setenv("LONGTERM_MEM_ENGRAM_DB", dbPath)

	stderr := captureStderr(t, func() {
		captureStdout(t, func() { run([]string{"promote", "--project", "degraded-write-project", "--id", "1"}) })
	})

	if !strings.Contains(stderr, "snapshot") {
		t.Fatalf("promote wrote from a degraded (immutable=1) Engram connection without declaring it; stderr:\n%s", stderr)
	}
}
