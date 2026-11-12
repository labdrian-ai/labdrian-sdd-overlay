package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memorycard"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/promote"
)

// Both adapters satisfy the port promotion owns, and promotion names neither.
var (
	_ promote.Source = (*engram.Source)(nil)
	_ promote.Source = (*memorycard.Source)(nil)
)

// lineageWorld builds one Engram database and one memorycard database that hold the same pieces of memory:
// observation 1 was imported into memory 10 (an import_map row), observation 2 was mirrored by hand into memory
// 11 (no row, the same content), and memory 12 has no counterpart.
func lineageWorld(t *testing.T) (*engram.Store, *memorycard.Store) {
	t.Helper()
	dir := realTempDir(t)

	schema, err := os.ReadFile(filepath.Join("..", "..", "internal", "engram", "testdata", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	engramPath := filepath.Join(dir, "engram.db")
	cardPath := filepath.Join(dir, "memory.db")
	for path, stmts := range map[string][]string{
		engramPath: {
			string(schema),
			`insert into observations (id, sync_id, session_id, type, title, content, project, revision_count, created_at) values
				(1, 's1', 'x', 'decision', 'Imported', 'Imported body', 'p', 1, '2026-01-01 00:00:00'),
				(2, 's2', 'x', 'decision', 'Mirrored', 'Mirrored   body', 'p', 1, '2026-01-02 00:00:00'),
				(3, 's3', 'x', 'decision', 'Split', 'Split body', 'p', 1, '2026-01-03 00:00:00'),
				(4, 's4', 'x', 'decision', 'Parts', 'Parts body', 'p', 1, '2026-01-04 00:00:00')`,
		},
		cardPath: {
			`create table memories (id integer primary key autoincrement, project text not null, scope text not null default 'project',
				type text not null default 'manual', title text not null, body text not null, topic_key text, content_hash text not null default '',
				pinned integer not null default 0, status text not null default 'active', superseded_by integer, created_at text not null default '', updated_at text not null default '')`,
			`create table memory_versions (memory_id integer not null, version integer not null, title text not null, body text not null, saved_at text not null, primary key (memory_id, version))`,
			`create table import_map (source text not null, source_key text not null, part integer not null default 1, memory_id integer not null, primary key (source, source_key, part))`,
			`insert into memories(id, project, type, title, body, content_hash) values
				(10, 'p', 'decision', 'Imported', 'Imported body', 'ignored'),
				(11, 'p', 'decision', 'Mirrored', 'Mirrored body', '` + memory.CardContentHash("Mirrored", "Mirrored   body") + `'),
				(12, 'p', 'decision', 'Alone', 'Alone body', '` + memory.CardContentHash("Alone", "Alone body") + `'),
				(13, 'q', 'decision', 'Split', 'Split body', 'x'),
				(14, 'q', 'decision', 'Parts one', 'Parts body', 'y'),
				(15, 'q', 'decision', 'Parts two', 'Parts body', 'z')`,
			// Observation 3 was imported with a dangling part 1; observation 4 as two parts.
			`insert into import_map values ('engram', '1', 1, 10), ('engram', '3', 1, 99), ('engram', '3', 2, 13), ('engram', '4', 1, 14), ('engram', '4', 2, 15)`,
		},
	} {
		db, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		for _, stmt := range stmts {
			if _, err := db.Exec(stmt); err != nil {
				t.Fatalf("%s: %v", stmt, err)
			}
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	}
	engramStore, err := engram.Open(engramPath)
	if err != nil {
		t.Fatal(err)
	}
	cards, err := memorycard.Open(cardPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = engramStore.Close()
		_ = cards.Close()
	})
	return engramStore, cards
}

// lineageOf is the identities of the other store a memory is tied to, by an import record or by its content.
func lineageOf(t *testing.T, source promote.Source, id string) []memory.Identity {
	t.Helper()
	m := memoryOf(t, source, id)
	return append(append([]memory.Identity(nil), m.Lineage...), m.Mirrored...)
}

func memoryOf(t *testing.T, source promote.Source, id string) memory.Memory {
	t.Helper()
	m, ok, err := source.ByID(id)
	if err != nil || !ok {
		t.Fatalf("ByID(%s) = %v, %v", id, ok, err)
	}
	return m
}

// Lineage runs both ways: a memorycard memory names the observation it was imported from (its import_map row) and
// the observations that hold its content when it has no row; an observation names the memory imported from it.
func TestLineageRunsBothWaysBetweenTheTwoStores(t *testing.T) {
	engramStore, cards := lineageWorld(t)
	engramSource := newEngramSource(engramStore, cards)
	cardSource := newMemorycardSource(cards, engramStore)

	want := map[string][]memory.Identity{
		"10": {{Source: "engram", ID: "1"}},
		"11": {{Source: "engram", ID: "2"}},
		"12": nil,
	}
	for id, lineage := range want {
		if got := lineageOf(t, cardSource, id); !reflect.DeepEqual(got, lineage) {
			t.Errorf("memorycard memory %s lineage = %v, want %v", id, got, lineage)
		}
	}
	if got := lineageOf(t, engramSource, "1"); !reflect.DeepEqual(got, []memory.Identity{{Source: "memorycard", ID: "10"}}) {
		t.Errorf("observation 1 lineage = %v, want memorycard memory 10 (its import_map row, read in reverse)", got)
	}
	if got := lineageOf(t, engramSource, "2"); !reflect.DeepEqual(got, []memory.Identity{{Source: "memorycard", ID: "11"}}) {
		t.Errorf("observation 2 lineage = %v, want memorycard memory 11: the memory mirrored by hand holds its content and has no import_map row, so the twin rule runs in reverse", got)
	}
	// An import record names a lineage identity; content alone names a mirror, which a refusal says so about.
	if m := memoryOf(t, engramSource, "1"); len(m.Lineage) != 1 || len(m.Mirrored) != 0 {
		t.Errorf("observation 1: lineage = %v, mirrors = %v, want its import record as lineage and no mirror", m.Lineage, m.Mirrored)
	}
	if m := memoryOf(t, engramSource, "2"); len(m.Lineage) != 0 || len(m.Mirrored) != 1 {
		t.Errorf("observation 2: lineage = %v, mirrors = %v, want the memory that only holds its content as a mirror", m.Lineage, m.Mirrored)
	}
	if got := lineageOf(t, engramSource, "3"); !reflect.DeepEqual(got, []memory.Identity{{Source: "memorycard", ID: "13"}}) {
		t.Errorf("observation 3 lineage = %v, want memory 13: a dangling part 1 row hides nothing", got)
	}
	if got := lineageOf(t, engramSource, "4"); !reflect.DeepEqual(got, []memory.Identity{{Source: "memorycard", ID: "14"}, {Source: "memorycard", ID: "15"}}) {
		t.Errorf("observation 4 lineage = %v, want the memory of every part, 14 and 15", got)
	}

	// Every memory of the project comes through a listing with its lineage as ByID gives it.
	listed, err := cardSource.Candidates("p")
	if err != nil || len(listed) != 3 {
		t.Fatalf("Candidates(p) = %v, %v", listed, err)
	}
	for _, m := range listed {
		if got := append(append([]memory.Identity(nil), m.Lineage...), m.Mirrored...); !reflect.DeepEqual(got, want[m.Identity.ID]) {
			t.Errorf("listed memory %s lineage = %v, want %v", m.Identity.ID, got, want[m.Identity.ID])
		}
	}
}
