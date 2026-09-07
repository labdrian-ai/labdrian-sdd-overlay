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

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
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
			{Sources: []string{query.SourceVault}, Rank: 1, PageAddress: "c-000001", Title: "Dragonscale"},
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

// TestServer_GetReturnsTheWholeObservation is the other half of the
// truncation contract. query now returns an extract of each matched body,
// which is only defensible if the whole body is one call away: a caller
// that can see a preview was cut, and is told how long the full text is,
// must have somewhere to go for it.
func TestServer_GetReturnsTheWholeObservation(t *testing.T) {
	body := strings.Repeat("the whole body ", 400)
	deps := Deps{
		Get: func(_ context.Context, id int64) (GetOutcome, error) {
			if id != 4242 {
				t.Fatalf("Get called with id %d, want 4242", id)
			}
			return GetOutcome{Found: true, Observation: engram.Observation{
				ID: 4242, Title: "the whole thing", Content: body,
				Project: "proj-a", Type: "decision", CreatedAt: "2026-09-01T00:00:00Z",
			}}, nil
		},
	}
	session := connectInMemory(t, deps)

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "get", Arguments: map[string]any{"engram_id": 4242},
	})
	if err != nil {
		t.Fatalf("CallTool(get): %v", err)
	}
	var out GetOut
	decodeStructured(t, res, &out)

	if out.Content != body {
		t.Fatalf("Content is %d bytes, want the whole %d-byte body: get exists precisely so a caller can stop guessing at an extract", len(out.Content), len(body))
	}
	if out.EngramID != 4242 || out.Title != "the whole thing" {
		t.Fatalf("out = %+v, want the observation's own identity", out)
	}
}

// TestServer_GetSaysSoWhenThereIsNoSuchObservation keeps a missing id from
// arriving as an empty body. An observation that does not exist and an
// observation whose content is empty must not look the same to a caller.
func TestServer_GetSaysSoWhenThereIsNoSuchObservation(t *testing.T) {
	deps := Deps{
		Get: func(context.Context, int64) (GetOutcome, error) { return GetOutcome{Found: false}, nil },
	}
	session := connectInMemory(t, deps)

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "get", Arguments: map[string]any{"engram_id": 9},
	})
	if err != nil {
		t.Fatalf("CallTool(get): %v", err)
	}
	var out GetOut
	decodeStructured(t, res, &out)
	if out.Found {
		t.Fatalf("Found = true for an id that does not exist: %+v", out)
	}
	if out.Detail == "" {
		t.Fatalf("a not-found result says nothing about why it is empty: %+v", out)
	}
}

// TestServer_ToolListingListsGet: a tool a caller cannot discover is a
// tool that does not exist. The truncation markers in a query result point
// at this tool, so it has to be in the handshake.
func TestServer_ToolListingListsGet(t *testing.T) {
	session := connectInMemory(t, Deps{})

	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	for _, tool := range res.Tools {
		if tool.Name == "get" {
			return
		}
	}
	t.Fatalf("the tool listing does not offer \"get\"; a truncated snippet then has nowhere to point")
}

// TestServer_QueryDoesNotShipItsResultTwice is the payload duplication.
// The handler returned a nil *mcp.CallToolResult, and the go-sdk fills an
// absent Content field with a byte-identical JSON serialization of
// StructuredContent (server.go: `if res.Content == nil`). Measured on the
// wire before this change: a 182,813 byte response carrying 89,741 bytes
// of content text and 90,689 bytes of structuredContent, the first
// parsing to exactly the second.
func TestServer_QueryDoesNotShipItsResultTwice(t *testing.T) {
	result := query.Result{
		Project: "proj-a", Query: "zephyr", VaultStatus: query.VaultStatusOK,
		Results: []query.ResultRow{
			{Sources: []string{query.SourceEngramFTS}, Rank: 1, EngramID: 7, Title: "a decision", Snippet: strings.Repeat("body text ", 40), SnippetTruncated: true, FullLength: 9000},
		},
	}
	deps := Deps{Query: func(context.Context, query.Request) (query.Result, error) { return result, nil }}
	session := connectInMemory(t, deps)

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "query", Arguments: map[string]any{"project": "proj-a", "query": "zephyr"},
	})
	if err != nil {
		t.Fatalf("CallTool(query): %v", err)
	}

	structured, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal StructuredContent: %v", err)
	}
	text := textOf(t, res)
	if text == "" {
		t.Fatalf("the response carries no human-readable content at all")
	}
	var parsed any
	if json.Unmarshal([]byte(text), &parsed) == nil {
		t.Fatalf("the text block is still a JSON copy of the structured result (%d bytes beside %d)", len(text), len(structured))
	}
	if len(text) >= len(structured) {
		t.Fatalf("the text block (%d bytes) is no smaller than the structured result (%d bytes)", len(text), len(structured))
	}
}

// TestServer_QueryTextBlockStaysReadable guards what the duplication was
// accidentally providing. Removing the JSON copy is only safe if what
// replaces it still tells a person what came back; an empty text block
// would be a regression dressed as a saving.
func TestServer_QueryTextBlockStaysReadable(t *testing.T) {
	result := query.Result{
		Project: "proj-a", Query: "zephyr", VaultStatus: query.VaultStatusOK,
		Results: []query.ResultRow{
			{Sources: []string{query.SourceEngramFTS}, Rank: 1, EngramID: 7, Title: "a decision", Snippet: "the zephyr decision", SnippetTruncated: true, FullLength: 9000},
		},
		Diagnostics: []query.Diagnostic{{Code: query.DiagnosticSearchWidened, Detail: "widened"}},
	}
	deps := Deps{Query: func(context.Context, query.Request) (query.Result, error) { return result, nil }}
	session := connectInMemory(t, deps)

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "query", Arguments: map[string]any{"project": "proj-a", "query": "zephyr"},
	})
	if err != nil {
		t.Fatalf("CallTool(query): %v", err)
	}
	text := textOf(t, res)
	for _, want := range []string{"a decision", "engram:7", "the zephyr decision", query.DiagnosticSearchWidened} {
		if !strings.Contains(text, want) {
			t.Fatalf("the text block does not mention %q:\n%s", want, text)
		}
	}
	if !strings.Contains(text, "9000") {
		t.Fatalf("the text block does not say how long the truncated full text is:\n%s", text)
	}
}

// TestServer_GetDoesNotShipTheBodyTwice: get exists to deliver one whole
// observation, so a JSON copy beside it doubles precisely the payload the
// tool is for.
func TestServer_GetDoesNotShipTheBodyTwice(t *testing.T) {
	body := strings.Repeat("the whole body ", 400)
	deps := Deps{Get: func(context.Context, int64) (GetOutcome, error) {
		return GetOutcome{Found: true, Observation: engram.Observation{ID: 1, Title: "t", Content: body}}, nil
	}}
	session := connectInMemory(t, deps)

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "get", Arguments: map[string]any{"engram_id": 1},
	})
	if err != nil {
		t.Fatalf("CallTool(get): %v", err)
	}
	text := textOf(t, res)
	if !strings.Contains(text, "the whole body") {
		t.Fatalf("the text block does not carry the observation a person asked to read:\n%.200s", text)
	}
	var parsed any
	if json.Unmarshal([]byte(text), &parsed) == nil {
		t.Fatalf("get still ships its body twice: the text block is a JSON copy of the structured result")
	}
}

// textOf concatenates the text of every TextContent block in res.
func textOf(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}
