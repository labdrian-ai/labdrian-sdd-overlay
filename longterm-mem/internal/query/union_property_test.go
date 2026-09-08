package query

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
)

// TestMergedSetContainsEachRequestedSourcesOwnRows is R-058's property,
// generalized: however routeRank1 decides which arm owns rank 1 -- the
// only decision Branch A's gate makes -- mergeResults's engram portion
// must still contain every row either requested Engram source produced.
// The gate reorders which source's rows are offered first each round; it
// must never cause interleaveEngramSources to drop one.
//
// This is checked directly against mergeResults (not just
// interleaveEngramSources) across many randomly generated FTS/embed row
// sets and query shapes, both identifier-shaped (routes FTS-first) and
// paraphrase-shaped (routes embed-first), so the property holds regardless
// of which way any single case's gate decision falls.
func TestMergedSetContainsEachRequestedSourceRow(t *testing.T) {
	seed := int64(20260906)
	rnd := rand.New(rand.NewSource(seed))

	queries := []struct {
		text      string
		matchMode string
	}{
		{"internal/query/query.go", engram.MatchAll},                 // identifier-shaped: routes FTS
		{"searchTokens()", engram.MatchAll},                          // identifier-shaped: routes FTS
		{"what conventions does this module apply", engram.MatchAll}, // paraphrase: routes embed
		{"anything", engram.MatchAny},                                // widened search: routes FTS regardless of shape
	}

	for iter := 0; iter < 200; iter++ {
		nFTS := rnd.Intn(6)
		nEmbed := rnd.Intn(6)
		q := queries[rnd.Intn(len(queries))]

		var ftsRows []engram.Row
		wantIDs := make(map[int64]bool)
		nextID := int64(1)
		for i := 0; i < nFTS; i++ {
			id := nextID
			nextID++
			ftsRows = append(ftsRows, engram.Row{ID: id, Title: fmt.Sprintf("fts-%d", id)})
			wantIDs[id] = true
		}
		var embedRows []ResultRow
		for i := 0; i < nEmbed; i++ {
			id := nextID
			nextID++
			embedRows = append(embedRows, ResultRow{EngramID: id, Title: fmt.Sprintf("embed-%d", id)})
			wantIDs[id] = true
		}

		merged := mergeResults(false, true, true, nil, ftsRows, embedRows, NoLinkResolver, q.text, q.matchMode)

		if len(merged) != len(wantIDs) {
			t.Fatalf("iter %d (query %q, nFTS=%d, nEmbed=%d): len(merged) = %d, want %d", iter, q.text, nFTS, nEmbed, len(merged), len(wantIDs))
		}
		for _, row := range merged {
			if !wantIDs[row.EngramID] {
				t.Fatalf("iter %d: unexpected row in merged set: %+v", iter, row)
			}
			delete(wantIDs, row.EngramID)
		}
		if len(wantIDs) != 0 {
			t.Fatalf("iter %d (query %q): rows missing from the merged set: %v", iter, q.text, wantIDs)
		}
	}
}
