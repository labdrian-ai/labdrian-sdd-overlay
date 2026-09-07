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
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vault"
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
	// requested source (R-060). Naming it is refused until the embedding
	// pipeline this change also adds (internal/embed, internal/vecindex,
	// the embedding arm itself) ships in a later PR of this same change --
	// answering a request this code cannot back up would be exactly the
	// silent narrowing R-060 exists to prevent, worn as a different
	// costume.
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

// defaultSources is what Run queries when Request.Sources is empty.
//
// It is engram-fts only, not "engram-fts and engram-embed" as R-060's own
// final shape reads, because the embedding source does not exist to query
// yet in this PR -- its client, its index, and the arm that reads it are
// later slices of this same change. Shipping a default that silently asks
// for a source it cannot honour would be worse than an honest smaller
// default.
//
// The user-visible consequence is worth stating loudly here too, not only
// in the change's own notes: the vault, which every call used to query
// unconditionally, is no longer queried by default. A caller that wants it
// back names it: sources: ["vault", "engram-fts"].
var defaultSources = []string{SourceEngramFTS}

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
	// defaultSources: engram-fts only, in this PR (see defaultSources for
	// why the vault is no longer queried unconditionally). An unknown name
	// is rejected with ErrUnknownSource rather than silently skipped.
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
}

// Run fans a query out to every requested source, then merges them
// (R-006, R-026, R-060).
func Run(ctx context.Context, deps Deps, req Request) (Result, error) {
	if req.Project == "" {
		return Result{}, ErrMissingProject
	}
	sources := req.Sources
	if len(sources) == 0 {
		sources = defaultSources
	}
	for _, s := range sources {
		if !knownSources[s] {
			return Result{}, fmt.Errorf("%w: %q", ErrUnknownSource, s)
		}
		if s == SourceEngramEmbed {
			return Result{}, fmt.Errorf("query: source %q is not available yet: the embedding index and its query arm ship in a later PR of union-retrieval", s)
		}
	}
	wantVault := containsSource(sources, SourceVault)
	wantFTS := containsSource(sources, SourceEngramFTS)

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
	if wantFTS {
		search, err := deps.Engram.Search(req.Project, req.Query, top, req.ExcludeTypes...)
		if err != nil {
			return Result{}, fmt.Errorf("query: search engram: %w", err)
		}
		engramRows = search.Rows
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

	result.Results = mergeResults(sources, vaultRows, engramRows, resolveLink)
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
func dropLargestSourceRow(result *Result) {
	counts := make(map[string]int)
	for _, row := range result.Results {
		for _, s := range row.Sources {
			counts[s]++
		}
	}
	var largest string
	for name, n := range counts {
		if n > counts[largest] {
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
func renderRowSnippet(row *ResultRow, budget int) {
	content, offset := row.Content, row.MatchOffset
	if content == "" {
		content, offset = row.Snippet, 0
	}
	window := budget - snippetMarkerAllowance
	if window < 1 {
		window = 1
	}
	snippet, truncated := engram.SnippetAt(content, offset, window)
	row.Snippet, row.SnippetTruncated = snippet, truncated
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
// rows only when the vault is a requested source (3b.8: MatchLinkedEngramRow
// is the extracted matcher, reused unchanged by promote/MCP query later),
// and Engram's own requested sources are round-robin-interleaved by
// interleaveEngramSources, deduplicated by engram_id.
func mergeResults(sources []string, vaultRows []vault.Candidate, engramRows []engram.Row, resolveLink func(string) (int64, bool)) []ResultRow {
	consumed := make(map[int64]bool, len(engramRows))
	var merged []ResultRow

	if containsSource(sources, SourceVault) {
		for _, c := range vaultRows {
			if er, ok := MatchLinkedEngramRow(c.PageAddress, engramRows, resolveLink); ok && !consumed[er.ID] {
				consumed[er.ID] = true
				merged = append(merged, ResultRow{
					Sources: []string{SourceLinked}, PageAddress: c.PageAddress, PagePath: c.AbsolutePath,
					EngramID: er.ID, Title: er.Title, Snippet: c.Snippet,
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

	if containsSource(sources, SourceEngramFTS) {
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
		// PR-1 ever supplies one engram source (engram-fts); Phase 4 adds
		// engram-embed to this slice, and interleaveEngramSources is
		// written for that already so wiring it in is not a second
		// rewrite of this function.
		merged = append(merged, interleaveEngramSources([]engramSourceRows{{name: SourceEngramFTS, rows: ftsRows}})...)
	}

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
