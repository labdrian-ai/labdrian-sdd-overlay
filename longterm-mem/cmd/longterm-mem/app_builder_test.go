package main

import (
	"context"
	"os"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/embed"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vecindex"
)

// Two builds of one embedding index must exclude each other (vecindex.Builder), and what excludes them is the Builder
// they are made through: so every path of a process that builds an index is handed the one Builder the process has,
// made where the process is composed (newApp), and not one of its own. The file lock would keep the builds apart
// whichever Builder they used, on the platforms where one process can lock a file twice, so no behavior tells a
// shared Builder from a Builder made for the call; what tells them is whose it is. These cases hold each path to the
// app's.

func newBuilderTestApp(t *testing.T) (app, *engram.Store) {
	t.Helper()
	store, err := engram.Open(newTestEngramDBWithObservation(t, "builder-project", "T", "C"))
	if err != nil {
		t.Fatalf("engram.Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return commands{newEmbedClient: testEmbedClient, clock: promoteGoldenClock}.newApp("mcp"), store
}

func TestApp_EveryPathThatBuildsAnIndexBuildsThroughTheBuilderOfTheApp(t *testing.T) {
	a, store := newBuilderTestApp(t)
	if a.builder == nil {
		t.Fatal("the app has no Builder")
	}
	embedClient, err := testEmbedClient(embed.Config{Model: vecindex.DefaultModel})
	if err != nil {
		t.Fatalf("the embedding client: %v", err)
	}
	indexing, ok := a.embeddingIndexing(store, t.TempDir(), embedConfig{}).Builder.(*vecindex.Repository)
	if !ok {
		t.Fatal("the builder of `index --embeddings` is not a vecindex.Repository")
	}

	for name, used := range map[string]*vecindex.Builder{
		"the top-up of a query run from the command line":   a.cliQueryWiring().builder,
		"the top-up of a query tool call of an MCP session": a.sessionQueryWiring(embedClient, vecindex.NewLoadCache()).builder,
		"`index --embeddings`":                              indexing.Builder,
	} {
		if used != a.builder {
			t.Errorf("%s builds through %p, want the app's Builder, %p", name, used, a.builder)
		}
	}
}

// What a process builds through is the process's own: two runs of the program are two processes, and share nothing.
func TestApp_TwoAppsShareNoBuilder(t *testing.T) {
	first, _ := newBuilderTestApp(t)
	second, _ := newBuilderTestApp(t)

	if first.builder == second.builder {
		t.Error("two apps share a Builder")
	}
}

// The MCP promote tool rebuilds the vault's retrieval index, which its own scripts keep, and builds no embedding
// index: it has no Builder to share, and an index built by a query top-up is not one it can race. (A promotion that
// reindexed the embeddings too would have to be handed the session's Builder; this holds the reason it is not.)
func TestMCPPromoteTool_BuildsNoEmbeddingIndex(t *testing.T) {
	w := enterClockWorld(t)
	stateDir := t.TempDir()
	t.Setenv("LONGTERM_MEM_STATE_DIR", stateDir)
	store, err := engram.Open(w.dbPath)
	if err != nil {
		t.Fatalf("open the Engram database: %v", err)
	}
	defer store.Close()

	if _, err := (commands{newEmbedClient: testEmbedClient, clock: promoteGoldenClock}).mcpPromote(store)(context.Background(), promoteGoldenProject, 1); err != nil {
		t.Fatalf("the promote tool: %v", err)
	}

	entries, err := os.ReadDir(stateDir)
	if err != nil {
		t.Fatalf("read the state directory: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("the promote tool left %d entries in the state directory, want none: it builds no embedding index", len(entries))
	}
}
