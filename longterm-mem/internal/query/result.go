package query

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
	// the embedding index (EmbeddingIndexRepository), populated by `index --embeddings`.
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
	Standing *Standing `json:"standing,omitempty"`
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
