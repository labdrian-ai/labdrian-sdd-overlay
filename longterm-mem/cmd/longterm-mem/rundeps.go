package main

import (
	"context"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/embed"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/ops"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/promote"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/query"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vault"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vecindex"
)

// runQuery builds query.Deps for vaultRoot/store and calls query.Run
// (task 8b.11): the one construction+call path cmdQuery (the CLI query
// subcommand) uses. It deliberately constructs a fresh Embed closure and
// holds no index cache on every call, because the CLI process serves
// exactly one query and exits -- there is no session for a shared client
// or a load cache to help with (issue #286's session-scoped caching is a
// benefit specific to a long-lived process; cmd_mcp.go's cmdMCP builds its
// own deps per call via queryDeps directly, passing a client and load cache
// it constructs once for the whole MCP session, rather than calling
// runQuery).
func runQuery(ctx context.Context, store *engram.Store, vaultRoot string, newClient embedClientFactory, req query.Request) (query.Result, error) {
	deps := queryDeps(store, vaultRoot, queryWiring{newClient: newClient, embed: freshEmbedFunc(newClient)})
	return query.Run(ctx, deps, req)
}

// queryWiring is what a surface decides about the embedding side of a query, each part under its own name
// because the parts answer different steps and a positional list of them is easy to mis-order.
//
// The two clients are not redundant: embed answers the query's own embedding (the embedding arm, one call
// per query), and newClient builds the client of the bounded top-up (the index adapter's build), which asks
// for the model the existing index was built under and so cannot be the one client a session holds for its
// default model.
type queryWiring struct {
	// newClient builds the embedding client of the top-up build.
	newClient embedClientFactory
	// embed embeds the query text.
	embed query.EmbedFunc
	// cache is what the embedding index is read through, and forgets an index a top-up wrote; nil when nothing
	// is cached (the CLI), and the MCP session's own for the whole session.
	cache *vecindex.LoadCache
}

// freshEmbedFunc builds a new embed.Client on every call it makes -- the
// CLI's own one-query-per-process convention, unchanged by this issue.
func freshEmbedFunc(newClient embedClientFactory) query.EmbedFunc {
	return func(ctx context.Context, text string) ([]float32, error) {
		client, err := newClient(embed.Config{Model: vecindex.DefaultModel})
		if err != nil {
			return nil, err
		}
		return client.Embed(ctx, text)
	}
}

// queryDeps builds query.Deps for vaultRoot/store: the one construction
// cmdQuery (via runQuery) and cmd_mcp.go's MCP query tool wiring both use,
// so neither surface can drift from the other -- extracted out of
// cmdQuery's own inline construction, which cmd_mcp.go originally
// duplicated verbatim.
//
// The embedding index and Embed are always set, not only when a caller
// names engram-embed: query.Run only reads the index/invokes Embed when that
// source is actually requested (R-071), so wiring them here unconditionally
// costs nothing on the default engram-fts-only path and lets a caller who
// does name the source (over the MCP tool's own `sources` field) reach it
// without a second construction path to keep in sync.
//
// wiring is supplied by the caller rather than constructed here, because
// the CLI (one query per process) and the MCP server (many queries per
// session) want different lifetimes for its parts: runQuery builds a fresh
// embed.Client per embedding and holds no cache on every call, while
// cmdMCP builds one embed.Client and one *vecindex.LoadCache for the whole
// session and passes them on every call. wiring.newClient is the factory
// the commands were wired with, so that no query reaches an embedding
// backend its caller did not choose. The cache is nil for the CLI (nothing
// to invalidate) and the session's for the MCP server, so a bounded top-up
// that writes a fresh index is immediately visible to the very query that
// triggered it, rather than served stale from the cache until an unrelated
// mtime/size change happens to be noticed.
func queryDeps(store *engram.Store, vaultRoot string, wiring queryWiring) query.Deps {
	index := embeddingIndexOf(store, wiring)
	return query.Deps{
		Memory:       store,
		Vault:        vault.Retriever{Root: vaultRoot},
		ResolveLink:  query.NoLinkResolver,
		Embed:        wiring.embed,
		Index:        index,
		IndexBuilder: index,
	}
}

// embeddingIndexOf is the embedding index adapter of this process, wired to what it reads and writes with: the
// state directory the module keeps its derived state in, the live rows of a project as the Engram store has
// them (the same source cmd_index_embeddings.go's own -embeddings flag reads, store.ListObservations, already
// R-020-scoped), the embedding client of the model a build asks for -- exactly the model the embedding arm read
// off the existing index, never this process's own default, which could silently disagree with whatever
// contract the index was actually built under (issue #286's bounded top-up) -- and the wall clock a build stamps
// its index with.
func embeddingIndexOf(store *engram.Store, wiring queryWiring) *vecindex.Repository {
	return &vecindex.Repository{
		StateDir: defaultStateDir(),
		Cache:    wiring.cache,
		LiveRows: func(project string) ([]vecindex.Row, error) { return observationRowsForIndex(store, project) },
		NewEmbedder: func(model string) (vecindex.Embedder, error) {
			client, err := wiring.newClient(embed.Config{Model: model})
			if err != nil {
				return nil, err
			}
			return client, nil
		},
		Now: buildNowRFC3339,
	}
}

