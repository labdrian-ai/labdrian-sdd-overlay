package vecindex

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// fakeEmbedder records every text it was asked to embed and returns a
// deterministic vector derived from a call counter, so a test can assert
// exactly which rows were (re-)embedded without depending on real
// embedding output.
type fakeEmbedder struct {
	dim     int
	calls   []string
	nextVal float32
}

func (f *fakeEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	f.calls = append(f.calls, text)
	f.nextVal++
	v := make([]float32, f.dim)
	for i := range v {
		v[i] = f.nextVal
	}
	return v, nil
}

const (
	testModel = "nomic-embed-text"
	testDim   = 4
	testLimit = 2000
)

func fixedNow() string { return "2026-01-01T00:00:00Z" }

// TestBuildFromScratchEmbedsEveryLiveRow: no prior index -- every row is
// new, so Build embeds all of them and reports none reused or removed.
func TestBuildFromScratchEmbedsEveryLiveRow(t *testing.T) {
	dir := t.TempDir()
	rows := []Row{
		{EngramID: 1, Title: "a", Content: "alpha"},
		{EngramID: 2, Title: "b", Content: "beta"},
	}
	embedder := &fakeEmbedder{dim: testDim}

	idx, result, err := Build(context.Background(), dir, testModel, testDim, testLimit, rows, embedder, fixedNow)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if result.Embedded != 2 || result.Reused != 0 || result.Removed != 0 {
		t.Errorf("BuildResult = %+v, want {Embedded:2 Reused:0 Removed:0}", result)
	}
	if len(embedder.calls) != 2 {
		t.Fatalf("embedder was called %d times, want 2", len(embedder.calls))
	}
	if len(idx.Manifest.Entries) != 2 || len(idx.Vectors) != 2 {
		t.Fatalf("index holds %d entries / %d vectors, want 2/2", len(idx.Manifest.Entries), len(idx.Vectors))
	}

	// Persisted to disk, loadable, and consistent (exercises the Save/Load
	// path this build call must go through).
	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load after Build: %v", err)
	}
	if len(loaded.Manifest.Entries) != 2 {
		t.Fatalf("loaded index holds %d entries, want 2", len(loaded.Manifest.Entries))
	}
}

// TestBuildReembedsOnlyMissingOrChanged is R-069's own scenario: a second
// Build over an index that already covers most rows re-embeds only the row
// whose fingerprint changed and the row that is genuinely new, reusing the
// unchanged row's vector without calling the embedder for it.
func TestBuildReembedsOnlyMissingOrChanged(t *testing.T) {
	dir := t.TempDir()
	embedder := &fakeEmbedder{dim: testDim}

	// First build: two rows.
	rows := []Row{
		{EngramID: 1, Title: "a", Content: "alpha"},
		{EngramID: 2, Title: "b", Content: "beta"},
	}
	if _, _, err := Build(context.Background(), dir, testModel, testDim, testLimit, rows, embedder, fixedNow); err != nil {
		t.Fatalf("first Build: %v", err)
	}
	firstCallCount := len(embedder.calls)

	// Second build: row 1 unchanged, row 2's content changed, row 3 is new.
	rows2 := []Row{
		{EngramID: 1, Title: "a", Content: "alpha"},
		{EngramID: 2, Title: "b", Content: "beta CHANGED"},
		{EngramID: 3, Title: "c", Content: "gamma"},
	}
	idx, result, err := Build(context.Background(), dir, testModel, testDim, testLimit, rows2, embedder, fixedNow)
	if err != nil {
		t.Fatalf("second Build: %v", err)
	}

	if result.Embedded != 2 {
		t.Errorf("second Build embedded %d rows, want 2 (changed row 2 + new row 3)", result.Embedded)
	}
	if result.Reused != 1 {
		t.Errorf("second Build reused %d rows, want 1 (unchanged row 1)", result.Reused)
	}
	if got := len(embedder.calls) - firstCallCount; got != 2 {
		t.Errorf("second Build called the embedder %d times, want exactly 2 (row 1 must not be re-embedded)", got)
	}
	if len(idx.Manifest.Entries) != 3 {
		t.Fatalf("index holds %d entries after second Build, want 3", len(idx.Manifest.Entries))
	}
}

// TestBuildRemovesEntriesForRowsNoLongerLive is R-069's other half: a row
// present in the index but absent from the live rows passed to Build
// (because it was soft-deleted or the project narrowed) is dropped from
// the index entirely, not left stale.
func TestBuildRemovesEntriesForRowsNoLongerLive(t *testing.T) {
	dir := t.TempDir()
	embedder := &fakeEmbedder{dim: testDim}

	rows := []Row{
		{EngramID: 1, Title: "a", Content: "alpha"},
		{EngramID: 2, Title: "b", Content: "beta"},
	}
	if _, _, err := Build(context.Background(), dir, testModel, testDim, testLimit, rows, embedder, fixedNow); err != nil {
		t.Fatalf("first Build: %v", err)
	}

	// Row 2 is no longer live.
	rows2 := []Row{
		{EngramID: 1, Title: "a", Content: "alpha"},
	}
	idx, result, err := Build(context.Background(), dir, testModel, testDim, testLimit, rows2, embedder, fixedNow)
	if err != nil {
		t.Fatalf("second Build: %v", err)
	}

	if result.Removed != 1 {
		t.Errorf("second Build removed %d entries, want 1", result.Removed)
	}
	if len(idx.Manifest.Entries) != 1 || idx.Manifest.Entries[0].EngramID != 1 {
		t.Fatalf("index holds entries %+v, want exactly [{EngramID:1}]", idx.Manifest.Entries)
	}
}

// TestBuildRefusesOnCorruptedExistingIndex: Build must not silently start
// fresh over a corrupted index -- that would mask the corruption Load is
// designed to surface (R-066).
func TestBuildRefusesOnCorruptedExistingIndex(t *testing.T) {
	dir := t.TempDir()
	embedder := &fakeEmbedder{dim: testDim}
	rows := []Row{{EngramID: 1, Title: "a", Content: "alpha"}}
	if _, _, err := Build(context.Background(), dir, testModel, testDim, testLimit, rows, embedder, fixedNow); err != nil {
		t.Fatalf("first Build: %v", err)
	}

	// Save always recomputes its own revision, so corrupt the on-disk bytes
	// directly rather than trying to force Save to persist a bad one.
	manifestPath := filepath.Join(dir, manifestFileName)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	m.Revision = "0000000000000000000000000000000000000000000000000000000000000000"
	corrupted, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("re-marshal corrupted manifest: %v", err)
	}
	if err := os.WriteFile(manifestPath, corrupted, 0o600); err != nil {
		t.Fatalf("write corrupted manifest: %v", err)
	}

	if _, _, err := Build(context.Background(), dir, testModel, testDim, testLimit, rows, embedder, fixedNow); !errors.Is(err, ErrCorrupted) {
		t.Errorf("Build over a corrupted index: got %v, want ErrCorrupted", err)
	}
}
