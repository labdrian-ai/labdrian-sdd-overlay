package engram

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// newFixtureDB creates a SQLite file at dir/engram.db from testdata/schema.sql
// through a writable connection (never through the production read-only
// Open), then closes it and returns the resulting path. Tests build fixtures
// under t.TempDir() only — the real ~/.engram/engram.db is never touched.
// The fixture is put in WAL journal mode, matching Engram's own concurrent
// writer in production, so tests do not exercise a journal mode the real
// database is unlikely to use.
func newFixtureDB(t *testing.T, dir string) string {
	t.Helper()

	schema, err := os.ReadFile(filepath.Join("testdata", "schema.sql"))
	if err != nil {
		t.Fatalf("read testdata/schema.sql: %v", err)
	}

	path := filepath.Join(dir, "engram.db")
	setup, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open fixture setup connection: %v", err)
	}
	defer setup.Close()

	if _, err := setup.Exec("PRAGMA journal_mode=WAL"); err != nil {
		t.Fatalf("set fixture journal_mode=WAL: %v", err)
	}
	if _, err := setup.Exec(string(schema)); err != nil {
		t.Fatalf("apply testdata/schema.sql: %v", err)
	}

	return path
}

// insertObservation inserts one fixture observation row through a writable
// setup connection to dbPath (never through the production Store).
func insertObservation(t *testing.T, dbPath, title, project string, deletedAt sql.NullString) {
	t.Helper()

	setup, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open fixture setup connection: %v", err)
	}
	defer setup.Close()

	_, err = setup.Exec(
		`INSERT INTO observations (session_id, type, title, content, project, deleted_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"sess-1", "discovery", title, "fixture content", project, deletedAt,
	)
	if err != nil {
		t.Fatalf("insert fixture observation %q: %v", title, err)
	}
}

func TestOpen_DefaultIsReadOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	engramDir := filepath.Join(home, ".engram")
	if err := os.MkdirAll(engramDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", engramDir, err)
	}
	newFixtureDB(t, engramDir)
	// newFixtureDB names the file <dir>/engram.db, matching Engram's own
	// default file name, so no rename is needed for the default path case.

	store, err := Open("")
	if err != nil {
		t.Fatalf("Open(\"\") on default path: %v", err)
	}
	defer store.Close()

	wantPath := filepath.Join(home, ".engram", "engram.db")
	if store.Path() != wantPath {
		t.Fatalf("Path() = %q, want %q", store.Path(), wantPath)
	}

	var count int
	if err := store.db.QueryRow("SELECT count(*) FROM observations").Scan(&count); err != nil {
		t.Fatalf("read via default connection: %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0 on an empty fixture", count)
	}

	if _, err := store.db.Exec(`INSERT INTO observations (session_id, type, title, content) VALUES ('s', 't', 'x', 'y')`); err == nil {
		t.Fatal("INSERT on the default connection succeeded; want a read-only failure")
	}
}

func TestOpen_OverridePathStaysReadOnly(t *testing.T) {
	dir := t.TempDir()
	dbPath := newFixtureDB(t, dir)

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open(%q): %v", dbPath, err)
	}
	defer store.Close()

	if store.Path() != dbPath {
		t.Fatalf("Path() = %q, want %q", store.Path(), dbPath)
	}

	// (a) A WAL fixture whose writer connection (newFixtureDB's setup conn)
	// has already been closed cleanly must open fine on the primary
	// mode=ro path, with no fallback needed.
	if degraded, cause := store.Degraded(); degraded {
		t.Fatalf("Degraded() = true (%q), want false: primary open should succeed against a cleanly-closed WAL fixture", cause)
	}

	if _, err := store.db.Exec(`UPDATE observations SET title = 'x'`); err == nil {
		t.Fatal("UPDATE on the overridden connection succeeded; want a read-only failure")
	}
}

// TestOpen_ReadsCommittedRowsWhileWriterConnectionOpen covers scenario (b):
// a live writer connection (simulating Engram) must not block the primary
// mode=ro connection from reading committed rows.
func TestOpen_ReadsCommittedRowsWhileWriterConnectionOpen(t *testing.T) {
	dir := t.TempDir()
	dbPath := newFixtureDB(t, dir)

	writer, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open live writer connection: %v", err)
	}
	t.Cleanup(func() { _ = writer.Close() })

	if _, err := writer.Exec(
		`INSERT INTO observations (session_id, type, title, content, project) VALUES (?, ?, ?, ?, ?)`,
		"sess-1", "discovery", "live-writer-row", "fixture content", "labdrian-sdd-overlay",
	); err != nil {
		t.Fatalf("insert via live writer connection: %v", err)
	}
	// writer stays open for the rest of the test.

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open(%q) while writer connection open: %v", dbPath, err)
	}
	defer store.Close()

	if degraded, cause := store.Degraded(); degraded {
		t.Fatalf("Degraded() = true (%q), want false: primary open should succeed against a WAL db with a live writer", cause)
	}

	got, err := store.ListObservations("labdrian-sdd-overlay")
	if err != nil {
		t.Fatalf("ListObservations while writer open: %v", err)
	}
	if len(got) != 1 || got[0].Title != "live-writer-row" {
		t.Fatalf("got = %+v, want one row titled live-writer-row", got)
	}
}

// TestOpen_FallsBackToImmutableWhenPrimaryReadOnlyOpenFails covers scenario
// (c): when the directory is unwritable and no -shm/-wal exists, the primary
// mode=ro connection cannot create the WAL shared-memory index and fails
// (empirically, modernc.org/sqlite v1.57.0 returns "attempt to write a
// readonly database" / SQLITE_READONLY_DIRECTORY here, not SQLITE_CANTOPEN).
// Open must fall back to immutable=1 and return a usable degraded Store.
func TestOpen_FallsBackToImmutableWhenPrimaryReadOnlyOpenFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root ignores directory permissions; fallback path cannot be forced")
	}

	dir := t.TempDir()
	dbPath := newFixtureDB(t, dir)
	insertObservation(t, dbPath, "degraded-row", "labdrian-sdd-overlay", sql.NullString{})

	for _, suffix := range []string{"-wal", "-shm"} {
		_ = os.Remove(dbPath + suffix)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("chmod %s 0o555: %v", dir, err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open(%q) on unwritable dir: %v", dbPath, err)
	}
	defer store.Close()

	degraded, cause := store.Degraded()
	if !degraded {
		t.Fatal("Degraded() = false, want true: primary read-only open should have failed on the unwritable dir")
	}
	if cause == "" {
		t.Fatal("Degraded() cause is empty, want the primary open error text")
	}
	t.Logf("observed primary open error: %s", cause)

	got, err := store.ListObservations("labdrian-sdd-overlay")
	if err != nil {
		t.Fatalf("ListObservations on degraded store: %v", err)
	}
	if len(got) != 1 || got[0].Title != "degraded-row" {
		t.Fatalf("got = %+v, want one row titled degraded-row", got)
	}
}

// insertObservationFull inserts one fixture row with caller-controlled
// type/revision_count/pinned/sync_id, letting R-007 eligibility tests
// (promote.Eligible) exercise real data instead of insertObservation's
// fixed "discovery"/1/false/NULL defaults.
func insertObservationFull(t *testing.T, dbPath, title, project, obsType string, revisionCount int, pinned bool, syncID string) int64 {
	t.Helper()

	setup, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open fixture setup connection: %v", err)
	}
	defer setup.Close()

	res, err := setup.Exec(
		`INSERT INTO observations (session_id, sync_id, type, title, content, project, revision_count, pinned) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"sess-1", syncID, obsType, title, "fixture content", project, revisionCount, pinned,
	)
	if err != nil {
		t.Fatalf("insert fixture observation %q: %v", title, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("last insert id for observation %q: %v", title, err)
	}
	return id
}

