package engram

import (
	"database/sql"
	"testing"
)

// Standings answers the only question a reader actually has: should this
// memory be treated as current? Direction is the whole of it -- being the
// TARGET of a supersedes means something replaced you; being the SOURCE
// means you are the replacement. Getting that backwards would flag the new
// decision as stale and leave the abandoned one looking current, which is
// worse than not looking at all.
func TestStandings_SupersededIsDirectional(t *testing.T) {
	dbPath := newFixtureDB(t, t.TempDir())
	oldID := insertObservationFull(t, dbPath, "the abandoned approach", "p", "decision", 1, false, "sync-old")
	newID := insertObservationFull(t, dbPath, "the approach that replaced it", "p", "decision", 1, false, "sync-new")
	insertRelation(t, dbPath, "rel-1", "sync-new", "sync-old", "supersedes", "judged", sql.NullString{})

	got := standings(t, dbPath, oldID, newID)

	if len(got[oldID].SupersededBy) != 1 || got[oldID].SupersededBy[0].ID != newID {
		t.Fatalf("the replaced memory must name its replacement: %+v", got[oldID])
	}
	if len(got[newID].SupersededBy) != 0 {
		t.Fatalf("the replacement must not be marked as replaced: %+v", got[newID])
	}
}

// A conflict cuts both ways: neither side can be trusted as current until
// somebody decides, so both carry it.
func TestStandings_ConflictIsMutual(t *testing.T) {
	dbPath := newFixtureDB(t, t.TempDir())
	a := insertObservationFull(t, dbPath, "a", "p", "decision", 1, false, "sync-a")
	b := insertObservationFull(t, dbPath, "b", "p", "decision", 1, false, "sync-b")
	insertRelation(t, dbPath, "rel-1", "sync-a", "sync-b", "conflicts_with", "judged", sql.NullString{})

	got := standings(t, dbPath, a, b)
	if len(got[a].ConflictsWith) != 1 || len(got[b].ConflictsWith) != 1 {
		t.Fatalf("a conflict must be visible from both sides: %+v %+v", got[a], got[b])
	}
}

// The unjudged pile is the one that matters most in practice: on the real
// database 34 relations were raised and never decided, and every one of
// them was invisible to every reader. An undecided conflict is not the same
// as no conflict, and reporting it as nothing is how it stays undecided
// forever.
func TestStandings_PendingIsReportedNotSwallowed(t *testing.T) {
	dbPath := newFixtureDB(t, t.TempDir())
	a := insertObservationFull(t, dbPath, "a", "p", "decision", 1, false, "sync-a")
	b := insertObservationFull(t, dbPath, "b", "p", "decision", 1, false, "sync-b")
	insertRelation(t, dbPath, "rel-1", "sync-a", "sync-b", "pending", "pending", sql.NullString{})

	got := standings(t, dbPath, a, b)
	if len(got[a].Unjudged) != 1 || len(got[b].Unjudged) != 1 {
		t.Fatalf("an undecided conflict must be reported from both sides: %+v %+v", got[a], got[b])
	}
	if len(got[a].SupersededBy) != 0 || len(got[a].ConflictsWith) != 0 {
		t.Fatalf("an undecided conflict must not be reported as a decided one: %+v", got[a])
	}
}

// "related" and "not_conflict" are verdicts that something is FINE. Turning
// them into a warning would put a marker on almost every memory in the
// database -- 120 of 169 relations -- and a warning that fires on
// everything is one nobody reads.
func TestStandings_BenignVerdictsAreNotWarnings(t *testing.T) {
	dbPath := newFixtureDB(t, t.TempDir())
	a := insertObservationFull(t, dbPath, "a", "p", "decision", 1, false, "sync-a")
	b := insertObservationFull(t, dbPath, "b", "p", "decision", 1, false, "sync-b")
	insertRelation(t, dbPath, "rel-1", "sync-a", "sync-b", "related", "judged", sql.NullString{})
	insertRelation(t, dbPath, "rel-2", "sync-a", "sync-b", "not_conflict", "judged", sql.NullString{})

	got := standings(t, dbPath, a, b)
	if len(got) != 0 {
		t.Fatalf("a verdict that everything is fine is not a warning: %+v", got)
	}
}

// A relation that was itself re-judged carries superseded_at. Reading it
// would report a verdict its own author has already withdrawn.
func TestStandings_WithdrawnRelationIsIgnored(t *testing.T) {
	dbPath := newFixtureDB(t, t.TempDir())
	a := insertObservationFull(t, dbPath, "a", "p", "decision", 1, false, "sync-a")
	b := insertObservationFull(t, dbPath, "b", "p", "decision", 1, false, "sync-b")
	insertRelation(t, dbPath, "rel-1", "sync-a", "sync-b", "supersedes", "judged",
		sql.NullString{String: "2026-08-01T00:00:00Z", Valid: true})

	if got := standings(t, dbPath, a, b); len(got) != 0 {
		t.Fatalf("a withdrawn verdict must not be reported: %+v", got)
	}
}

func standings(t *testing.T, dbPath string, ids ...int64) map[int64]Standing {
	t.Helper()
	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()
	got, err := store.Standings(ids)
	if err != nil {
		t.Fatalf("Standings: %v", err)
	}
	return got
}
