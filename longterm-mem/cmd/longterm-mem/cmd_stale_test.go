package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stale must report and stop: it names what the repository disagrees with,
// carries the commit as evidence, and changes nothing. Its exit code stays
// 0 on findings -- a memory going out of date is not a build breaking, and
// an exit that reads as failure would put this in CI's way where it does
// not belong.
func TestCmdStale_ReportsFindingsAndChangesNothing(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(repo, "gone.go"), []byte("package a\n// a distinctive body\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	runGit(t, repo, "add", "-A")
	runGit(t, repo, "commit", "-q", "-m", "first")
	runGit(t, repo, "rm", "-q", "gone.go")
	runGit(t, repo, "commit", "-q", "-m", "remove gone.go")

	t.Setenv(engramDBEnvVar, staleFixtureDB(t, "widgets", "**Where**: gone.go\n"))
	t.Setenv(vaultsFileEnvVar, writeVaults(t))
	t.Chdir(repo)

	var exit int
	stdout := captureStdout(t, func() { exit = run([]string{"stale", "--project", "widgets"}) })

	if exit != exitOK {
		t.Fatalf("stale exited %d, want %d: a memory going out of date is not a failure", exit, exitOK)
	}
	if !strings.Contains(stdout, "REMOVED") || !strings.Contains(stdout, "gone.go") {
		t.Fatalf("stale did not report the removed path: %q", stdout)
	}
	if !strings.Contains(stdout, "Nothing was changed") {
		t.Errorf("stale must say plainly that it changed nothing: %q", stdout)
	}
}

// staleFixtureDB builds an Engram database holding one observation, written
// long before any commit the caller's fixture repository makes.
func staleFixtureDB(t *testing.T, project, content string) string {
	t.Helper()
	schema, err := os.ReadFile(filepath.Join("..", "..", "internal", "engram", "testdata", "schema.sql"))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	path := filepath.Join(t.TempDir(), "engram.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open fixture db: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(string(schema)); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO observations (session_id, type, title, content, project, revision_count, pinned, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, 0, 0, ?, ?)`,
		"sess-1", "discovery", "how gone.go works", content, project,
		"2000-01-01 00:00:00", "2000-01-01 00:00:00",
	); err != nil {
		t.Fatalf("insert fixture observation: %v", err)
	}
	return path
}
