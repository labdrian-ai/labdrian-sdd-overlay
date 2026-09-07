package query

import (
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
)

// TestGateRoutesIdentifierShapesToLexicalArm (R-059): a token carrying an
// identifier shape -- an interior CamelCase boundary, a path/underscore
// separator, a call-shaped parenthesis, or a dotted word.word -- routes
// rank 1 to the FTS source, the arm that finds an exact lexical match, not
// the embedding source, which finds only an approximate one.
//
// matchMode is engram.MatchAny, isolating the shape clause from the
// match-mode clause (the gate is "shape OR match-mode==MatchAll"; asserted
// on MatchAll instead, every case here would pass regardless of whether
// the shape check does anything at all).
func TestGateRoutesIdentifierShapesToLexicalArm(t *testing.T) {
	cases := []struct {
		name  string
		query []string
	}{
		{"path with a dotted extension", []string{"search.go"}},
		{"path with a slash", []string{"internal/query"}},
		{"underscore-separated identifier", []string{"my_function"}},
		{"call-shaped parenthesis", []string{"routeRank1()"}},
		{"interior CamelCase boundary", []string{"MyType"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := routeRank1(tc.query, engram.MatchAny); got != SourceEngramFTS {
				t.Fatalf("routeRank1(%v) = %q, want %q", tc.query, got, SourceEngramFTS)
			}
		})
	}
}

// TestGateRoutesToFTSOnMatchAllEvenWithoutIdentifierShape (R-059, the
// decision record's own "shape OR match-mode" rule): FTS matching every
// query token precisely -- MatchAll, no widening needed -- routes rank 1
// to FTS on its own, independent of the shape clause. This is the exact
// case the decision record's own realistic-query measurement needed
// (openspec/decisions/union-retrieval.md §4.3: "R-021 exec allowlist"
// misrouted under shape-only alone) and the exact clause an early-return
// implementation would have pre-empted rather than ORed.
func TestGateRoutesToFTSOnMatchAllEvenWithoutIdentifierShape(t *testing.T) {
	query := []string{"exec", "allowlist"} // no identifier-shaped token
	if got := routeRank1(query, engram.MatchAll); got != SourceEngramFTS {
		t.Fatalf("routeRank1(%v, MatchAll) = %q, want %q (match-mode alone must route FTS)", query, got, SourceEngramFTS)
	}
}

// TestGateDoesNotFireOnHyphenatedEnglish guards the shape the gate must
// NOT treat as an identifier: an ordinary hyphenated English compound. A
// gate that fired on hyphens would route most natural-language paraphrase
// queries to the lexical arm by accident, defeating the whole point of
// having an embedding arm to route to.
//
// matchMode is engram.MatchAny here, not MatchAll: MatchAll is itself an
// independent (correct) routing signal for the FTS source (the gate is
// "shape OR match-mode==MatchAll"), so asserting this on MatchAll would
// pass regardless of whether the shape clause fires -- it would not
// isolate what this test exists to check. A realistic paraphrase query
// almost always widens to MatchAny in practice (its own words rarely all
// co-occur), which is also the shape this test needs to isolate the shape
// clause from the match-mode clause.
func TestGateDoesNotFireOnHyphenatedEnglish(t *testing.T) {
	query := []string{"well-known", "state-of-the-art", "results"}
	if got := routeRank1(query, engram.MatchAny); got != SourceEngramEmbed {
		t.Fatalf("routeRank1(%v) = %q, want %q (hyphenated English must not trip the identifier gate)", query, got, SourceEngramEmbed)
	}
}

// TestAnIncorrectRank1RoutingDoesNotShrinkTheGuarantee (R-058): whatever
// routeRank1 decides, the union's merged set still contains both
// requested sources' own top rows -- the gate only ever affects which row
// happens to be first, never the set. This is provable now, even before
// PR-4 wires the gate live, because interleaveEngramSources -- the
// function that actually decides set membership -- never consults
// routeRank1 at all: nothing here can shrink the guarantee, correctly, no
// matter what the gate says.
func TestAnIncorrectRank1RoutingDoesNotShrinkTheGuarantee(t *testing.T) {
	fts := engramSourceRows{name: SourceEngramFTS, rows: []ResultRow{
		{EngramID: 1, Title: "fts top"}, {EngramID: 2, Title: "fts second"},
	}}
	embed := engramSourceRows{name: SourceEngramEmbed, rows: []ResultRow{
		{EngramID: 3, Title: "embed top"}, {EngramID: 4, Title: "embed second"},
	}}

	for _, tc := range []struct {
		name      string
		tokens    []string
		matchMode string
	}{
		{"routes to fts", []string{"search.go"}, engram.MatchAll},
		{"routes to embed", []string{"what", "conventions", "apply"}, engram.MatchAny},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The routing decision is computed to prove it varies across
			// the two cases; the merge below never reads it.
			routed := routeRank1(tc.tokens, tc.matchMode)
			_ = routed

			merged := interleaveEngramSources([]engramSourceRows{fts, embed})
			want := map[int64]bool{1: true, 2: true, 3: true, 4: true}
			if len(merged) != len(want) {
				t.Fatalf("len(merged) = %d, want %d: both sources' top rows must all survive", len(merged), len(want))
			}
			for _, row := range merged {
				if !want[row.EngramID] {
					t.Fatalf("unexpected row %+v", row)
				}
				delete(want, row.EngramID)
			}
			if len(want) != 0 {
				t.Fatalf("rows missing from the merged set: %v", want)
			}
		})
	}
}
