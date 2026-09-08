package query

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vecindex"
)

// insertExtraLiveObservation writes one more live observation directly
// through dbPath's write connection, the same trick softDeleteEmbedFixtureRow
// already uses to reach past the read-only production Store -- it exists
// only to leave the manifest behind the live corpus by exactly one row
// without going through vecindex.Build itself, which is the very thing
// these tests are proving Run does or does not invoke.
func insertExtraLiveObservation(t *testing.T, dbPath, title, content, project string) int64 {
	t.Helper()
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open fixture setup connection: %v", err)
	}
	defer conn.Close()
	res, err := conn.Exec(
		`INSERT INTO observations (session_id, type, title, content, project) VALUES (?, ?, ?, ?, ?)`,
		"sess-1", "discovery", title, content, project,
	)
	if err != nil {
		t.Fatalf("insert extra observation %q: %v", title, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("last insert id for %q: %v", title, err)
	}
	return id
}

// fakeBuildIndex returns a Deps.BuildIndex stand-in counting its calls,
// running behavior into apply for each call, and reporting err.
func fakeBuildIndex(calls *int, apply func(), err error) BuildIndexFunc {
	return func(_ context.Context, _, _ string, _, _ int) error {
		*calls++
		if err == nil && apply != nil {
			apply()
		}
		return err
	}
}

// TestEmbeddingArm_ToppedUpWhenUnindexedWithinBudget is this issue's
// bounded top-up: one row left behind the manifest (Unindexed=1, well
// within TopUpMaxRows) must be embedded and folded into the index BEFORE
// the query scans it, not left for a human to notice in Coverage and fix
// out of band later.
func TestEmbeddingArm_ToppedUpWhenUnindexedWithinBudget(t *testing.T) {
	store, dbPath, _, stateDir := newEmbedArmFixture(t, []embedArmFixture{
		{title: "already indexed", content: "alpha content", project: "proj-embed", vec: []float32{1, 0, 0}},
	})
	newID := insertExtraLiveObservation(t, dbPath, "fresh row", "beta content", "proj-embed")

	buildCalls := 0
	buildFn := fakeBuildIndex(&buildCalls, func() {
		// Simulate a real incremental build: the fresh row joins the index
		// alongside the one already there, exactly as vecindex.Build would
		// leave it on disk.
		idx, err := vecindex.Load(vecindex.Dir(stateDir, "proj-embed"))
		if err != nil {
			t.Fatalf("load index to simulate top-up: %v", err)
		}
		idx.Manifest.Entries = append(idx.Manifest.Entries, vecindex.ManifestEntry{
			EngramID:    newID,
			Fingerprint: vecindex.Fingerprint(idx.Manifest.Model, idx.Manifest.Dimension, idx.Manifest.InputLimit, "fresh row", "beta content"),
		})
		idx.Vectors = append(idx.Vectors, []float32{0, 1, 0})
		if err := idx.Save(vecindex.Dir(stateDir, "proj-embed")); err != nil {
			t.Fatalf("save topped-up index: %v", err)
		}
	}, nil)

	rows, coverage, diags := runEmbeddingArm(context.Background(), store, stateDir, "proj-embed", "alpha", 5, fakeEmbed([]float32{1, 0, 0}, nil), buildFn)

	if buildCalls != 1 {
		t.Fatalf("BuildIndex was called %d times, want exactly 1", buildCalls)
	}
	if coverage.Unindexed != 0 {
		t.Fatalf("coverage.Unindexed = %d after a successful top-up, want 0", coverage.Unindexed)
	}
	if !hasDiagCode(diags, DiagnosticEmbeddingToppedUp) {
		t.Fatalf("diagnostics = %+v, want one naming the top-up", diags)
	}
	found := false
	for _, r := range rows {
		if r.EngramID == newID {
			found = true
		}
	}
	if !found {
		t.Fatalf("rows = %+v, want the freshly topped-up row %d among them", rows, newID)
	}
	if d := coverageIncompleteDiagnostic(coverage); d != nil {
		t.Fatalf("coverage still reports incomplete after a successful top-up: %+v", d)
	}
}

// TestEmbeddingArm_NoTopUpWhenOverBudget: when the gap exceeds
// TopUpMaxRows, the arm must not build at all -- today's degrade-and-name-
// the-CLI-command behavior stands unchanged.
func TestEmbeddingArm_NoTopUpWhenOverBudget(t *testing.T) {
	store, dbPath, _, stateDir := newEmbedArmFixture(t, []embedArmFixture{
		{title: "already indexed", content: "alpha content", project: "proj-embed", vec: []float32{1, 0, 0}},
	})
	for i := 0; i < TopUpMaxRows+1; i++ {
		insertExtraLiveObservation(t, dbPath, "extra", "beta content", "proj-embed")
	}

	buildCalls := 0
	buildFn := fakeBuildIndex(&buildCalls, nil, nil)

	_, coverage, _ := runEmbeddingArm(context.Background(), store, stateDir, "proj-embed", "alpha", 5, fakeEmbed([]float32{1, 0, 0}, nil), buildFn)

	if buildCalls != 0 {
		t.Fatalf("BuildIndex was called %d times, want 0: the gap exceeds TopUpMaxRows", buildCalls)
	}
	if coverage.Unindexed != TopUpMaxRows+1 {
		t.Fatalf("coverage.Unindexed = %d, want %d", coverage.Unindexed, TopUpMaxRows+1)
	}
}

// TestEmbeddingArm_TopUpFailureDegradesWithoutFailingTheQuery: a top-up
// build failure (backend down, etc.) must never surface as a query error --
// it degrades exactly like today, plus a diagnostic naming the failure.
func TestEmbeddingArm_TopUpFailureDegradesWithoutFailingTheQuery(t *testing.T) {
	store, dbPath, _, stateDir := newEmbedArmFixture(t, []embedArmFixture{
		{title: "already indexed", content: "alpha content", project: "proj-embed", vec: []float32{1, 0, 0}},
	})
	insertExtraLiveObservation(t, dbPath, "fresh row", "beta content", "proj-embed")

	buildCalls := 0
	buildErr := errors.New("embedding backend unreachable")
	buildFn := fakeBuildIndex(&buildCalls, nil, buildErr)

	rows, coverage, diags := runEmbeddingArm(context.Background(), store, stateDir, "proj-embed", "alpha", 5, fakeEmbed([]float32{1, 0, 0}, nil), buildFn)

	if buildCalls != 1 {
		t.Fatalf("BuildIndex was called %d times, want exactly 1", buildCalls)
	}
	if coverage.Unindexed != 1 {
		t.Fatalf("coverage.Unindexed = %d after a failed top-up, want 1 (unchanged)", coverage.Unindexed)
	}
	if !hasDiagCode(diags, DiagnosticEmbeddingTopUpFailed) {
		t.Fatalf("diagnostics = %+v, want one naming the top-up failure", diags)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %+v, want the one row the stale index still knows about", rows)
	}
}

func hasDiagCode(diags []Diagnostic, code string) bool {
	for _, d := range diags {
		if d.Code == code {
			return true
		}
	}
	return false
}
