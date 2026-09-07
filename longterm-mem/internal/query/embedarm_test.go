package query

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/embed"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vecindex"

	_ "modernc.org/sqlite"
)

// embedArmFixture is one observation newEmbedArmFixture indexes: its title
// and content are embedded verbatim through vecindex.EmbedInput/Fingerprint,
// so a caller can soft-delete it afterward via dbPath without going through
// the read-only production Store.
type embedArmFixture struct {
	title, content, project string
	vec                     []float32
}

const (
	testEmbedModel      = "test-model"
	testEmbedDimension  = 3
	testEmbedInputLimit = 2000
)

// newEmbedArmFixture builds a temp Engram DB from rows, a matching
// vecindex manifest+blob under stateDir's embedding index directory, and
// returns the opened (read-only) Store, the writable dbPath (for a test to
// soft-delete a row afterward), the assigned ids in rows order, and
// stateDir.
func newEmbedArmFixture(t *testing.T, rows []embedArmFixture) (store *engram.Store, dbPath string, ids []int64, stateDir string) {
	t.Helper()

	schema, err := os.ReadFile(filepath.Join("..", "engram", "testdata", "schema.sql"))
	if err != nil {
		t.Fatalf("read engram schema fixture: %v", err)
	}
	dbPath = filepath.Join(t.TempDir(), "engram.db")
	setup, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open fixture setup connection: %v", err)
	}
	if _, err := setup.Exec(string(schema)); err != nil {
		setup.Close()
		t.Fatalf("apply engram schema fixture: %v", err)
	}

	var project string
	manifest := vecindex.Manifest{Model: testEmbedModel, Dimension: testEmbedDimension, InputLimit: testEmbedInputLimit, BuiltAt: "2026-01-01T00:00:00Z"}
	var vectors [][]float32
	for _, r := range rows {
		project = r.project
		res, err := setup.Exec(
			`INSERT INTO observations (session_id, type, title, content, project) VALUES (?, ?, ?, ?, ?)`,
			"sess-1", "discovery", r.title, r.content, r.project,
		)
		if err != nil {
			setup.Close()
			t.Fatalf("insert fixture observation %q: %v", r.title, err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			setup.Close()
			t.Fatalf("last insert id for %q: %v", r.title, err)
		}
		ids = append(ids, id)
		fp := vecindex.Fingerprint(manifest.Model, manifest.Dimension, manifest.InputLimit, r.title, r.content)
		manifest.Entries = append(manifest.Entries, vecindex.ManifestEntry{EngramID: id, Fingerprint: fp})
		vectors = append(vectors, r.vec)
	}
	setup.Close()

	stateDir = t.TempDir()
	idx := &vecindex.Index{Manifest: manifest, Vectors: vectors}
	if err := idx.Save(vecindex.Dir(stateDir, project)); err != nil {
		t.Fatalf("save fixture index: %v", err)
	}

	store, err = engram.Open(dbPath)
	if err != nil {
		t.Fatalf("engram.Open(%q): %v", dbPath, err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, dbPath, ids, stateDir
}

func softDeleteEmbedFixtureRow(t *testing.T, dbPath string, id int64) {
	t.Helper()
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open fixture setup connection: %v", err)
	}
	defer conn.Close()
	if _, err := conn.Exec(`UPDATE observations SET deleted_at = ? WHERE id = ?`, "2026-02-01 00:00:00", id); err != nil {
		t.Fatalf("soft-delete observation %d: %v", id, err)
	}
}

func fakeEmbed(vec []float32, err error) EmbedFunc {
	return func(context.Context, string) ([]float32, error) { return vec, err }
}

// TestSoftDeletedObservationNeverSurfacesFromTheIndex (design: "the
// embedding arm may not reuse ObservationByID"): a row is indexed, then
// soft-deleted. A query against the embedding source must never return it,
// even though the on-disk index still names it, and the soft delete must
// not distort Coverage.Unindexed -- the row leaves Live and Indexed
// together, so the gap between them is unchanged.
func TestSoftDeletedObservationNeverSurfacesFromTheIndex(t *testing.T) {
	store, dbPath, ids, stateDir := newEmbedArmFixture(t, []embedArmFixture{
		{title: "will be deleted", content: "alpha content", project: "proj-embed", vec: []float32{1, 0, 0}},
		{title: "stays live", content: "beta content", project: "proj-embed", vec: []float32{0, 1, 0}},
	})
	softDeleteEmbedFixtureRow(t, dbPath, ids[0])

	rows, coverage, diags := runEmbeddingArm(context.Background(), store, stateDir, "proj-embed", "alpha", 5, fakeEmbed([]float32{1, 0, 0}, nil))
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diags)
	}
	for _, r := range rows {
		if r.EngramID == ids[0] {
			t.Fatalf("soft-deleted observation %d surfaced from the index: %+v", ids[0], rows)
		}
	}
	if len(rows) != 1 || rows[0].EngramID != ids[1] {
		t.Fatalf("rows = %+v, want exactly the live row %d", rows, ids[1])
	}

	// Live=1 (only the surviving row), Indexed=1 (the manifest's other
	// entry no longer resolves live), so Unindexed=0 -- unchanged by the
	// deletion, not inflated by it.
	if coverage.Unindexed != 0 {
		t.Fatalf("coverage.Unindexed = %d, want 0 (the soft-deleted row must leave Live and Indexed together)", coverage.Unindexed)
	}
	if coverage.Live != 1 || coverage.Indexed != 1 {
		t.Fatalf("coverage = %+v, want Live=1 Indexed=1", coverage)
	}
}

