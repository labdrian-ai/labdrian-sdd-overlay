package engram

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

// TestCoverageSnapshot_BothCountsComeFromOneRead is the whole reason this
// method exists.
//
// Coverage.Live and Coverage.Indexed used to come from two independent
// calls on the same connection, so nothing guaranteed indexed <= live and
// the query package carried a clamp, a diagnostic, and an untestable
// branch to cope with a disagreement it could not prevent. Reading both
// inside one transaction makes the disagreement impossible instead of
// survivable, which is what lets all three be deleted.
func TestCoverageSnapshot_BothCountsComeFromOneRead(t *testing.T) {
	store, ids := newSnapshotFixture(t, []snapshotRow{
		{title: "live one", project: "p", deleted: false},
		{title: "live two", project: "p", deleted: false},
		{title: "soft deleted", project: "p", deleted: true},
		{title: "other project", project: "q", deleted: false},
	})

	// Every id the index could name, including the soft-deleted one and
	// the other project's -- exactly what a stale manifest looks like.
	live, byID, err := store.CoverageSnapshot("p", ids)
	if err != nil {
		t.Fatalf("CoverageSnapshot: %v", err)
	}

	if live != 2 {
		t.Fatalf("live = %d, want 2 (two live rows in project p)", live)
	}
	if len(byID) != 2 {
		t.Fatalf("len(byID) = %d, want 2; a soft-deleted or foreign-project row was resolved: %+v", len(byID), byID)
	}
	if len(byID) > live {
		t.Fatalf("indexed (%d) exceeded live (%d): the snapshot did not make the two counts consistent", len(byID), live)
	}
}

// TestCoverageSnapshot_NoIDsStillCountsLive: the index may have failed to
// load, and coverage must still be able to say how much memory exists.
func TestCoverageSnapshot_NoIDsStillCountsLive(t *testing.T) {
	store, _ := newSnapshotFixture(t, []snapshotRow{
		{title: "live one", project: "p", deleted: false},
		{title: "live two", project: "p", deleted: false},
	})

	live, byID, err := store.CoverageSnapshot("p", nil)
	if err != nil {
		t.Fatalf("CoverageSnapshot: %v", err)
	}
	if live != 2 {
		t.Fatalf("live = %d, want 2", live)
	}
	if len(byID) != 0 {
		t.Fatalf("len(byID) = %d, want 0", len(byID))
	}
}

// TestCoverageSnapshot_ClosedStoreFailsAsOneError: both halves fail
// together or not at all. The old shape had two error paths, one of which
// was discarded outright -- a failed lookup silently became "indexed = 0",
// which then read as "the whole index is missing".
func TestCoverageSnapshot_ClosedStoreFailsAsOneError(t *testing.T) {
	store, ids := newSnapshotFixture(t, []snapshotRow{{title: "live one", project: "p"}})
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if _, _, err := store.CoverageSnapshot("p", ids); err == nil {
		t.Fatalf("CoverageSnapshot on a closed store returned no error")
	}
}

type snapshotRow struct {
	title   string
	project string
	deleted bool
}

func newSnapshotFixture(t *testing.T, rows []snapshotRow) (*Store, []int64) {
	t.Helper()
	schema, err := os.ReadFile(filepath.Join("testdata", "schema.sql"))
	if err != nil {
		t.Fatalf("read schema fixture: %v", err)
	}
	dbPath := filepath.Join(t.TempDir(), "engram.db")
	setup, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	if _, err := setup.Exec(string(schema)); err != nil {
		setup.Close()
		t.Fatalf("apply schema: %v", err)
	}
	var ids []int64
	for _, r := range rows {
		deletedAt := any(nil)
		if r.deleted {
			deletedAt = "2026-08-01T00:00:00Z"
		}
		res, err := setup.Exec(
			`INSERT INTO observations (session_id, type, title, content, project, deleted_at) VALUES (?, ?, ?, ?, ?, ?)`,
			"sess-1", "discovery", r.title, "body", r.project, deletedAt,
		)
		if err != nil {
			setup.Close()
			t.Fatalf("insert %q: %v", r.title, err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			setup.Close()
			t.Fatalf("last insert id: %v", err)
		}
		ids = append(ids, id)
	}
	setup.Close()

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store, ids
}
