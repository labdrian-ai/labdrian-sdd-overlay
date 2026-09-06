package vecindex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func testIndex() *Index {
	return &Index{
		Manifest: Manifest{
			Model:      "nomic-embed-text",
			Dimension:  4,
			InputLimit: 2000,
			BuiltAt:    "2026-01-01T00:00:00Z",
			Entries: []ManifestEntry{
				{EngramID: 1, Fingerprint: "fp1"},
				{EngramID: 2, Fingerprint: "fp2"},
			},
		},
		Vectors: [][]float32{
			{1, 2, 3, 4},
			{5, 6, 7, 8},
		},
	}
}

// TestSaveThenLoadRoundTrips is the GREEN companion to the corruption test
// below: an index that was never tampered with must load back identical.
func TestSaveThenLoadRoundTrips(t *testing.T) {
	dir := t.TempDir()
	want := testIndex()

	if err := want.Save(dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got.Manifest.Model != want.Manifest.Model || got.Manifest.Dimension != want.Manifest.Dimension || got.Manifest.InputLimit != want.Manifest.InputLimit {
		t.Errorf("manifest contract mismatch: got %+v, want %+v", got.Manifest, want.Manifest)
	}
	if len(got.Manifest.Entries) != len(want.Manifest.Entries) {
		t.Fatalf("entries: got %d, want %d", len(got.Manifest.Entries), len(want.Manifest.Entries))
	}
	if len(got.Vectors) != len(want.Vectors) {
		t.Fatalf("vectors: got %d, want %d", len(got.Vectors), len(want.Vectors))
	}
	for i := range want.Vectors {
		for j := range want.Vectors[i] {
			if got.Vectors[i][j] != want.Vectors[i][j] {
				t.Errorf("vector[%d][%d]: got %v, want %v", i, j, got.Vectors[i][j], want.Vectors[i][j])
			}
		}
	}
}

// TestLoadDetectsManifestRevisionMismatch is design's own corruption test:
// a manifest whose recorded revision does not match a digest of its
// entries must fail to load, rather than serving stale or invalid vectors
// (R-066's "A corrupted manifest is reported, not silently trusted").
func TestLoadDetectsManifestRevisionMismatch(t *testing.T) {
	dir := t.TempDir()
	idx := testIndex()
	if err := idx.Save(dir); err != nil {
		t.Fatalf("Save: %v", err)
	}

	manifestPath := filepath.Join(dir, manifestFileName)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	// Corrupt the recorded revision's VALUE, leaving the JSON otherwise
	// valid, so the mismatch is unambiguously in the self-digest, not in a
	// parse error.
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("test setup: parse manifest: %v", err)
	}
	if m.Revision == "" {
		t.Fatal("test setup: manifest has no revision recorded")
	}
	m.Revision = "0000000000000000000000000000000000000000000000000000000000000000"
	corrupted, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("test setup: re-marshal corrupted manifest: %v", err)
	}
	if err := os.WriteFile(manifestPath, corrupted, 0o600); err != nil {
		t.Fatalf("write corrupted manifest: %v", err)
	}

	if _, err := Load(dir); err == nil {
		t.Fatal("Load must fail on a manifest whose revision does not match its entries, want a corruption error, got nil")
	}
}

// TestLoadNoIndexReportsErrNoIndex distinguishes "nothing built yet" from
// corruption -- Build (3.3/3.4) needs to tell the two apart to build fresh
// versus refuse a corrupted index outright.
func TestLoadNoIndexReportsErrNoIndex(t *testing.T) {
	dir := t.TempDir()
	if _, err := Load(dir); err != ErrNoIndex {
		t.Fatalf("Load on an empty directory: got %v, want ErrNoIndex", err)
	}
}
