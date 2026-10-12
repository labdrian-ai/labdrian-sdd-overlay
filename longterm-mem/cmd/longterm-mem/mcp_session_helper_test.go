package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/query"
)

// sessionFailed is what a watchedT panics with when the session it was given fails, so that the test which
// gave it can see the failure and go on, as a real T would end the test.
type sessionFailed struct{}

// watchedT is a T whose fatal failures a test can catch and whose other failures it can read.
type watchedT struct {
	*testing.T
	errors []string
	// stacksAtFailure is every goroutine's stack at the moment the session failed, when its server is still
	// running: the control that shows serverFrame can find a live server.
	stacksAtFailure string
}

// serverFrame is how a goroutine running the command shows in a stack dump.
const serverFrame = "cmd/longterm-mem.commands.run("

func allStacks() string {
	stacks := make([]byte, 1<<20)
	return string(stacks[:runtime.Stack(stacks, true)])
}

func (w *watchedT) Fatal(...any)          { w.stacksAtFailure = allStacks(); panic(sessionFailed{}) }
func (w *watchedT) Fatalf(string, ...any) { w.stacksAtFailure = allStacks(); panic(sessionFailed{}) }
func (w *watchedT) Errorf(format string, args ...any) {
	w.errors = append(w.errors, fmt.Sprintf(format, args...))
}

