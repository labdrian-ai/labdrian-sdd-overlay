package query

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// ResponseTokenCeiling is the hard bound on one response, in tokens.
//
// It is on the WHOLE response rather than on a row count, because rows
// vary by orders of magnitude -- the measured Engram body p50 is 4,483
// bytes and the maximum 42,757 -- so any fixed number of rows bounds
// nothing. It is a ceiling rather than an expectation because a limit
// that is hoped for is not a limit: a caller may ask for fifty rows, and
// the only place that can be refused is where the response is assembled.
const ResponseTokenCeiling = 2000

// ResponseByteCeiling is ResponseTokenCeiling in bytes.
//
// Four bytes per token is an approximation, not a tokenizer. It is used
// deliberately: this package has no tokenizer available and inventing a
// precise-looking one would be worse than an honest ratio. The ratio is
// the same one every measurement behind this change was derived at, so
// the ceiling is stated in the same units the evidence was.
const ResponseByteCeiling = ResponseTokenCeiling * bytesPerToken

const bytesPerToken = 4

// MinSnippetBudget is the smallest per-row snippet share allocateSnippetBudget
// will render before it stops shrinking and lets capResponse fall back to
// dropping rows.
//
// Below it a snippet cannot carry the sentence a match sits in -- the same
// reasoning memory.SnippetBudget (480, derived from 2 sources x 5 rows) is
// built from, at roughly the row count (about 20, at this ceiling) where
// the bound should again correctly fall on row count rather than on
// snippet length. It completes memory.SnippetBudget's derivation rather
// than contradicting it: at ordinary row counts share never gets near 120,
// and only a caller asking for many rows at once reaches it.
const MinSnippetBudget = 120

// snippetMarkerAllowance reserves bytes for the up-to-two truncation
// markers memory.SnippetAt may add around a cut, and for the small JSON
// overhead a row's own "sources" list carries beyond a bare string, so a
// row rendered at share does not exceed it once those are counted. It is a
// constant rather than an exact per-row computation because the exact
// count depends on where in the body the match falls and how many sources
// a row carries, and reserving for a representative case is what keeps
// the whole response inside the measured ceiling rather than narrowly
// over it.
const snippetMarkerAllowance = 24

// capResponse allocates ResponseByteCeiling across result's rows before
// dropping any of them (R-062's budget-before-render), then, only if the
// response still does not fit, drops rows from whichever source holds the
// most surviving slots, iteratively, until it does (R-063).
//
// Shrinking snippets first is not optional politeness: the union this
// change exists to deliver is a set of ROWS, and a layer that recovers a
// byte overrun by deleting rows is deleting exactly what the union bought
// -- in the one place forbidden to re-rank, so it cannot even choose which
// loss hurts least. A shorter snippet on every row costs less than a
// missing row.
//
// It measures the encoded response rather than estimating it, because the
// bound is on what goes on the wire and an estimate of that is not a
// bound.
func capResponse(result *Result) {
	allocateSnippetBudget(result)
	if responseBytes(*result) <= ResponseByteCeiling {
		return
	}

	full := len(result.Results)
	// The diagnostic is appended before the fit is measured so its own
	// bytes are inside the ceiling, never pushing the response back over
	// it after the trimming is done.
	result.Diagnostics = append(result.Diagnostics, Diagnostic{Code: DiagnosticResponseCapped})

	for len(result.Results) > 0 {
		result.Diagnostics[len(result.Diagnostics)-1].Detail = cappedDetail(full-len(result.Results), full)
		if responseBytes(*result) <= ResponseByteCeiling {
			return
		}
		dropLargestSourceRow(result)
	}
	result.Diagnostics[len(result.Diagnostics)-1].Detail = cappedDetail(full, full)
}

// dropLargestSourceRow removes the lowest-ranked surviving row belonging
// to whichever source currently holds the most slots (R-063), rather than
// trimming from the tail of the merged list regardless of which source it
// belongs to.
//
// Ranks are left untouched on the survivors: a kept row's rank is a fact
// about the merge, not about how many rows survived the budget, so a drop
// leaves a gap in the sequence rather than renumbering around itself.
// This is the one place the cap looks at more than one source at once --
// it compares slot counts, not scores, so D8 survives, but it is named
// here rather than presented as free.
//
// A tie -- two sources holding the same number of slots -- is broken by
// source name, alphabetically first: Go's map iteration order is
// unspecified, so picking "whichever the runtime visits first" makes an
// identical call return a different body on different runs. Names are
// sorted before comparing, and the comparison stays strict ">", so the
// alphabetically-first tied name is the one still standing when the loop
// ends and is the one dropLargestSourceRow drops from.
func dropLargestSourceRow(result *Result) {
	counts := make(map[string]int)
	for _, row := range result.Results {
		for _, s := range row.Sources {
			counts[s]++
		}
	}
	names := make([]string, 0, len(counts))
	for name := range counts {
		names = append(names, name)
	}
	sort.Strings(names)

	var largest string
	for _, name := range names {
		if counts[name] > counts[largest] {
			largest = name
		}
	}

	for i := len(result.Results) - 1; i >= 0; i-- {
		for _, s := range result.Results[i].Sources {
			if s == largest {
				result.Results = append(result.Results[:i], result.Results[i+1:]...)
				return
			}
		}
	}
	// Every row carries at least one source, so this is unreachable in
	// practice; it exists only so a future bug here degrades to the old
	// tail-drop rather than looping forever.
	if len(result.Results) > 0 {
		result.Results = result.Results[:len(result.Results)-1]
	}
}

