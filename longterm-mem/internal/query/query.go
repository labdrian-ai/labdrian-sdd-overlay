// Package query implements longterm-mem's unified query fan-out and merge
// (R-006, D8): vault matches first in vault order, then Engram matches in
// Engram order -- never re-ranked -- with any linked pair collapsed into
// one row. A not-provisioned vault degrades to Engram-only results (R-026).
package query

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vault"
)

// DefaultTopN mirrors vault.DefaultTopN (D8: both sources share one bound).
const DefaultTopN = vault.DefaultTopN

// ErrMissingProject rejects a call with no project (R-006).
var ErrMissingProject = errors.New("query: project is required")

// Result.VaultStatus values.
const (
	VaultStatusOK             = "ok"
	VaultStatusNotProvisioned = "not_provisioned"
	VaultStatusError          = "error"
)

// ResultRow.Source values.
const (
	SourceVault  = "vault"
	SourceEngram = "engram"
	SourceLinked = "linked"
)

// Diagnostic.Code values.
const (
	// DiagnosticVaultSubprocessFailed reports that the vault's retrieval
	// entrypoint failed, so these results are Engram-only (D8).
	DiagnosticVaultSubprocessFailed = "vault_subprocess_failed"

	// DiagnosticRelationsUnreadable: the relation ledger could not be
	// read, so no result carries what it says. Silence here would read as
	// "nothing is superseded", which is the reading that lets an abandoned
	// decision pass as current.
	DiagnosticRelationsUnreadable = "relations_unreadable"
	// DiagnosticEngramDegradedSnapshot reports that Engram is being read
	// through engram.Open's immutable=1 fallback: the results come from a
	// point-in-time snapshot taken when the connection was opened, not
	// from the live database.
	//
	// It matters most where the connection outlives the call. The MCP
	// server opens Engram once for a whole session (cmd_mcp.go), so a
	// degraded fallback there serves a frozen corpus for as long as the
	// client stays connected -- observations saved during the session are
	// simply absent, and the results look complete. Store.Degraded had
	// exactly one production reader (the status command), which a client
	// calling the query tool never sees, so the freeze was invisible
	// precisely where it lasts longest.
	DiagnosticEngramDegradedSnapshot = "engram_degraded_snapshot"

	// DiagnosticSearchWidened reports that requiring every query token
	// found nothing, so the search was retried requiring any one of them.
	// The rows below it are real matches on part of the query, not on the
	// whole of it, and are worth correspondingly less trust -- which is
	// invisible in the rows themselves.
	//
	// The alternative was to widen silently. It is worse than it looks:
	// the failure being fixed here IS a silent one, and answering it with
	// a second silence trades an empty result nobody can see for a broad
	// result nobody can see either.
	DiagnosticSearchWidened = "search_widened"

	// DiagnosticSearchStopwordsDropped names the query tokens that were
	// removed before searching, so a caller can tell a corpus with no
	// answer from a query that was quietly rewritten.
	DiagnosticSearchStopwordsDropped = "search_stopwords_dropped"

	// DiagnosticResponseCapped reports that the assembled response did not
	// fit under ResponseTokenCeiling, so its tail was dropped. It names how
	// many rows were dropped, because "these are the results" and "these
	// are the results that fit" are different statements and only one of
	// them is true here.
	DiagnosticResponseCapped = "response_capped"
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

// Request is one Run call's input; Project is required.
type Request struct {
	Project string
	Query   string
	Top     int
}

// Deps are Run's dependencies. RetrieveVault/ResolveLink are function seams
// for tests; Engram is a real *engram.Store (temp DB in tests).
type Deps struct {
	Engram        *engram.Store
	RetrieveVault func(ctx context.Context, project, query string, top int) (vault.Result, error)
	// ResolveLink reports the Engram id an existing promotion links to
	// vault page pageAddress (D6 store, not built until slice 4/5).
	ResolveLink func(pageAddress string) (engramID int64, ok bool)
}

// NoLinkResolver reports every page as unlinked (default until D6 exists).
func NoLinkResolver(string) (int64, bool) { return 0, false }

// Score carries a row's native per-source scores, never fused (D8).
type Score struct {
	BM25   float64 `json:"bm25,omitempty"`
	Rerank float64 `json:"rerank,omitempty"`
}

// ResultRow is one merged result (D8's JSON shape).
type ResultRow struct {
	Source      string `json:"source"`
	Rank        int    `json:"rank"`
	PageAddress string `json:"page_address,omitempty"`
	PagePath    string `json:"page_path,omitempty"`
	EngramID    int64  `json:"engram_id,omitempty"`
	Title       string `json:"title,omitempty"`
	Snippet     string `json:"snippet,omitempty"`
	// SnippetTruncated reports that Snippet is an extract of a longer
	// body, and FullLength says how long that body is in bytes.
	//
	// They are the machine-readable half of a statement the snippet text
	// also makes with a "…" at each cut edge, and both halves are
	// required. A person reading the text needs the marker; a program
	// deciding whether to fetch the rest needs the fields, and cannot be
	// asked to look for an ellipsis. Without them a caller has no way to
	// tell a preview from a whole memory, and will make a decision on a
	// fragment that looked complete -- which is exactly the failure a cap
	// introduces if it is shipped without visibility.
	//
	// A vault row leaves both zero: its snippet was cut by the vault's own
	// retriever before this module saw it, so there is no full body here
	// to measure and no claim to make about one.
	SnippetTruncated bool   `json:"snippet_truncated,omitempty"`
	FullLength       int    `json:"full_length,omitempty"`
	Score            *Score `json:"score,omitempty"`
	// Standing is what Engram's relation ledger says about this
	// observation: replaced, contradicted, or flagged and never decided.
	// It is nil when there is nothing to say, and absent from a vault-only
	// row, which has no observation behind it.
	//
	// It is carried here because Engram's own search does not carry it.
	// Verified on a copy of a real database: inserting "B supersedes A"
	// left A's results byte-identical, still first, unmarked. A memory that
	// was explicitly replaced therefore reads as current, and gets
	// reintroduced. This module cannot fix that search (R-002 keeps its
	// connection read-only); it can decline to repeat the omission.
	Standing *engram.Standing `json:"standing,omitempty"`
}

// Diagnostic is one non-fatal condition alongside a Result.
type Diagnostic struct {
	Code   string `json:"code"`
	Detail string `json:"detail"`
}

// Result is Run's output.
type Result struct {
	Project     string       `json:"project"`
	Query       string       `json:"query"`
	VaultStatus string       `json:"vault_status"`
	Results     []ResultRow  `json:"results"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}

// Run fans a query out to Engram and the vault, then merges by source
// (R-006, R-026).
func Run(ctx context.Context, deps Deps, req Request) (Result, error) {
	if req.Project == "" {
		return Result{}, ErrMissingProject
	}
	top := req.Top
	if top <= 0 {
		top = DefaultTopN
	}
	resolveLink := deps.ResolveLink
	if resolveLink == nil {
		resolveLink = NoLinkResolver
	}

	result := Result{Project: req.Project, Query: req.Query}
	search, err := deps.Engram.Search(req.Project, req.Query, top)
	if err != nil {
		return Result{}, fmt.Errorf("query: search engram: %w", err)
	}
	engramRows := search.Rows
	if search.MatchMode == engram.MatchAny {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{
			Code:   DiagnosticSearchWidened,
			Detail: "no observation matched every term of this query, so it was retried matching any one of them: these rows answer part of the query, not all of it",
		})
	}
	if len(search.DroppedTokens) > 0 {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{
			Code:   DiagnosticSearchStopwordsDropped,
			Detail: fmt.Sprintf("these terms were not searched, as words too common to narrow anything down: %s", strings.Join(search.DroppedTokens, ", ")),
		})
	}
	if degraded, cause := deps.Engram.Degraded(); degraded {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{
			Code:   DiagnosticEngramDegradedSnapshot,
			Detail: fmt.Sprintf("engram is being read through the immutable=1 fallback, so these results come from the snapshot taken when the connection was opened, not the live database: %s", cause),
		})
	}

	var vaultRows []vault.Candidate
	vaultResult, vaultErr := deps.RetrieveVault(ctx, req.Project, req.Query, top)
	switch {
	case vaultErr != nil:
		// D8: a subprocess failure degrades to Engram-only + a diagnostic
		// rather than failing the call. Follow-up: distinguish the
		// runner's synthetic timeout exit (124) once vault.Retrieve
		// exposes exit codes typed (retrieve.go/status.go are reused, not
		// modified, in this slice).
		result.VaultStatus = VaultStatusError
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Code: DiagnosticVaultSubprocessFailed, Detail: vaultErr.Error()})
	case vaultResult.Status == vault.StatusNotProvisioned:
		result.VaultStatus = VaultStatusNotProvisioned
	default:
		result.VaultStatus = VaultStatusOK
		vaultRows = vaultResult.Candidates
	}

	result.Results = mergeResults(vaultRows, engramRows, resolveLink)
	result.Diagnostics = append(result.Diagnostics, attachStandings(deps.Engram, result.Results)...)
	capResponse(&result)
	return result, nil
}

// capResponse drops rows from the end of result until it encodes within
// ResponseByteCeiling.
//
// From the END, and only from the end. The rows are already in D8's merge
// order and that order is a correctness guarantee, so the only thing this
// may do is keep a prefix of it: dropping from the middle, reordering, or
// re-scoring to pack more in would all be re-ranking, which this module
// is forbidden to do. Rank numbers are left exactly as merged for the same
// reason -- a kept row's rank is a fact about the merge, not about how
// many rows survived the budget.
//
// It measures the encoded response rather than estimating it, because the
// bound is on what goes on the wire and an estimate of that is not a
// bound. The cost is one marshal per dropped row, on a response that is by
// definition already too big to be cheap.
func capResponse(result *Result) {
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
		result.Results = result.Results[:len(result.Results)-1]
	}
	result.Diagnostics[len(result.Diagnostics)-1].Detail = cappedDetail(full, full)
}

// cappedDetail states what was dropped in the terms a caller needs to act
// on it: how many results exist that they are not looking at.
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

// attachStandings annotates each row that has an observation behind it.
//
// A relation ledger that cannot be read degrades to a diagnostic rather
// than failing the query, exactly as a failing vault does: the results are
// still the results, and losing the annotation is a smaller harm than
// losing the answer. It is reported rather than swallowed, because silence
// here is indistinguishable from "nothing is superseded" -- the reading
// that lets an abandoned decision pass as current.
func attachStandings(store *engram.Store, rows []ResultRow) []Diagnostic {
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		if r.EngramID != 0 {
			ids = append(ids, r.EngramID)
		}
	}
	if len(ids) == 0 {
		return nil
	}

	standings, err := store.Standings(ids)
	if err != nil {
		return []Diagnostic{{
			Code:   DiagnosticRelationsUnreadable,
			Detail: fmt.Sprintf("engram's relation ledger could not be read, so no result is marked as superseded or contradicted even if it is: %v", err),
		}}
	}

	for i := range rows {
		if st, ok := standings[rows[i].EngramID]; ok && !st.Empty() {
			standing := st
			rows[i].Standing = &standing
		}
	}
	return nil
}

// mergeResults implements D8's merge (3b.8: MatchLinkedEngramRow is the
// extracted matcher, reused unchanged by promote/MCP query later).
func mergeResults(vaultRows []vault.Candidate, engramRows []engram.Row, resolveLink func(string) (int64, bool)) []ResultRow {
	consumed := make(map[int64]bool, len(engramRows))
	var merged []ResultRow
	for _, c := range vaultRows {
		if er, ok := MatchLinkedEngramRow(c.PageAddress, engramRows, resolveLink); ok && !consumed[er.ID] {
			consumed[er.ID] = true
			merged = append(merged, ResultRow{
				Source: SourceLinked, PageAddress: c.PageAddress, PagePath: c.AbsolutePath,
				EngramID: er.ID, Title: er.Title, Snippet: c.Snippet,
				Score: &Score{BM25: c.BM25Score, Rerank: c.RerankScore},
			})
			continue
		}
		merged = append(merged, ResultRow{
			Source: SourceVault, PageAddress: c.PageAddress, PagePath: c.AbsolutePath, Snippet: c.Snippet,
			Score: &Score{BM25: c.BM25Score, Rerank: c.RerankScore},
		})
	}
	for _, er := range engramRows {
		if consumed[er.ID] {
			continue
		}
		merged = append(merged, ResultRow{
			Source: SourceEngram, EngramID: er.ID, Title: er.Title,
			Snippet: er.Snippet, SnippetTruncated: er.SnippetTruncated, FullLength: er.ContentLength,
		})
	}
	for i := range merged {
		merged[i].Rank = i + 1
	}
	return merged
}

// MatchLinkedEngramRow reports whether pageAddress links (via resolveLink)
// to one of engramRows (3b.8 REFACTOR).
func MatchLinkedEngramRow(pageAddress string, engramRows []engram.Row, resolveLink func(string) (int64, bool)) (engram.Row, bool) {
	if resolveLink == nil {
		return engram.Row{}, false
	}
	id, ok := resolveLink(pageAddress)
	if !ok {
		return engram.Row{}, false
	}
	for _, row := range engramRows {
		if row.ID == id {
			return row, true
		}
	}
	return engram.Row{}, false
}
