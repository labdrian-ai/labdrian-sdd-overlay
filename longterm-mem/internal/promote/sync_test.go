package promote

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"

	_ "modernc.org/sqlite"
)

// fixtureObs is one row newFixtureEngramStore inserts before opening the
// (read-only) production engram.Store -- promote's own copy of
// query_test.go's newFixtureEngramStore convention (shared schema.sql,
// package-local builder), extended with the eligibility/revision/
// timestamp/soft-delete fields Sync and Propagate need that query's
// fixture never touches. An empty createdAt/deletedAt lets SQLite's own
// defaults apply (datetime('now') / NULL).
type fixtureObs struct {
	title, content, project, obsType string
	revisionCount                    int
	pinned                           bool
	syncID                           string
	createdAt                        string
	deletedAt                        string
	topicKey                         string
}

// fixtureRelation is one memory_relations row newFixtureEngramStore
// inserts, always pre-judged so Store.RelatedEdges (relations.go) surfaces
// it without a separate judgment-status fixture.
type fixtureRelation struct {
	syncID, sourceSyncID, targetSyncID, relation string
}

// newFixtureEngramStore builds a temp SQLite DB from internal/engram's own
// schema.sql fixture and opens it through the real production engram.Open
// path, returning the auto-assigned id of each inserted observation row in
// order -- Sync's revised/unchanged cases and Propagate's supersession case
// need a real id/sync_id to pre-seed a promoted page's engram_id
// frontmatter field, and a relation's endpoints, against.
func newFixtureEngramStore(t *testing.T, rows []fixtureObs, relations []fixtureRelation) (*engram.Store, []int64) {
	t.Helper()

	schema, err := os.ReadFile(filepath.Join("..", "engram", "testdata", "schema.sql"))
	if err != nil {
		t.Fatalf("read engram schema fixture: %v", err)
	}
	dbPath := filepath.Join(t.TempDir(), "engram.db")
	setup, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open fixture setup connection: %v", err)
	}
	if _, err := setup.Exec(string(schema)); err != nil {
		setup.Close()
		t.Fatalf("apply engram schema fixture: %v", err)
	}

	ids := make([]int64, len(rows))
	for i, r := range rows {
		res, err := setup.Exec(
			`INSERT INTO observations
			   (session_id, sync_id, type, title, content, project, revision_count, pinned, created_at, deleted_at, topic_key)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, COALESCE(NULLIF(?, ''), datetime('now')), NULLIF(?, ''), NULLIF(?, ''))`,
			"sess-1", r.syncID, r.obsType, r.title, r.content, r.project, r.revisionCount, r.pinned, r.createdAt, r.deletedAt, r.topicKey,
		)
		if err != nil {
			setup.Close()
			t.Fatalf("insert fixture observation %q: %v", r.title, err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			setup.Close()
			t.Fatalf("last insert id for %q: %v", r.title, err)
		}
		ids[i] = id
	}
	for _, r := range relations {
		if _, err := setup.Exec(
			`INSERT INTO memory_relations (sync_id, source_id, target_id, relation, judgment_status) VALUES (?, ?, ?, ?, 'judged')`,
			r.syncID, r.sourceSyncID, r.targetSyncID, r.relation,
		); err != nil {
			setup.Close()
			t.Fatalf("insert fixture relation %q: %v", r.syncID, err)
		}
	}
	setup.Close()

	store, err := engram.Open(dbPath)
	if err != nil {
		t.Fatalf("engram.Open(%q): %v", dbPath, err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, ids
}

// seedPromotedPage writes obs as an already-promoted page at address addr,
// records its precedence entry in store, and returns the written Page --
// TestSync's revised/unchanged cases, and Propagate's tests, build on this
// to simulate a promotion Sync/Propagate must decide whether to touch.
func seedPromotedPage(t *testing.T, vaultRoot string, store PrecedenceStore, obs memory.Observation, addr string) Page {
	t.Helper()
	page, err := EmitPage(obsMemory(obs), addr, nil, testInstant)
	if err != nil {
		t.Fatalf("EmitPage (seed): %v", err)
	}
	writePromotedPage(t, vaultRoot, page)
	seedPrecedence(store, page)
	return page
}
