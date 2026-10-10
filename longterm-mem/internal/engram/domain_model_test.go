package engram

import (
	"database/sql"
	"reflect"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// The store maps rows to the domain model and defines no types of its own: every method answers with
// the types of internal/memory, so a consumer depends on the model and never on this adapter's
// vocabulary. The assignments below are the contract; they stop compiling if a method answers with
// anything else.
func TestTheStoreAnswersInTheTypesOfTheDomainModel(t *testing.T) {
	dbPath := newFixtureDB(t, t.TempDir())
	const project = "labdrian-sdd-overlay"
	first := insertObservationFull(t, dbPath, "first", project, "decision", 3, true, "sy-first")
	second := insertObservationFull(t, dbPath, "second", project, "decision", 1, false, "sy-second")
	insertRelation(t, dbPath, "rel-1", "sy-second", "sy-first", "supersedes", "judged", sql.NullString{})
	insertRelation(t, dbPath, "rel-2", "sy-first", "sy-second", "related", "judged", sql.NullString{})

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()

	var listed []memory.Observation
	if listed, err = store.ListObservations(project); err != nil || len(listed) != 2 {
		t.Fatalf("ListObservations = %v, %v, want two observations", listed, err)
	}
	var withDeleted []memory.Observation
	if withDeleted, err = store.ObservationsIncludingDeleted(project); err != nil || len(withDeleted) != 2 {
		t.Fatalf("ObservationsIncludingDeleted = %v, %v, want two observations", withDeleted, err)
	}
	var one memory.Observation
	var found bool
	if one, found, err = store.ObservationByID(first); err != nil || !found {
		t.Fatalf("ObservationByID(%d) = %v, %v, %v, want the observation", first, one, found, err)
	}
	wantFirst := memory.Observation{
		ID: first, SyncID: "sy-first", Type: "decision", Title: "first", Content: one.Content, Project: project,
		RevisionCount: 3, Pinned: true, CreatedAt: one.CreatedAt, UpdatedAt: one.UpdatedAt,
	}
	if !reflect.DeepEqual(one, wantFirst) {
		t.Errorf("ObservationByID(%d) = %+v, want the row mapped field by field to %+v", first, one, wantFirst)
	}
	var live int
	var byID map[int64]memory.Observation
	if live, byID, err = store.CoverageSnapshot(project, []int64{first, second}); err != nil || live != 2 || len(byID) != 2 {
		t.Fatalf("CoverageSnapshot = %d, %v, %v, want two live and two resolved", live, byID, err)
	}
	var result memory.SearchResult
	if result, err = store.Search(project, "first", 5); err != nil || result.MatchMode != memory.MatchAll || len(result.Rows) != 1 {
		t.Fatalf("Search = %+v, %v, want one row matched on every token", result, err)
	}
	var row memory.Row = result.Rows[0]
	if row.ID != first {
		t.Errorf("Search row = %+v, want the first observation", row)
	}
	var standings map[int64]memory.Standing
	if standings, err = store.Standings([]int64{first}); err != nil {
		t.Fatalf("Standings: %v", err)
	}
	if want := []memory.Neighbour{{ID: second, Title: "second"}}; !reflect.DeepEqual(standings[first].SupersededBy, want) {
		t.Errorf("Standings[%d].SupersededBy = %v, want %v", first, standings[first].SupersededBy, want)
	}
	var edges []memory.Edge
	if edges, err = store.RelatedEdges(first); err != nil || len(edges) != 2 {
		t.Fatalf("RelatedEdges = %v, %v, want the two accepted edges", edges, err)
	}
}
