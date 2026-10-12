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
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vault"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vecindex"
)

// DefaultTopN mirrors vault.DefaultTopN (D8: both sources share one bound).
const DefaultTopN = vault.DefaultTopN

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

// Result.VaultStatus values.
const (
	VaultStatusOK             = "ok"
	VaultStatusNotProvisioned = "not_provisioned"
	// VaultStatusNotRequested reports that the vault was never asked,
	// because the caller's sources did not name it (R-060). It is a
	// distinct value from "not_provisioned" -- one says the vault has
	// nothing to offer, the other says it was never given the chance to
	// answer -- and collapsing the two into one value would hide which is
	// true.
	VaultStatusNotRequested = "not_requested"
	VaultStatusError        = "error"
)

// ResultRow.Sources values.
const (
	// SourceVault names the vault as a requested source (R-060).
	SourceVault = "vault"
	// SourceEngramFTS names Engram's own lexical (bm25/FTS5) search as a
	// requested source (R-060).
	SourceEngramFTS = "engram-fts"
	// SourceEngramEmbed names Engram's embedding-index search as a
	// requested source (R-060): a cosine-similarity search over
	// internal/vecindex's own index, populated by `index --embeddings`.
	// Naming it when no index has ever been built degrades to zero rows
	// plus a Coverage entry saying so (design), rather than an error --
	// the same soft-degradation rule R-026 applies to a not-provisioned
	// vault.
	SourceEngramEmbed = "engram-embed"
	// SourceLinked names a row emitted from an existing vault<->Engram
	// promotion link (R-006's "linked pair" scenario): one vault page and
	// one Engram observation known to refer to the same memory, merged
	// into a single row rather than two.
	SourceLinked = "linked"
)

// knownSources is every name Request.Sources may contain.
var knownSources = map[string]bool{
	SourceEngramFTS:   true,
	SourceEngramEmbed: true,
	SourceVault:       true,
}

// defaultSources is what Run queries when Request.Sources is empty:
// engram-fts and engram-embed together (R-060's own final shape) the
// moment project's embedding index actually exists, since a caller has no
// way to know from the outside whether one has ever been built -- and
// engram-fts alone otherwise, since asking the embedding arm to run on
// every query for a project with nothing indexed would cost a network
// round trip for a guaranteed-empty answer (embedarm.go's own
// "no index built yet" degradation covers a caller who names engram-embed
// explicitly against such a project; this is only about what an omitted
// Sources defaults to).
//
// The vault is unaffected either way: it is opt-in, queried only when a
// caller names it explicitly (R-060), regardless of what the two Engram
// arms default to. A caller that wants it names it:
// sources: ["vault", "engram-fts"].
func defaultSources(deps Deps, project string) []string {
	loadIndex := resolveLoadIndex(deps)
	if _, err := loadIndex(vecindex.Dir(deps.StateDir, project)); err == nil {
		return []string{SourceEngramFTS, SourceEngramEmbed}
	}
	return []string{SourceEngramFTS}
}

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
	// DiagnosticEngramDegradedSnapshot reports that the memory behind
	// Deps.Memory says it is degraded (DegradationReporter.Degraded). The
	// one implementation is internal/engram's Store, wired by
	// cmd/longterm-mem, which falls back to SQLite's immutable=1 read when
	// its primary read-only connection cannot be established: the results
	// come from a point-in-time snapshot taken when the connection was
	// opened, not from the live database. The diagnostic's code and text
	// name Engram and immutable=1 on purpose: they are what the user sees
	// and what the operator can act on, and are part of the output that
	// must not change.
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

	// DiagnosticTypesExcluded names the observation types the caller asked
	// to leave out. A corpus quietly narrowed is the same failure as a
	// corpus quietly empty: in both cases the caller reads "there is
	// nothing else" from a result that does not say that.
	DiagnosticTypesExcluded = "types_excluded"

	// DiagnosticEmbeddingBackendUnreachable reports that the embedding
	// backend never answered at all (R-070) -- distinct from
	// DiagnosticEmbeddingModelMissing so a caller can act on which
	// condition applies rather than treating both as one opaque failure.
	DiagnosticEmbeddingBackendUnreachable = "embedding_backend_unreachable"
	// DiagnosticEmbeddingModelMissing reports that the embedding backend
	// answered but the configured model is not pulled (R-070).
	DiagnosticEmbeddingModelMissing = "embedding_model_missing"
	// DiagnosticEmbeddingIndexIncomplete names the exact command that
	// fixes an embedding index behind the live corpus (or never built),
	// so a caller reading a thin or empty paraphrase result is not left
	// to infer the fix from Coverage's bare numbers.
	DiagnosticEmbeddingIndexIncomplete = "embedding_index_incomplete"
	// DiagnosticCoverageUnreadable reports that Coverage's counts could
	// not be read from Engram at all, so the zeros below are not
	// measurements. Left unnamed this degrades twice over: a Live of 0 is
	// indistinguishable from a project with no memory, and an Indexed of 0
	// makes every live observation look unindexed, which the incomplete-
	// index diagnostic then turns into an instruction to rebuild an index
	// that was never the problem.
	DiagnosticCoverageUnreadable = "coverage_unreadable"

	// DiagnosticEmbeddingToppedUp reports that the embedding arm found a
	// small, bounded gap between the manifest and the live corpus (see
	// TopUpMaxRows) and closed it with an incremental build BEFORE
	// scanning, so this call's results already reflect the topped-up
	// index rather than the stale one Coverage would otherwise have
	// described.
	DiagnosticEmbeddingToppedUp = "embedding_index_topped_up"
	// DiagnosticEmbeddingTopUpFailed reports that a bounded top-up was
	// attempted (the gap was within TopUpMaxRows) but the build itself
	// failed -- the embedding backend was unreachable, for example. The
	// query still answers: it degrades exactly as an index that was never
	// topped up would, from the stale index it already had, plus this
	// diagnostic naming what went wrong so the failure is not silent.
	DiagnosticEmbeddingTopUpFailed = "embedding_topup_failed"
)

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

