package memorycard

import (
	"database/sql"
	"reflect"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// kinFixture is the fixture plus the memories the lineage and mirror rules read, all of project p unless said.
//
//   - Observation 402 was imported as two parts, memories 1 (title "Old", body "old body") and 6 ("part two
//     body"); a third part's row names a memory that is gone. Memory 8 was mirrored by hand with the text of the
//     whole observation, "old body" and "part two body" in two paragraphs, and has no content hash.
//   - Observation 500 is memory 9, whose stored content hash is wrong; observation 502 is memory 15, archived,
//     with the same text; memory 12 is a hand mirror of that text with no content hash. Memory 10 is mapped by
//     another source with the same text, memory 11 is another project's import of it (observation 501).
//   - Observation 600 was cut in the middle of a word into memories 16 and 17; memory 18 is the whole text.
//   - Memory 13 is a hand mirror of nothing; memory 14 is imported (observation 503) and holds nothing of 13's.
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
			(6, 'p', 'decision', 'Old (part 2/2)', 'part two body', 'active', 'wrong6'),
			(8, 'p', 'decision', 'Old', 'old body

part two body', 'active', ''),
			(9, 'p', 'decision', 'Twin', 'twin body', 'active', 'wrong9'),
			(10, 'p', 'decision', 'Twin', 'twin body', 'active', ''),
			(11, 'other', 'decision', 'Twin', 'twin body', 'active', ''),
			(12, 'p', 'decision', 'Twin', 'twin body', 'active', ''),
			(13, 'p', 'decision', 'Nothing', 'nothing body', 'active', ''),
			(14, 'p', 'decision', 'Imported alone', 'alone body', 'active', ''),
			(15, 'p', 'decision', 'Twin', 'twin body', 'archived', ''),
			(16, 'p', 'decision', 'Cut (part 1/2)', 'alpha beta gam', 'active', ''),
			(17, 'p', 'decision', 'Cut (part 2/2)', 'ma delta', 'active', ''),
			(18, 'p', 'decision', 'Cut', 'alpha beta gamma delta', 'active', '')`,
		`update memories set title = 'Old (part 1/2)' where id = 1`,
		`insert into import_map values ('engram', '402', 2, 6), ('engram', '402', 3, 99), ('engram', '500', 1, 9), ('notes', 'n1', 1, 10),
			('engram', '501', 1, 11), ('engram', '502', 1, 15), ('engram', '503', 1, 14), ('engram', '600', 1, 16), ('engram', '600', 2, 17)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	return path
}

func engramObservation(id string) memory.Identity { return memory.Identity{Source: "engram", ID: id} }

func memoryOf(t *testing.T, source *Source, id string) memory.Memory {
	t.Helper()
	m, ok, err := source.ByID(id)
	if err != nil || !ok {
		t.Fatalf("ByID(%s) = %v, %v", id, ok, err)
	}
	return m
}

// The other parts of an observation are lineage of each other, so a page re-keyed to one part's memory still
// bridges the others; a part whose row names a memory that is gone names nothing.
func TestSourceLineageNamesTheObservationAndTheOtherMemoriesImportedFromIt(t *testing.T) {
	source := openSource(t, kinFixture(t))

	for id, want := range map[string][]memory.Identity{
		"1": {engramObservation("402"), card("6")},
		"6": {engramObservation("402"), card("1")},
		"2": nil,
	} {
		if got := memoryOf(t, source, id).Lineage; !reflect.DeepEqual(got, want) {
			t.Errorf("memory %s lineage = %v, want %v", id, got, want)
		}
	}
}

// A memory with no import_map row holds the content of the observations whose text, rebuilt from their imported
// parts, is its own, and has as mirrors every memory of those observations, all their parts, and the observations.
// The stored content hashes of the imported memories say nothing: here they are empty or wrong.
func TestSourceMirrorsAreEveryMemoryOfTheObservationsWhoseRebuiltTextTheyHold(t *testing.T) {
	source := openSource(t, kinFixture(t))

	for id, want := range map[string][]memory.Identity{
		// An observation imported as several parts, whose whole text no part's hash equals (b), with every part (c).
		"8": {card("1"), card("6"), engramObservation("402")},
		// An imported memory with a wrong hash (a), whatever the status of the imported memory, and not another
		// project's, nor a memory mapped by another source.
		"12": {card("9"), engramObservation("500"), card("15"), engramObservation("502")},
		// A part cut inside a word is joined with nothing.
		"18": {card("16"), card("17"), engramObservation("600")},
		// A memory that holds the content of no imported observation has no mirror.
		"13": nil,
		"3":  nil,
	} {
		m := memoryOf(t, source, id)
		if !reflect.DeepEqual(m.Mirrored, want) || m.Lineage != nil {
			t.Errorf("memory %s mirrors = %v, lineage = %v, want %v and no lineage", id, m.Mirrored, m.Lineage, want)
		}
	}
	// A mapped memory has an import record, not mirrors.
	for _, id := range []string{"1", "9", "10", "14"} {
		if m := memoryOf(t, source, id); m.Mirrored != nil {
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
		if want := memoryOf(t, source, listed.Identity.ID); !reflect.DeepEqual(listed, want) {
			t.Errorf("listed memory %s = %+v, ByID says %+v", listed.Identity.ID, listed, want)
		}
	}
}