// TestEmbeddingArmDropsAStaleFingerprint (R-069): a manifest entry whose
// fingerprint no longer matches the observation's current title/content --
// edited since it was embedded -- is dropped rather than served under a
// vector that no longer describes it.
func TestEmbeddingArmDropsAStaleFingerprint(t *testing.T) {
	store, dbPath, ids, stateDir := newEmbedArmFixture(t, []embedArmFixture{
		{title: "original title", content: "original content", project: "proj-embed", vec: []float32{1, 0, 0}},
	})
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open fixture setup connection: %v", err)
	}
	if _, err := conn.Exec(`UPDATE observations SET content = ? WHERE id = ?`, "edited content", ids[0]); err != nil {
		t.Fatalf("edit fixture observation: %v", err)
	}
	conn.Close()

	rows, _, diags := runEmbeddingArm(context.Background(), store, stateDir, "proj-embed", "original", 5, fakeEmbed([]float32{1, 0, 0}, nil))
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diags)
	}
	if len(rows) != 0 {
		t.Fatalf("rows = %+v, want none: the fingerprint no longer matches the edited content", rows)
	}
}

// TestEmbeddingArmReportsNeverBuiltWhenNoIndexExists guards the "never"
// literal Coverage.BuiltAt must carry when no index has been built at all,
// distinct from an empty string that could be mistaken for a genuine
// (if oddly formatted) timestamp.
func TestEmbeddingArmReportsNeverBuiltWhenNoIndexExists(t *testing.T) {
	store, _, _, _ := newEmbedArmFixture(t, nil)
	stateDir := t.TempDir() // deliberately empty: no index was ever saved here
	_, coverage, diags := runEmbeddingArm(context.Background(), store, stateDir, "proj-embed", "anything", 5, fakeEmbed([]float32{1, 0, 0}, nil))
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diags)
	}
	if coverage.BuiltAt != embeddingIndexNeverBuilt {
		t.Fatalf("coverage.BuiltAt = %q, want %q", coverage.BuiltAt, embeddingIndexNeverBuilt)
	}
}

// TestRunEmbeddingArm_LiveCountFailureIsNamedAndNeverNegative (JD-2): a
// swallowed CountLiveObservations error must not vanish silently. It must
// surface as a diagnostic, the same way every other degradation path in
// this file already does, and Coverage.Unindexed must never go negative
// because of it.
func TestRunEmbeddingArm_LiveCountFailureIsNamedAndNeverNegative(t *testing.T) {
	store, _, _, stateDir := newEmbedArmFixture(t, []embedArmFixture{
		{title: "one", content: "alpha", project: "proj-embed", vec: []float32{1, 0, 0}},
	})
	if err := store.Close(); err != nil {
		t.Fatalf("close fixture store: %v", err)
	}

	_, coverage, diags := runEmbeddingArm(context.Background(), store, stateDir, "proj-embed", "alpha", 5, fakeEmbed([]float32{1, 0, 0}, nil))

	found := false
	for _, d := range diags {
		if d.Code == DiagnosticLiveCountUnreadable {
			found = true
		}
	}
	if !found {
		t.Fatalf("diagnostics = %+v, want one %q naming the swallowed CountLiveObservations error", diags, DiagnosticLiveCountUnreadable)
	}
	if coverage.Unindexed < 0 {
		t.Fatalf("coverage.Unindexed = %d, must never be negative", coverage.Unindexed)
	}
}

