package main

import (
	"context"
	"database/sql"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/embed"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/embedindex"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vecindex"
)

// TestLiveRows_MatchesLiveEngramRows is the RED/GREEN proof for the embedding
// index adapter's row source (issue #286's top-up wiring) over the real Engram
// store: the build use case's LiveRows must map exactly the live rows of one
// project into rows, carrying id/title/content through unchanged, and leave
// out a soft-deleted row -- the same R-020 scoping the command's own build
// relies on via store.ListObservations. (The mapping itself is held by
// internal/embedindex over a table; this holds what the store hands it.)
func TestLiveRows_MatchesLiveEngramRows(t *testing.T) {
	const project = "rundeps-topup-project"
	dbPath := newTestEngramDBWithObservation(t, project, "Live Title", "Live content.")

	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open fixture setup connection: %v", err)
	}
	res, err := conn.Exec(`INSERT INTO observations (session_id, sync_id, type, title, content, project, revision_count, pinned, created_at)
		 VALUES ('sess-1', 'sync-2', 'decision', ?, ?, ?, 1, 0, '2026-08-01 00:00:00')`,
		"Deleted Title", "Deleted content.", project)
	if err != nil {
		conn.Close()
		t.Fatalf("insert soft-deleted fixture observation: %v", err)
	}
	deletedID, err := res.LastInsertId()
	if err != nil {
		conn.Close()
		t.Fatalf("last insert id: %v", err)
	}
	if _, err := conn.Exec(`UPDATE observations SET deleted_at = ? WHERE id = ?`, "2026-08-02 00:00:00", deletedID); err != nil {
		conn.Close()
		t.Fatalf("soft-delete fixture observation: %v", err)
	}
	conn.Close()

	store, err := engram.Open(dbPath)
	if err != nil {
		t.Fatalf("engram.Open(%q): %v", dbPath, err)
	}
	t.Cleanup(func() { _ = store.Close() })

	rows, err := embedindex.LiveRows(store, project)
	if err != nil {
		t.Fatalf("LiveRows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %+v, want exactly the one live row (soft-deleted row must be excluded)", rows)
	}
	if rows[0].Title != "Live Title" || rows[0].Content != "Live content." {
		t.Fatalf("rows[0] = %+v, want the live fixture row's title/content carried through unchanged", rows[0])
	}
}

// TestEmbeddingIndexOf_WiresWhatTheProcessReadsAndWritesWith is the proof of the wiring of the embedding index
// adapter (issue #286's top-up): the repository keeps the indexes in the state directory the module keeps its
// derived state in, builds from the live rows of the Engram store, makes its embedder with the factory the
// commands were wired with (for the model the build is asked for), and reads through the cache it was given. How
// the adapter builds and caches is held by its own tests; this holds what the process hands it.
func TestEmbeddingIndexOf_WiresWhatTheProcessReadsAndWritesWith(t *testing.T) {
	const project = "rundeps-index-project"
	dbPath := newTestEngramDBWithObservation(t, project, "Live Title", "Live content.")
	store, err := engram.Open(dbPath)
	if err != nil {
		t.Fatalf("engram.Open(%q): %v", dbPath, err)
	}
	t.Cleanup(func() { _ = store.Close() })
	stateDir := t.TempDir()
	t.Setenv("LONGTERM_MEM_STATE_DIR", stateDir)
	cache := vecindex.NewLoadCache()
	var models []string
	factory := func(cfg embed.Config) (*embed.Client, error) {
		models = append(models, cfg.Model)
		return testEmbedClient(cfg)
	}
	index := embeddingIndexOf(store, queryWiring{newClient: factory, cache: cache, builder: &vecindex.Builder{}})
	before := testEmbedRequests.Load()

	if err := index.BuildEmbeddingIndex(context.Background(), project, "wired-model", vecindex.DefaultDimension, 2000); err != nil {
		t.Fatalf("BuildEmbeddingIndex: %v", err)
	}

	if len(models) != 1 || models[0] != "wired-model" {
		t.Errorf("the embedding client was made for %v, want the model the build was asked for", models)
	}
	if got := testEmbedRequests.Load() - before; got != 1 {
		t.Errorf("the embedding backend of the commands was asked %d times, want once: one live row", got)
	}
	built, err := vecindex.Load(vecindex.Dir(stateDir, project))
	if err != nil {
		t.Fatalf("the index is not in the state directory: %v", err)
	}
	if len(built.Manifest.Entries) != 1 || built.Manifest.Model != "wired-model" {
		t.Errorf("the index holds %+v, want the one live row, under the model of the build", built.Manifest)
	}
	loaded, err := index.LoadEmbeddingIndex(project)
	if err != nil || len(loaded.Entries) != 1 {
		t.Errorf("LoadEmbeddingIndex = (%+v, %v), want the index just built", loaded, err)
	}
	// Read through the cache the repository was given: a second read answers the vectors of the first, the same
	// memory, where a read from the disk would make them anew.
	again, err := index.LoadEmbeddingIndex(project)
	if err != nil || len(again.Vectors) != 1 || &again.Vectors[0][0] != &loaded.Vectors[0][0] {
		t.Errorf("the second read did not come from the cache the repository was given (%v)", err)
	}
}

// The locks that keep two top-ups of one index apart are the Builder's, and the repository of a call builds through
// the one the surface gave it: a session makes a repository for each call and gives every one of them the session's.
func TestEmbeddingIndexOf_BuildsThroughTheBuilderOfTheWiring(t *testing.T) {
	store, err := engram.Open(newTestEngramDBWithObservation(t, "p", "T", "C"))
	if err != nil {
		t.Fatalf("engram.Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	t.Setenv("LONGTERM_MEM_STATE_DIR", t.TempDir())
	session := &vecindex.Builder{}

	first := embeddingIndexOf(store, queryWiring{newClient: testEmbedClient, builder: session})
	second := embeddingIndexOf(store, queryWiring{newClient: testEmbedClient, builder: session})

	if first.Builder != session || second.Builder != session {
		t.Errorf("the repositories of two calls build through %p and %p, want the session's %p", first.Builder, second.Builder, session)
	}
}

// A run of the CLI serves one query, with no cache and a fresh embedding client, and has a Builder of its own for
// the top-up it may make: nothing is shared between two runs, which are two processes.
func TestCLIQueryWiring_HasNoCacheAndABuilderOfItsOwn(t *testing.T) {
	first, second := cliQueryWiring(testEmbedClient), cliQueryWiring(testEmbedClient)

	if first.cache != nil {
		t.Errorf("cache = %v, want none for a surface that serves one query", first.cache)
	}
	if first.builder == nil || first.embed == nil || first.newClient == nil {
		t.Errorf("wiring = %+v, want a Builder, an embedding function and a client factory", first)
	}
	if first.builder == second.builder {
		t.Error("two runs of the CLI share a Builder")
	}
}

// A repository built with no cache reads the disk every time, as the CLI's one query does.
func TestEmbeddingIndexOf_WithoutACacheReadsTheDisk(t *testing.T) {
	store, err := engram.Open(newTestEngramDBWithObservation(t, "p", "T", "C"))
	if err != nil {
		t.Fatalf("engram.Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	t.Setenv("LONGTERM_MEM_STATE_DIR", t.TempDir())

	index := embeddingIndexOf(store, queryWiring{newClient: testEmbedClient, builder: &vecindex.Builder{}})

	if index.Cache != nil {
		t.Errorf("Cache = %v, want none for a surface that serves one query", index.Cache)
	}
}
