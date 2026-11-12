package memorycard

import (
	"reflect"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// The Source adapter serves memorycard's memories as memory.Memory over the fixtures of store_test.go: memory 1 is
// superseded by 2 and imported from Engram observation 402, 2 is mapped by another source, 3 has no import_map
// row, 4 is archived and 5 belongs to another project.

func openSource(t *testing.T, path string, twins TwinsFunc) *Source {
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
	return NewSource(store, twins)
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
	source := openSource(t, listFixture(t), nil)

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
	source := openSource(t, listFixture(t), nil)

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

func TestSourceMirrorsAreTheTwinsOfAMemoryWithNoImportMapRowOnly(t *testing.T) {
	var asked [][2]string
	twins := func(project, contentHash string) ([]memory.Identity, error) {
		asked = append(asked, [2]string{project, contentHash})
		return []memory.Identity{{Source: "engram", ID: "11"}, {Source: "engram", ID: "12"}}, nil
	}
	source := openSource(t, listFixture(t), twins)

	all, err := source.Memories("p")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(asked, [][2]string{{"p", "h3"}, {"p", "h4"}}) {
		t.Errorf("twins asked for %v, want the memories with no import_map row (3 and 4) by their project and hash, and not 1 or 2", asked)
	}
	wantMirrored := []memory.Identity{{Source: "engram", ID: "11"}, {Source: "engram", ID: "12"}}
	if !reflect.DeepEqual(all[2].Mirrored, wantMirrored) || all[2].Lineage != nil {
		t.Errorf("memory 3 mirrors = %v and lineage = %v, want the twins as mirrors %v and no lineage: no import record names them", all[2].Mirrored, all[2].Lineage, wantMirrored)
	}
	if !reflect.DeepEqual(all[0].Lineage, []memory.Identity{{Source: "engram", ID: "402"}}) {
		t.Errorf("memory 1 lineage = %v, want its import_map row only", all[0].Lineage)
	}
	if all[1].Lineage != nil || all[1].Mirrored != nil {
		t.Errorf("memory 2 lineage = %v and mirrors = %v, want none (mapped by a source other than engram)", all[1].Lineage, all[1].Mirrored)
	}
	if all[0].Mirrored != nil {
		t.Errorf("memory 1 mirrors = %v, want none: it has an import_map row, so its twins are not asked", all[0].Mirrored)
	}
}
