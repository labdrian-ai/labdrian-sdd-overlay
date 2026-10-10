package engram

import (
	"database/sql"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// TestUnionGoldenUsesProductionTokenizer guards the coupling a golden
// harness depends on: Store.Search's own tokenization is observably
// identical to memory.SearchTokens's for the same query, which is only true
// because Search calls memory.SearchTokens directly rather than a private
// copy of the same logic. A caller that reproduces production's query
// semantics -- a golden test harness, or any future caller outside this
// package -- gets that guarantee by calling memory.SearchTokens, and this
// test is what would catch someone forking it. (How a query is split is
// pinned where the rule lives, in internal/memory.)
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

	_, wantDropped := memory.SearchTokens(query)
	if len(got.DroppedTokens) != len(wantDropped) {
		t.Fatalf("Search's DroppedTokens = %v, want SearchTokens's own %v: Search must consume SearchTokens, not a reimplementation", got.DroppedTokens, wantDropped)
	}
	for i := range wantDropped {
		if got.DroppedTokens[i] != wantDropped[i] {
			t.Fatalf("Search's DroppedTokens = %v, want SearchTokens's own %v: Search must consume SearchTokens, not a reimplementation", got.DroppedTokens, wantDropped)
		}
	}
}
