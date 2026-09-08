// Package mcpserver implements longterm-mem's MCP stdio server (R-012,
// D3): the query, get and promote tools, each wired through Deps' function
// seams (matching query.Deps/promote.Deps's own convention elsewhere in
// this module) so tests never need a real Engram database or vault
// subprocess, and a real caller (cmd_mcp.go) wires those seams to the same
// construction helpers the CLI query/promote subcommands use -- so the
// CLI and MCP surfaces cannot drift from each other (task 8b.11).
package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
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
	// call, the same path the CLI promote subcommand uses (R-012, R-032),
	// and -- when that promotion actually wrote a page -- rebuilds the
	// vault index so the page is queryable over this same session
	// (cmd_mcp.go wires that; see PromoteOutcome).
	Promote func(ctx context.Context, project string, engramID int64) (PromoteOutcome, error)
	// Get reads one observation whole, by id (cmd_mcp.go wires it to
	// engram.Store.ObservationByID, which is read-only like every other
	// path in that package -- R-002).
	Get func(ctx context.Context, engramID int64) (GetOutcome, error)
}

// GetOutcome is what Deps.Get found: the observation, and whether there
// was one. The two are separate because "no such observation" is an
// answer, not a failure -- an id that names nothing is a normal thing for
// a caller holding a stale result to ask about, and returning it as an
// error would make a routine miss look like a broken server.
type GetOutcome struct {
	Observation engram.Observation
	Found       bool
}

// PromoteOutcome is what Deps.Promote reports back: the promotion itself,
// plus whether the index rebuild that follows a written page succeeded.
//
// The two are separate on purpose. By the time the rebuild runs the page
// is already written and durable, so a rebuild failure is NOT a promotion
// failure -- returning it as the call's error would tell a caller that a
// page it can see on disk was never written. Nor may it be swallowed: the
// page exists but query cannot find it until the index is rebuilt, and a
// caller told nothing would read an empty query result as "the promotion
// did not happen". It is therefore carried beside a successful Result and
// rendered as PromoteOut's own index_stale fields.
type PromoteOutcome struct {
	// Result is what the promotion did.
	Result promote.Result
	// IndexRebuildErr is the vault index rebuild's failure, or nil --
	// which also covers "no rebuild was attempted", since a promotion
	// that wrote nothing leaves the index correct as it stands.
	IndexRebuildErr error
}

// Project stays an explicit, required field on both tool inputs below, and
// is deliberately NOT defaulted from, or validated against, the server's
// working directory.
//
// The CLI does resolve a missing --project from the working directory
// (internal/projectid, wired in cmd/longterm-mem/project_resolve.go),
// because there the working directory IS the operator standing in a
// project. The MCP server is different: it is launched by a runtime -- an
// editor, an agent host -- whose working directory has no relationship to
// the project any given call is about. Defaulting from it would silently
// bind observations to whatever directory the host happened to start in,
// which is exactly the misattribution longterm-mem's project identity work
// exists to prevent; validating against it would warn on every correct
// call. So do not add a cwd default or a cwd correspondence check here:
// the caller names the project, and that is the only trustworthy source.

// QueryIn is the query tool's input (D3 contract: query{project,query,top?}).
type QueryIn struct {
	Project string `json:"project" jsonschema:"the project to search"`
	Query   string `json:"query" jsonschema:"the query text"`
	Top     int    `json:"top,omitempty" jsonschema:"results per source (default 5 when omitted or 0)"`
	// ExcludeTypes is opt-in and defaults to excluding nothing; see
	// query.Request.ExcludeTypes for why no type is filtered by default.
	ExcludeTypes []string `json:"exclude_types,omitempty" jsonschema:"Engram observation types to leave out, e.g. session_summary (default: none excluded)"`
	// Sources mirrors query.Request.Sources: which sources to query
	// (R-060). Empty means the union of engram-fts and engram-embed once
	// the project's embedding index exists, engram-fts alone otherwise --
	// the vault is never defaulted in, only queried when named here
	// explicitly.
	Sources []string `json:"sources,omitempty" jsonschema:"sources to query: engram-fts, engram-embed, vault (default: engram-fts plus engram-embed once the project's embedding index exists, else engram-fts only; vault is not queried unless named)"`
}

// QueryOut is the query tool's output: query.Result's own JSON shape,
// unchanged, so the MCP surface never re-renders D8's merge contract in a
// second shape.
type QueryOut = query.Result

