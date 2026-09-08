package main

import (
	"database/sql"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
)

// TestObservationRowsForIndex_MatchesLiveEngramRows is the RED/GREEN proof
// for buildIndexForQuery's own row source (issue #286's top-up wiring): it
// must map exactly the live rows of one project into vecindex.Row shape,
// carrying id/title/content through unchanged, and leave out a
// soft-deleted row -- the same R-020 scoping cmd_index_embeddings.go
// already relies on via store.ListObservations.
func TestObservationRowsForIndex_MatchesLiveEngramRows(t *testing.T) {
	const project = "rundeps-topup-project"
	dbPath := newTestEngramDBWithObservation(t, project, "Live Title", "Live content.")

	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open fixture setup connection: %v", err)
	}
	res, err := conn.Exec(`INSERT INTO observations (session_id, sync_id, type, title, content, project, revision_count, pinned, created_at)
		 VALUES ('sess-1', 'sync-2', 'decision', ?, ?, ?, 1, 0, '2026-08-01 00:00:00')`,
		"Deleted Title", "Deleted content.", project)
	if err != nil {
		conn.Close()
		t.Fatalf("insert soft-deleted fixture observation: %v", err)
	}
	deletedID, err := res.LastInsertId()
	if err != nil {
		conn.Close()
		t.Fatalf("last insert id: %v", err)
	}
	if _, err := conn.Exec(`UPDATE observations SET deleted_at = ? WHERE id = ?`, "2026-08-02 00:00:00", deletedID); err != nil {
		conn.Close()
		t.Fatalf("soft-delete fixture observation: %v", err)
	}
	conn.Close()

	store, err := engram.Open(dbPath)
	if err != nil {
		t.Fatalf("engram.Open(%q): %v", dbPath, err)
	}
	t.Cleanup(func() { _ = store.Close() })

	rows, err := observationRowsForIndex(store, project)
	if err != nil {
		t.Fatalf("observationRowsForIndex: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %+v, want exactly the one live row (soft-deleted row must be excluded)", rows)
	}
	if rows[0].Title != "Live Title" || rows[0].Content != "Live content." {
		t.Fatalf("rows[0] = %+v, want the live fixture row's title/content carried through unchanged", rows[0])
	}
}
