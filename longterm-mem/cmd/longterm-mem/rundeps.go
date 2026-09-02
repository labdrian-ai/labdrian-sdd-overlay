package main

import (
	"context"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/promote"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/query"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vault"
)

// runQuery builds query.Deps for vaultRoot/store and calls query.Run
// (task 8b.11): the one construction+call path cmdQuery (the CLI query
// subcommand) and cmd_mcp.go's MCP query tool wiring both use, so neither
// surface can drift from the other -- extracted out of cmdQuery's own
// inline construction, which cmd_mcp.go originally duplicated verbatim.
func runQuery(ctx context.Context, store *engram.Store, vaultRoot string, req query.Request) (query.Result, error) {
	runner := &vault.Runner{Root: vaultRoot}
	deps := query.Deps{
		Engram: store,
		RetrieveVault: func(ctx context.Context, project, q string, n int) (vault.Result, error) {
			return vault.Retrieve(ctx, runner, project, q, n)
		},
		ResolveLink: query.NoLinkResolver,
	}
	return query.Run(ctx, deps, req)
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
