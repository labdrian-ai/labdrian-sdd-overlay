package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCmdSync_DryRunWritesNothingAndNamesWhatItWould pins the CLI wiring,
// not the planner. internal/promote's own test proves Plan predicts what
// Sync does; this proves the flag actually reaches it and that the command
// stops before every writing step -- the half a unit test cannot see, and
// the half an operator is relying on when they type --dry-run on a vault
// they care about.
func TestCmdSync_DryRunWritesNothingAndNamesWhatItWould(t *testing.T) {
	vaultRoot := t.TempDir()
	scriptsDir := filepath.Join(vaultRoot, "scripts")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", scriptsDir, err)
	}
	if err := os.WriteFile(filepath.Join(scriptsDir, "allocate-address.sh"), []byte("#!/bin/sh\nprintf 'c-000900\\n'\n"), 0o755); err != nil {
		t.Fatalf("write allocate fixture: %v", err)
	}

	// A hand-authored index the operator wrote themselves. If the dry run
	// touches this, it has already done the thing they were checking on.
	wikiDir := filepath.Join(vaultRoot, "wiki")
	if err := os.MkdirAll(wikiDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", wikiDir, err)
	}
	const handAuthored = "# My index\n\nNotes I wrote by hand.\n"
	indexPath := filepath.Join(wikiDir, "index.md")
	if err := os.WriteFile(indexPath, []byte(handAuthored), 0o644); err != nil {
		t.Fatalf("write hand-authored index: %v", err)
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
	if _, err := db.Exec(`INSERT INTO observations (session_id, sync_id, type, title, content, project, revision_count, pinned, created_at)
		 VALUES ('sess-1', 'sync-dry', 'decision', 'A Decision Worth Promoting', 'Body.', 'dry-run-project', 1, 0, '2026-08-01 00:00:00')`); err != nil {
		t.Fatalf("insert observation: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close fixture db: %v", err)
	}

	t.Setenv("HOME", t.TempDir())
	t.Setenv("LONGTERM_MEM_VAULT", vaultRoot)
	t.Setenv("LONGTERM_MEM_ENGRAM_DB", dbPath)

	before := cmdVaultSnapshot(t, vaultRoot)

	var exit int
	stdout := captureStdout(t, func() { exit = run([]string{"sync", "--project", "dry-run-project", "--dry-run"}) })

	if exit != exitOK {
		t.Fatalf("run([sync --dry-run]) = %d, want %d; stdout:\n%s", exit, exitOK, stdout)
	}
	if after := cmdVaultSnapshot(t, vaultRoot); !cmdEqualSnapshots(before, after) {
		t.Fatalf("--dry-run changed the vault.\nbefore: %v\nafter:  %v", before, after)
	}
	got, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read index after dry run: %v", err)
	}
	if string(got) != handAuthored {
		t.Fatalf("--dry-run rewrote the hand-authored index:\n%s", got)
	}
	if !strings.Contains(stdout, "patch 0 page(s)") {
		t.Fatalf("dry run did not report the second pass at all, so it describes half the command; stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, "would promote 1 observation") {
		t.Fatalf("dry run did not report the count it found; stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, "A Decision Worth Promoting") {
		t.Fatalf("dry run counted without naming, so the operator cannot recognise what would land; stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, "nothing was written") {
		t.Fatalf("dry run did not say it wrote nothing; stdout:\n%s", stdout)
	}
}

func cmdVaultSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	snap := map[string]string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		b, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		snap[rel] = string(b)
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot vault: %v", err)
	}
	return snap
}

func cmdEqualSnapshots(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
