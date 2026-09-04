package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/mcpserver"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/query"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vault"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultreg"
)

// cmdMCP implements `longterm-mem mcp` (R-012, R-034): serve the query and
// promote tools over MCP stdio until the client closes stdin or the
// process receives SIGINT/SIGTERM, at which point Run returns and the
// process exits -- no persistent daemon survives the session. Each tool
// call resolves its own project's vault fresh (the call carries project,
// not the process), matching cmd_query.go/cmd_promote.go's own per-call
// resolution; the Engram connection is opened once and shared across the
// whole session, mirroring how a long-lived MCP server should not reopen
// a read-only database on every call.
func cmdMCP(args []string) int {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, err := engram.Open(os.Getenv(engramDBEnvVar))
	if err != nil {
		fmt.Fprintf(os.Stderr, "longterm-mem: mcp: %v\n", err)
		return exitEngramUnavailable
	}
	defer store.Close()

	// Both closures resolve their own project's vault fresh on every call
	// (a session can serve more than one project) and then call runQuery/
	// runPromote (rundeps.go, task 8b.11) -- the exact same
	// construction+call functions cmdQuery/cmdPromote use for the CLI
	// query/promote subcommands, so the CLI and MCP surfaces cannot drift.
	deps := mcpserver.Deps{
		Query: func(ctx context.Context, req query.Request) (query.Result, error) {
			vaultRoot, err := vaultreg.Resolve(defaultVaultsPath(), req.Project, "")
			if err != nil {
				return query.Result{}, err
			}
			return runQuery(ctx, store, vaultRoot, req)
		},
		// A promotion that wrote a page rebuilds the vault index before
		// the call returns, exactly as cmd_sync.go does: without it a page
		// promoted over MCP stayed invisible to MCP query until someone
		// ran `sync` out of band. The rebuild is keyed off the promotion's
		// outcome (reindexAfterPromote), and its failure is reported
		// beside a SUCCESSFUL promotion rather than as the call's error --
		// the page is written and durable by then.
		Promote: func(ctx context.Context, project string, engramID int64) (mcpserver.PromoteOutcome, error) {
			vaultRoot, err := vaultreg.Resolve(defaultVaultsPath(), project, "")
			if err != nil {
				return mcpserver.PromoteOutcome{}, err
			}
			result, err := runPromote(store, vaultRoot, engramID)
			if err != nil {
				return mcpserver.PromoteOutcome{}, err
			}
			runner := &vault.Runner{Root: vaultRoot}
			rebuildErr := reindexAfterPromote(ctx, result, func(ctx context.Context) error {
				return vault.Rebuild(ctx, runner, false)
			})
			return mcpserver.PromoteOutcome{Result: result, IndexRebuildErr: rebuildErr}, nil
		},
	}

	server := mcpserver.New(deps)
	// context.Canceled here means SIGINT/SIGTERM asked for a graceful
	// shutdown (R-034), not a failure; only a genuine transport/session
	// error is reported and turns into a non-zero exit.
	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintf(os.Stderr, "longterm-mem: mcp: %v\n", err)
		return exitInternal
	}
	return exitOK
}
