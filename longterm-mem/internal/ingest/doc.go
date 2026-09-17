// Package ingest turns already-fetched text and already-local files into
// records ready to hand to Engram's mem_save unchanged (R-005..R-007).
//
// It never fetches anything. It imports neither net nor net/http (R-071)
// nor os/exec (R-021); the module-wide static guards in
// net_allowlist_test.go and exec_allowlist_test.go cover this package
// without amendment. The record shape it emits is defined normatively in
// skills/_shared/ingested-observation-contract.md, which this package
// implements rather than re-specifies.
package ingest

const (
	// DefaultMaxChunkBytes is query.ResponseByteCeiling / query.DefaultTopN
	// (8000 / 5 = 1600): one chunk is worth at most one row's share of one
	// whole query response, and it sits inside vecindex.DefaultInputLimit
	// (2000) so no part of a chunk's body is invisible to the embedding
	// arm — only the provenance trailer falls outside that window, by
	// design (see design.md's "trailer goes after the body" decision).
	DefaultMaxChunkBytes = 1600
	// DefaultMinChunkBytes is engram.SnippetBudget: the floor at which a
	// chunk is still at most one rendered snippet, four times
	// query.MinSnippetBudget (120).
	DefaultMinChunkBytes = 480
	// DefaultMaxSourceBytes bounds one source document (4 MiB).
	DefaultMaxSourceBytes = 4 << 20
	// MaxChunks bounds one source's chunk count so ordinals stay four
	// digits (c0001..c9999); a source that would exceed it is refused
	// rather than silently renumbered.
	MaxChunks = 9999
)
