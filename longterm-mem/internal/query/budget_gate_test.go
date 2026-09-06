package query

import (
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
)

// Gate 0.4 of the `union-retrieval` plan: the per-row snippet budget was
// chosen from an ESTIMATED Result, and the design requires it be fixed
// against a real one before any code depends on the number.
//
// The worst case is not hypothetical: every diagnostic the query path can
// emit, a Standing object on every row that could carry one, and full-width
// snippets on every row. If a response in that shape exceeds the ceiling,
// capResponse recovers by DROPPING ROWS — and it drops from the tail, in the
// one layer forbidden to re-rank, so it cannot choose which loss hurts least.
// Under a union that is precisely the rows the union exists to deliver.
//
// This test reports the measurement and fails while the worst case does not
// fit, so the number that gets fixed is measured rather than reasoned.
func TestSnippetBudgetGate_WorstCaseFitsWithoutDroppingRows(t *testing.T) {
	const rowsPerSource = DefaultTopN

	standing := &engram.Standing{
		SupersededBy:  []engram.Neighbour{{ID: 3239, Title: "CORRECTION: Engram MCP search DOES surface supersession — my probes were unisolated"}},
		ConflictsWith: []engram.Neighbour{{ID: 3238, Title: "Engram conflict surfacing is save-time by design — not a retrieval defect"}},
		Unjudged:      []engram.Neighbour{{ID: 3237, Title: "Engram's search never joins memory_relations — the judgment loop is open"}},
	}

	result := Result{
		Project:     "labdrian-sdd-overlay",
		Query:       "what conventions apply when editing the register writer",
		VaultStatus: VaultStatusOK,
	}
	for i := 0; i < rowsPerSource; i++ {
		result.Results = append(result.Results, ResultRow{
			Sources: []string{SourceVault}, Rank: i + 1,
			PageAddress: "c-000123456789", PagePath: "/home/labdrian/labdrian-brain/wiki/memory/c-000123456789.md",
			Title:   "A promoted page with a title of the length these actually reach in practice",
			Snippet: "…" + strings.Repeat("x", engram.SnippetBudget) + "…",
		})
		result.Results = append(result.Results, ResultRow{
			Sources: []string{SourceEngramFTS}, Rank: i + 1, EngramID: 3254,
			Title:    "Union retrieval validated as a fourth arm: matches the better arm in every class",
			Snippet:  "…" + strings.Repeat("x", engram.SnippetBudget) + "…",
			Standing: standing,
		})
	}
	for _, code := range []string{
		DiagnosticVaultSubprocessFailed, DiagnosticEngramDegradedSnapshot,
		DiagnosticRelationsUnreadable, DiagnosticResponseCapped,
	} {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{
			Code:   code,
			Detail: "a diagnostic detail of the length these reach when they name a cause and a remedy, which is how this codebase writes them",
		})
	}

	before := len(result.Results)
	measured := responseBytes(result)
	capped := result
	capResponse(&capped)
	dropped := before - len(capped.Results)

	t.Logf("worst case: %d rows, snippet budget %d, encoded %d bytes against a %d ceiling",
		before, engram.SnippetBudget, measured, ResponseByteCeiling)
	t.Logf("rows dropped by capResponse: %d", dropped)
	if dropped > 0 {
		t.Logf("per-row budget that would fit: about %d characters",
			(ResponseByteCeiling-(measured-before*engram.SnippetBudget))/before)
	}

	if dropped > 0 {
		t.Fatalf("the worst-case response drops %d of %d rows to fit the ceiling; "+
			"the budget must be allocated before snippets are rendered, not recovered by "+
			"deleting the rows the union exists to deliver", dropped, before)
	}
}
