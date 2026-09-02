package engram

import (
	"database/sql"
	"testing"
)

// insertSearchRow inserts one fixture row with caller-controlled content
// (and an optional deleted_at), letting Search tests distinguish rows by
// relevance and prove the soft-delete exclusion for real -- unlike
// store_test.go's insertObservation, which fixes content to "fixture
// content" for every row.
func insertSearchRow(t *testing.T, dbPath, title, content, project string, deletedAt sql.NullString) {
	t.Helper()

	setup, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open fixture setup connection: %v", err)
	}
	defer setup.Close()

	_, err = setup.Exec(
		`INSERT INTO observations (session_id, type, title, content, project, deleted_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"sess-1", "discovery", title, content, project, deletedAt,
	)
	if err != nil {
		t.Fatalf("insert fixture observation %q: %v", title, err)
	}
}

func TestSearch_ScopesProjectAndExcludesSoftDeleted(t *testing.T) {
	dir := t.TempDir()
	dbPath := newFixtureDB(t, dir)

	insertSearchRow(t, dbPath, "in project", "alpha keyword match", "labdrian-sdd-overlay", sql.NullString{})
	insertSearchRow(t, dbPath, "other project", "alpha keyword match", "some-other-project", sql.NullString{})
	insertSearchRow(t, dbPath, "soft deleted", "alpha keyword match", "labdrian-sdd-overlay", sql.NullString{String: "2026-08-01T00:00:00Z", Valid: true})

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open(%q): %v", dbPath, err)
	}
	defer store.Close()

	got, err := store.Search("labdrian-sdd-overlay", "alpha", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1; got %+v", len(got), got)
	}
	if got[0].Title != "in project" {
		t.Fatalf("got[0].Title = %q, want %q", got[0].Title, "in project")
	}
	if got[0].Project != "labdrian-sdd-overlay" {
		t.Fatalf("got[0].Project = %q, want %q", got[0].Project, "labdrian-sdd-overlay")
	}
}

// TestSearch_TokenStartingWithMinusIsTreatedAsLiteralText proves query
// tokenization double-quotes each token before it reaches FTS5: an
// unescaped "-secret" would be parsed as the FTS5 NOT operator (a syntax
// error with nothing to negate, or worse, silently excluding matches),
// instead of matching the literal text "secret".
func TestSearch_TokenStartingWithMinusIsTreatedAsLiteralText(t *testing.T) {
	dir := t.TempDir()
	dbPath := newFixtureDB(t, dir)

	insertSearchRow(t, dbPath, "confidential", "internal secret notes", "labdrian-sdd-overlay", sql.NullString{})

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open(%q): %v", dbPath, err)
	}
	defer store.Close()

	got, err := store.Search("labdrian-sdd-overlay", "-secret", 10)
	if err != nil {
		t.Fatalf("Search(-secret) returned an unexpected error (a leading '-' token must be quoted as literal text, not sent as an FTS5 NOT operator): %v", err)
	}
	if len(got) != 1 || got[0].Title != "confidential" {
		t.Fatalf("got = %+v, want the one row containing the literal text \"secret\"", got)
	}
}
