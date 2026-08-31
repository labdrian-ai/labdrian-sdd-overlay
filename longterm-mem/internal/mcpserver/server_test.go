package mcpserver

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/promote"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/query"
)

// connectInMemory connects a server built from deps to a fresh in-process
// client over mcp.NewInMemoryTransports (the SDK's own in-memory
// handshake fixture), returning the connected client session. Servers
// must connect before clients (SDK contract), matching the package's own
// example convention.
func connectInMemory(t *testing.T, deps Deps) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()

	server := New(deps)
	t1, t2 := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, t1, nil); err != nil {
		t.Fatalf("server.Connect: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0.0.1"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// decodeStructured re-marshals a CallToolResult's StructuredContent (an
// untyped map[string]any once it round-trips through the wire's own JSON
// codec, even over an in-memory transport) into out.
func decodeStructured(t *testing.T, res *mcp.CallToolResult, out any) {
	t.Helper()
	data, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal StructuredContent: %v", err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatalf("unmarshal StructuredContent into %T: %v", out, err)
	}
}

// TestServer_ToolListingListsQueryAndPromote (8b.1): a connected client's
// tool-listing handshake must list both tools R-012 promises, not just
// whichever one a caller happens to exercise first.
func TestServer_ToolListingListsQueryAndPromote(t *testing.T) {
	session := connectInMemory(t, Deps{})

	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	var names []string
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	for _, want := range []string{"query", "promote"} {
		found := false
		for _, name := range names {
			if name == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("tool listing %v does not include %q", names, want)
		}
	}
}

// TestServer_QueryRoundTripsOverStdio (8b.2): a connected client calling
// query with a valid project/query string must receive the grouped result
// list back over the same connection, and the handler must forward the
// call's own arguments rather than a fixed/ignored value.
func TestServer_QueryRoundTripsOverStdio(t *testing.T) {
	want := query.Result{
		Project:     "labdrian-sdd-overlay",
		Query:       "dragonscale",
		VaultStatus: query.VaultStatusOK,
		Results: []query.ResultRow{
			{Source: query.SourceVault, Rank: 1, PageAddress: "c-000001", Title: "Dragonscale"},
		},
	}
	var gotReq query.Request
	deps := Deps{
		Query: func(_ context.Context, req query.Request) (query.Result, error) {
			gotReq = req
			return want, nil
		},
	}
	session := connectInMemory(t, deps)

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "query",
		Arguments: map[string]any{"project": "labdrian-sdd-overlay", "query": "dragonscale"},
	})
	if err != nil {
		t.Fatalf("CallTool(query): %v", err)
	}
	if gotReq.Project != "labdrian-sdd-overlay" || gotReq.Query != "dragonscale" {
		t.Fatalf("handler received %+v, want the call's own project/query forwarded", gotReq)
	}

	var got query.Result
	decodeStructured(t, res, &got)
	if got.Project != want.Project || got.VaultStatus != want.VaultStatus {
		t.Fatalf("query round trip = %+v, want %+v", got, want)
	}
	if len(got.Results) != 1 || got.Results[0].PageAddress != "c-000001" {
		t.Fatalf("query round trip results = %+v, want the one linked result carried through", got.Results)
	}
}

// TestServer_PromoteRoundTripsOverStdio: supplementary to R-012/R-032's
// named scenarios (no dedicated RED task lists a promote round-trip
// scenario, matching the CLI-wiring precedent set in slice 8a for
// cmd_status.go/cmd_doctor.go's own dispatch tests), proving the promote
// tool -- wired in the same 8b.3 GREEN step as query -- actually forwards
// its call and renders Writer.Promote's Result over the wire, not just
// that it is listed.
func TestServer_PromoteRoundTripsOverStdio(t *testing.T) {
	want := promote.Result{
		Page:   promote.Page{Address: "c-000042", Path: "wiki/memory/c-000042.md"},
		Action: promote.Action{Kind: promote.ActionCreated},
	}
	var gotProject string
	var gotID int64
	deps := Deps{
		Promote: func(_ context.Context, project string, engramID int64) (promote.Result, error) {
			gotProject = project
			gotID = engramID
			return want, nil
		},
	}
	session := connectInMemory(t, deps)

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "promote",
		Arguments: map[string]any{"project": "labdrian-sdd-overlay", "engram_id": 501},
	})
	if err != nil {
		t.Fatalf("CallTool(promote): %v", err)
	}
	if gotProject != "labdrian-sdd-overlay" || gotID != 501 {
		t.Fatalf("handler received project=%q engram_id=%d, want the call's own arguments forwarded", gotProject, gotID)
	}

	var got PromoteOut
	decodeStructured(t, res, &got)
	if got.PageAddress != "c-000042" || got.Action != "created" {
		t.Fatalf("promote round trip = %+v, want page_address=c-000042 action=created", got)
	}
}
