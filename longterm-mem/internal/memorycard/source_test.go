package memorycard

import (
	"database/sql"
	"reflect"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// The Source adapter serves memorycard's memories as memory.Memory over the fixtures of store_test.go: memory 1 is
// superseded by 2 and imported from Engram observation 402, 2 is mapped by another source, 3 has no import_map
// row, 4 is archived and 5 belongs to another project.

func openSource(t *testing.T, path string) *Source {
	t.Helper()
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close the store: %v", err)
		}
	})
	return NewSource(store)
}

func card(id string) memory.Identity { return memory.Identity{Source: SourceName, ID: id} }

func wantMemory1() memory.Memory {
	return memory.Memory{
		Identity: card("1"), Project: "p", Type: "decision", Title: "Old", Body: "old body", TopicKey: "a/b", Pinned: true,
		Status: memory.StatusSuperseded, Successor: card("2"), SuccessorTitle: "New", Revision: 3,
		Lineage: []memory.Identity{{Source: "engram", ID: "402"}},
	}
}

func wantMemory2() memory.Memory {
	return memory.Memory{Identity: card("2"), Project: "p", Type: "decision", Title: "New", Body: "new body", Status: memory.StatusActive, Revision: 1}
}

func wantMemory4() memory.Memory {
	return memory.Memory{
		Identity: card("4"), Project: "p", Type: "decision", Title: "Gone", Body: "archived body", TopicKey: "a/gone", Pinned: true,
		Status: memory.StatusArchived, Revision: 1,
	}
}

func TestSourceMapsAMemoryWithItsStatusSuccessorRevisionAndEngramLineage(t *testing.T) {
	source := openSource(t, listFixture(t))

	got, ok, err := source.ByID("1")
	if err != nil || !ok {
		t.Fatalf("ByID(1) = %v, %v", ok, err)
	}
	if want := wantMemory1(); !reflect.DeepEqual(got, want) {
		t.Errorf("ByID(1) = %+v\nwant %+v", got, want)
	}
	got, ok, err = source.ByID("4")
	if err != nil || !ok {
		t.Fatalf("ByID(4) = %v, %v", ok, err)
	}
	if want := wantMemory4(); !reflect.DeepEqual(got, want) {
		t.Errorf("ByID(4) = %+v\nwant %+v: an archived memory is served by id", got, want)
	}
	for _, id := range []string{"99", "not-a-number"} {
		if got, ok, err := source.ByID(id); ok || err != nil {
			t.Errorf("ByID(%q) = %+v, %v, %v, want not found and no error", id, got, ok, err)
		}
	}
}

func TestSourceCandidatesAreTheActiveAndSupersededAndMemoriesIsEveryStatus(t *testing.T) {
	source := openSource(t, listFixture(t))

	candidates, err := source.Candidates("p")
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 3 || !reflect.DeepEqual(candidates[0], wantMemory1()) || !reflect.DeepEqual(candidates[1], wantMemory2()) {
		t.Errorf("Candidates(p) = %+v, want memories 1, 2 and 3 in id order (no archived 4, no other project's 5)", candidates)
	}
	all, err := source.Memories("p")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 4 || !reflect.DeepEqual(all[0], wantMemory1()) || !reflect.DeepEqual(all[3], wantMemory4()) {
		t.Errorf("Memories(p) = %+v, want memories 1 to 4 in id order, the archived one last and mapped as ByID maps it", all)
	}
}