// Deps are Run's dependencies. RetrieveVault/ResolveLink are function seams
// for tests; Memory is the port the package owns (a fake, or a temp DB behind
// the Engram adapter, in tests).
type Deps struct {
	Memory        Memory
	RetrieveVault func(ctx context.Context, project, query string, top int) (vault.Result, error)
	// ResolveLink reports the Engram id an existing promotion links to
	// vault page pageAddress (D6 store, not built until slice 4/5).
	ResolveLink func(pageAddress string) (engramID int64, ok bool)
	// StateDir is the directory internal/vecindex's embedding index lives
	// under (StateDir/index/<project>/, vecindex.Dir). It is read only
	// when a caller names engram-embed in Sources; every other call
	// leaves it unused.
	StateDir string
	// Embed embeds one query string into a vector for the embedding arm.
	// It is invoked only when engram-embed is requested, so a call that
	// never names that source never reaches the network (R-071). nil
	// degrades exactly like an index that was never built: zero rows,
	// Coverage says so.
	Embed EmbedFunc
	// BuildIndex incrementally (re)builds project's embedding index under
	// the given model/dimension/inputLimit contract -- the same contract
	// the embedding arm just read off the existing manifest, so a top-up
	// never silently reinterprets what "the index" means for this project.
	// It is invoked only when the embedding arm finds a small, bounded gap
	// (see TopUpMaxRows) between the manifest and the live corpus, right
	// before scanning; nil skips the top-up entirely and leaves today's
	// degrade-and-name-the-CLI-command behaviour unchanged. A production
	// caller wires it to read live rows and call vecindex.Build, the same
	// way cmd_index_embeddings.go already does.
	BuildIndex BuildIndexFunc
	// LoadIndex loads one project's embedding index, defaulting to
	// vecindex.Load when nil so an existing or non-MCP caller (the CLI
	// query subcommand, every test that does not set this field) is
	// unaffected. A long-lived caller that serves more than one query per
	// process -- the MCP server -- wires it to a *vecindex.LoadCache's Load
	// method instead, so a project's index is read from disk at most once
	// per change rather than on every query that names or defaults to
	// engram-embed.
	LoadIndex func(dir string) (*vecindex.Index, error)
}

