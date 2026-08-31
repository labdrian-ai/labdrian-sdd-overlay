package promote

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"

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
			   (session_id, sync_id, type, title, content, project, revision_count, pinned, created_at, deleted_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, COALESCE(NULLIF(?, ''), datetime('now')), NULLIF(?, ''))`,
			"sess-1", r.syncID, r.obsType, r.title, r.content, r.project, r.revisionCount, r.pinned, r.createdAt, r.deletedAt,
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
func seedPromotedPage(t *testing.T, vaultRoot string, store PrecedenceStore, obs engram.Observation, addr string) Page {
	t.Helper()
	page, err := EmitPage(obs, addr, nil)
	if err != nil {
		t.Fatalf("EmitPage (seed): %v", err)
	}
	writePromotedPage(t, vaultRoot, page)
	seedPrecedence(store, page)
	return page
}

// uniqueAllocateAddressFixture is a fake scripts/allocate-address.sh that
// allocates a fresh, distinct address on every invocation (a persistent
// counter file under the vault root) -- unlike address_test.go's
// allocateAddressFixture (always "c-000042"), a multi-observation Sync run
// needs one real address per never-promoted observation, not a collision.
const uniqueAllocateAddressFixture = `#!/bin/sh
count_file=".allocate-counter"
n=0
if [ -f "$count_file" ]; then
  n=$(cat "$count_file")