// TestCoverageUnindexed_StaleCountIsNamedNotJustClamped (JD-2, JD-5) is the guard
// on the clamp itself. Clamping `live < indexed` to 0 removes a negative
// number that was at least visibly wrong and replaces it with 0 -- which
// is byte-for-byte the value a fully-indexed project reports. A caller
// cannot tell the two apart from the number, so the clamp must say it
// fired; otherwise this is the same silent wrong answer JD-2 was raised to
// remove, surviving one call deeper.
func TestCoverageUnindexed_StaleCountIsNamedNotJustClamped(t *testing.T) {
	cases := []struct {
		name          string
		live, indexed int
		liveKnown     bool
		want          int
		wantStale     bool
	}{
		{name: "normal gap", live: 10, indexed: 4, liveKnown: true, want: 6, wantStale: false},
		{name: "fully indexed", live: 5, indexed: 5, liveKnown: true, want: 0, wantStale: false},
		{name: "live unknown", live: 0, indexed: 3, liveKnown: false, want: 0, wantStale: false},
		{name: "stale count, clamp fires", live: 2, indexed: 5, liveKnown: true, want: 0, wantStale: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, stale := coverageUnindexed(tc.live, tc.indexed, tc.liveKnown)
			if got != tc.want {
				t.Fatalf("coverageUnindexed(%d, %d, %v) = %d, want %d", tc.live, tc.indexed, tc.liveKnown, got, tc.want)
			}
			if got < 0 {
				t.Fatalf("coverageUnindexed returned a negative value: %d (JD-2)", got)
			}
			if stale != tc.wantStale {
				t.Fatalf("coverageUnindexed(%d, %d, %v) reported stale=%v, want %v -- a clamped 0 that does not say it was clamped is indistinguishable from full coverage", tc.live, tc.indexed, tc.liveKnown, stale, tc.wantStale)
			}
		})
	}
}

// TestStaleCoverageDiagnostic_CarriesBothCounts (JD-5) covers what is
// coverable here, and its name says so. runEmbeddingArm's clamp branch is
// NOT exercised: coverage.Indexed is len(LiveObservationsByID(project, ...)),
// which filters by the same project CountLiveObservations counts, so
// indexed <= live holds on every path a test can construct. The branch
// exists for the race between those two reads -- a writer soft-deleting
// between them -- which needs a seam in engram.Store to force, and this
// package has none. Naming that here is the point: a test called
// TestRunEmbeddingArm_* that never calls runEmbeddingArm would be the
// green check that cannot fail, which is the defect class this whole
// change keeps re-finding.
func TestStaleCoverageDiagnostic_CarriesBothCounts(t *testing.T) {
	diags := staleCoverageDiagnostics(3, 7)
	if len(diags) != 1 || diags[0].Code != DiagnosticCoverageCountsInconsistent {
		t.Fatalf("staleCoverageDiagnostics = %+v, want one %q", diags, DiagnosticCoverageCountsInconsistent)
	}
	// Both counts must appear: "the counts disagree" without saying which
	// pair disagreed leaves the reader exactly where the bare 0 did.
	for _, want := range []string{"7", "3"} {
		if !strings.Contains(diags[0].Detail, want) {
			t.Fatalf("diagnostic detail %q omits the count %q", diags[0].Detail, want)
		}
	}
	if !strings.Contains(diags[0].Detail, "not mean this project is fully indexed") {
		t.Fatalf("diagnostic detail %q does not deny the reading it exists to deny", diags[0].Detail)
	}
}

// TestUnreachableBackendAndMissingModelAreDistinctDiagnostics (R-070): the
// two ways an embedding call can fail must never collapse into one code.
func TestUnreachableBackendAndMissingModelAreDistinctDiagnostics(t *testing.T) {
	store, _, _, stateDir := newEmbedArmFixture(t, []embedArmFixture{
		{title: "t", content: "c", project: "proj-embed", vec: []float32{1, 0, 0}},
	})

	_, _, diags := runEmbeddingArm(context.Background(), store, stateDir, "proj-embed", "q", 5, fakeEmbed(nil, &embed.BackendUnreachableError{Err: errors.New("dial tcp: connection refused")}))
	if len(diags) != 1 || diags[0].Code != DiagnosticEmbeddingBackendUnreachable {
		t.Fatalf("diags = %+v, want exactly one %q", diags, DiagnosticEmbeddingBackendUnreachable)
	}

	_, _, diags = runEmbeddingArm(context.Background(), store, stateDir, "proj-embed", "q", 5, fakeEmbed(nil, &embed.ModelMissingError{Model: "nomic-embed-text"}))
	if len(diags) != 1 || diags[0].Code != DiagnosticEmbeddingModelMissing {
		t.Fatalf("diags = %+v, want exactly one %q", diags, DiagnosticEmbeddingModelMissing)
	}
}
