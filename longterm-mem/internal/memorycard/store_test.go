package memorycard

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// fixture builds a memorycard database with the tables of the schema this adapter reads.
func fixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "memory.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, stmt := range []string{
		`create table memories (id integer primary key autoincrement, project text not null, scope text not null default 'project',
			type text not null default 'manual', title text not null, body text not null, topic_key text, content_hash text not null default '',
			pinned integer not null default 0, status text not null default 'active', superseded_by integer, created_at text not null default '', updated_at text not null default '')`,
		`create table memory_versions (memory_id integer not null, version integer not null, title text not null, body text not null, saved_at text not null, primary key (memory_id, version))`,
		`create table import_map (source text not null, source_key text not null, part integer not null default 1, memory_id integer not null, primary key (source, source_key, part))`,
		`insert into memories(id, project, type, title, body, topic_key, pinned, status, superseded_by) values
			(1, 'p', 'decision', 'Old', 'old body', 'a/b', 1, 'superseded', 2),
			(2, 'p', 'decision', 'New', 'new body', null, 0, 'active', null)`,
		`insert into memory_versions values (1, 1, 'Old', 'v1', ''), (1, 2, 'Old', 'v2', '')`,
		`insert into import_map values ('engram', '402', 1, 1), ('notes', 'x', 1, 2)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	return path
}

func TestMemoryByID_MapsTheRowTheBridgeAndTheRevision(t *testing.T) {
	store, err := Open(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	m, ok, err := store.MemoryByID(1)
	if err != nil || !ok {
		t.Fatalf("MemoryByID(1) = %v, %v", ok, err)
	}
	if m.Title != "Old" || m.Body != "old body" || m.TopicKey != "a/b" || !m.Pinned || m.Status != "superseded" {
		t.Errorf("row = %+v", m)
	}
	if m.SupersededBy != 2 || m.SupersededByTitle != "New" {
		t.Errorf("superseded_by = %d (%q), want 2 (New)", m.SupersededBy, m.SupersededByTitle)
	}
	if m.Revision != 3 {
		t.Errorf("Revision = %d, want 3 (max version 2, plus one)", m.Revision)
	}
	if m.EngramID != 402 {
		t.Errorf("EngramID = %d, want 402", m.EngramID)
	}

	n, ok, _ := store.MemoryByID(2)
	if !ok || n.Revision != 1 || n.EngramID != 0 || n.TopicKey != "" {
		t.Errorf("memory 2 = %+v, want revision 1, no Engram id (its import_map source is not engram), no topic", n)
	}
	if _, ok, err := store.MemoryByID(99); ok || err != nil {
		t.Errorf("MemoryByID(99) = %v, %v, want not found and no error", ok, err)
	}
}

func TestOpen_IsReadOnly(t *testing.T) {
	path := fixture(t)
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.db.Exec(`update memories set title = 'x'`); err == nil {
		t.Fatal("a write through the adapter's connection succeeded")
	}
}

func TestOpen_AMissingDatabaseIsAnErrorAndCreatesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.db")
	if _, err := Open(path); err == nil {
		t.Fatal("Open of a missing database succeeded")
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("Open created the database it was asked to open")
	}
}
