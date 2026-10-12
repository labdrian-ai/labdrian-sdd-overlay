// Package query implements longterm-mem's unified query fan-out and merge
// (R-006, D8): a caller-selected set of sources, vault matches first in
// vault order when the vault is requested, then Engram's own sources
// round-robin-interleaved in their own native order -- never re-ranked --
// with any linked pair or cross-arm duplicate collapsed into one row. A
// not-provisioned vault degrades to Engram-only results (R-026).
package query

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// ErrMissingProject rejects a call with no project (R-006).
var ErrMissingProject = errors.New("query: project is required")

// ErrUnknownSource rejects a Request naming a source this function does
// not recognise. An unrecognised name is refused, never ignored: silently
// narrowing the corpus to the sources that happen to be spelled correctly
// is the failure R-060 exists to remove.
var ErrUnknownSource = errors.New("query: unknown source")

// ErrNoMemory rejects a call that asks for a source read from the memory (the lexical and the embedding
// sources) when Deps carries no Memory, or one that holds a nil pointer: the call fails in the
// query's own words instead of panicking inside a store that is not there.
var ErrNoMemory = errors.New("query: no memory is configured for a source that reads it")

// Request is one Run call's input; Project is required.
type Request struct {
	Project string
	Query   string
	Top     int
	// ExcludeTypes are Engram observation types to leave out.
	//
	// It is a filter, not a re-ranking, and the distinction is what makes
	// it permissible: D8 forbids fusing scores across sources, and the
	// rows that survive an exclusion keep exactly the merge order they
	// had. Excluding a type is declining to return a row, not deciding it
	// is worth less than another one.
	//
	// The default excludes nothing, and that is measured rather than
	// timid. session_summary is the obvious candidate -- 71 of the live
	// project's 581 observations, and the type whose bodies run 15k-43k
	// bytes -- but the payload argument for excluding it is spent: those
	// bodies now cost one snippet like every other row. The relevance
	// argument does not survive measurement either: across eight real
	// queries session_summary took 5 of 36 top-5 slots, about its 12%
	// share of the corpus, so bm25 is already ranking it fairly. A
	// default exclusion would drop real answers to buy a gain the numbers
	// do not show. A caller who knows it wants implementation memory
	// rather than session narrative can still say so.
	ExcludeTypes []string
	// Sources names which sources to query (R-060). Empty means
	// defaultSources: engram-fts plus engram-embed when the project's
	// embedding index exists, engram-fts alone otherwise (see
	// defaultSources; the vault is never defaulted in, queried only when
	// named explicitly). An unknown name is rejected with ErrUnknownSource
	// rather than silently skipped.
	Sources []string
}

// Deps are Run's dependencies. Memory and Vault are ports the package owns (a
// fake, or a temp DB behind the Engram adapter, in tests); ResolveLink is a
// function seam.
type Deps struct {
	Memory Memory
	// Vault retrieves from the project's vault. It is asked only when the
	// vault is a requested source (R-060); a call that names the vault with
	// none wired is refused with ErrNoVault.
	Vault VaultRetriever
	// ResolveLink reports the Engram id an existing promotion links to
	// vault page pageAddress (D6 store, not built until slice 4/5).
	ResolveLink func(pageAddress string) (engramID int64, ok bool)
	// Embed embeds one query string into a vector for the embedding arm.
	// It is invoked only when engram-embed is requested, so a call that
	// never names that source never reaches the network (R-071). nil
	// degrades exactly like an index that was never built: zero rows,
	// Coverage says so.
	Embed EmbedFunc
	// Index reads a project's embedding index. It is read when a caller
	// names engram-embed in Sources, and by the default sources to learn
	// whether an index exists; every other call leaves it unused. None
	// wired (or a nil pointer) is no index: the arm degrades as it does for
	// a project nothing was built for. A long-lived caller that serves more
	// than one query per process -- the MCP server -- hands it a repository
	// that caches, so a project's index is read from disk at most once per
	// change rather than on every query that names or defaults to
	// engram-embed.
	Index EmbeddingIndexRepository
	// IndexBuilder incrementally (re)builds project's embedding index
	// under the contract the embedding arm just read off the existing
	// index, so a top-up never silently reinterprets what "the index"
	// means for this project. It is asked only when the arm finds a small,
	// bounded gap (see TopUpMaxRows) between the index and the live corpus,
	// right before scanning; none wired (or a nil pointer) skips the top-up
	// entirely and leaves the degrade-and-name-the-CLI-command behaviour
	// unchanged.
	IndexBuilder EmbeddingIndexBuilder
}

