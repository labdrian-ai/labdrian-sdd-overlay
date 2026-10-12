package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// retrieveScript is the vault's retrieval entrypoint as a test writes it: it answers count candidates, each
// with a snippet of padding bytes after the words that say what it was asked, so that the query text and the
// bound reach the transcript.
func retrieveScript(count, padding int) string {
	return strings.NewReplacer("COUNT", fmt.Sprint(count), "PADDING", fmt.Sprint(padding)).Replace(`import json, os, sys
query = sys.argv[1]
top = sys.argv[sys.argv.index("--top") + 1]
root = os.getcwd()
candidates = []
for i in range(COUNT):
    n = i + 1
    candidates.append({
        "page_address": "c-%06d" % n,
        "absolute_path": root + "/wiki/memory/c-%06d.md" % n,
        "bm25_score": 2.0 - i * 0.5,
        "rerank_score": 0.25 * n,
        "snippet": "page %d for %s (top %s) " % (n, query, top) + "x" * PADDING,
        "chunk_id": i,
    })
print(json.dumps({"candidates": candidates}))
`)
}

// retrieveScriptThatExits is an entrypoint that writes what it says to stderr and exits with code.
func retrieveScriptThatExits(code int, stderr string) string {
	return fmt.Sprintf("import sys\nsys.stderr.write(%q)\nsys.exit(%d)\n", stderr, code)
}

// mcpToolCall is one tools/call request of a session.
type mcpToolCall struct {
	tool string
	args map[string]any
}

// mcpGoldenCase is one session: what is set up in the world first, then the tools it calls.
type mcpGoldenCase struct {
	name  string
	setup func(t *testing.T, w *retrievalWorld)
	calls []mcpToolCall
	// handshake keeps the answer to initialize and lists the tools.
	handshake bool
	// after renders more of the world once the session is over.
	after func(t *testing.T, w *retrievalWorld) string
}

func withVaultScript(script string) func(t *testing.T, w *retrievalWorld) {
	return func(t *testing.T, w *retrievalWorld) { w.writeVaultFile(t, "scripts/retrieve.py", script) }
}

