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

	rows, coverage, diags := runEmbeddingArm(context.Background(), store, stateDir, "proj-embed", "alpha", 5, fakeEmbed([]float32{1, 0, 0}, nil), nil)
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

	rows, _, diags := runEmbeddingArm(context.Background(), store, stateDir, "proj-embed", "original", 5, fakeEmbed([]float32{1, 0, 0}, nil), nil)
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
	_, coverage, diags := runEmbeddingArm(context.Background(), store, stateDir, "proj-embed", "anything", 5, fakeEmbed([]float32{1, 0, 0}, nil), nil)
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", diags)
	}
	if coverage.BuiltAt != embeddingIndexNeverBuilt {
		t.Fatalf("coverage.BuiltAt = %q, want %q", coverage.BuiltAt, embeddingIndexNeverBuilt)
	}
}

// TestRunEmbeddingArm_UnreadableCoverageIsNamedAndReportsNoMeasurements
// replaces three tests that no longer have anything to guard.
//
// Coverage's two counts now come from one CoverageSnapshot, so there is no
// clamp to test, no "the counts disagree" diagnostic to carry, and no
// unreachable branch to apologise for in a comment. What IS reachable, and
// was not before, is the whole failure path end to end: one closed store,
// one error, one diagnostic, and counts that stay zero and say so.
//
// The shape this replaces discarded LiveObservationsByID's error into an
// empty map. That reported Indexed as 0 while Live stayed correct, so every
// live observation looked unindexed and coverageIncompleteDiagnostic then
// told the operator to rebuild an index that was never the problem -- a
// read failure turned into a confident, actionable, wrong instruction.
func TestRunEmbeddingArm_UnreadableCoverageIsNamedAndReportsNoMeasurements(t *testing.T) {
	store, _, _, stateDir := newEmbedArmFixture(t, []embedArmFixture{
		{title: "one", content: "alpha", project: "proj-embed", vec: []float32{1, 0, 0}},
	})
	if err := store.Close(); err != nil {
		t.Fatalf("close fixture store: %v", err)
	}

	_, coverage, diags := runEmbeddingArm(context.Background(), store, stateDir, "proj-embed", "alpha", 5, fakeEmbed([]float32{1, 0, 0}, nil), nil)

	var found *Diagnostic
	for i, d := range diags {
		if d.Code == DiagnosticCoverageUnreadable {
			found = &diags[i]
		}
	}
	if found == nil {
		t.Fatalf("diagnostics = %+v, want one %q naming the failed coverage read", diags, DiagnosticCoverageUnreadable)
	}
	if !strings.Contains(found.Detail, "not measurements") {
		t.Fatalf("diagnostic detail %q does not deny that the zeros below it are measurements", found.Detail)
	}
	if coverage.Live != 0 || coverage.Indexed != 0 || coverage.Unindexed != 0 {
		t.Fatalf("coverage = %+v, want all zero: nothing was measured, so nothing may be claimed", coverage)
	}
	// The specific wrong instruction this replaces: Unindexed must not be
	// left equal to Live, which is what makes the incomplete-index
	// diagnostic tell the operator to rebuild.
	if d := coverageIncompleteDiagnostic(coverage); d != nil && strings.Contains(d.Detail, "rebuild") {
		t.Fatalf("a failed coverage read still produced a rebuild instruction: %q", d.Detail)
	}
}

// TestRunEmbeddingArm_UnindexedIsAPlainSubtraction: with both counts from
// one snapshot every indexed row is one of the live rows counted, so the
// subtraction is total and needs no guard. This pins that the arm still
// reports a real gap rather than having lost the number along with the
// clamp.
func TestRunEmbeddingArm_UnindexedIsAPlainSubtraction(t *testing.T) {
	store, dbPath, _, stateDir := newEmbedArmFixture(t, []embedArmFixture{
		{title: "indexed one", content: "alpha", project: "proj-embed", vec: []float32{1, 0, 0}},
	})
	if err := store.Close(); err != nil {
		t.Fatalf("close fixture store: %v", err)
	}
	// A live observation the index does not know about, written before the
	// store is reopened so the read-only connection sees it.
	writeConn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open write connection: %v", err)
	}
	if _, err := writeConn.Exec(
		`INSERT INTO observations (session_id, type, title, content, project) VALUES (?, ?, ?, ?, ?)`,
		"sess-1", "discovery", "unindexed one", "beta", "proj-embed",
	); err != nil {
		writeConn.Close()
		t.Fatalf("insert unindexed observation: %v", err)
	}
	writeConn.Close()

	reopened, err := engram.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	t.Cleanup(func() { reopened.Close() })

	_, coverage, _ := runEmbeddingArm(context.Background(), reopened, stateDir, "proj-embed", "alpha", 5, fakeEmbed([]float32{1, 0, 0}, nil), nil)

	if coverage.Live != 2 || coverage.Indexed != 1 {
		t.Fatalf("coverage = %+v, want Live 2 / Indexed 1", coverage)
	}
	if coverage.Unindexed != 1 {
		t.Fatalf("coverage.Unindexed = %d, want 1", coverage.Unindexed)
	}
}

// TestUnreachableBackendAndMissingModelAreDistinctDiagnostics (R-070): the
// two ways an embedding call can fail must never collapse into one code.
func TestUnreachableBackendAndMissingModelAreDistinctDiagnostics(t *testing.T) {
	store, _, _, stateDir := newEmbedArmFixture(t, []embedArmFixture{
		{title: "t", content: "c", project: "proj-embed", vec: []float32{1, 0, 0}},
	})

	_, _, diags := runEmbeddingArm(context.Background(), store, stateDir, "proj-embed", "q", 5, fakeEmbed(nil, &embed.BackendUnreachableError{Err: errors.New("dial tcp: connection refused")}), nil)
	if len(diags) != 1 || diags[0].Code != DiagnosticEmbeddingBackendUnreachable {
		t.Fatalf("diags = %+v, want exactly one %q", diags, DiagnosticEmbeddingBackendUnreachable)
	}

	_, _, diags = runEmbeddingArm(context.Background(), store, stateDir, "proj-embed", "q", 5, fakeEmbed(nil, &embed.ModelMissingError{Model: "nomic-embed-text"}), nil)
	if len(diags) != 1 || diags[0].Code != DiagnosticEmbeddingModelMissing {
		t.Fatalf("diags = %+v, want exactly one %q", diags, DiagnosticEmbeddingModelMissing)
	}
}
