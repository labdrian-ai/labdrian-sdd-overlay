package engram

import (
	"database/sql"
	"testing"
)

// insertRelation inserts one fixture memory_relations row (live schema
// #3129: source_id/target_id key on sync_id, not the integer id).
func insertRelation(t *testing.T, dbPath, syncID, sourceSyncID, targetSyncID, relation, judgmentStatus string, supersededAt sql.NullString) {
	t.Helper()

	setup, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open fixture setup connection: %v", err)
	}
	defer setup.Close()

	_, err = setup.Exec(
		`INSERT INTO memory_relations (sync_id, source_id, target_id, relation, judgment_status, superseded_at) VALUES (?, ?, ?, ?, ?, ?)`,
		syncID, sourceSyncID, targetSyncID, relation, judgmentStatus, supersededAt,
	)
	if err != nil {
		t.Fatalf("insert fixture relation %q: %v", syncID, err)
	}
}

// TestRelatedEdges_AcceptedOnly proves RelatedEdges' D7 filter: only
// judgment_status='judged', superseded_at IS NULL, relation in the
// accepted set, on either side of the edge, come back -- a pending
// judgment, a superseded relation, an unaccepted relation kind, and an
// edge that does not touch the subject observation are all excluded.
func TestRelatedEdges_AcceptedOnly(t *testing.T) {
	dir := t.TempDir()
	dbPath := newFixtureDB(t, dir)

	subjectID := insertObservationFull(t, dbPath, "subject", "labdrian-sdd-overlay", "decision", 1, false, "sync-subject")
	insertObservationFull(t, dbPath, "other", "labdrian-sdd-overlay", "decision", 1, false, "sync-other")
	insertObservationFull(t, dbPath, "third", "labdrian-sdd-overlay", "decision", 1, false, "sync-third")

	insertRelation(t, dbPath, "rel-accepted-1", "sync-subject", "sync-other", "related", "judged", sql.NullString{})
	insertRelation(t, dbPath, "rel-accepted-2", "sync-other", "sync-subject", "supersedes", "judged", sql.NullString{})
	insertRelation(t, dbPath, "rel-pending", "sync-subject", "sync-other", "related", "pending", sql.NullString{})
	insertRelation(t, dbPath, "rel-superseded", "sync-subject", "sync-other", "compatible", "judged", sql.NullString{String: "2026-08-01T00:00:00Z", Valid: true})
	insertRelation(t, dbPath, "rel-unaccepted-kind", "sync-subject", "sync-other", "not_conflict", "judged", sql.NullString{})
	insertRelation(t, dbPath, "rel-unrelated", "sync-other", "sync-third", "related", "judged", sql.NullString{})

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open(%q): %v", dbPath, err)
	}
	defer store.Close()

	got, err := store.RelatedEdges(subjectID)
	if err != nil {
		t.Fatalf("RelatedEdges: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2 (accepted edges only); got %+v", len(got), got)
	}
	relations := map[string]bool{got[0].Relation: true, got[1].Relation: true}
	if !relations["related"] || !relations["supersedes"] {
		t.Fatalf("got relations = %+v, want both \"related\" and \"supersedes\"", got)
	}
}

// TestRelatedEdges_SkipsNullEndpointRows proves a half-dangling relation
// row (an endpoint left NULL by an orphaned or partially written record —
// both columns are nullable in the live schema) is skipped rather than
// aborting the whole edge set with a NULL-scan error.
func TestRelatedEdges_SkipsNullEndpointRows(t *testing.T) {
	dir := t.TempDir()
	dbPath := newFixtureDB(t, dir)

	subjectID := insertObservationFull(t, dbPath, "subject", "labdrian-sdd-overlay", "decision", 1, false, "sync-subject")
	insertObservationFull(t, dbPath, "other", "labdrian-sdd-overlay", "decision", 1, false, "sync-other")

	insertRelation(t, dbPath, "rel-ok", "sync-subject", "sync-other", "related", "judged", sql.NullString{})

	setup, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open fixture setup connection: %v", err)
	}
	defer setup.Close()
	if _, err := setup.Exec(
		`INSERT INTO memory_relations (sync_id, source_id, target_id, relation, judgment_status) VALUES (?, ?, NULL, ?, ?)`,
		"rel-dangling", "sync-subject", "related", "judged",
	); err != nil {
		t.Fatalf("insert dangling fixture relation: %v", err)
	}

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open(%q): %v", dbPath, err)
	}
	defer store.Close()

	got, err := store.RelatedEdges(subjectID)
	if err != nil {
		t.Fatalf("RelatedEdges with a NULL-endpoint row: %v", err)
	}
	if len(got) != 1 || got[0].Relation != "related" {
		t.Fatalf("got %+v, want exactly the one complete edge", got)
	}
}