// GetIn is the get tool's input.
type GetIn struct {
	EngramID int64 `json:"engram_id" jsonschema:"the Engram observation id to read whole"`
}

// GetOut is the get tool's output: one observation, untruncated.
//
// This tool is what makes the query tool's truncation honest. query
// returns an extract of each matched body and says so -- a "…" at each
// cut edge, plus snippet_truncated and full_length. A caller reading that
// has been told two things: that it is holding a fragment, and how much
// it is missing. Neither is worth anything without somewhere to go for
// the rest, and before this tool existed there was nowhere: the only way
// to see a whole observation over MCP was to make query ship every body
// in full, which is the cost this change removed. A cap with no way past
// it is not a cap, it is data loss.
type GetOut struct {
	// Found is false when no observation carries that id. Content is then
	// empty for a reason Detail names, rather than looking like an
	// observation that happens to say nothing.
	Found    bool   `json:"found"`
	EngramID int64  `json:"engram_id,omitempty"`
	Title    string `json:"title,omitempty"`
	// Content is the whole body, never truncated. That is the entire
	// point of the tool.
	Content   string `json:"content,omitempty"`
	Project   string `json:"project,omitempty"`
	Type      string `json:"type,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
	// DeletedAt is non-empty for a soft-deleted observation. It is
	// returned rather than hidden: a caller following a link out of an
	// older result needs to know the memory was retired, and an empty
	// result would say only that it is gone.
	DeletedAt string `json:"deleted_at,omitempty"`
	// Detail explains a false Found.
	Detail string `json:"detail,omitempty"`
}

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
	// IndexStale reports that the page was written and is durable but the
	// vault index rebuild that follows it failed, so query cannot find the
	// page yet. It is a condition of a SUCCESSFUL promotion, never an
	// error: the caller must not read it as "the page was not written".
	IndexStale bool `json:"index_stale,omitempty"`
	// IndexStaleDetail names what failed and the remedy.
	IndexStaleDetail string `json:"index_stale_detail,omitempty"`
}

// New builds an MCP server exposing the query, get and promote tools
// (R-012), each wired to deps' function seams. New is called exactly once per
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
		Name:        "get",
		Description: "Read one Engram observation whole by id, untruncated -- the full text behind a query result whose snippet was cut.",
	}, getHandler(deps))

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
		result, err := deps.Query(ctx, query.Request{Project: in.Project, Query: in.Query, Top: in.Top, ExcludeTypes: in.ExcludeTypes, Sources: in.Sources})
		if err != nil {
			return nil, QueryOut{}, err
		}
		return textResult(renderQuery(result)), result, nil
	}
}

// textResult carries a handler's own human-readable content block.
//
// Supplying one is the entire mechanism for not sending a result twice.
// The go-sdk fills an absent Content field with a byte-identical JSON
// serialization of the structured result (server.go: `if res.Content ==
// nil`), which measured on the wire as a 182,813 byte response carrying
// 89,741 bytes of text beside 90,689 bytes of structuredContent, the
// first parsing to exactly the second. Returning nil and hoping is what
// produced that.
//
// The duplication is not suppressed, it is replaced, and the difference
// matters. That fallback exists so a pre-SEP-2106 client -- one that
// cannot read structuredContent at all -- can still recover the payload
// from the text block, so deleting it outright would leave those clients
// with an empty response. What they get instead is a compact rendering of
// the same result: readable by a person, and shorter. The trade is
// explicit and worth stating plainly -- such a client can no longer
// re-parse the text block back into the structured shape, only read it.
// For a result whose consumer is an agent or a human, prose is the better
// half of that trade; a client that needs the structure has
// structuredContent, which is where the structure belongs.
func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}

