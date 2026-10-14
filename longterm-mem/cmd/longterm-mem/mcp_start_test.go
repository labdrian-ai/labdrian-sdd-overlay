package main

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/embed"
)

// A session that cannot be set up is refused before the server is started, with the name of the command and the
// code of what failed: Engram that cannot be opened is engram_unavailable, an embedding client that cannot be made
// is the program's own fault. Nothing is read from stdin in either case.

func TestCmdMCP_AnEngramDatabaseThatCannotBeOpenedEndsTheSessionBeforeItStarts(t *testing.T) {
	t.Setenv("LONGTERM_MEM_ENGRAM_DB", filepath.Join(t.TempDir(), "no", "such", "directory", "engram.db"))

	var code int
	stderr := captureStderr(t, func() {
		code = commands{newEmbedClient: testEmbedClient, clock: fixedClock{at: fixtureInstant}}.run([]string{"mcp"})
	})

	if code != exitEngramUnavailable {
		t.Errorf("mcp = exit %d, want %d", code, exitEngramUnavailable)
	}
	if want := "longterm-mem: mcp: engram: connect "; len(stderr) < len(want) || stderr[:len(want)] != want {
		t.Errorf("stderr = %q, want it to begin %q", stderr, want)
	}
}

func TestCmdMCP_AnEmbeddingClientThatCannotBeMadeEndsTheSessionBeforeItStarts(t *testing.T) {
	t.Setenv("LONGTERM_MEM_ENGRAM_DB", newTestEngramDBWithObservation(t, "mcp-start-project", "T", "C"))
	boom := errors.New("no embedding backend for this session")
	failing := func(embed.Config) (*embed.Client, error) { return nil, boom }

	var code int
	stderr := captureStderr(t, func() {
		code = commands{newEmbedClient: failing, clock: fixedClock{at: fixtureInstant}}.run([]string{"mcp"})
	})

	if want := "longterm-mem: mcp: no embedding backend for this session\n"; code != exitInternal || stderr != want {
		t.Errorf("mcp = exit %d with stderr %q, want exit %d and %q", code, stderr, exitInternal, want)
	}
}