// resolveLoadIndex returns deps.LoadIndex, or vecindex.Load when deps did
// not set one -- the one place that default is decided, so defaultSources
// and runEmbeddingArm's own index load can never drift into resolving it
// two different ways.
func resolveLoadIndex(deps Deps) func(dir string) (*vecindex.Index, error) {
	if deps.LoadIndex != nil {
		return deps.LoadIndex
	}
	return vecindex.Load
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
	// Sources names every requested source that produced this row. A row
	// found by more than one source (R-058's union recall guarantee makes
	// this possible for the first time) names all of them, not just the
	// one that happened to win a tie-break -- there is no tie-break, D8
	// forbids one.
	Sources     []string `json:"sources"`
	Rank        int      `json:"rank"`
	PageAddress string   `json:"page_address,omitempty"`
	PagePath    string   `json:"page_path,omitempty"`
	EngramID    int64    `json:"engram_id,omitempty"`
	Title       string   `json:"title,omitempty"`
	Snippet     string   `json:"snippet,omitempty"`
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
	// It is carried here because Engram's own search does not carry it,
	// and that is established from the SQL rather than from an
	// experiment. engram.Search selects from observations_fts joined to
	// observations and nothing else: memory_relations is not in the query,
	// so no relation can reach a result through it, whatever any
	// particular database happens to contain.
	//
	// An earlier version of this comment claimed the same conclusion from
	// a probe run "on a copy of a real database". That claim was wrong and
	// is corrected rather than quietly deleted, because it is the kind of
	// evidence a later reader would rely on. ENGRAM_DATABASE_URL is
	// silently ignored, so every probe said to run against a copy in fact
	// read the live database (Engram observation #3239); the probes
	// therefore measured nothing about a copy, and an experiment whose
	// subject is not what it says it is cannot support anything.
	//
	// The consequence is unchanged: a memory that was explicitly replaced
	// reads as current, and gets reintroduced. This module cannot fix that
	// search (R-002 keeps its connection read-only); it can decline to
	// repeat the omission.
	Standing *memory.Standing `json:"standing,omitempty"`
	// MatchOffset is the byte offset memory.SnippetAt should centre a
	// re-render on, mirroring memory.Row.MatchOffset. It only matters for
	// a row with Content set.
	MatchOffset int `json:"-"`
	// Content is the row's full Engram body, carried through the merge so
	// the budget allocator can re-render Snippet at a share computed after
	// every row is known, without a second query (R-062). It is never
	// serialised: a vault or linked row leaves it empty, having no full
	// body this module holds, and an engram-sourced row's Content is
	// discarded once its snippet is rendered.
	Content string `json:"-"`
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
	// Coverage reports, per requested source that depends on an index
	// this module owns, how much of the live corpus that index currently
	// describes (design: "coverage is a response field, not a
	// diagnostic"). Deliberately no `omitempty`: an absent field and "the
	// index is complete" must never look alike.
	Coverage []Coverage `json:"coverage"`
}

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
		rows, coverage, diags := runEmbeddingArm(ctx, deps.Memory, deps.StateDir, req.Project, req.Query, top, deps.Embed, deps.BuildIndex, resolveLoadIndex(deps))
		embedRows = rows
		result.Coverage = append(result.Coverage, coverage)
		result.Diagnostics = append(result.Diagnostics, diags...)
		if d := coverageIncompleteDiagnostic(coverage); d != nil {
			result.Diagnostics = append(result.Diagnostics, *d)
		}
	}

	var vaultRows []vault.Candidate
	if wantVault {
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
	} else {
		result.VaultStatus = VaultStatusNotRequested
	}

	result.Results = mergeResults(wantVault, wantFTS, wantEmbed, vaultRows, engramRows, embedRows, resolveLink, req.Query, matchMode)
	result.Diagnostics = append(result.Diagnostics, attachStandings(deps.Memory, result.Results)...)
	capResponse(&result)
	return result, nil
}

// containsSource reports whether name is in sources.
func containsSource(sources []string, name string) bool {
	for _, s := range sources {
		if s == name {
			return true
		}
	}
	return false
}
