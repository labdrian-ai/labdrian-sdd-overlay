// Package query implements longterm-mem's unified query fan-out and merge
// (R-006, D8): a caller-selected set of sources, vault matches first in
// vault order when the vault is requested, then Engram's own sources
// round-robin-interleaved in their own native order -- never re-ranked --
// with any linked pair or cross-arm duplicate collapsed into one row. A
// not-provisioned vault degrades to Engram-only results (R-026).
package query

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
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
// for tests; Engram is a real *engram.Store (temp DB in tests).
type Deps struct {
	Engram        *engram.Store
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
	Standing *engram.Standing `json:"standing,omitempty"`
	// MatchOffset is the byte offset engram.SnippetAt should centre a
	// re-render on, mirroring engram.Row.MatchOffset. It only matters for
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

	top := req.Top
	if top <= 0 {
		top = DefaultTopN
	}
	resolveLink := deps.ResolveLink
	if resolveLink == nil {
		resolveLink = NoLinkResolver
	}

	result := Result{Project: req.Project, Query: req.Query}

	var engramRows []engram.Row
	matchMode := engram.MatchAll
	if wantFTS {
		search, err := deps.Engram.Search(req.Project, req.Query, top, req.ExcludeTypes...)
		if err != nil {
			return Result{}, fmt.Errorf("query: search engram: %w", err)
		}
		engramRows = search.Rows
		matchMode = search.MatchMode
		if search.MatchMode == engram.MatchAny {
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
		if degraded, cause := deps.Engram.Degraded(); degraded {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{
				Code:   DiagnosticEngramDegradedSnapshot,
				Detail: fmt.Sprintf("engram is being read through the immutable=1 fallback, so these results come from the snapshot taken when the connection was opened, not the live database: %s", cause),
			})
		}
	}

	var embedRows []ResultRow
	if wantEmbed {
		rows, coverage, diags := runEmbeddingArm(ctx, deps.Engram, deps.StateDir, req.Project, req.Query, top, deps.Embed, deps.BuildIndex, resolveLoadIndex(deps))
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
	result.Diagnostics = append(result.Diagnostics, attachStandings(deps.Engram, result.Results)...)
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

// MinSnippetBudget is the smallest per-row snippet share allocateSnippetBudget
// will render before it stops shrinking and lets capResponse fall back to
// dropping rows.
//
// Below it a snippet cannot carry the sentence a match sits in -- the same
// reasoning engram.SnippetBudget (480, derived from 2 sources x 5 rows) is
// built from, at roughly the row count (about 20, at this ceiling) where
// the bound should again correctly fall on row count rather than on
// snippet length. It completes engram.SnippetBudget's derivation rather
// than contradicting it: at ordinary row counts share never gets near 120,
// and only a caller asking for many rows at once reaches it.
const MinSnippetBudget = 120

// snippetMarkerAllowance reserves bytes for the up-to-two truncation
// markers engram.SnippetAt may add around a cut, and for the small JSON
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
// start, the same honest head-slice engram.SnippetAt already falls back
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
	if share > engram.SnippetBudget {
		share = engram.SnippetBudget
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
	if share2 > engram.SnippetBudget {
		share2 = engram.SnippetBudget
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
// truncation markers engram.SnippetAt adds -- do not exceed budget.
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
	snippet, truncated := engram.SnippetAt(content, offset, window)
	row.Snippet = snippet
	if hasFullBody {
		row.SnippetTruncated = truncated
	} else {
		row.SnippetTruncated = false
	}
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

// mergeResults implements the amended R-006: vault rows precede Engram
// rows only when the vault is a requested source (matchLinkedObservation
// is the matcher; it consults both Engram-backed arms, see JD-4 there),
// and Engram's own requested sources are round-robin-interleaved by
// interleaveEngramSources, deduplicated by engram_id. When both engram-fts
// and engram-embed are requested, routeRank1 (R-058/R-059, Branch A: gate
// wired live, published routing accuracy 89%/86%) decides which of the two
// sources' own rows are offered first each round -- the only thing that
// decides is which row lands at rank 1; interleaveEngramSources itself
// never consults it, so the union guarantee is untouched by the decision.
func mergeResults(wantVault, wantFTS, wantEmbed bool, vaultRows []vault.Candidate, engramRows []engram.Row, embedRows []ResultRow, resolveLink func(string) (int64, bool), queryText, matchMode string) []ResultRow {
	consumed := make(map[int64]bool, len(engramRows)+len(embedRows))
	var merged []ResultRow

	if wantVault {
		for _, c := range vaultRows {
			if id, title, ok := matchLinkedObservation(c.PageAddress, engramRows, embedRows, resolveLink); ok && !consumed[id] {
				consumed[id] = true
				merged = append(merged, ResultRow{
					Sources: []string{SourceLinked}, PageAddress: c.PageAddress, PagePath: c.AbsolutePath,
					EngramID: id, Title: title, Snippet: c.Snippet,
					Score: &Score{BM25: c.BM25Score, Rerank: c.RerankScore},
				})
				continue
			}
			merged = append(merged, ResultRow{
				Sources: []string{SourceVault}, PageAddress: c.PageAddress, PagePath: c.AbsolutePath, Snippet: c.Snippet,
				Score: &Score{BM25: c.BM25Score, Rerank: c.RerankScore},
			})
		}
	}

	var engramSourceList []engramSourceRows

	if wantFTS {
		var ftsRows []ResultRow
		for _, er := range engramRows {
			if consumed[er.ID] {
				continue
			}
			ftsRows = append(ftsRows, ResultRow{
				EngramID: er.ID, Title: er.Title,
				Snippet: er.Snippet, SnippetTruncated: er.SnippetTruncated, FullLength: er.ContentLength,
				MatchOffset: er.MatchOffset, Content: er.Content,
			})
		}
		engramSourceList = append(engramSourceList, engramSourceRows{name: SourceEngramFTS, rows: ftsRows})
	}
	if wantEmbed {
		var embRows []ResultRow
		for _, er := range embedRows {
			if consumed[er.EngramID] {
				continue
			}
			embRows = append(embRows, er)
		}
		engramSourceList = append(engramSourceList, engramSourceRows{name: SourceEngramEmbed, rows: embRows})
	}

	if wantFTS && wantEmbed {
		// engramSourceList is [FTS, Embed] by construction above; swap
		// only when the gate names the embedding arm as rank 1's owner.
		tokens, _ := engram.SearchTokens(queryText)
		if routeRank1(tokens, matchMode) == SourceEngramEmbed {
			engramSourceList[0], engramSourceList[1] = engramSourceList[1], engramSourceList[0]
		}
	}
	merged = append(merged, interleaveEngramSources(engramSourceList)...)

	for i := range merged {
		merged[i].Rank = i + 1
	}
	return merged
}

// engramSourceRows is one requested Engram-backed source's own ranked
// rows, already in ResultRow shape (Sources not yet set).
type engramSourceRows struct {
	name string
	rows []ResultRow
}

// interleaveEngramSources round-robins across sources's own rows,
// preserving each source's native rank order as a subsequence of the
// result and deduplicating by EngramID -- the first occurrence is kept
// and every source that also produced it is added to its Sources (R-006's
// "a row found by both sources appears once, at its earliest rank").
func interleaveEngramSources(sources []engramSourceRows) []ResultRow {
	idx := make([]int, len(sources))
	seen := make(map[int64]int, len(sources))
	var merged []ResultRow
	for {
		progressed := false
		for s := range sources {
			if idx[s] >= len(sources[s].rows) {
				continue
			}
			progressed = true
			row := sources[s].rows[idx[s]]
			idx[s]++
			if pos, ok := seen[row.EngramID]; ok {
				merged[pos].Sources = appendSourceOnce(merged[pos].Sources, sources[s].name)
				continue
			}
			seen[row.EngramID] = len(merged)
			row.Sources = []string{sources[s].name}
			merged = append(merged, row)
		}
		if !progressed {
			break
		}
	}
	return merged
}

// appendSourceOnce appends name to list unless it is already there.
func appendSourceOnce(list []string, name string) []string {
	for _, s := range list {
		if s == name {
			return list
		}
	}
	return append(list, name)
}

// matchLinkedObservation resolves pageAddress's linked observation and
// finds it among the rows EITHER Engram-backed arm returned.
//
// It consults both arms because the linked-pair rule is about the
// observation, not about which retriever happened to surface it. Checking
// only the FTS rows -- which is what this did until JD-4 -- shipped the
// same observation twice whenever the embedding arm was the one that found
// it: once as a vault row and once as an engram_embed row. That failure
// got WORSE the better paraphrase retrieval worked, since the paraphrase
// queries the embedding arm exists to answer are exactly the ones FTS
// misses, so the duplication would have arrived with the feature.
//
// FTS is searched first so that when both arms hold the observation the
// consumed row is the FTS one, matching the pre-JD-4 behaviour exactly;
// the embed copy is then dropped by the consumed check downstream.
func matchLinkedObservation(pageAddress string, engramRows []engram.Row, embedRows []ResultRow, resolveLink func(string) (int64, bool)) (int64, string, bool) {
	if resolveLink == nil {
		return 0, "", false
	}
	id, ok := resolveLink(pageAddress)
	if !ok {
		return 0, "", false
	}
	for _, row := range engramRows {
		if row.ID == id {
			return row.ID, row.Title, true
		}
	}
	for _, row := range embedRows {
		if row.EngramID == id {
			return row.EngramID, row.Title, true
		}
	}
	return 0, "", false
}
