package engram

import (
	"database/sql"
	"testing"
)

// TestSearchTokensSplitsOnFieldsNotPunctuation pins the regex/Fields
// divergence point the design calls out: a tokenizer that split on
// punctuation would tear apart exactly the identifier shapes (paths,
// dotted names) the rank-1 routing gate (R-059) reads to decide which
// source to trust for rank 1. SearchTokens must split on whitespace only.
func TestSearchTokensSplitsOnFieldsNotPunctuation(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  []string
	}{
		{
			name:  "a path with a colon and line number stays one token",
			query: "search.go:181",
			want:  []string{"search.go:181"},
		},
		{
			name:  "whitespace splits tokens, punctuation inside one does not",
			query: "a/b c",
			want:  []string{"a/b", "c"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := SearchTokens(tc.query)
			if len(got) != len(tc.want) {
				t.Fatalf("SearchTokens(%q) = %v, want %v", tc.query, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("SearchTokens(%q) = %v, want %v", tc.query, got, tc.want)
				}
			}
		})
	}
}

// TestUnionGoldenUsesProductionTokenizer guards the coupling a golden
// harness depends on (Phase 4 builds the harness itself; this pins the
// coupling now, before anything can drift): Store.Search's own
// tokenization is observably identical to SearchTokens's for the same
// query, which is only true because Search calls SearchTokens directly
// rather than a private copy of the same logic. A caller that reproduces
// production's query semantics -- a golden test harness, or any future
// caller outside this package -- gets that guarantee by calling
// SearchTokens, and this test is what would catch someone forking it.
func TestUnionGoldenUsesProductionTokenizer(t *testing.T) {
	dir := t.TempDir()
	dbPath := newFixtureDB(t, dir)
	insertSearchRow(t, dbPath, "writer", "the register writer sorts its keys", "labdrian-sdd-overlay", sql.NullString{})

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open(%q): %v", dbPath, err)
	}
	defer store.Close()

	query := "what is the register writer"
	got, err := store.Search("labdrian-sdd-overlay", query, 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	_, wantDropped := SearchTokens(query)
	if len(got.DroppedTokens) != len(wantDropped) {
		t.Fatalf("Search's DroppedTokens = %v, want SearchTokens's own %v: Search must consume SearchTokens, not a reimplementation", got.DroppedTokens, wantDropped)
	}
	for i := range wantDropped {
		if got.DroppedTokens[i] != wantDropped[i] {
			t.Fatalf("Search's DroppedTokens = %v, want SearchTokens's own %v: Search must consume SearchTokens, not a reimplementation", got.DroppedTokens, wantDropped)
		}
	}
}
