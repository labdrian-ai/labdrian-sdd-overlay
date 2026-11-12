package engram

import (
	"database/sql"
	"reflect"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// sourceFixture builds an Engram database of project p whose observations carry the cases propagation decides:
//
//	1  superseded by 2 (an accepted supersedes edge, 2 is newer by created_at)
//	2  active
//	3  soft-deleted, no edge
//	4  soft-deleted and also the older side of an edge to 2: superseded wins, as resolveStatus decides
//	5  live, a related edge only
//	6  another project
//	7  live, the older side of an edge to 6, which is not in p: the other side is unrecognized, so active
func sourceFixture(t *testing.T) string {
	t.Helper()
	path := newFixtureDB(t, t.TempDir())
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, stmt := range []string{
		`insert into observations (id, sync_id, session_id, type, title, content, project, topic_key, revision_count, pinned, created_at, deleted_at) values
			(1, 's-old', 'x', 'decision', 'Old', 'old content', 'p', 'a/b', 4, 1, '2026-01-01 00:00:00', null),
			(2, 's-new', 'x', 'decision', 'New', 'new content', 'p', null, 1, 0, '2026-02-01 00:00:00', null),
			(3, 's-gone', 'x', 'learning', 'Gone', 'gone content', 'p', null, 2, 0, '2026-01-02 00:00:00', '2026-03-01 00:00:00'),
			(4, 's-both', 'x', 'learning', 'Both', 'both content', 'p', null, 1, 0, '2026-01-03 00:00:00', '2026-03-01 00:00:00'),
			(5, 's-loose', 'x', 'learning', 'Loose', 'loose content', 'p', null, 1, 0, '2026-01-04 00:00:00', null),
			(6, 's-else', 'x', 'learning', 'Else', 'else content', 'q', null, 1, 0, '2026-04-01 00:00:00', null),
			(7, 's-edge', 'x', 'learning', 'Edge', 'edge content', 'p', null, 1, 0, '2026-03-01 00:00:00', null)`,
		`insert into memory_relations (sync_id, source_id, target_id, relation, judgment_status, superseded_at) values
			('r1', 's-new', 's-old', 'supersedes', 'judged', null),
			('r2', 's-new', 's-both', 'supersedes', 'judged', null),
			('r3', 's-loose', 's-new', 'related', 'judged', null),
			('r4', 's-else', 's-edge', 'supersedes', 'judged', null)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	return path
}

func openEngramSource(t *testing.T, path string, lineage LineageFunc) *Source {
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
	return NewSource(store, lineage)
}

func obs(id string) memory.Identity { return memory.Identity{Source: SourceName, ID: id} }

func wantObservation(id string) memory.Memory {
	switch id {
	case "1":
		return memory.Memory{
			Identity: obs("1"), Project: "p", Type: "decision", Title: "Old", Body: "old content", TopicKey: "a/b", Pinned: true,
			Status: memory.StatusSuperseded, Successor: obs("2"), SuccessorTitle: "New", Revision: 4,
		}
	case "2":
		return memory.Memory{Identity: obs("2"), Project: "p", Type: "decision", Title: "New", Body: "new content", Status: memory.StatusActive, Revision: 1}
	case "3":
		return memory.Memory{Identity: obs("3"), Project: "p", Type: "learning", Title: "Gone", Body: "gone content", Status: memory.StatusArchived, Revision: 2}
	case "4":
		return memory.Memory{
			Identity: obs("4"), Project: "p", Type: "learning", Title: "Both", Body: "both content",
			Status: memory.StatusSuperseded, Successor: obs("2"), SuccessorTitle: "New", Revision: 1,
		}
	case "5":
		return memory.Memory{Identity: obs("5"), Project: "p", Type: "learning", Title: "Loose", Body: "loose content", Status: memory.StatusActive, Revision: 1}
	default:
		return memory.Memory{Identity: obs("7"), Project: "p", Type: "learning", Title: "Edge", Body: "edge content", Status: memory.StatusActive, Revision: 1}
	}
}

func ids(memories []memory.Memory) []string {
	var got []string
	for _, m := range memories {
		got = append(got, m.Identity.ID)
	}
	return got
}

// resolveStatus's rule, behind the adapter: the older side of an accepted supersedes edge is superseded and names
// its successor (the other side, newer by created_at) whether or not it was soft-deleted; a soft-deleted
// observation with no such edge is archived; a related edge, or a supersedes edge whose other side is not in the
// project, leaves an observation active.
func TestSourceMapsStatusAndSuccessorByResolveStatusRule(t *testing.T) {
	source := openEngramSource(t, sourceFixture(t), nil)

	for _, id := range []string{"1", "2", "3", "4", "5", "7"} {
		got, ok, err := source.ByID(id)
		if err != nil || !ok {
			t.Fatalf("ByID(%s) = %v, %v", id, ok, err)
		}
		if want := wantObservation(id); !reflect.DeepEqual(got, want) {
			t.Errorf("ByID(%s) = %+v\nwant %+v", id, got, want)
		}
	}
	for _, id := range []string{"99", "not-a-number"} {
		if got, ok, err := source.ByID(id); ok || err != nil {
			t.Errorf("ByID(%q) = %+v, %v, %v, want not found and no error", id, got, ok, err)
		}
	}
}

func TestSourceCandidatesAreTheLiveObservationsAndMemoriesIsEveryRowOfTheProject(t *testing.T) {
	source := openEngramSource(t, sourceFixture(t), nil)

	candidates, err := source.Candidates("p")
	if err != nil {
		t.Fatal(err)
	}
	all, err := source.Memories("p")
	if err != nil {
		t.Fatal(err)
	}
	byID := func(list []memory.Memory) map[string]memory.Memory {
		m := map[string]memory.Memory{}
		for _, x := range list {
			m[x.Identity.ID] = x
		}
		return m
	}
	if got := byID(candidates); len(got) != 4 || len(candidates) != 4 {
		t.Errorf("Candidates(p) = %v, want the live observations 1, 2, 5 and 7 only (not the soft-deleted 3 and 4, not the other project's 6)", ids(candidates))
	}
	gotAll := byID(all)
	if len(all) != 6 || len(gotAll) != 6 {
		t.Fatalf("Memories(p) = %v, want 1, 2, 3, 4, 5 and 7 (soft-deleted ones included, not 6)", ids(all))
	}
	for id, m := range gotAll {
		if want := wantObservation(id); !reflect.DeepEqual(m, want) {
			t.Errorf("Memories(p) mapped %s as %+v\nwant %+v", id, m, want)
		}
	}
	for id, m := range byID(candidates) {
		if !reflect.DeepEqual(m, gotAll[id]) {
			t.Errorf("Candidates(p) mapped %s as %+v, Memories as %+v: one mapping", id, m, gotAll[id])
		}
	}
}

// The lineage is asked of the composition root once for a listing, not once per observation, with the project and
// the observations of the listing; ByID asks for the one observation.
func TestSourceLineageIsAskedOncePerListingForItsObservations(t *testing.T) {
	type call struct {
		project string
		ids     []int64
	}
	var calls []call
	lineage := func(project string, observations []memory.Observation) (map[int64][]memory.Identity, error) {
		c := call{project: project}
		for _, o := range observations {
			c.ids = append(c.ids, o.ID)
		}
		calls = append(calls, c)
		return map[int64][]memory.Identity{1: {{Source: "memorycard", ID: "9"}}}, nil
	}
	source := openEngramSource(t, sourceFixture(t), lineage)

	candidates, err := source.Candidates("p")
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0].project != "p" || len(calls[0].ids) != 4 {
		t.Fatalf("lineage asked %+v for Candidates(p), want one call for the project and its 4 live observations", calls)
	}
	for _, m := range candidates {
		want := []memory.Identity(nil)
		if m.Identity.ID == "1" {
			want = []memory.Identity{{Source: "memorycard", ID: "9"}}
		}
		if !reflect.DeepEqual(m.Lineage, want) {
			t.Errorf("observation %s lineage = %v, want %v", m.Identity.ID, m.Lineage, want)
		}
	}

	calls = nil
	got, ok, err := source.ByID("1")
	if err != nil || !ok {
		t.Fatalf("ByID(1) = %v, %v", ok, err)
	}
	if !reflect.DeepEqual(calls, []call{{project: "p", ids: []int64{1}}}) {
		t.Errorf("lineage asked %+v for ByID(1), want one call for observation 1", calls)
	}
	if want := []memory.Identity{{Source: "memorycard", ID: "9"}}; !reflect.DeepEqual(got.Lineage, want) {
		t.Errorf("ByID(1) lineage = %v, want %v", got.Lineage, want)
	}
}

// The edges of a whole project, read in one query, are the edges RelatedEdges answers observation by observation.
func TestRelatedEdgesOfProject_AreTheEdgesOfEachObservation(t *testing.T) {
	path := sourceFixture(t)
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	got, err := store.RelatedEdgesOfProject("p")
	if err != nil {
		t.Fatal(err)
	}
	withEdges := 0
	for _, id := range []int64{1, 2, 3, 4, 5, 7} {
		want, err := store.RelatedEdges(id)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got[id], want) && !(len(want) == 0 && len(got[id]) == 0) {
			t.Errorf("edges of observation %d = %+v, RelatedEdges says %+v", id, got[id], want)
		}
		if len(want) > 0 {
			withEdges++
		}
	}
	if withEdges != 5 {
		t.Fatalf("%d observations of the fixture have edges, want 5 (1, 2, 4, 5 and 7): the comparison would pass over nothing", withEdges)
	}
	if _, ok := got[6]; ok {
		t.Errorf("edges of another project's observation 6 came back: %+v", got[6])
	}
}