// TestListObservations_IncludesEligibilityAndExtraFields proves
// ListObservations' extended SELECT (R-007's Pinned/Type/RevisionCount and
// R-027's SyncID extra) round-trips real column values, not just the four
// fields Slice 1 read.
func TestListObservations_IncludesEligibilityAndExtraFields(t *testing.T) {
	dir := t.TempDir()
	dbPath := newFixtureDB(t, dir)
	_ = insertObservationFull(t, dbPath, "pinned-decision", "labdrian-sdd-overlay", "decision", 5, true, "sync-abc")

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open(%q): %v", dbPath, err)
	}
	defer store.Close()

	got, err := store.ListObservations("labdrian-sdd-overlay")
	if err != nil {
		t.Fatalf("ListObservations: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1; got %+v", len(got), got)
	}

	want := Observation{
		ID: got[0].ID, SyncID: "sync-abc", Type: "decision", Title: "pinned-decision",
		Content: "fixture content", Project: "labdrian-sdd-overlay", RevisionCount: 5, Pinned: true,
	}
	if got[0] != want {
		t.Fatalf("got[0] = %+v, want %+v", got[0], want)
	}
}

func TestListObservations_ScopesProjectAndExcludesSoftDeleted(t *testing.T) {
	dir := t.TempDir()
	dbPath := newFixtureDB(t, dir)

	insertObservation(t, dbPath, "active-in-project", "labdrian-sdd-overlay", sql.NullString{})
	insertObservation(t, dbPath, "soft-deleted-in-project", "labdrian-sdd-overlay", sql.NullString{String: "2026-08-01T00:00:00Z", Valid: true})
	insertObservation(t, dbPath, "active-other-project", "some-other-project", sql.NullString{})

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open(%q): %v", dbPath, err)
	}
	defer store.Close()

	got, err := store.ListObservations("labdrian-sdd-overlay")
	if err != nil {
		t.Fatalf("ListObservations: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1; got %+v", len(got), got)
	}
	if got[0].Title != "active-in-project" {
		t.Fatalf("got[0].Title = %q, want %q", got[0].Title, "active-in-project")
	}
	if got[0].Project != "labdrian-sdd-overlay" {
		t.Fatalf("got[0].Project = %q, want %q", got[0].Project, "labdrian-sdd-overlay")
	}
}
