package mcpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/promote"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/query"

	_ "modernc.org/sqlite"
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
		Promote: func(_ context.Context, project string, engramID int64) (PromoteOutcome, error) {
			gotProject = project
			gotID = engramID
			return PromoteOutcome{Result: want}, nil
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
	if got.IndexStale {
		t.Fatalf("promote round trip reported a stale index for a run whose rebuild succeeded: %+v", got)
	}
}

// TestServer_PromoteReportsAStaleIndexWithoutFailingThePromotion pins the
// third state a promote call can end in, and the reason it needs a field
// of its own.
//
// By the time the vault index is rebuilt the page is already written and
// durable. So a rebuild failure is NOT a promotion failure -- reporting it
// as an error would tell the caller a page it can see on disk was never
// written -- and it is not nothing either: the page exists but query
// cannot find it until the index is rebuilt. It is therefore reported
// alongside a successful action, with the remedy named.
func TestServer_PromoteReportsAStaleIndexWithoutFailingThePromotion(t *testing.T) {
	deps := Deps{
		Promote: func(context.Context, string, int64) (PromoteOutcome, error) {
			return PromoteOutcome{
				Result: promote.Result{
					Page:   promote.Page{Address: "c-000043", Path: "wiki/memory/c-000043.md"},
					Action: promote.Action{Kind: promote.ActionUpdated},
				},
				IndexRebuildErr: errors.New("vault rebuild script exited 1"),
			}, nil
		},
	}
	session := connectInMemory(t, deps)

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "promote",
		Arguments: map[string]any{"project": "labdrian-sdd-overlay", "engram_id": 502},
	})
	if err != nil {
		t.Fatalf("CallTool(promote) reported the whole promotion as failed because the index rebuild failed: %v", err)
	}

	var got PromoteOut
	decodeStructured(t, res, &got)
	if got.Action != "updated" || got.PageAddress != "c-000043" {
		t.Fatalf("promote result = %+v, want the promotion still reported as updated", got)
	}
	if !got.IndexStale {
		t.Fatalf("promote result = %+v, want index_stale set so the caller knows the page exists but is not indexed", got)
	}
	if !strings.Contains(got.IndexStaleDetail, "sync") {
		t.Fatalf("index_stale_detail = %q, want it to name the remedy (sync)", got.IndexStaleDetail)
	}
}

// fixtureEngramDB creates a real, empty (schema-only) Engram database at a
// scratch path via the shared schema.sql fixture (internal/engram's own
// testdata), so the "mcp" subprocess's engram.Open call in
// TestServer_ExitsWhenStdinCloses succeeds -- the test only needs the
// server to reach its blocking stdio loop, never to actually serve a
// call, so no rows are inserted.
func fixtureEngramDB(t *testing.T) string {
	t.Helper()
	schema, err := os.ReadFile(filepath.Join("..", "engram", "testdata", "schema.sql"))
	if err != nil {
		t.Fatalf("read engram schema fixture: %v", err)
	}
	dbPath := filepath.Join(t.TempDir(), "engram.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open fixture db: %v", err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close fixture db: %v", err)
	}
	return dbPath
}

// buildLongtermMemBinary compiles the real longterm-mem binary to a
// scratch path, mirroring cmd/longterm-mem/main_test.go's own
// TestMain_BuildsIndependentModule build invocation, so
// TestServer_ExitsWhenStdinCloses exercises the real "mcp" subcommand
// dispatch and the real StdioTransport, not an in-memory fake.
func buildLongtermMemBinary(t *testing.T) string {
	t.Helper()
	moduleRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve module root: %v", err)
	}
	binPath := filepath.Join(t.TempDir(), "longterm-mem")
	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/longterm-mem")
	cmd.Dir = moduleRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/longterm-mem failed: %v\n%s", err, out)
	}
	return binPath
}

// TestServer_ExitsWhenStdinCloses (8b.8, -short-skippable integration,
// R-034): a real "mcp" subprocess must (1) block on its stdio session
// rather than exit immediately -- proving a real MCP server is actually
// running, not merely dispatched and abandoned -- and (2) exit on its own
// once stdin closes, leaving no residual child process, per R-034's "no
// persistent daemon" and the "MCP server exits with its session" scenario.
func TestServer_ExitsWhenStdinCloses(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs the real binary as a subprocess; skipped under -short")
	}
	if _, err := exec.LookPath("pgrep"); err != nil {
		t.Skip("pgrep not on PATH; cannot assert no residual process remains")
	}

	binPath := buildLongtermMemBinary(t)
	dbPath := fixtureEngramDB(t)

	stdinReader, stdinWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdin pipe: %v", err)
	}

	cmd := exec.Command(binPath, "mcp")
	cmd.Stdin = stdinReader
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "LONGTERM_MEM_ENGRAM_DB="+dbPath)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start mcp subprocess: %v", err)
	}
	if err := stdinReader.Close(); err != nil {
		t.Fatalf("close the parent's copy of the stdin pipe's read end: %v", err)
	}

	// Give the server a moment to reach its blocking Run loop, then prove
	// it is still alive: signal 0 checks liveness without affecting the
	// process. If "mcp" dispatch were broken (e.g. falling through to the
	// unknown-subcommand path), the process would already have exited by
	// now instead of blocking on stdio.
	time.Sleep(200 * time.Millisecond)
	if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("mcp subprocess is not alive 200ms after starting (signal probe: %v); it must block on its stdio session until stdin closes", err)
	}

	if err := stdinWriter.Close(); err != nil {
		t.Fatalf("close stdin: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("mcp subprocess exited with an error after stdin closed: %v", err)
		}
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("mcp subprocess did not exit within 5s of stdin closing")
	}

	if residual, err := exec.Command("pgrep", "-P", strconv.Itoa(cmd.Process.Pid)).CombinedOutput(); err == nil {
		t.Fatalf("mcp subprocess left a residual child process behind: %s", residual)
	}
}
