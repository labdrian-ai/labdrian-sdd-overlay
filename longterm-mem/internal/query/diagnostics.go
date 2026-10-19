package query

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
	// fit under ResponseTokenCeiling, so rows were dropped: one at a time, the
	// lowest-ranked of the source holding the most rows (dropLargestSourceRow),
	// not the tail of the merged list. It names how
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
	// DiagnosticEmbeddingIndexUnreadable reports that the project's embedding index is there and cannot be read
	// or trusted (a corrupted manifest, for example). The query answers without embedding rows, as it does for an
	// index nobody built, and this diagnostic carries the load error so the failure is not read as "no index".
	DiagnosticEmbeddingIndexUnreadable = "embedding_index_unreadable"
	// DiagnosticVaultPageTagsUnreadable names the vault pages a query under a goal left out because their
	// frontmatter tags could not be read: such a page may be another goal's, so it is not shown, and the
	// narrowing is said rather than left to look like a vault with nothing more to offer.
	DiagnosticVaultPageTagsUnreadable = "vault_page_tags_unreadable"
)
