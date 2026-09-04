package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"

	_ "modernc.org/sqlite"
)

// newProbeFixtureStore builds an Engram database from the package's own
// schema fixture, applies mutate (a chance to break one table), and opens
// it the way the commands do.
func newProbeFixtureStore(t *testing.T, mutate string) *engram.Store {
	t.Helper()

	schema, err := os.ReadFile(filepath.Join("..", "..", "internal", "engram", "testdata", "schema.sql"))
	if err != nil {
		t.Fatalf("read engram schema fixture: %v", err)
	}
	dbPath := filepath.Join(t.TempDir(), "engram.db")
	setup, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open fixture db: %v", err)
	}
	if _, err := setup.Exec(string(schema)); err != nil {
		t.Fatalf("apply schema fixture: %v", err)
	}
	if mutate != "" {
		if _, err := setup.Exec(mutate); err != nil {
			t.Fatalf("apply fixture mutation %q: %v", mutate, err)
		}
	}
	if err := setup.Close(); err != nil {
		t.Fatalf("close fixture setup connection: %v", err)
	}

	store, err := engram.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// engramReadable answers one question -- "is Engram unavailable?" -- for
// the whole of sync's exit-code classification, and it used to answer it
// by probing ONE of the two tables longterm-mem reads. Observations are
// read by promote.Sync; relations are read by promote.Propagate ->
// Store.RelatedEdges, in the very same command, joined into the very same
// error value. An Engram failure that reaches only memory_relations (the
// table dropped, renamed by a migration, or unreadable) therefore left the
// single probe answering "readable" and the command answering exit 1
// (internal, "longterm-mem's own side went wrong") for a database that is
// exactly as unavailable as one whose observations table is gone.
//
// This is the reported finding's TWIN made explicit: one probe cannot
// stand for a corpus read through two tables.
func TestEngramReadable_ReportsUnavailableWhenOnlyRelationsAreUnreadable(t *testing.T) {
	store := newProbeFixtureStore(t, "DROP TABLE memory_relations")

	if engramReadable(store, "probe-project") {
		t.Fatal("engramReadable reported a readable Engram while memory_relations is gone: an Engram failure the relations read hits is classified exit 1 (internal) instead of exit 4 (engram_unavailable)")
	}
	if got := syncExitCode(store, "probe-project", nil); got != exitEngramUnavailable {
		t.Fatalf("syncExitCode = %d, want %d (engram_unavailable)", got, exitEngramUnavailable)
	}
}

// The original half of the pair, kept so widening the probe cannot be
// "fixed" by widening it into something that always answers false.
func TestEngramReadable_ReportsUnavailableWhenObservationsAreUnreadable(t *testing.T) {
	store := newProbeFixtureStore(t, "DROP TABLE observations")

	if engramReadable(store, "probe-project") {
		t.Fatal("engramReadable reported a readable Engram while observations is gone")
	}
}

// And the healthy case, which is what makes the two above mean anything: a
// complete database must still classify as readable, so a failed sync over
// a healthy Engram keeps answering exit 1 rather than blaming Engram.
func TestEngramReadable_ReportsReadableForAHealthyDatabase(t *testing.T) {
	store := newProbeFixtureStore(t, "")

	if !engramReadable(store, "probe-project") {
		t.Fatal("engramReadable reported an unavailable Engram for a complete schema; every non-Engram sync failure would now be reported as exit 4")
	}
	if got := syncExitCode(store, "probe-project", nil); got != exitInternal {
		t.Fatalf("syncExitCode = %d, want %d (internal)", got, exitInternal)
	}
}