// embeddingIndexReader is the embedding index as `status` and `doctor` read it (ops.EmbeddingIndexReader): the
// repository over the state directory the module keeps its derived state in, with nothing it builds with and no
// cache, because the process reads an index to report on it and exits. The caller resolves the state directory
// (defaultStateDir) and hands it in: `status` refuses an empty one before it gets here, and `doctor` passes it on.
// When nothing names the state directory there is nowhere to look, and the reader says so (noStateDirectory)
// instead of looking in the working directory, which it did until the owner's decision of 2026-10-12.
func embeddingIndexReader(stateDir string) ops.EmbeddingIndexReader {
	if stateDir == "" {
		return noStateDirectory{}
	}
	return &vecindex.Repository{StateDir: stateDir}
}

// stateDirUnresolved is the error of a process that cannot name the directory it keeps its derived state in: no
// HOME and no LONGTERM_MEM_STATE_DIR. It says what it is in the terms of the diagnostics (ops.StateDirUnresolved).
type stateDirUnresolved struct{}

func (stateDirUnresolved) Error() string {
	return "could not resolve the state directory; " + ops.StateDirHint
}

func (stateDirUnresolved) StateDirUnresolved() {}

// noStateDirectory is the ops.EmbeddingIndexReader of a process with no state directory: it holds no index, and
// every question about one is answered with stateDirUnresolved.
type noStateDirectory struct{}

func (noStateDirectory) EmbeddingIndexLocation(string) string { return "" }

func (noStateDirectory) EmbeddingIndexBuiltAt(string) (string, error) {
	return "", stateDirUnresolved{}
}

func (noStateDirectory) IndexedEngramIDs(string) ([]int64, error) { return nil, stateDirUnresolved{} }

// observationRowsForIndex reads project's live Engram rows in the shape
// vecindex.Build wants, the same source cmd_index_embeddings.go's own
// -embeddings flag reads (store.ListObservations, already R-020 soft-delete
// scoped) -- extracted so this mapping can be tested without a network
// round trip to an embedding backend.
func observationRowsForIndex(store *engram.Store, project string) ([]vecindex.Row, error) {
	observations, err := store.ListObservations(project)
	if err != nil {
		return nil, err
	}
	rows := make([]vecindex.Row, 0, len(observations))
	for _, o := range observations {
		rows = append(rows, vecindex.Row{EngramID: o.ID, Title: o.Title, Content: o.Content})
	}
	return rows, nil
}

// newPromoteWriter is the Writer of the vault at vaultRoot, as every command
// that promotes (promote, the MCP promote tool, sync) uses it: the vault's
// address allocator, the clock it is given, and the vault file system adapter
// its precedence store is loaded from and saved through, its address map is
// recorded in, its pages are read and written through, and its catalog, log
// and sync state are registered in and recorded to. It is the one
// construction, so two commands cannot come to wire a Writer differently; a
// precedence sidecar that cannot be loaded is the error, as the repository
// reported it.
func newPromoteWriter(vaultRoot string, clock promote.Clock) (*promote.Writer, error) {
	vaultFiles := openVault(vaultRoot)
	writer := &promote.Writer{
		Pages:      vaultFiles,
		Index:      vaultFiles,
		Log:        vaultFiles,
		SyncState:  vaultFiles,
		Addresses:  vault.AddressAllocator{Root: vaultRoot},
		AddressMap: vaultFiles,
		Clock:      clock,
	}
	if err := writer.UsePrecedence(vaultFiles); err != nil {
		return nil, err
	}
	return writer, nil
}

// runPromote builds a Writer for vaultRoot and calls
// promote.ExplicitPromote against store (task 8b.11): the construction+call
// path of cmdPromote, the CLI promote subcommand. The MCP promote tool
// (cmd_mcp.go) builds its Writer the same way, with newPromoteWriter, and
// promotes through promote.PromoteAndReindex, which is this call and the
// rebuild of the index that follows a page being written.
func runPromote(ctx context.Context, store *engram.Store, vaultRoot string, clock promote.Clock, engramID int64) (promote.Result, error) {
	writer, err := newPromoteWriter(vaultRoot, clock)
	if err != nil {
		return promote.Result{}, err
	}
	return promote.ExplicitPromote(ctx, writer, store.ObservationByID, engramID)
}
