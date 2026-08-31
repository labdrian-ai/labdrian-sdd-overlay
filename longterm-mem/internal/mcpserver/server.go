// Package mcpserver implements longterm-mem's MCP stdio server (R-012,
// D3): the query and promote tools, both wired through Deps' function
// seams (matching query.Deps/promote.Deps's own convention elsewhere in
// this module) so tests never need a real Engram database or vault
// subprocess, and a real caller (cmd_mcp.go) wires those seams to the same
// construction helpers the CLI query/promote subcommands use -- so the
// CLI and MCP surfaces cannot drift from each other (task 8b.11).
package mcpserver

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/promote"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/query"
)

// Name and Version identify this server in its MCP handshake (D3).
const (
	Name    = "longterm-mem"
	Version = "0.1.0"
)

// Deps are New's dependencies. Query and Promote are function seams: a
// real caller (cmd_mcp.go) wires Query to run.RunQuery and Promote to
// run.RunPromote, the exact same construction+call functions the CLI
// query/promote subcommands use (task 8b.11); server_test.go wires fakes
// so no test here ever opens a real Engram database or invokes a vault
// subprocess.
type Deps struct {
	// Query resolves req's project's vault and runs query.Run against it
	// (R-012's "Query round-trips over stdio" scenario).
	Query func(ctx context.Context, req query.Request) (query.Result, error)
	// Promote resolves project's vault and promotes engramID by explicit
	// call, the same path the CLI promote subcommand uses (R-012, R-032).
	Promote func(ctx context.Context, project string, engramID int64) (promote.Result, error)
}

// QueryIn is the query tool's input (D3 contract: query{project,query,top?}).
type QueryIn struct {
	Project string `json:"project" jsonschema:"the project to search"`
	Query   string `json:"query" jsonschema:"the query text"`
	Top     int    `json:"top,omitempty" jsonschema:"results per source (default 5 when omitted or 0)"`
}

// QueryOut is the query tool's output: query.Result's own JSON shape,
// unchanged, so the MCP surface never re-renders D8's merge contract in a
// second shape.
type QueryOut = query.Result

// PromoteIn is the promote tool's input (D3 contract: promote{project,engram_id}).
type PromoteIn struct {
	Project  string `json:"project" jsonschema:"the project owning the observation"`
	EngramID int64  `json:"engram_id" jsonschema:"the Engram observation id to promote"`
}

// PromoteOut is the promote tool's output: the address and outcome of the
// page Writer.Promote wrote, updated, or skipped.
type PromoteOut struct {
	PageAddress string `json:"page_address,omitempty"`
	PagePath    string `json:"page_path,omitempty"`
	Action      string `json:"action"`
}

// New builds an MCP server exposing the query and promote tools (R-012),
// both wired to deps' function seams. New is called exactly once per
// longterm-mem session (cmd_mcp.go): the returned *mcp.Server is run
// against exactly one stdio transport and exits with that session,
// spawning nothing else itself (R-034) -- any subprocess a handler
// triggers happens inside deps.Query/deps.Promote's own bounded,
// awaited internal/vault.Runner calls, never here.
func New(deps Deps) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: Name, Version: Version}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "query",
		Description: "Search a project's Engram observations and vault pages, merged by source and never re-ranked (D8).",
	}, queryHandler(deps))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "promote",
		Description: "Explicitly promote one Engram observation to a vault page, regardless of its automatic eligibility (R-032).",
	}, promoteHandler(deps))

	return server
}

// queryHandler adapts Deps.Query to the query tool's typed handler shape.
func queryHandler(deps Deps) mcp.ToolHandlerFor[QueryIn, QueryOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in QueryIn) (*mcp.CallToolResult, QueryOut, error) {
		if deps.Query == nil {
			return nil, QueryOut{}, fmt.Errorf("mcpserver: query dependency is not configured")
		}
		result, err := deps.Query(ctx, query.Request{Project: in.Project, Query: in.Query, Top: in.Top})
		if err != nil {
			return nil, QueryOut{}, err
		}
		return nil, result, nil
	}
}

// promoteHandler adapts Deps.Promote to the promote tool's typed handler
// shape.
func promoteHandler(deps Deps) mcp.ToolHandlerFor[PromoteIn, PromoteOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in PromoteIn) (*mcp.CallToolResult, PromoteOut, error) {
		if deps.Promote == nil {
			return nil, PromoteOut{}, fmt.Errorf("mcpserver: promote dependency is not configured")
		}
		result, err := deps.Promote(ctx, in.Project, in.EngramID)
		if err != nil {
			return nil, PromoteOut{}, err
		}
		return nil, PromoteOut{
			PageAddress: result.Page.Address,
			PagePath:    result.Page.Path,
			// ActionKind.String() (promote/update.go, task 8b.11) is the
			// one source of truth for this rendering: cmd_promote.go's
			// CLI output calls the same method, so the two surfaces
			// cannot drift into two different names for one outcome.
			Action: result.Action.Kind.String(),
		}, nil
	}
}