// NoLinkResolver reports every page as unlinked (default until D6 exists).
func NoLinkResolver(string) (int64, bool) { return 0, false }

// Run fans a query out to every requested source, then merges them
// (R-006, R-026, R-060).
func Run(ctx context.Context, deps Deps, req Request) (Result, error) {
	if req.Project == "" {
		return Result{}, ErrMissingProject
	}
	sources := req.Sources
	if len(sources) == 0 {
		sources = defaultSources(deps, req.Project)
	}
	for _, s := range sources {
		if !knownSources[s] {
			return Result{}, fmt.Errorf("%w: %q", ErrUnknownSource, s)
		}
	}
	wantVault := containsSource(sources, SourceVault)
	wantFTS := containsSource(sources, SourceEngramFTS)
	wantEmbed := containsSource(sources, SourceEngramEmbed)
	if (wantFTS || wantEmbed) && memory.IsMissing(deps.Memory) {
		return Result{}, ErrNoMemory
	}
	if wantVault && memory.IsMissing(deps.Vault) {
		return Result{}, ErrNoVault
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

	var engramRows []memory.Row
	matchMode := memory.MatchAll
	if wantFTS {
		search, err := deps.Memory.Search(req.Project, req.Query, top, req.ExcludeTypes...)
		if err != nil {
			return Result{}, fmt.Errorf("query: search engram: %w", err)
		}
		engramRows = search.Rows
		matchMode = search.MatchMode
		if search.MatchMode == memory.MatchAny {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{
				Code:   DiagnosticSearchWidened,
				Detail: "no observation matched every term of this query, so it was retried matching any one of them: these rows answer part of the query, not all of it",
			})
		}
		if len(req.ExcludeTypes) > 0 {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{
				Code:   DiagnosticTypesExcluded,
				Detail: fmt.Sprintf("these observation types were excluded from the search at the caller's request, so matching memories of these kinds are not shown: %s", strings.Join(req.ExcludeTypes, ", ")),
			})
		}
		if len(search.DroppedTokens) > 0 {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{
				Code:   DiagnosticSearchStopwordsDropped,
				Detail: fmt.Sprintf("these terms were not searched, as words too common to narrow anything down: %s", strings.Join(search.DroppedTokens, ", ")),
			})
		}
		if degraded, cause := deps.Memory.Degraded(); degraded {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{
				Code:   DiagnosticEngramDegradedSnapshot,
				Detail: fmt.Sprintf("engram is being read through the immutable=1 fallback, so these results come from the snapshot taken when the connection was opened, not the live database: %s", cause),
			})
		}
	}

	var embedRows []ResultRow
	if wantEmbed {
		rows, coverage, diags := embeddingArm{Memory: deps.Memory, Index: deps.Index, Builder: deps.IndexBuilder, Embed: deps.Embed}.run(ctx, req.Project, req.Query, top)
		embedRows = rows
		result.Coverage = append(result.Coverage, coverage)
		result.Diagnostics = append(result.Diagnostics, diags...)
		if d := coverageIncompleteDiagnostic(coverage); d != nil {
			result.Diagnostics = append(result.Diagnostics, *d)
		}
	}

	var vaultRows []VaultCandidate
	if wantVault {
		candidates, vaultErr := deps.Vault.RetrieveVault(ctx, req.Project, req.Query, top)
		switch {
		case errors.Is(vaultErr, ErrVaultNotProvisioned):
			result.VaultStatus = VaultStatusNotProvisioned
		case vaultErr != nil:
			// D8: a subprocess failure degrades to Engram-only + a diagnostic
			// rather than failing the call. Follow-up: distinguish the
			// runner's synthetic timeout exit (124) once the retriever
			// exposes exit codes typed.
			result.VaultStatus = VaultStatusError
			result.Diagnostics = append(result.Diagnostics, Diagnostic{Code: DiagnosticVaultSubprocessFailed, Detail: vaultErr.Error()})
		default:
			result.VaultStatus = VaultStatusOK
			vaultRows = candidates
		}
	} else {
		result.VaultStatus = VaultStatusNotRequested
	}

	result.Results = mergeResults(wantVault, wantFTS, wantEmbed, vaultRows, engramRows, embedRows, resolveLink, req.Query, matchMode)
	result.Diagnostics = append(result.Diagnostics, attachStandings(deps.Memory, result.Results)...)
	capResponse(&result)
	return result, nil
}
