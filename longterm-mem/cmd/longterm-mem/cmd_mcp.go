package main

import (
	"context"
	"errors"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/embed"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/mcpserver"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/promote"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/query"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vault"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vecindex"
)

// cmdMCP implements `longterm-mem mcp` (R-012, R-034): serve the query,
// get and promote tools over MCP stdio until the client closes stdin or the
// process receives SIGINT/SIGTERM, at which point Run returns and the
// process exits -- no persistent daemon survives the session. Each tool
// call resolves its own project's vault fresh (the call carries project,
// not the process), matching cmd_query.go/cmd_promote.go's own per-call
// resolution; the Engram connection is opened once and shared across the
// whole session, mirroring how a long-lived MCP server should not reopen
// a read-only database on every call.
func (c commands) cmdMCP(args []string) int {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	a := c.newApp("mcp")
	store, exit := a.memory()
	if exit != exitOK {
		return exit
	}
	defer store.Close()

	// The embedding client and the embedding index's load cache are each
	// constructed exactly once for the whole session (issue #286): a long-
	// lived MCP server otherwise pays a fresh loopback HTTP client and a
	// full manifest.json+vectors.blob disk read on every single query that
	// names or defaults to engram-embed, even when the same project is
	// queried repeatedly and nothing about its index has changed between
	// calls. embedClient.Embed and loadCache.Load are safe for concurrent
	// use (http.Client and LoadCache's own mutex, respectively), matching
	// how store is already shared across the whole session below.
	embedClient, err := c.newEmbedClient(embed.Config{Model: vecindex.DefaultModel})
	if err != nil {
		return a.refuse(err, exitInternal)
	}

	server := mcpserver.New(c.mcpDeps(store, embedClient, vecindex.NewLoadCache()))
	// context.Canceled here means SIGINT/SIGTERM asked for a graceful
	// shutdown (R-034), not a failure; only a genuine transport/session
	// error is reported and turns into a non-zero exit.
	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil && !errors.Is(err, context.Canceled) {
		return a.refuse(err, exitInternal)
	}
	return exitOK
}

// mcpDeps are the tools of the MCP session. Each resolves its own project's vault fresh on every call (a session
// can serve more than one project) and then calls the same construction and use cases the CLI subcommands do
// (queryDeps, promote.PromoteAndReindex), so the CLI and MCP surfaces cannot drift -- the query tool's Embed and
// index cache being the one deliberate exception (issue #286): this session's shared client and load cache, not
// runQuery's per-call construction.
func (c commands) mcpDeps(store *engram.Store, embedClient *embed.Client, loadCache *vecindex.LoadCache) mcpserver.Deps {
	return mcpserver.Deps{
		Query:   c.mcpQuery(store, embedClient, loadCache),
		Get:     mcpGet(store),
		Promote: c.mcpPromote(store),
	}
}

// mcpQuery is the query tool of the MCP session: the vault of the call's project, and the query use case over the
// session's store, embedding client and index cache. It only reads, so it leaves the vault registry as it found it.
func (c commands) mcpQuery(store *engram.Store, embedClient *embed.Client, loadCache *vecindex.LoadCache) func(context.Context, query.Request) (query.Result, error) {
	return func(ctx context.Context, req query.Request) (query.Result, error) {
		vaultRoot, err := machineVaults().ToRead(req.Project, "")
		if err != nil {
			return query.Result{}, err
		}
		qdeps := queryDeps(store, vaultRoot, queryWiring{
			newClient: c.newEmbedClient,
			embed:     embedClient.Embed,
			cache:     loadCache,
		})
		return query.Run(ctx, qdeps, req)
	}
}

// mcpGet is the get tool of the MCP session: it reads one observation whole, through the same read-only store the
// query tool searches. It takes no project: an id already names exactly one row, and asking for a project too would
// let a caller be told "not found" for an observation that exists, which is the one answer this tool must never
// give wrongly -- it is where a truncated snippet sends people.
func mcpGet(store *engram.Store) func(context.Context, int64) (mcpserver.GetOutcome, error) {
	return func(_ context.Context, engramID int64) (mcpserver.GetOutcome, error) {
		o, found, err := store.ObservationByID(engramID)
		if err != nil {
			return mcpserver.GetOutcome{}, err
		}
		return mcpserver.GetOutcome{Observation: o, Found: found}, nil
	}
}

// mcpPromote is the promote tool of the MCP session: it resolves the vault of the call's project and promotes the
// observation with the clock the commands were composed with (promote.PromoteAndReindex), which rebuilds the vault
// index when the promotion wrote a page, before the call returns, exactly as a sync does. The rebuild's failure is
// reported beside a SUCCESSFUL promotion rather than as the call's error -- the page is written and durable by
// then. It writes, so it seeds the vault registry of a machine that has none, as the commands that write do.
func (c commands) mcpPromote(store *engram.Store) func(ctx context.Context, project string, engramID int64) (mcpserver.PromoteOutcome, error) {
	return func(ctx context.Context, project string, engramID int64) (mcpserver.PromoteOutcome, error) {
		vaultRoot, err := machineVaults().ToWrite(project, "")
		if err != nil {
			return mcpserver.PromoteOutcome{}, err
		}
		writer, err := newPromoteWriter(vaultRoot, c.clock)
		if err != nil {
			return mcpserver.PromoteOutcome{}, err
		}
		outcome, err := promote.PromoteAndReindex(ctx, writer, store.ObservationByID, engramID, vault.IndexRebuilder{Root: vaultRoot})
		if err != nil {
			return mcpserver.PromoteOutcome{}, err
		}
		return mcpserver.PromoteOutcome{Result: outcome.Result, IndexRebuildErr: outcome.IndexRebuildErr}, nil
	}
}
