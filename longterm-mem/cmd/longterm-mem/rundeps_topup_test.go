package main

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vecindex"
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

// TestBuildIndexForQuery_InvalidatesCacheOnlyOnSuccess is the RED/GREEN
// proof for the session cache's invalidation wiring (issue #286): a
// successful build must invalidate the load cache for the exact directory
// it just wrote, so the very query that triggered the top-up sees the
// fresh index rather than a stale cached one; a failed build must not
// invalidate anything, since nothing changed on disk for a cache to need
// to forget.
//
// The "no rows" project deliberately never calls the embedding backend at
// all (vecindex.Build's embed loop never runs when there is nothing to
// embed), so this proves the invalidation wiring without any network
// dependency. The failure case is forced offline too, by corrupting the
// index buildIndexForQuery just wrote so its own internal Load fails.
func TestBuildIndexForQuery_InvalidatesCacheOnlyOnSuccess(t *testing.T) {
	const project = "rundeps-invalidate-project"
	// A different project's row keeps the fixture's schema realistic
	// without giving `project` itself any live rows to embed.
	dbPath := newTestEngramDBWithObservation(t, "other-project", "Other Title", "Other content.")
	store, err := engram.Open(dbPath)
	if err != nil {
		t.Fatalf("engram.Open(%q): %v", dbPath, err)
	}
	t.Cleanup(func() { _ = store.Close() })

	stateDir := t.TempDir()
	t.Setenv("LONGTERM_MEM_STATE_DIR", stateDir)

	var invalidated []string
	buildFn := buildIndexForQuery(store, func(dir string) { invalidated = append(invalidated, dir) })

	if err := buildFn(context.Background(), project, "test-model", 3, 2000); err != nil {
		t.Fatalf("buildIndexForQuery with nothing to embed: %v", err)
	}
	wantDir := vecindex.Dir(stateDir, project)
	if len(invalidated) != 1 || invalidated[0] != wantDir {
		t.Fatalf("invalidated = %v, want exactly [%s] after a successful build", invalidated, wantDir)
	}

	// Force the next build to fail without any network call: a manifest
	// that is not valid JSON makes vecindex.Build's own internal Load
	// return a wrapped ErrCorrupted before anything is embedded.
	if err := os.WriteFile(filepath.Join(wantDir, "manifest.json"), []byte("not json"), 0o600); err != nil {
		t.Fatalf("corrupt manifest: %v", err)
	}
	invalidated = nil
	if err := buildFn(context.Background(), project, "test-model", 3, 2000); err == nil {
		t.Fatalf("buildIndexForQuery over a corrupted index succeeded, want an error")
	}
	if len(invalidated) != 0 {
		t.Fatalf("invalidated = %v, want none: the build failed, so nothing on disk changed", invalidated)
	}
}
