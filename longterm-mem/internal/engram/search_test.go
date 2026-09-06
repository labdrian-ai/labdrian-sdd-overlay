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

	if len(got.Rows) != 1 {
		t.Fatalf("len(got.Rows) = %d, want 1; got %+v", len(got.Rows), got.Rows)
	}
	if got.Rows[0].Title != "in project" {
		t.Fatalf("got.Rows[0].Title = %q, want %q", got.Rows[0].Title, "in project")
	}
	if got.Rows[0].Project != "labdrian-sdd-overlay" {
		t.Fatalf("got.Rows[0].Project = %q, want %q", got.Rows[0].Project, "labdrian-sdd-overlay")
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
	if len(got.Rows) != 1 || got.Rows[0].Title != "confidential" {
		t.Fatalf("got = %+v, want the one row containing the literal text \"-secret\"", got.Rows)
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
	if len(got.Rows) != 1 {
		t.Fatalf("searching the mid-word fragment %q returned %d rows, want 1: the fixture index is not trigram-tokenized like the live one", "onvention", len(got.Rows))
	}
}

// TestSearch_NaturalQuestionIsNotAndedIntoSilence is the regression this
// change exists for. Every whitespace token was AND-joined, so a question
// phrased the way a person phrases one required all eight of its words --
// stopwords included -- to appear in a single observation. Measured on the
// live corpus, "what conventions apply when editing the register writer"
// returned 0 of 577 rows; the same tokens OR-joined returned 556.
//
// A silently empty result is the worst failure mode available here: it
// costs nothing, reports nothing, and reads as "there is no such memory".
func TestSearch_NaturalQuestionIsNotAndedIntoSilence(t *testing.T) {
	dir := t.TempDir()
	dbPath := newFixtureDB(t, dir)

	insertSearchRow(t, dbPath, "register writer conventions",
		"the register writer sorts its keys before writing", "labdrian-sdd-overlay", sql.NullString{})

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open(%q): %v", dbPath, err)
	}
	defer store.Close()

	got, err := store.Search("labdrian-sdd-overlay", "what conventions apply when editing the register writer", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got.Rows) != 1 {
		t.Fatalf("len(Rows) = %d, want 1: a natural-language question must not be AND-joined into an empty result", len(got.Rows))
	}
	if got.MatchMode != MatchAny {
		t.Fatalf("MatchMode = %q, want %q: a widened query must say so, or the caller cannot tell a precise hit from a broad one", got.MatchMode, MatchAny)
	}
}

// TestSearch_KeepsEveryTokenRequiredWhenThatFindsSomething guards the
// other half of the trade. Widening is a fallback, not the default: where
// requiring every token already finds rows, the broader OR match -- which
// on the live corpus turned 13 rows into 114 -- must never be reached.
func TestSearch_KeepsEveryTokenRequiredWhenThatFindsSomething(t *testing.T) {
	dir := t.TempDir()
	dbPath := newFixtureDB(t, dir)

	insertSearchRow(t, dbPath, "both", "canonical identity resolution", "labdrian-sdd-overlay", sql.NullString{})
	insertSearchRow(t, dbPath, "one", "identity only, nothing else here", "labdrian-sdd-overlay", sql.NullString{})

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open(%q): %v", dbPath, err)
	}
	defer store.Close()

	got, err := store.Search("labdrian-sdd-overlay", "canonical identity", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got.Rows) != 1 || got.Rows[0].Title != "both" {
		t.Fatalf("Rows = %+v, want only the row matching every token", got.Rows)
	}
	if got.MatchMode != MatchAll {
		t.Fatalf("MatchMode = %q, want %q", got.MatchMode, MatchAll)
	}
}

// TestSearch_StopwordOnlyQueryStillSearchesItsWords keeps stopword
// stripping from turning a deliberate query into no query at all. If every
// token is a stopword there is nothing else the caller can have meant, so
// the words are searched as written rather than discarded into silence.
func TestSearch_StopwordOnlyQueryStillSearchesItsWords(t *testing.T) {
	dir := t.TempDir()
	dbPath := newFixtureDB(t, dir)

	insertSearchRow(t, dbPath, "phrase", "the way that this works", "labdrian-sdd-overlay", sql.NullString{})

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open(%q): %v", dbPath, err)
	}
	defer store.Close()

	got, err := store.Search("labdrian-sdd-overlay", "the way that", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got.Rows) != 1 {
		t.Fatalf("len(Rows) = %d, want 1: a query made entirely of stopwords must still search them", len(got.Rows))
	}
}

// TestSearch_ReportsTheStopwordsItDropped keeps the stripping visible. A
// caller that cannot see which of its words were ignored cannot tell a
// corpus with no answer from a query that was quietly rewritten.
func TestSearch_ReportsTheStopwordsItDropped(t *testing.T) {
	dir := t.TempDir()
	dbPath := newFixtureDB(t, dir)

	insertSearchRow(t, dbPath, "writer", "the register writer", "labdrian-sdd-overlay", sql.NullString{})

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open(%q): %v", dbPath, err)
	}
	defer store.Close()

	got, err := store.Search("labdrian-sdd-overlay", "what is the register writer", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	want := []string{"what", "is", "the"}
	if len(got.DroppedTokens) != len(want) {
		t.Fatalf("DroppedTokens = %v, want %v", got.DroppedTokens, want)
	}
	for i, w := range want {
		if got.DroppedTokens[i] != w {
			t.Fatalf("DroppedTokens = %v, want %v", got.DroppedTokens, want)
		}
	}
}