// kinFixture is the fixture plus the memories the lineage rules read. 6 is part 2 of Engram observation 402 (whose
// part 1 is memory 1), and a third part's row names a memory that is gone. 8 and 12 have no import_map row and hold
// the content hash "hm", as do the memories 9 (imported from observation 500), 10 (mapped by another source), 15
// (imported from 502, archived), 11 (another project's) and 16 (no row, so no twin); 13 has no row and 14 has one, and
// both have no content hash.
func kinFixture(t *testing.T) string {
	t.Helper()
	path := fixture(t)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("close the fixture: %v", err)
		}
	}()
	for _, stmt := range []string{
		`insert into memories(id, project, type, title, body, status, content_hash) values
			(6, 'p', 'decision', 'Part two', 'part two body', 'active', 'h6'),
			(8, 'p', 'decision', 'Mirror', 'mirror body', 'active', 'hm'),
			(9, 'p', 'decision', 'Twin', 'twin body', 'active', 'hm'),
			(10, 'p', 'decision', 'Twin by notes', 'notes body', 'active', 'hm'),
			(11, 'other', 'decision', 'Twin elsewhere', 'other body', 'active', 'hm'),
			(12, 'p', 'decision', 'Second mirror', 'second body', 'active', 'hm'),
			(13, 'p', 'decision', 'No hash', 'no hash body', 'active', ''),
			(14, 'p', 'decision', 'No hash mapped', 'no hash mapped body', 'active', ''),
			(15, 'p', 'decision', 'Twin archived', 'archived twin body', 'archived', 'hm')`,
		`insert into import_map values ('engram', '402', 2, 6), ('engram', '402', 3, 99), ('engram', '500', 1, 9), ('notes', 'n1', 1, 10),
			('engram', '501', 1, 11), ('engram', '502', 1, 15), ('engram', '503', 1, 14)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	return path
}

func engramObservation(id string) memory.Identity { return memory.Identity{Source: "engram", ID: id} }

// The other parts of an observation are lineage of each other, so a page re-keyed to one part's memory still
// bridges the others; a part whose row names a memory that is gone names nothing.
func TestSourceLineageNamesTheObservationAndTheOtherMemoriesImportedFromIt(t *testing.T) {
	source := openSource(t, kinFixture(t))

	for id, want := range map[string][]memory.Identity{
		"1": {engramObservation("402"), card("6")},
		"6": {engramObservation("402"), card("1")},
		"2": nil,
	} {
		m, ok, err := source.ByID(id)
		if err != nil || !ok {
			t.Fatalf("ByID(%s) = %v, %v", id, ok, err)
		}
		if !reflect.DeepEqual(m.Lineage, want) {
			t.Errorf("memory %s lineage = %v, want %v", id, m.Lineage, want)
		}
	}
}

// A memory with no import_map row has as mirrors the memories of its project with the same content hash that have
// one, whatever their status, and their observations: found in memorycard alone.
func TestSourceMirrorsAreTheMappedMemoriesOfTheSameContentAndTheirObservations(t *testing.T) {
	source := openSource(t, kinFixture(t))

	wantTwins := []memory.Identity{card("9"), engramObservation("500"), card("10"), card("15"), engramObservation("502")}
	for _, id := range []string{"8", "12"} {
		m, ok, err := source.ByID(id)
		if err != nil || !ok {
			t.Fatalf("ByID(%s) = %v, %v", id, ok, err)
		}
		if !reflect.DeepEqual(m.Mirrored, wantTwins) || m.Lineage != nil {
			t.Errorf("memory %s mirrors = %v, lineage = %v, want the mapped twins %v and no lineage: another project's memory 11 and the unmapped 8 and 12 are no twins", id, m.Mirrored, m.Lineage, wantTwins)
		}
	}
	// A mapped memory has an import record, not mirrors; a memory with no content hash holds the content of nothing.
	for _, id := range []string{"9", "10", "14", "13"} {
		m, ok, err := source.ByID(id)
		if err != nil || !ok {
			t.Fatalf("ByID(%s) = %v, %v", id, ok, err)
		}
		if m.Mirrored != nil {
			t.Errorf("memory %s mirrors = %v, want none", id, m.Mirrored)
		}
	}
}

// Every memory of a listing carries the lineage and mirrors ByID gives it.
func TestSourceListingsCarryTheLineageAndMirrorsByIDGives(t *testing.T) {
	source := openSource(t, kinFixture(t))

	all, err := source.Memories("p")
	if err != nil {
		t.Fatal(err)
	}
	for _, listed := range all {
		want, ok, err := source.ByID(listed.Identity.ID)
		if err != nil || !ok {
			t.Fatalf("ByID(%s) = %v, %v", listed.Identity.ID, ok, err)
		}
		if !reflect.DeepEqual(listed, want) {
			t.Errorf("listed memory %s = %+v, ByID says %+v", listed.Identity.ID, listed, want)
		}
	}
}