// The ceiling of a response is the query package's, and a session over it is told what was dropped. Golden 09
// holds the whole answer; this is what explains a change of it: the structured answer fits the byte ceiling,
// the cap is named with the token ceiling, and the rows shown and the rows dropped add up to the rows found.
// A change of how tokens are counted moves the golden and leaves these three claims as they are, so a golden
// that moved without one of them failing moved for the estimator alone.
func TestMCPSession_ACappedResponseFitsTheCeilingAndAccountsForEveryRow(t *testing.T) {
	requirePython3(t)
	w := newRetrievalWorld(t)
	withVaultScript(retrieveScript(30, 400))(t, w)
	call, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 10, "method": "tools/call",
		"params": map[string]any{"name": "query", "arguments": map[string]any{
			"project": queryGoldenProject, "query": "cache", "top": 50, "sources": []string{"vault", "engram-fts"},
		}},
	})
	if err != nil {
		t.Fatalf("encode the call: %v", err)
	}
	answers, code, stderr := w.mcpSession(t, []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"ceiling","version":"0"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		string(call),
	})
	if code != exitOK || stderr != "" || len(answers) != 2 {
		t.Fatalf("session = %d answers, exit %d, stderr %q; want 2 answers, exit %d and nothing on stderr", len(answers), code, stderr, exitOK)
	}
	var answer struct {
		Result struct {
			StructuredContent json.RawMessage `json:"structuredContent"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(answers[1]), &answer); err != nil {
		t.Fatalf("the answer is not JSON: %v", err)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, answer.Result.StructuredContent); err != nil {
		t.Fatalf("the structured answer is not JSON: %v", err)
	}
	if compact.Len() > query.ResponseByteCeiling {
		t.Errorf("the structured answer is %d bytes, over the %d-byte (%d-token) ceiling", compact.Len(), query.ResponseByteCeiling, query.ResponseTokenCeiling)
	}

	var structured struct {
		Results     []json.RawMessage `json:"results"`
		Diagnostics []struct {
			Code   string `json:"code"`
			Detail string `json:"detail"`
		} `json:"diagnostics"`
	}
	if err := json.Unmarshal(answer.Result.StructuredContent, &structured); err != nil {
		t.Fatalf("the structured answer has an unexpected shape: %v", err)
	}
	var capped []string
	for _, d := range structured.Diagnostics {
		if d.Code == query.DiagnosticResponseCapped {
			capped = append(capped, d.Detail)
		}
	}
	if len(capped) != 1 {
		t.Fatalf("the answer carries %d %s diagnostics, want 1: %+v", len(capped), query.DiagnosticResponseCapped, structured.Diagnostics)
	}
	if want := fmt.Sprintf("%d-token ceiling", query.ResponseTokenCeiling); !strings.Contains(capped[0], want) {
		t.Errorf("the diagnostic %q does not name the %q", capped[0], want)
	}
	counts := regexp.MustCompile(`(\d+) of (\d+) results were dropped`).FindStringSubmatch(capped[0])
	if counts == nil {
		t.Fatalf("the diagnostic %q does not say how many results were dropped", capped[0])
	}
	dropped, _ := strconv.Atoi(counts[1])
	found, _ := strconv.Atoi(counts[2])
	if dropped == 0 || len(structured.Results)+dropped != found {
		t.Errorf("the answer shows %d rows and says %d of %d were dropped: they must add up, and something must have been dropped", len(structured.Results), dropped, found)
	}
}

// The ceiling is measured on the bytes of the answer, and the answer carries the path of every page the vault
// names. Where a world's directories are depends on where the tests run (TMPDIR on a laptop is not the one on a
// runner), so a golden that is bounded by the ceiling holds only if the world does not let that path reach the
// bytes. Golden 09 is the one that is: the same session in two directories of very different lengths must say
// the same thing, for the rows that fit are the rows the ceiling leaves room for.
func TestMCPSessionGolden_ACappedAnswerDoesNotDependOnWhereTheWorldIs(t *testing.T) {
	requirePython3(t)
	const capped = "09-a-response-over-the-ceiling-drops-rows-and-says-so"
	var tc mcpGoldenCase
	for _, candidate := range mcpGoldenCases() {
		if candidate.name == capped {
			tc = candidate
		}
	}
	if tc.name == "" {
		t.Fatalf("the golden case %q is gone", capped)
	}
	base := t.TempDir()
	answers := map[string]string{}
	for name, dir := range map[string]string{
		"short": filepath.Join(base, "s"),
		"long":  filepath.Join(base, strings.Repeat("a-long-directory-", 6)),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("create the %s temporary directory: %v", name, err)
		}
		t.Run(name, func(t *testing.T) {
			t.Setenv("TMPDIR", dir)
			_, answers[name] = runMCPGoldenCase(t, tc)
		})
	}
	if answers["short"] == "" || answers["long"] == "" {
		t.Fatalf("a session printed nothing: short %d bytes, long %d bytes", len(answers["short"]), len(answers["long"]))
	}
	if answers["short"] != answers["long"] {
		t.Errorf("the capped answer depends on the length of the temporary directory the world is in:\n--- short ---\n%s\n--- long ---\n%s", answers["short"], answers["long"])
	}
}

type countingCloser struct{ closed int }

func (c *countingCloser) Close() error { c.closed++; return nil }

// A server is stopped by closing its input, and its answer is the exit code it returns; one that never returns
// is reported as not stopped (the wait is the watchdog's channel, here one the test closes or leaves open).
func TestStopServerWaitsForTheExitCodeOrGivesUp(t *testing.T) {
	exited := make(chan int, 1)
	exited <- 7
	input := &countingCloser{}
	if code, ok := stopServer(input, strings.NewReader("late output"), exited, nil); !ok || code != 7 || input.closed != 1 {
		t.Errorf("stopServer = (%d, %v), input closed %d times; want (7, true) and the input closed once", code, ok, input.closed)
	}

	givenUp := make(chan time.Time)
	close(givenUp)
	if code, ok := stopServer(&countingCloser{}, strings.NewReader(""), make(chan int), givenUp); ok || code != 0 {
		t.Errorf("stopServer of a server that never returns = (%d, %v), want (0, false)", code, ok)
	}
}

// A session that ends with its server stopped gives the streams back and closes its pipes. One whose server did
// not stop gives the streams back too (the test's own output must work) but is reported, and its pipes stay open:
// a server that is still running is not handed closed pipes to write to.
func TestFinishSessionLeavesThePipesOfAServerThatDidNotStop(t *testing.T) {
	for name, tc := range map[string]struct {
		stopped    bool
		wantClosed int
		wantErrors int
	}{
		"the server stopped":      {true, 2, 0},
		"the server did not stop": {false, 0, 1},
	} {
		t.Run(name, func(t *testing.T) {
			watched := &watchedT{T: t}
			restored := 0
			a, b := &countingCloser{}, &countingCloser{}

			finishSession(watched, tc.stopped, func() { restored++ }, a, b)

			if restored != 1 {
				t.Errorf("the streams were given back %d times, want once", restored)
			}
			if a.closed+b.closed != tc.wantClosed {
				t.Errorf("%d pipes closed, want %d", a.closed+b.closed, tc.wantClosed)
			}
			if len(watched.errors) != tc.wantErrors {
				t.Errorf("errors = %v, want %d", watched.errors, tc.wantErrors)
			}
		})
	}
}

// A session that fails before it is over (an answer that never comes, a message that is not JSON) must not
// leave the server it started behind: that server would sit on a stdin nobody closes, and could write to the
// stdout the test gives back. The server is stopped and the streams are restored whatever way the session ends.
func TestMCPSession_AFailureBeforeTheEndStopsTheServerAndGivesTheStreamsBack(t *testing.T) {
	w := newRetrievalWorld(t)
	realIn, realOut, realErr := os.Stdin, os.Stdout, os.Stderr
	watched := &watchedT{T: t}

	failed := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				if _, ok := r.(sessionFailed); !ok {
					panic(r)
				}
				failed = true
			}
		}()
		w.mcpSession(watched, []string{
			`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"early","version":"0"}}}`,
			`this is not JSON`,
		})
	}()

	if !failed {
		t.Fatal("the session did not fail on a message that is not JSON, so this test proves nothing")
	}
	if os.Stdin != realIn || os.Stdout != realOut || os.Stderr != realErr {
		t.Error("the process streams were not given back after the session failed")
	}
	if len(watched.errors) != 0 {
		t.Errorf("the session reported %v while stopping the server", watched.errors)
	}
	// The control: at the moment of the failure the server was running and the frame finds it, so that an
	// absence after the session means something.
	if !strings.Contains(watched.stacksAtFailure, serverFrame) {
		t.Fatalf("the frame %q does not find the live server in a stack dump, so its absence proves nothing:\n%s", serverFrame, watched.stacksAtFailure)
	}
	if stacks := allStacks(); strings.Contains(stacks, serverFrame) {
		t.Errorf("the server of the failed session is still running:\n%s", stacks)
	}
}
