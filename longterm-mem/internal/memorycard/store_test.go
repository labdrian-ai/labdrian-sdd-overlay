package memorycard

import (
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
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
		`insert into memories(id, project, type, title, body, topic_key, pinned, status, superseded_by, content_hash) values
			(1, 'p', 'decision', 'Old', 'old body', 'a/b', 1, 'superseded', 2, 'h1'),
			(2, 'p', 'decision', 'New', 'new body', null, 0, 'active', null, 'h2'),
			(3, 'p', 'learning', 'Loose', 'no row', null, 0, 'active', null, 'h3')`,
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
	if !m.Mapped || m.ContentHash != "h1" {
		t.Errorf("Mapped = %v, ContentHash = %q, want true and h1", m.Mapped, m.ContentHash)
	}

	n, ok, _ := store.MemoryByID(2)
	if !ok || n.Revision != 1 || n.EngramID != 0 || n.TopicKey != "" || !n.Mapped {
		t.Errorf("memory 2 = %+v, want revision 1, no Engram id (its import_map source is not engram), no topic", n)
	}
	if l, ok, _ := store.MemoryByID(3); !ok || l.Mapped || l.ContentHash != "h3" {
		t.Errorf("memory 3 = %+v, want no import_map row and hash h3", l)
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

// listFixture is the fixture plus an archived memory and a memory of another project.
func listFixture(t *testing.T) string {
	t.Helper()
	path := fixture(t)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close the fixture: %v", err)
		}
	})
	stmt := `insert into memories(id, project, type, title, body, topic_key, pinned, status, content_hash) values
		(4, 'p', 'decision', 'Gone', 'archived body', 'a/gone', 1, 'archived', 'h4'),
		(5, 'other', 'decision', 'Elsewhere', 'other project', 'a/else', 1, 'active', 'h5')`
	if _, err := db.Exec(stmt); err != nil {
		t.Fatalf("%s: %v", stmt, err)
	}
	return path
}

// The listing serves the memories promotion reads: the active and the superseded ones of the project, not the
// archived (memorycard's soft delete, as Engram's deleted_at is), each mapped exactly as MemoryByID maps it.
func TestListMemories_ListsTheProjectsLiveMemoriesMappedAsMemoryByID(t *testing.T) {
	store, err := Open(listFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close the store: %v", err)
		}
	})

	listed, err := store.ListMemories("p")
	if err != nil {
		t.Fatalf("ListMemories: %v", err)
	}
	var ids []int64
	for _, m := range listed {
		ids = append(ids, m.ID)
		want, ok, err := store.MemoryByID(m.ID)
		if err != nil || !ok {
			t.Fatalf("MemoryByID(%d) = %v, %v", m.ID, ok, err)
		}
		if !reflect.DeepEqual(m, want) {
			t.Errorf("listed memory %d = %+v, MemoryByID says %+v", m.ID, m, want)
		}
	}
	if !reflect.DeepEqual(ids, []int64{1, 2, 3}) {
		t.Fatalf("listed ids = %v, want [1 2 3]: active and superseded of project p in id order, no archived (4), no other project (5)", ids)
	}
	if listed[0].EngramID != 402 || listed[0].Revision != 3 || listed[0].SupersededByTitle != "New" {
		t.Errorf("memory 1 = %+v, want the bridge, the revision and the successor title MemoryByID gives", listed[0])
	}
	if none, err := store.ListMemories("nobody"); err != nil || len(none) != 0 {
		t.Errorf("ListMemories(nobody) = %v, %v, want nothing and no error", none, err)
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
