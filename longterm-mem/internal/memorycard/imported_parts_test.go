package memorycard

import "testing"

// The rows of memories that are gone are the rows of some project's observations: a project's read names them only
// for an observation one of whose memories is in the project, never for another project's.
func TestImportedPartsLeavesOutAnotherProjectsGoneRows(t *testing.T) {
	path := kinFixture(t)
	writeFixture(t, path,
		`insert into memories(id, project, type, title, body, status, content_hash) values
			(80, 'q', 'decision', 'Q one (part 1/2)', 'q body', 'active', '')`,
		// Observation 960 is q's: its part 2 is gone. Observation 961 has no memory left at all.
		`insert into import_map values ('engram', '960', 1, 80), ('engram', '960', 2, 97), ('engram', '961', 1, 96)`,
	)
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	parts, err := store.ImportedParts("p")
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range parts {
		if part.Observation == 960 || part.Observation == 961 {
			t.Errorf("project p was given %+v, a row of an observation that is not its own", part)
		}
	}
	// Its own observation keeps its gone row: observation 402 of p has a third part whose memory is gone.
	found := false
	for _, part := range parts {
		found = found || (part.Observation == 402 && part.Memory == 0 && part.Part == 3)
	}
	if !found {
		t.Errorf("project p lost the gone part 3 of its own observation 402: %+v", parts)
	}
	owner, err := store.ImportedParts("q")
	if err != nil {
		t.Fatal(err)
	}
	if len(owner) != 2 || owner[1].Observation != 960 || owner[1].Part != 2 || owner[1].Memory != 0 {
		t.Errorf("project q = %+v, want its observation 960 with the gone part 2", owner)
	}
}
