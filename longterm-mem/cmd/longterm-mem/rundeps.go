package main

import (
	"context"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/embed"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/promote"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/query"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vault"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vecindex"
)

// runQuery builds query.Deps for vaultRoot/store and calls query.Run
// (task 8b.11): the one construction+call path cmdQuery (the CLI query
// subcommand) and cmd_mcp.go's MCP query tool wiring both use, so neither
// surface can drift from the other -- extracted out of cmdQuery's own
// inline construction, which cmd_mcp.go originally duplicated verbatim.
//
// StateDir and Embed are always set, not only when a caller names
// engram-embed: query.Run only reads StateDir/invokes Embed when that
// source is actually requested (R-071), so wiring them here unconditionally
// costs nothing on the default engram-fts-only path and lets a caller who
// does name the source (over the MCP tool's own `sources` field) reach it
// without a second construction path to keep in sync.
func runQuery(ctx context.Context, store *engram.Store, vaultRoot string, req query.Request) (query.Result, error) {
	runner := &vault.Runner{Root: vaultRoot}
	deps := query.Deps{
		Engram: store,
		RetrieveVault: func(ctx context.Context, project, q string, n int) (vault.Result, error) {
			return vault.Retrieve(ctx, runner, project, q, n)
		},
		ResolveLink: query.NoLinkResolver,
		StateDir:    defaultStateDir(),
		Embed: func(ctx context.Context, text string) ([]float32, error) {
			client, err := embed.NewClient(embed.Config{Model: vecindex.DefaultModel})
			if err != nil {
				return nil, err
			}
			return client.Embed(ctx, text)
		},
		BuildIndex: buildIndexForQuery(store),
	}
	return query.Run(ctx, deps, req)
}

// buildIndexForQuery wires query.Deps.BuildIndex for a query.Run bounded
// top-up (issue #286): read project's live rows the same way
// cmd_index_embeddings.go's own -embeddings flag does
// (store.ListObservations, already R-020-scoped), then run one incremental
// vecindex.Build under exactly the model/dimension/inputLimit contract the
// embedding arm read off the existing manifest -- never this process's own
// defaults, which could silently disagree with whatever contract the index
// was actually built under.
func buildIndexForQuery(store *engram.Store) func(ctx context.Context, project, model string, dimension, inputLimit int) error {
	return func(ctx context.Context, project, model string, dimension, inputLimit int) error {
		rows, err := observationRowsForIndex(store, project)
		if err != nil {
			return err
		}
		client, err := embed.NewClient(embed.Config{Model: model})
		if err != nil {
			return err
		}
		dir := vecindex.Dir(defaultStateDir(), project)
		_, _, err = vecindex.Build(ctx, dir, model, dimension, inputLimit, rows, client, buildNowRFC3339)
		return err
	}
}

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

// runPromote builds a Writer for vaultRoot and calls
// promote.ExplicitPromote against store (task 8b.11): the one
// construction+call path cmdPromote (the CLI promote subcommand) and
// cmd_mcp.go's MCP promote tool wiring both use, mirroring runQuery's own
// extraction so R-012 and R-032 genuinely share one code path rather than
// two callers separately reconstructing the same Writer.
func runPromote(store *engram.Store, vaultRoot string, engramID int64) (promote.Result, error) {
	precedence, err := promote.LoadPrecedenceStore(vaultRoot)
	if err != nil {
		return promote.Result{}, err
	}
	writer := &promote.Writer{VaultRoot: vaultRoot, Store: precedence}
	return promote.ExplicitPromote(writer, store.ObservationByID, engramID)
}

// reindexAfterPromote rebuilds the vault index after a promotion, but only
// when that promotion actually WROTE a page.
//
// A page promoted over MCP used to stay invisible to MCP query until
// someone ran `sync` out of band, because cmd_sync.go rebuilds the index
// and the promote tool did not. Rebuilding here closes that, consistent
// with sync.
//
// The rebuild is keyed off the promotion's OUTCOME, never off "promote was
// called". A rebuild walks the whole vault and shells out to the vault's
// own tooling; paying that for a promotion that wrote nothing is the entire
// cost for none of the benefit, on a surface a client can call in a loop.
// Two outcomes write nothing and both reach here as ordinary, non-error
// results: a refusal under local-edit precedence (ActionSkippedLocalEdit),
// and an ineligible observation, which Writer.Promote reports as a true
// no-op. "No error was returned" therefore does not mean a page was
// written, and cannot be the test.
//
// The returned error is the rebuild's own. It is deliberately NOT reported
// as a promotion failure by any caller: the page is already written and
// durable by the time the rebuild runs, so failing the call would tell the
// caller that a page it can see on disk was never written. It is surfaced
// separately instead (mcpserver.PromoteOutcome.IndexRebuildErr).
func reindexAfterPromote(ctx context.Context, result promote.Result, rebuild func(context.Context) error) error {
	if !promotionWrotePage(result) {
		return nil
	}
	return rebuild(ctx)
}

// promotionWrotePage reports whether result describes a promotion that put
// bytes on disk.
//
// The Page.Address guard is not redundant with the switch, and removing it
// is the trap this function exists to close: Writer.Promote reports an
// INELIGIBLE observation as a zero promote.Result, and a zero
// promote.ActionKind is promote.ActionCreated -- the first iota. A bare
// switch on the kind therefore answers "created" for a promotion that
// touched no file at all. A real write always carries the address of the
// page it wrote.
func promotionWrotePage(result promote.Result) bool {
	if result.Page.Address == "" {
		return false
	}
	switch result.Action.Kind {
	case promote.ActionCreated, promote.ActionUpdated:
		return true
	default:
		return false
	}
}
