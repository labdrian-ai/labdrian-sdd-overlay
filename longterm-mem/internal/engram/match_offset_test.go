package engram

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// The offset of a match comes from the highlight markers: highlight() brackets each match with them,
// and the index of the first opener in the highlighted body is the match's byte index in the body. A
// marker that is empty or absent from the SQL (an invisible character written as itself and lost, say)
// leaves that index at 0, and every snippet silently becomes the head of its body. A match near the
// head cannot tell the two apart, so this one sits late in a long body.
func TestSearch_AMatchLateInABodyHasItsOffsetAndASnippetCentredOnIt(t *testing.T) {
	dbPath := newFixtureDB(t, t.TempDir())
	const lead = 2000
	content := strings.Repeat("a", lead) + " needle " + strings.Repeat("b", 800)
	insertSearchRow(t, dbPath, "late match", content, "p", sql.NullString{})
	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()

	got, err := store.Search("p", "needle", 5)
	if err != nil || len(got.Rows) != 1 {
		t.Fatalf("Search = %+v, %v, want one row", got, err)
	}
	row := got.Rows[0]

	wantOffset := strings.Index(content, "needle")
	if row.MatchOffset != wantOffset {
		t.Fatalf("MatchOffset = %d, want %d (the byte index of the match in the body)", row.MatchOffset, wantOffset)
	}
	wantSnippet, wantTruncated := memory.SnippetAt(content, wantOffset, memory.SnippetBudget)
	if row.Snippet != wantSnippet || row.SnippetTruncated != wantTruncated {
		t.Fatalf("Snippet = %q (truncated %v), want the window centred on the match %q (truncated %v)", row.Snippet, row.SnippetTruncated, wantSnippet, wantTruncated)
	}
	if !strings.Contains(row.Snippet, "needle") || strings.HasPrefix(row.Snippet, strings.Repeat("a", 20)) {
		t.Fatalf("Snippet = %q, want it to hold the match and not be the head of the body", row.Snippet)
	}
}
