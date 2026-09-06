package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// newTestEngramDBWithObservation builds a temp Engram-schema database with
// one observation for project, mirroring main_test.go's own fixture
// convention (schema.sql + a direct INSERT), and returns its path.
func newTestEngramDBWithObservation(t *testing.T, project, title, content string) string {
	t.Helper()
	schema, err := os.ReadFile(filepath.Join("..", "..", "internal", "engram", "testdata", "schema.sql"))
	if err != nil {
		t.Fatalf("read engram schema fixture: %v", err)
	}
	dbPath := filepath.Join(t.TempDir(), "engram.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open fixture db: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(string(schema)); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO observations (session_id, sync_id, type, title, content, project, revision_count, pinned, created_at)
		 VALUES ('sess-1', 'sync-1', 'decision', ?, ?, ?, 1, 0, '2026-08-01 00:00:00')`, title, content, project); err != nil {
		t.Fatalf("insert observation: %v", err)
	}
	return dbPath
}

// fakeOllamaServer answers every /api/embeddings POST with a fixed-length
// vector, never inspecting the prompt -- this test proves the CLI wiring
// end-to-end (R-069's "explicit index --embeddings invocation"), not the
// embedding client itself (already covered by internal/embed's own tests).
func fakeOllamaServer(t *testing.T, dim int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vec := make([]float64, dim)
		for i := range vec {
			vec[i] = 0.5
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"embedding": vec})
	}))
}

// TestCmdIndexEmbeddings_BuildsIndexOnDisk is the RED/GREEN proof for task
// 3.4's CLI wiring: `index --embeddings` reads live Engram rows, embeds
// them through the (fake, loopback) backend, and persists a loadable
// vecindex under the resolved state directory -- exercising the real
// runtime path a query later PR will read from, not just Build in
// isolation (already covered by internal/vecindex's own tests).
func TestCmdIndexEmbeddings_BuildsIndexOnDisk(t *testing.T) {
	const project = "cmd-index-embeddings-project"
	dbPath := newTestEngramDBWithObservation(t, project, "Title One", "Content one.")

	server := fakeOllamaServer(t, 4)
	defer server.Close()

	stateDir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LONGTERM_MEM_ENGRAM_DB", dbPath)
	t.Setenv("LONGTERM_MEM_STATE_DIR", stateDir)

	exit := run([]string{
		"index", "--project", project, "--embeddings",
		"--embed-endpoint", server.URL,
		"--embed-dimension", "4",
	})
	if exit != exitOK {
		t.Fatalf("run([index --embeddings ...]) = %d, want %d (exitOK)", exit, exitOK)
	}

	indexDir := filepath.Join(stateDir, "index", project)
	if _, err := os.Stat(filepath.Join(indexDir, "manifest.json")); err != nil {
		t.Fatalf("manifest.json was not written under %s: %v", indexDir, err)
	}
	if _, err := os.Stat(filepath.Join(indexDir, "vectors.blob")); err != nil {
		t.Fatalf("vectors.blob was not written under %s: %v", indexDir, err)
	}
}

// TestCmdIndexEmbeddings_NonLoopbackRefusedWithoutFlag proves the CLI
// wires --allow-remote-embedder through to embed.NewClient rather than
// silently swallowing the refusal (R-071).
func TestCmdIndexEmbeddings_NonLoopbackRefusedWithoutFlag(t *testing.T) {
	const project = "cmd-index-embeddings-remote-project"
	dbPath := newTestEngramDBWithObservation(t, project, "Title", "Content.")

	stateDir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LONGTERM_MEM_ENGRAM_DB", dbPath)
	t.Setenv("LONGTERM_MEM_STATE_DIR", stateDir)

	exit := run([]string{
		"index", "--project", project, "--embeddings",
		"--embed-endpoint", "http://93.184.216.34:11434",
	})
	if exit == exitOK {
		t.Fatal("run([index --embeddings --embed-endpoint <non-loopback>]) = exitOK, want a refusal: --allow-remote-embedder was not passed")
	}
}
