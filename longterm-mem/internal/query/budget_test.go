package query

import (
	"fmt"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
)

// TestSnippetShareIsPerRowNotPerSource (R-062): the byte budget is divided
// across ROWS, not across sources. Eight rows from one source and two from
// another must all receive the exact same per-row share -- a per-source
// split would have given the two-row source a much larger share than the
// eight-row source's rows, which is precisely the comparison-between-
// sources design deliberately dissolves rather than answers.
func TestSnippetShareIsPerRowNotPerSource(t *testing.T) {
	longBody := strings.Repeat("z", 5000)
	result := Result{Project: "proj-a", Query: "zephyr"}
	for i := 0; i < 8; i++ {
		result.Results = append(result.Results, ResultRow{
			Sources: []string{SourceEngramEmbed}, Rank: len(result.Results) + 1,
			EngramID: int64(i + 1), Content: longBody, Snippet: longBody[:480],
		})
	}
	for i := 0; i < 2; i++ {
		result.Results = append(result.Results, ResultRow{
			Sources: []string{SourceEngramFTS}, Rank: len(result.Results) + 1,
			EngramID: int64(100 + i), Content: longBody, Snippet: longBody[:480],
		})
	}

	allocateSnippetBudget(&result)

	want := len(result.Results[0].Snippet)
	if want == 0 {
		t.Fatalf("row 0 rendered an empty snippet")
	}
	for i, row := range result.Results {
		if len(row.Snippet) != want {
			t.Fatalf("Results[%d] snippet is %d bytes, want %d: every row must receive the same per-row share regardless of how many rows its own source contributed", i, len(row.Snippet), want)
		}
	}
}

// TestUnusedSnippetShareIsRedistributedExactlyOnce (R-062): a short body
// leaves part of its row's share unused, and that leftover is reclaimed
// once for whatever is still truncated -- so a long row ends up with MORE
// than the naive equal-division share once the short rows' unused bytes
// are accounted for, and the response still fits under the ceiling.
func TestUnusedSnippetShareIsRedistributedExactlyOnce(t *testing.T) {
	shortBody := "a short body well under any share"
	longBody := strings.Repeat("z", 5000)
	result := Result{Project: "proj-a", Query: "zephyr"}
	// Enough short rows that the naive equal share (available/n) is well
	// under engram.SnippetBudget's own 480-byte ceiling -- otherwise that
	// ceiling, not the division, would be the only thing capping a long
	// row's snippet, and redistribution would have nothing to prove.
	for i := 0; i < 20; i++ {
		result.Results = append(result.Results, ResultRow{
			Sources: []string{SourceEngramFTS}, Rank: len(result.Results) + 1,
			EngramID: int64(i + 1), Content: shortBody, Snippet: shortBody,
		})
	}
	for i := 0; i < 2; i++ {
		result.Results = append(result.Results, ResultRow{
			Sources: []string{SourceEngramFTS}, Rank: len(result.Results) + 1,
			EngramID: int64(100 + i), Content: longBody, Snippet: longBody[:480],
		})
	}

	// naiveShare is the same clamped overhead/n computation
	// allocateSnippetBudget itself performs, measured here BEFORE the
	// call so it reflects the single-pass equal division the
	// redistribution is supposed to beat.
	blanked := make([]ResultRow, len(result.Results))
	copy(blanked, result.Results)
	for i := range blanked {
		blanked[i].Snippet, blanked[i].SnippetTruncated = "", false
	}
	overhead := responseBytes(Result{Project: result.Project, Query: result.Query, Results: blanked})
	naiveShare := (ResponseByteCeiling - overhead) / len(result.Results)
	if naiveShare >= engram.SnippetBudget {
		t.Fatalf("fixture's naive share (%d) is not below engram.SnippetBudget (%d); the test proves nothing", naiveShare, engram.SnippetBudget)
	}

	allocateSnippetBudget(&result)

	for i := 0; i < 20; i++ {
		if result.Results[i].Snippet != shortBody || result.Results[i].SnippetTruncated {
			t.Fatalf("Results[%d]: a short body must be returned whole, unmarked: %+v", i, result.Results[i])
		}
	}
	long1, long2 := len(result.Results[20].Snippet), len(result.Results[21].Snippet)
	if long1 != long2 {
		t.Fatalf("both long rows must receive the same redistributed share: %d vs %d", long1, long2)
	}
	if long1 <= naiveShare {
		t.Fatalf("a long row's rendered snippet is %d bytes, want more than the naive equal share %d bytes: the short rows' unused share must be reclaimed once by whatever is still truncated", long1, naiveShare)
	}
	if got := responseBytes(result); got > ResponseByteCeiling {
		t.Fatalf("redistribution pushed the response to %d bytes, over the %d ceiling", got, ResponseByteCeiling)
	}
}

// TestCapResponseDropsFromTheLargestSourceNotTheTail (R-063): a response
// still over the ceiling after allocation must drop rows from whichever
// source holds the most slots, not from the tail of the merged list
// regardless of source. This test places the LARGER source at the FRONT
// and the smaller source at the TAIL specifically so a naive tail-drop
// (today's behaviour) and the required largest-source drop disagree: a
// tail-drop would remove the small source's rows first, purely because of
// where they sit, while the rule this change requires must instead remove
// the large source's rows and leave the tail alone.
func TestCapResponseDropsFromTheLargestSourceNotTheTail(t *testing.T) {
	result := Result{Project: "proj-a", Query: "zephyr"}
	bigBody := strings.Repeat("z", 3000)
	for i := 0; i < 35; i++ {
		result.Results = append(result.Results, ResultRow{
			Sources: []string{SourceEngramFTS}, Rank: len(result.Results) + 1,
			EngramID: int64(i + 1), Title: fmt.Sprintf("engram row %d", i), Content: bigBody, Snippet: bigBody[:480],
		})
	}
	for i := 0; i < 5; i++ {
		result.Results = append(result.Results, ResultRow{
			Sources: []string{SourceVault}, Rank: len(result.Results) + 1,
			PageAddress: fmt.Sprintf("c-%05d", i), Snippet: "a short vault snippet",
		})
	}

	before := responseBytes(result)
	if before <= ResponseByteCeiling {
		t.Fatalf("fixture does not exceed the ceiling before capping (%d bytes); the test proves nothing", before)
	}

	capResponse(&result)

	if got := responseBytes(result); got > ResponseByteCeiling {
		t.Fatalf("response is still %d bytes after capping, over the %d ceiling", got, ResponseByteCeiling)
	}

	vaultSurvivors := 0
	ftsSurvivors := 0
	for _, row := range result.Results {
		switch {
		case hasSource(row, SourceVault):
			vaultSurvivors++
		case hasSource(row, SourceEngramFTS):
			ftsSurvivors++
		}
	}
	if vaultSurvivors != 5 {
		t.Fatalf("vaultSurvivors = %d, want 5: the smaller source at the tail must not be the one dropped", vaultSurvivors)
	}
	if ftsSurvivors >= 35 {
		t.Fatalf("ftsSurvivors = %d, want fewer than 35: the larger source must be the one the cap drops from", ftsSurvivors)
	}
}
