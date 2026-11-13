package engram

import (
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// dropTable removes a table of the fixture database, as a store whose schema lost it would be.
func dropTable(t *testing.T, path, table string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("DROP TABLE " + table); err != nil {
		t.Fatal(err)
	}
}

// The listing the promotion pass reads does not need the relations: a store whose relations table is gone still
// lists its live observations, and each one says its status could not be resolved. The pass promotes them as it
// always did, and the propagation pass reports the failure for each observation, where it read the relations.
func TestSourceCandidatesListWithoutTheRelationsAndSayWhichStatusIsUnresolved(t *testing.T) {
	path := sourceFixture(t)
	dropTable(t, path, "memory_relations")
	source := openEngramSource(t, path, nil)

	candidates, err := source.Candidates("p")
	if err != nil {
		t.Fatalf("Candidates = %v, want the live observations listed without the relations", err)
	}
	if got := ids(candidates); len(got) != 4 {
		t.Fatalf("Candidates = %v, want the 4 live observations", got)
	}
	for _, m := range candidates {
		if m.Status != memory.StatusUnknown || m.StatusCause == nil || !strings.Contains(m.StatusCause.Error(), "related edges for observation "+m.Identity.ID) {
			t.Errorf("observation %s: status %q, cause %v, want an unknown status and the failure of its own relations", m.Identity.ID, m.Status, m.StatusCause)
		}
	}

	all, err := source.Memories("p")
	if err != nil || len(all) != 6 {
		t.Fatalf("Memories = (%v, %v), want the six rows of the project, each with an unknown status", ids(all), err)
	}
	for _, m := range all {
		if m.Status != memory.StatusUnknown || m.StatusCause == nil {
			t.Errorf("observation %s: status %q, cause %v, want an unknown status with its cause", m.Identity.ID, m.Status, m.StatusCause)
		}
	}
}

// A listing that cannot read the observations at all fails with the store's own words for the live listing, as the
// promotion pass has always reported it, not the words of the listing that includes the soft-deleted rows.
func TestSourceCandidatesFailWithTheWordsOfTheLiveListing(t *testing.T) {
	path := sourceFixture(t)
	dropTable(t, path, "observations")
	source := openEngramSource(t, path, nil)

	_, err := source.Candidates("p")
	if err == nil || !strings.Contains(err.Error(), `list observations for project "p"`) || strings.Contains(err.Error(), "including deleted") {
		t.Fatalf("Candidates = %v, want the live listing's words", err)
	}
}

// A lookup by id is not about status: an observation comes back, with an unknown status, when its relations cannot be
// read, so an explicit promote (which never uses a status) does not fail on them, as it never read them at the base.
func TestSourceByIDDoesNotFailOnUnreadableRelations(t *testing.T) {
	path := sourceFixture(t)
	dropTable(t, path, "memory_relations")
	source := openEngramSource(t, path, nil)

	m, ok, err := source.ByID("2")

	if err != nil || !ok {
		t.Fatalf("ByID(2) = (%+v, %v, %v), want the observation", m, ok, err)
	}
	if m.Title != "New" || m.Status != memory.StatusUnknown || m.StatusCause == nil {
		t.Errorf("ByID(2) = %+v, want the observation with an unknown status and its cause", m)
	}
}

// Only an unreadable edge read keeps a memory with an unknown status: any other failure of the mapping still fails
// the listing.
func TestSourceCandidatesStillFailOnAFailureThatIsNotTheRelations(t *testing.T) {
	path := sourceFixture(t)
	dropTable(t, path, "memory_relations")
	boom := errors.New("lineage offline")
	source := openEngramSource(t, path, func(string, []memory.Observation) (map[int64]memory.Kin, error) { return nil, boom })

	if _, err := source.Candidates("p"); !errors.Is(err, boom) {
		t.Fatalf("Candidates = %v, want the lineage failure to fail the listing", err)
	}
}

// The probe asks whether the relations can be read at all, which no listing of an empty project can say.
func TestSourceProbeSaysWhetherTheRelationsCanBeRead(t *testing.T) {
	path := sourceFixture(t)
	if err := openEngramSource(t, path, nil).Probe("empty"); err != nil {
		t.Fatalf("Probe = %v, want nil on a healthy store", err)
	}
	dropTable(t, path, "memory_relations")
	if err := openEngramSource(t, path, nil).Probe("empty"); err == nil {
		t.Fatal("Probe = nil, want the failure of the relations")
	}
}
