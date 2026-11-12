package main

import (
	"context"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
)

// An explicit promote reads the observation and nothing of what became of it: it never failed on a store whose
// relations cannot be read, and it still does not, through the command and through the MCP tool.
func TestExplicitPromoteDoesNotFailOnUnreadableRelations(t *testing.T) {
	w, _ := memorycardWorld(t)
	w.execSQL(t, `DROP TABLE memory_relations`)

	code, stdout, stderr := runPromoteGolden(t, []string{"promote", "--project", promoteGoldenProject, "--id", "1"})
	if code != exitOK || !strings.Contains(stdout, "promoted c-000901 (created)") {
		t.Fatalf("promote: exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	store, err := engram.Open(w.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	tool := commands{newEmbedClient: testEmbedClient, clock: promoteGoldenClock}.mcpPromote(store)
	if _, err := tool(context.Background(), promoteGoldenProject, "engram", 2); err != nil {
		t.Fatalf("MCP promote: %v", err)
	}
}