// renderQuery writes a query result as the lines a person reads.
//
// It names every truncation twice over, matching query.ResultRow's own
// contract: the snippet already carries a "…" at each cut edge, and this
// adds the full length and the call that fetches it, so a reader of the
// text block is never left holding a fragment they think is whole.
func renderQuery(result QueryOut) string {
	var b strings.Builder
	fmt.Fprintf(&b, "query %q in %s (vault_status=%s, %d results)\n", result.Query, result.Project, result.VaultStatus, len(result.Results))
	for _, row := range result.Results {
		label := row.PageAddress
		if label == "" {
			label = fmt.Sprintf("engram:%d", row.EngramID)
		}
		fmt.Fprintf(&b, "\n[%d] %s %s %s\n", row.Rank, strings.Join(row.Sources, "+"), label, row.Title)
		if row.Snippet != "" {
			fmt.Fprintf(&b, "    %s\n", row.Snippet)
		}
		if row.SnippetTruncated {
			fmt.Fprintf(&b, "    (extract of %d bytes; call get{engram_id: %d} for the whole text)\n", row.FullLength, row.EngramID)
		}
		if row.Standing != nil {
			// A memory that was explicitly replaced must not read as
			// current in the half of the response a person actually
			// reads. cmd_query.go says the same thing for the CLI.
			for _, n := range row.Standing.SupersededBy {
				fmt.Fprintf(&b, "    SUPERSEDED BY engram:%d %s — do not treat as current\n", n.ID, n.Title)
			}
			for _, n := range row.Standing.ConflictsWith {
				fmt.Fprintf(&b, "    CONFLICTS WITH engram:%d %s\n", n.ID, n.Title)
			}
			for _, n := range row.Standing.Unjudged {
				fmt.Fprintf(&b, "    UNDECIDED against engram:%d %s — nobody judged this conflict\n", n.ID, n.Title)
			}
		}
	}
	for _, d := range result.Diagnostics {
		fmt.Fprintf(&b, "\nWARN %s: %s\n", d.Code, d.Detail)
	}
	return b.String()
}

// renderObservation writes one whole observation as text. get exists to
// deliver a body, so the body IS the rendering; a JSON envelope around it
// would double the one payload this tool is for.
func renderObservation(o engram.Observation) string {
	var b strings.Builder
	fmt.Fprintf(&b, "engram:%d %s", o.ID, o.Title)
	if o.Type != "" {
		fmt.Fprintf(&b, " (%s)", o.Type)
	}
	if o.DeletedAt != "" {
		fmt.Fprintf(&b, " — RETIRED %s", o.DeletedAt)
	}
	b.WriteString("\n\n")
	b.WriteString(o.Content)
	return b.String()
}

// getHandler adapts Deps.Get to the get tool's typed handler shape.
func getHandler(deps Deps) mcp.ToolHandlerFor[GetIn, GetOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in GetIn) (*mcp.CallToolResult, GetOut, error) {
		if deps.Get == nil {
			return nil, GetOut{}, fmt.Errorf("mcpserver: get dependency is not configured")
		}
		outcome, err := deps.Get(ctx, in.EngramID)
		if err != nil {
			return nil, GetOut{}, err
		}
		if !outcome.Found {
			detail := fmt.Sprintf("no observation with id %d exists in Engram; it may have been hard-deleted, or the id may come from another database", in.EngramID)
			return textResult(detail), GetOut{Found: false, Detail: detail}, nil
		}
		o := outcome.Observation
		return textResult(renderObservation(o)), GetOut{
			Found: true, EngramID: o.ID, Title: o.Title, Content: o.Content,
			Project: o.Project, Type: o.Type,
			CreatedAt: o.CreatedAt, UpdatedAt: o.UpdatedAt, DeletedAt: o.DeletedAt,
		}, nil
	}
}

// promoteHandler adapts Deps.Promote to the promote tool's typed handler
// shape.
func promoteHandler(deps Deps) mcp.ToolHandlerFor[PromoteIn, PromoteOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in PromoteIn) (*mcp.CallToolResult, PromoteOut, error) {
		if deps.Promote == nil {
			return nil, PromoteOut{}, fmt.Errorf("mcpserver: promote dependency is not configured")
		}
		outcome, err := deps.Promote(ctx, in.Project, in.EngramID)
		if err != nil {
			return nil, PromoteOut{}, err
		}
		out := PromoteOut{
			PageAddress: outcome.Result.Page.Address,
			PagePath:    outcome.Result.Page.Path,
			// ActionKind.String() (promote/update.go, task 8b.11) is the
			// one source of truth for this rendering: cmd_promote.go's
			// CLI output calls the same method, so the two surfaces
			// cannot drift into two different names for one outcome.
			Action: outcome.Result.Action.Kind.String(),
		}
		if outcome.IndexRebuildErr != nil {
			out.IndexStale = true
			out.IndexStaleDetail = fmt.Sprintf(
				"the page was written and is durable, but rebuilding the vault index failed: %v; it will not be found by query until `longterm-mem sync --project %s` rebuilds it",
				outcome.IndexRebuildErr, in.Project)
		}
		return nil, out, nil
	}
}