// allocateSnippetBudget renders every row's snippet within a share of
// ResponseByteCeiling computed from the rows actually selected by the
// merge, before capResponse decides whether anything still needs to be
// dropped (R-062).
//
// A row carrying Content -- an Engram-sourced row, which always does --
// re-renders centred on MatchOffset, the same position its original
// snippet was cut from, so shrinking it never moves what it is showing.
// A row with no Content -- a vault or linked row, whose retriever already
// cut its snippet once, or a future embedding-arm row with no match
// position to centre on -- re-renders its existing Snippet from the
// start, the same honest head-slice memory.SnippetAt already falls back
// to when there is nothing to centre on.
func allocateSnippetBudget(result *Result) {
	rows := result.Results
	n := len(rows)
	if n == 0 {
		return
	}

	saved := make([]string, n)
	savedTrunc := make([]bool, n)
	for i := range rows {
		saved[i], savedTrunc[i] = rows[i].Snippet, rows[i].SnippetTruncated
		rows[i].Snippet, rows[i].SnippetTruncated = "", false
	}
	// overhead is the response's encoded size with every snippet blanked
	// -- measured, not estimated, the same discipline the drop pass above
	// already applies.
	overhead := responseBytes(*result)
	for i := range rows {
		rows[i].Snippet, rows[i].SnippetTruncated = saved[i], savedTrunc[i]
	}

	available := ResponseByteCeiling - overhead
	share := available / n
	if share > memory.SnippetBudget {
		share = memory.SnippetBudget
	}
	if share < MinSnippetBudget {
		share = MinSnippetBudget
	}

	rendered := 0
	var stillTruncated []int
	for i := range rows {
		renderRowSnippet(&rows[i], share)
		rendered += len(rows[i].Snippet)
		if rows[i].SnippetTruncated {
			stillTruncated = append(stillTruncated, i)
		}
	}

	// Short bodies leave part of their row's share unused; that leftover
	// is reclaimed exactly once for whatever is still truncated at share,
	// rather than iterated to convergence -- a loop's termination would
	// depend on the data, and one pass is deterministic and bounded.
	if len(stillTruncated) == 0 {
		return
	}
	leftover := available - rendered
	if leftover <= 0 {
		return
	}
	share2 := share + leftover/len(stillTruncated)
	if share2 > memory.SnippetBudget {
		share2 = memory.SnippetBudget
	}
	if share2 <= share {
		return
	}
	for _, i := range stillTruncated {
		renderRowSnippet(&rows[i], share2)
	}
}

// renderRowSnippet re-renders row's Snippet at budget, reserving
// snippetMarkerAllowance so the rendered bytes -- including whatever
// truncation markers memory.SnippetAt adds -- do not exceed budget.
//
// A row with no Content -- a vault row, whose snippet was already cut by
// the vault's own retriever before this module ever saw it -- has no full
// body here to measure and no claim to make about one (ResultRow.
// SnippetTruncated's own doc comment). Re-slicing that snippet down to
// budget can still shrink the text, but it must never set
// SnippetTruncated: doing so would ship "snippet_truncated: true" beside a
// FullLength that stays 0, the exact contradiction the field exists to
// forbid. FullLength is left untouched (0) either way.
func renderRowSnippet(row *ResultRow, budget int) {
	content, offset := row.Content, row.MatchOffset
	hasFullBody := content != ""
	if !hasFullBody {
		content, offset = row.Snippet, 0
	}
	window := budget - snippetMarkerAllowance
	if window < 1 {
		window = 1
	}
	snippet, truncated := memory.SnippetAt(content, offset, window)
	row.Snippet = snippet
	if hasFullBody {
		row.SnippetTruncated = truncated
	} else {
		row.SnippetTruncated = false
	}
}

// cappedDetail states what was dropped in the terms a caller needs to act
// on it: how many results exist that they are not looking at.
//
// "Dropped from the end" and "the highest-ranked ones that fit" are the wording of the tail drop this cap had
// before R-063, kept as it is because it is printed: rows now leave by dropLargestSourceRow, the lowest-ranked
// of the source holding the most, which is not always the tail.
func cappedDetail(dropped, full int) string {
	return fmt.Sprintf(
		"this response reached the %d-token ceiling, so %d of %d results were dropped from the end; the rows shown are the highest-ranked ones that fit, and narrowing the query or lowering top will surface the rest",
		ResponseTokenCeiling, dropped, full)
}

// responseBytes is the size of result as it will be encoded on the wire.
func responseBytes(result Result) int {
	encoded, err := json.Marshal(result)
	if err != nil {
		// A Result that cannot be marshalled cannot be sent either, so
		// there is nothing to cap; the encoder downstream reports it.
		return 0
	}
	return len(encoded)
}
