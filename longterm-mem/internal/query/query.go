// Package query implements longterm-mem's unified query fan-out and merge
// (R-006, D8): vault matches first in vault order, then Engram matches in
// Engram order -- never re-ranked -- with any linked pair collapsed into
// one row. A not-provisioned vault degrades to Engram-only results (R-026).
package query

import (
	"context"
	"errors"
	"fmt"

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
	Score       *Score `json:"score,omitempty"`
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
	engramRows, err := deps.Engram.Search(req.Project, req.Query, top)
	if err != nil {
		return Result{}, fmt.Errorf("query: search engram: %w", err)
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
		result.Diagnostics = append(result.Diagnostics, Diagnostic{Code: "vault_subprocess_failed", Detail: vaultErr.Error()})
	case vaultResult.Status == vault.StatusNotProvisioned:
		result.VaultStatus = VaultStatusNotProvisioned
	default:
		result.VaultStatus = VaultStatusOK
		vaultRows = vaultResult.Candidates
	}

	result.Results = mergeResults(vaultRows, engramRows, resolveLink)
	return result, nil
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
		merged = append(merged, ResultRow{Source: SourceEngram, EngramID: er.ID, Title: er.Title, Snippet: er.Content})
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