fi
n=$((n+1))
echo "$n" > "$count_file"
printf 'c-%06d\n' "$n"
`

// TestSync: R-009's three scenarios, table-driven.
func TestSync(t *testing.T) {
	cases := []struct {
		name            string
		seedRevision    int // 0 means never promoted
		currentRevision int
		wantPromoted    bool
	}{
		{name: "Never-promoted eligible observation is promoted", seedRevision: 0, currentRevision: 1, wantPromoted: true},
		{name: "Revised eligible observation is re-promoted", seedRevision: 2, currentRevision: 3, wantPromoted: true},
		{name: "Unchanged eligible observation is a no-op", seedRevision: 3, currentRevision: 3, wantPromoted: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vaultRoot := t.TempDir()
			fixedNow(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))
			writeAllocateScript(t, vaultRoot, allocateAddressFixture)

			store, ids := newFixtureEngramStore(t, []fixtureObs{
				{title: "Eligible Decision", content: "Body content.", project: "labdrian-sdd-overlay", obsType: "decision", revisionCount: tc.currentRevision, syncID: "sync-1"},
			}, nil)
			id := ids[0]

			precedence := PrecedenceStore{}
			var seededPage Page
			if tc.seedRevision > 0 {
				seedObs := engram.Observation{ID: id, Type: "decision", Title: "Eligible Decision", Content: "Body content.", Project: "labdrian-sdd-overlay", RevisionCount: tc.seedRevision}
				seededPage = seedPromotedPage(t, vaultRoot, precedence, seedObs, "c-000042")
			}

			w := &Writer{VaultRoot: vaultRoot, Store: precedence}
			report, err := Sync(context.Background(), Deps{Engram: store, Writer: w}, "labdrian-sdd-overlay")
			if err != nil {
				t.Fatalf("Sync: %v", err)
			}

			if tc.wantPromoted {
				if len(report.Promoted) != 1 {
					t.Fatalf("Promoted = %+v, want exactly 1 entry", report.Promoted)
				}
				got, err := os.ReadFile(filepath.Join(vaultRoot, report.Promoted[0].Page.Path))
				if err != nil {
					t.Fatalf("read promoted page: %v", err)
				}
				if !strings.Contains(string(got), "Body content.") {
					t.Fatalf("promoted page missing observation content; got:\n%s", got)
				}
				return
			}

			if len(report.Promoted) != 0 {
				t.Fatalf("Promoted = %+v, want none (no-op)", report.Promoted)
			}
			got, err := os.ReadFile(filepath.Join(vaultRoot, seededPage.Path))
			if err != nil {
				t.Fatalf("read seeded page: %v", err)
			}
			if string(got) != seededPage.Frontmatter+seededPage.Body {
				t.Fatalf("seeded page content changed despite being a declared no-op case; got:\n%s", got)
			}
		})
	}
}

// TestSync_IndexAndSyncStateReflectCompletion: R-031's scenario. A sync run
// promoting three observations must rebuild the vault index and record the
// completion timestamp in the vault's own sync-state record.
func TestSync_IndexAndSyncStateReflectCompletion(t *testing.T) {
	vaultRoot := t.TempDir()
	fixedNow(t, time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC))
	writeAllocateScript(t, vaultRoot, uniqueAllocateAddressFixture)

	store, _ := newFixtureEngramStore(t, []fixtureObs{
		{title: "Decision One", content: "Body one.", project: "labdrian-sdd-overlay", obsType: "decision", revisionCount: 1},
		{title: "Architecture Two", content: "Body two.", project: "labdrian-sdd-overlay", obsType: "architecture", revisionCount: 1},
		{title: "Pattern Three", content: "Body three.", project: "labdrian-sdd-overlay", obsType: "pattern", revisionCount: 1},
	}, nil)

	var rebuildCalled bool
	w := &Writer{VaultRoot: vaultRoot, Store: PrecedenceStore{}}
	deps := Deps{
		Engram: store,
		Writer: w,
		RebuildIndex: func(ctx context.Context) error {
			rebuildCalled = true
			return nil
		},
	}

	report, err := Sync(context.Background(), deps, "labdrian-sdd-overlay")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(report.Promoted) != 3 {
		t.Fatalf("Promoted = %+v, want 3", report.Promoted)
	}
	if !rebuildCalled {
		t.Fatal("RebuildIndex was not called; the vault index must be rebuilt after promotion (R-031)")
	}

	entries, err := os.ReadDir(filepath.Join(vaultRoot, pagePathPrefix))
	if err != nil {
		t.Fatalf("read %s: %v", pagePathPrefix, err)
	}
	if len(entries) != 3 {
		t.Fatalf("wiki/memory has %d entries, want 3 (the index rebuild's input must reflect all three new pages)", len(entries))
	}

	stateData, err := os.ReadFile(filepath.Join(vaultRoot, syncStateRelPath))
	if err != nil {
		t.Fatalf("read sync-state record: %v", err)
	}
	if !strings.Contains(string(stateData), "2026-08-31T12:00:00Z") {
		t.Fatalf("sync-state record missing the completion timestamp; got:\n%s", stateData)
	}
}

// TestSync_OneFailingObservationDoesNotWedgeTheRun: a single unpromotable
// observation must not abort the run. ListObservations returns the same
// set every time, so aborting mid-loop makes one persistently broken
// observation a poison pill: every later observation is never attempted,
// the index is never rebuilt, the sync-state record is never written, and
// each retry re-walks the same prefix and fails identically -- wedging the
// project's sync indefinitely with no way for the caller to tell "nothing
// left to promote" from "stuck behind observation N" (review finding
// R4-poison-pill-abort).
func TestSync_OneFailingObservationDoesNotWedgeTheRun(t *testing.T) {
	vaultRoot := t.TempDir()
	fixedNow(t, time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC))
	writeAllocateScript(t, vaultRoot, uniqueAllocateAddressFixture)

	store, ids := newFixtureEngramStore(t, []fixtureObs{
		{title: "Healthy One", content: "Body one.", project: "labdrian-sdd-overlay", obsType: "decision", revisionCount: 1},
		{title: "Poison Pill", content: "Body two.", project: "labdrian-sdd-overlay", obsType: "decision", revisionCount: 2},
		{title: "Healthy Three", content: "Body three.", project: "labdrian-sdd-overlay", obsType: "pattern", revisionCount: 1},
	}, nil)

	// The middle observation is already promoted to a page whose
	// engram_revision cannot be parsed, so deciding whether it needs
	// re-promotion fails every single run.
	memoryDir := filepath.Join(vaultRoot, pagePathPrefix)
	if err := os.MkdirAll(memoryDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", memoryDir, err)
	}
	poisoned := "---\ntype: concept\ntitle: \"Poison Pill\"\naddress: c-000500\nstatus: seed\nengram_id: " +
		strconv.FormatInt(ids[1], 10) + "\nengram_revision: not-a-number\nproject: labdrian-sdd-overlay\n---\n\nBody two.\n"
	if err := os.WriteFile(filepath.Join(memoryDir, "c-000500.md"), []byte(poisoned), 0o644); err != nil {
		t.Fatalf("write poisoned page: %v", err)
	}

	var rebuildCalled bool
	w := &Writer{VaultRoot: vaultRoot, Store: PrecedenceStore{}}
	deps := Deps{
		Engram:       store,
		Writer:       w,
		RebuildIndex: func(ctx context.Context) error { rebuildCalled = true; return nil },
	}

	report, err := Sync(context.Background(), deps, "labdrian-sdd-overlay")
	if err == nil {
		t.Fatal("Sync = nil error, want the failing observation surfaced so the caller cannot mistake it for a clean run")
	}
	if len(report.Promoted) != 2 {
		t.Fatalf("Promoted = %d observations, want the 2 healthy ones promoted despite the failing one", len(report.Promoted))
	}
	if len(report.Failed) != 1 {
		t.Fatalf("Failed = %+v, want exactly the one unpromotable observation", report.Failed)
	}
	if report.Failed[0].ObservationID != ids[1] {
		t.Fatalf("Failed[0].ObservationID = %d, want %d", report.Failed[0].ObservationID, ids[1])
	}
	if !rebuildCalled {
		t.Error("RebuildIndex was not called; a partially failed run still rebuilt real pages and must refresh the index")
	}
	if _, err := os.Stat(filepath.Join(vaultRoot, syncStateRelPath)); err != nil {
		t.Errorf("sync-state record missing after a partially failed run: %v", err)
	}
}
