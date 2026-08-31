package main

import (
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// TestMain_BuildsIndependentModule asserts that longterm-mem/ compiles as an
// independent Go module — capable of declaring its own third-party
// dependencies — separate from engine/'s zero-dependency module (R-001).
// The build output goes to a scratch directory (-o) so this test never
// leaves a compiled binary inside the module's own source tree.
func TestMain_BuildsIndependentModule(t *testing.T) {
	moduleRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve module root: %v", err)
	}

	cmd := exec.Command("go", "build", "-o", t.TempDir()+string(filepath.Separator), "./...")
	cmd.Dir = moduleRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build ./... failed in %s: %v\n%s", moduleRoot, err, out)
	}
}

// TestRun_DispatchesSyncSubcommand proves "sync" is registered in run's
// switch (7.7), not falling through to the "unknown subcommand" default
// branch -- both paths can return exit 2, so a distinct exit code
// (vault_not_configured, 3, only reachable from inside cmdSync's own
// vaultreg.Resolve call) disambiguates dispatch from the fallback.
func TestRun_DispatchesSyncSubcommand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LONGTERM_MEM_VAULT", "")
	t.Setenv("LONGTERM_MEM_VAULTS_FILE", "")

	got := run([]string{"sync", "--project", "definitely-unconfigured-project"})
	if got != 3 {
		t.Fatalf("run([sync --project ...]) = %d, want 3 (vault_not_configured), proving sync dispatches into cmdSync rather than the unknown-subcommand fallback", got)
	}
}

// TestCmdSync_BothPassesRunDespiteAFailingObservation: promote.Sync steps
// over a broken observation rather than letting it wedge the run, but that
// hardening is worthless if the command treats Sync's error as fatal --
// one unpromotable observation would then stop promote.Propagate from ever
// running, so R-033's soft-delete and supersession statuses would never
// again land for that project, identically on every retry (review finding
// R4-sync-error-aborts-propagate). The command must run both passes,
// report what landed, and exit non-zero.
func TestCmdSync_BothPassesRunDespiteAFailingObservation(t *testing.T) {
	vaultRoot := t.TempDir()

	scriptsDir := filepath.Join(vaultRoot, "scripts")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", scriptsDir, err)
	}
	if err := os.WriteFile(filepath.Join(scriptsDir, "allocate-address.sh"), []byte("#!/bin/sh\nprintf 'c-000900\\n'\n"), 0o755); err != nil {
		t.Fatalf("write allocate fixture: %v", err)
	}

	schema, err := os.ReadFile(filepath.Join("..", "..", "internal", "engram", "testdata", "schema.sql"))
	if err != nil {
		t.Fatalf("read engram schema fixture: %v", err)
	}
	dbPath := filepath.Join(t.TempDir(), "engram.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open fixture db: %v", err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	res, err := db.Exec(`INSERT INTO observations (session_id, sync_id, type, title, content, project, revision_count, pinned, created_at)
		 VALUES ('sess-1', 'sync-broken', 'decision', 'Broken', 'Body.', 'cmd-sync-project', 1, 0, '2026-08-01 00:00:00')`)
	if err != nil {
		t.Fatalf("insert observation: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("last insert id: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close fixture db: %v", err)
	}

	// The observation is already promoted to a page whose engram_revision
	// cannot be parsed, so deciding anything about it fails on every run.
	memoryDir := filepath.Join(vaultRoot, "wiki", "memory")
	if err := os.MkdirAll(memoryDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", memoryDir, err)
	}
	broken := "---\ntype: concept\ntitle: \"Broken\"\naddress: c-000900\nstatus: seed\nengram_id: " +
		strconv.FormatInt(id, 10) + "\nengram_revision: not-a-number\nproject: cmd-sync-project\n---\n\nBody.\n"
	if err := os.WriteFile(filepath.Join(memoryDir, "c-000900.md"), []byte(broken), 0o644); err != nil {
		t.Fatalf("write broken page: %v", err)
	}

	t.Setenv("HOME", t.TempDir())
	t.Setenv("LONGTERM_MEM_VAULT", vaultRoot)
	t.Setenv("LONGTERM_MEM_ENGRAM_DB", dbPath)

	stderrPath := filepath.Join(t.TempDir(), "stderr.txt")
	captured, err := os.Create(stderrPath)
	if err != nil {
		t.Fatalf("create stderr capture: %v", err)
	}
	realStderr := os.Stderr
	os.Stderr = captured
	exit := run([]string{"sync", "--project", "cmd-sync-project"})
	os.Stderr = realStderr
	if err := captured.Close(); err != nil {
		t.Fatalf("close stderr capture: %v", err)
	}

	if exit == 0 {
		t.Fatal("run([sync ...]) = 0, want a non-zero exit: the failing observation must be reported")
	}
	stderr, err := os.ReadFile(stderrPath)
	if err != nil {
		t.Fatalf("read stderr capture: %v", err)
	}
	// Propagate's own error can only appear if the command ran the second
	// pass -- which is exactly what treating Sync's error as fatal would
	// have prevented.
	if !strings.Contains(string(stderr), "propagate") {
		t.Fatalf("the second pass never ran: Sync's failure aborted the command instead of being reported alongside it; stderr:\n%s", stderr)
	}
}