// withEmbeddingIndex builds the embedding index of the golden project through the command that builds it.
func withEmbeddingIndex(t *testing.T, w *retrievalWorld) {
	t.Helper()
	if code, stdout, stderr := w.run(t, []string{"index", "--project", queryGoldenProject, "--embeddings", "--embed-dimension", "3"}); code != exitOK {
		t.Fatalf("build the embedding index: exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
}

func queryCall(args map[string]any) mcpToolCall {
	merged := map[string]any{"project": queryGoldenProject}
	for k, v := range args {
		merged[k] = v
	}
	return mcpToolCall{tool: "query", args: merged}
}

func mcpGoldenCases() []mcpGoldenCase {
	return []mcpGoldenCase{
		{name: "00-the-handshake-and-the-tools", handshake: true},
		{
			name:  "01-the-vault-alone-answers-in-its-own-order",
			setup: withVaultScript(retrieveScript(3, 20)),
			calls: []mcpToolCall{queryCall(map[string]any{"query": "layout", "sources": []string{"vault"}})},
		},
		{
			name:  "02-the-vault-comes-first-and-engram-follows",
			setup: withVaultScript(retrieveScript(2, 20)),
			calls: []mcpToolCall{queryCall(map[string]any{"query": "cache", "sources": []string{"vault", "engram-fts"}})},
		},
		{
			name:  "03-the-bound-and-the-query-reach-the-script",
			setup: withVaultScript(retrieveScript(5, 10)),
			calls: []mcpToolCall{
				queryCall(map[string]any{"query": "layout", "top": 2, "sources": []string{"vault"}}),
				queryCall(map[string]any{"query": "layout", "sources": []string{"vault"}}),
			},
		},
		{
			name:  "04-a-vault-that-is-not-provisioned-is-not-an-error",
			setup: withVaultScript(retrieveScriptThatExits(10, "not provisioned\n")),
			calls: []mcpToolCall{queryCall(map[string]any{"query": "cache", "sources": []string{"vault", "engram-fts"}})},
		},
		{
			name:  "05-a-script-that-fails-degrades-to-engram-and-says-so",
			setup: withVaultScript(retrieveScriptThatExits(3, "retrieval index is missing\n")),
			calls: []mcpToolCall{queryCall(map[string]any{"query": "cache", "sources": []string{"vault", "engram-fts"}})},
		},
		{
			name:  "06-a-script-that-prints-no-json-degrades-the-same-way",
			setup: withVaultScript("print('not json')\n"),
			calls: []mcpToolCall{queryCall(map[string]any{"query": "cache", "sources": []string{"vault", "engram-fts"}})},
		},
		{
			name:  "07-a-vault-with-no-retrieval-script-degrades-the-same-way",
			calls: []mcpToolCall{queryCall(map[string]any{"query": "cache", "sources": []string{"vault", "engram-fts"}})},
		},
		{
			name:  "08-a-vault-with-nothing-to-say-is-ok-and-empty",
			setup: withVaultScript(retrieveScript(0, 0)),
			calls: []mcpToolCall{queryCall(map[string]any{"query": "layout", "sources": []string{"vault"}})},
		},
		{
			name:  "09-a-response-over-the-ceiling-drops-rows-and-says-so",
			setup: withVaultScript(retrieveScript(30, 400)),
			calls: []mcpToolCall{queryCall(map[string]any{"query": "cache", "top": 50, "sources": []string{"vault", "engram-fts"}})},
		},
		{
			name:  "10-the-embedding-source-without-an-index-has-no-rows",
			calls: []mcpToolCall{queryCall(map[string]any{"query": "dragonscale", "sources": []string{"engram-embed"}})},
		},
		{
			name:  "11-the-embedding-source-with-an-index",
			setup: withEmbeddingIndex,
			calls: []mcpToolCall{queryCall(map[string]any{"query": "dragonscale", "top": 3, "sources": []string{"engram-embed"}})},
		},
		{
			name:  "12-no-sources-means-both-of-engrams-once-an-index-exists",
			setup: withEmbeddingIndex,
			calls: []mcpToolCall{queryCall(map[string]any{"query": "dragonscale"})},
		},
		{
			name:  "13-a-source-nobody-knows-is-refused",
			calls: []mcpToolCall{queryCall(map[string]any{"query": "cache", "sources": []string{"engram-fts", "elsewhere"}})},
		},
		{
			name:  "14-a-call-with-no-project-is-refused",
			calls: []mcpToolCall{{tool: "query", args: map[string]any{"query": "cache"}}},
		},
		{
			name: "15-get-reads-a-whole-observation-or-says-there-is-none",
			calls: []mcpToolCall{
				{tool: "get", args: map[string]any{"engram_id": 1}},
				{tool: "get", args: map[string]any{"engram_id": 999}},
				{tool: "get", args: map[string]any{"engram_id": 4}},
			},
		},
		{
			name:  "16-a-replaced-decision-says-so-in-both-halves-of-the-answer",
			calls: []mcpToolCall{queryCall(map[string]any{"query": "cache"})},
		},
		{
			name:  "17-types-left-out-are-named",
			calls: []mcpToolCall{queryCall(map[string]any{"query": "dragonscale", "exclude_types": []string{"session_summary"}})},
		},
		{
			name: "18-a-promotion-over-mcp-rebuilds-the-vault-index",
			setup: func(t *testing.T, w *retrievalWorld) {
				writeIndexStubs(t, w)
				w.writeVaultFile(t, "scripts/allocate-address.sh", promoteGoldenAllocator)
			},
			calls: []mcpToolCall{
				{tool: "promote", args: map[string]any{"project": queryGoldenProject, "engram_id": 3}},
				{tool: "promote", args: map[string]any{"project": queryGoldenProject, "engram_id": 3}},
			},
			after: func(t *testing.T, w *retrievalWorld) string { return renderIndexState(t, w) },
		},
		{
			name: "19-a-promotion-whose-index-rebuild-fails-still-wrote-the-page",
			setup: func(t *testing.T, w *retrievalWorld) {
				writeIndexStubs(t, w)
				w.writeVaultFile(t, "scripts/allocate-address.sh", promoteGoldenAllocator)
				w.writeVaultFile(t, "fail-setup", "")
			},
			calls: []mcpToolCall{{tool: "promote", args: map[string]any{"project": queryGoldenProject, "engram_id": 3}}},
			after: func(t *testing.T, w *retrievalWorld) string { return renderIndexState(t, w) },
		},
		{
			name: "20-an-unreachable-backend-is-named-for-the-embedding-source",
			setup: func(t *testing.T, w *retrievalWorld) {
				withEmbeddingIndex(t, w)
				w.backend.mode.Store(int32(backendFails))
			},
			calls: []mcpToolCall{queryCall(map[string]any{"query": "dragonscale", "sources": []string{"engram-fts", "engram-embed"}})},
		},
		{
			name: "21-a-top-up-is-seen-by-the-next-query-of-the-same-session",
			setup: func(t *testing.T, w *retrievalWorld) {
				withEmbeddingIndex(t, w)
				w.execSQL(t, newObservationSQL(20, "Fresh dragonscale note", "dragonscale fresh body"))
			},
			calls: []mcpToolCall{
				queryCall(map[string]any{"query": "dragonscale", "sources": []string{"engram-embed"}}),
				queryCall(map[string]any{"query": "dragonscale", "sources": []string{"engram-embed"}}),
			},
		},
		{
			name: "22-another-project-is-not-served-from-the-index-of-the-first",
			setup: func(t *testing.T, w *retrievalWorld) {
				withEmbeddingIndex(t, w)
			},
			calls: []mcpToolCall{
				queryCall(map[string]any{"query": "dragonscale", "sources": []string{"engram-embed"}}),
				// The session cannot change the world between two calls, so this one asks for a project whose
				// index does not exist: it is not served from the cache of the first.
				{tool: "query", args: map[string]any{"project": "otherproj", "query": "dragonscale", "sources": []string{"engram-embed"}}},
			},
		},
	}
}

func TestMCPSessionGolden(t *testing.T) {
	requirePython3(t)
	for _, tc := range mcpGoldenCases() {
		t.Run(tc.name, func(t *testing.T) {
			w := newRetrievalWorld(t)
			if tc.setup != nil {
				tc.setup(t, w)
			}
			messages := []string{
				`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"golden","version":"0"}}}`,
				`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
			}
			if tc.handshake {
				messages = append(messages, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
			}
			for i, call := range tc.calls {
				request, err := json.Marshal(map[string]any{
					"jsonrpc": "2.0", "id": i + 10, "method": "tools/call",
					"params": map[string]any{"name": call.tool, "arguments": call.args},
				})
				if err != nil {
					t.Fatalf("encode a call: %v", err)
				}
				messages = append(messages, string(request))
			}

			answers, code, stderr := w.mcpSession(t, messages)
			if !tc.handshake && len(answers) > 0 {
				answers = answers[1:] // the answer to initialize says nothing about the case
			}

			var b strings.Builder
			for i, answer := range answers {
				fmt.Fprintf(&b, "--- answer %d ---\n%s\n", i+1, answer)
			}
			fmt.Fprintf(&b, "--- exit ---\n%d\n--- stderr ---\n%s", code, stderr)
			if tc.after != nil {
				fmt.Fprintf(&b, "--- after ---\n%s", tc.after(t, w))
			}
			w.check(t, "mcp-"+tc.name, w.normalise(b.String()))
		})
	}
}

// mcpSession runs `longterm-mem mcp` in process over pipes and sends it the JSON-RPC messages, one at a time,
// waiting for each answer before it sends the next. A notification (a message with no id) gets no answer. It
// returns the answers in order, indented, then closes the server's input and returns the exit code and what the
// server wrote to stderr.
//
// The streams of the process are the test's for the length of the session and restored afterwards.
func (w *retrievalWorld) mcpSession(t *testing.T, messages []string) (answers []string, code int, stderr string) {
	t.Helper()
	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		t.Fatalf("create the stdin pipe: %v", err)
	}
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatalf("create the stdout pipe: %v", err)
	}
	errFile, err := os.CreateTemp(t.TempDir(), "mcp-stderr")
	if err != nil {
		t.Fatalf("create the stderr capture: %v", err)
	}
	realIn, realOut, realErr := os.Stdin, os.Stdout, os.Stderr
	os.Stdin, os.Stdout, os.Stderr = stdinR, stdoutW, errFile
	restore := func() { os.Stdin, os.Stdout, os.Stderr = realIn, realOut, realErr }
	defer restore()

	exit := make(chan int, 1)
	go func() { exit <- w.commands.run([]string{"mcp"}) }()

	reader := bufio.NewReader(stdoutR)
	for _, message := range messages {
		if _, err := fmt.Fprintln(stdinW, message); err != nil {
			t.Fatalf("send %s: %v", message, err)
		}
		var probe struct {
			ID *json.RawMessage `json:"id"`
		}
		if err := json.Unmarshal([]byte(message), &probe); err != nil {
			t.Fatalf("the message is not JSON: %v\n%s", err, message)
		}
		if probe.ID == nil {
			continue
		}
		// A watchdog only: it fails a session whose server never answers, and decides nothing about the output.
		_ = stdoutR.SetReadDeadline(time.Now().Add(30 * time.Second))
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read the answer to %s: %v", message, err)
		}
		var indented bytes.Buffer
		if err := json.Indent(&indented, []byte(strings.TrimSpace(line)), "", "  "); err != nil {
			t.Fatalf("the answer is not JSON: %v\n%s", err, line)
		}
		answers = append(answers, indented.String())
	}
	if err := stdinW.Close(); err != nil {
		t.Fatalf("close the server's input: %v", err)
	}
	select {
	case code = <-exit:
	case <-time.After(30 * time.Second):
		t.Fatal("the server did not stop when its input closed")
	}
	restore()
	_ = stdoutW.Close()
	_ = stdinR.Close()
	data, err := os.ReadFile(errFile.Name())
	if err != nil {
		t.Fatalf("read the stderr capture: %v", err)
	}
	return answers, code, string(data)
}
