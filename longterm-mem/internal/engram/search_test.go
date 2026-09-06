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
// instead of being matched as literal text.
//
// The fixture row carries the literal "-secret", hyphen included, because
// under the live trigram tokenizer that is what the quoted phrase means:
// the substring "-secret", not the word "secret". Against the FTS5 default
// tokenizer the hyphen would be a separator and a row containing only
// "secret" would match, which is why this test previously passed on a row
// that a real Engram database would never have returned.
func TestSearch_TokenStartingWithMinusIsTreatedAsLiteralText(t *testing.T) {
	dir := t.TempDir()
	dbPath := newFixtureDB(t, dir)

	insertSearchRow(t, dbPath, "confidential", "internal -secret notes", "labdrian-sdd-overlay", sql.NullString{})

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

// TestSearch_FixtureIndexMatchesLiveTokenizer pins the fixture's FTS5
// tokenizer to the live one. The live observations_fts is declared
// `tokenize='trigram'` (schema dumped from ~/.engram/engram.db), which
// matches on character trigrams, so a mid-word fragment finds the word --
// something the FTS5 default (unicode61, whole-token) cannot do.
//
// The difference is not cosmetic. Trigram tokenization decides how many
// tokens a document has, and therefore what any token-windowed extract
// (FTS5 snippet()) can return, and it decides that a query token shorter
// than three characters matches nothing at all. A fixture on the default
// tokenizer would let a snippet or query-semantics test pass here while
// the same code returned something else entirely against a real Engram
// database -- a test that proves the opposite of what it claims.
func TestSearch_FixtureIndexMatchesLiveTokenizer(t *testing.T) {
	dir := t.TempDir()
	dbPath := newFixtureDB(t, dir)

	insertSearchRow(t, dbPath, "conventions", "the conventions that apply here", "labdrian-sdd-overlay", sql.NullString{})

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open(%q): %v", dbPath, err)
	}
	defer store.Close()

	got, err := store.Search("labdrian-sdd-overlay", "onvention", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("searching the mid-word fragment %q returned %d rows, want 1: the fixture index is not trigram-tokenized like the live one", "onvention", len(got))
	}
}
