package vecindex

import (
	"testing"
)

// newLoadCacheForTest builds a LoadCache whose underlying loader is wrapped
// to count real disk loads, so a test can tell a cache hit from an actual
// read of manifest.json + vectors.blob.
func newLoadCacheForTest(loads *int) *LoadCache {
	c := NewLoadCache()
	real := c.load
	c.load = func(dir string) (*Index, error) {
		*loads++
		return real(dir)
	}
	return c
}

func mustSaveTestIndex(t *testing.T, dir string, entries []ManifestEntry, vectors [][]float32) {
	t.Helper()
	idx := &Index{
		Manifest: Manifest{Model: "test-model", Dimension: 3, InputLimit: 2000, BuiltAt: "2026-01-01T00:00:00Z", Entries: entries},
		Vectors:  vectors,
	}
	if err := idx.Save(dir); err != nil {
		t.Fatalf("save test index: %v", err)
	}
}

// TestLoadCache_LoadsFromDiskOnceForRepeatedCalls is this issue's session-
// scoped caching guarantee: two queries for the same project in one
// process must load the index from disk only once.
func TestLoadCache_LoadsFromDiskOnceForRepeatedCalls(t *testing.T) {
	dir := t.TempDir()
	mustSaveTestIndex(t, dir, []ManifestEntry{{EngramID: 1, Fingerprint: "fp1"}}, [][]float32{{1, 0, 0}})

	loads := 0
	cache := newLoadCacheForTest(&loads)

	if _, err := cache.Load(dir); err != nil {
		t.Fatalf("first Load: %v", err)
	}
	if _, err := cache.Load(dir); err != nil {
		t.Fatalf("second Load: %v", err)
	}
	if loads != 1 {
		t.Fatalf("underlying disk loads = %d, want exactly 1 across two Load calls", loads)
	}
}

// TestLoadCache_InvalidateForcesReload is the other half: a query that
// runs after this process's own bounded top-up build (embedarm.go) must
// see the topped-up index, not a stale cached one.
//
// It deliberately never rewrites dir's manifest file at all: an mtime/size
// change alone would already force Load to bypass the cache regardless of
// Invalidate, which would prove nothing about Invalidate itself. What this
// pins is Invalidate's own contract -- forcing a reload of the identical
// on-disk bytes -- which is exactly the guarantee query's bounded top-up
// needs when it calls Invalidate right after its own vecindex.Build write,
// rather than trusting that write to have visibly changed mtime/size
// within the same query.
func TestLoadCache_InvalidateForcesReload(t *testing.T) {
	dir := t.TempDir()
	mustSaveTestIndex(t, dir, []ManifestEntry{{EngramID: 1, Fingerprint: "fp1"}}, [][]float32{{1, 0, 0}})

	loads := 0
	cache := newLoadCacheForTest(&loads)

	if _, err := cache.Load(dir); err != nil {
		t.Fatalf("first Load: %v", err)
	}
	if _, err := cache.Load(dir); err != nil {
		t.Fatalf("second Load (should be a cache hit): %v", err)
	}
	if loads != 1 {
		t.Fatalf("underlying disk loads = %d after two Loads with no Invalidate, want 1", loads)
	}

	cache.Invalidate(dir)
	if _, err := cache.Load(dir); err != nil {
		t.Fatalf("Load after Invalidate: %v", err)
	}
	if loads != 2 {
		t.Fatalf("underlying disk loads = %d, want exactly 2 (initial load + forced reload after Invalidate)", loads)
	}
}

// TestLoadCache_FailedLoadIsNotCached: a directory with no index yet must
// not poison the cache with a permanent failure -- once an index appears
// there (e.g. the first `index --embeddings` run), the next Load must see
// it rather than replaying the earlier ErrNoIndex forever.
func TestLoadCache_FailedLoadIsNotCached(t *testing.T) {
	dir := t.TempDir()
	loads := 0
	cache := newLoadCacheForTest(&loads)

	if _, err := cache.Load(dir); err == nil {
		t.Fatalf("Load on an empty dir succeeded, want ErrNoIndex")
	}

	mustSaveTestIndex(t, dir, []ManifestEntry{{EngramID: 1, Fingerprint: "fp1"}}, [][]float32{{1, 0, 0}})

	idx, err := cache.Load(dir)
	if err != nil {
		t.Fatalf("Load after the index appeared: %v", err)
	}
	if len(idx.Manifest.Entries) != 1 {
		t.Fatalf("idx.Manifest.Entries = %+v, want 1 entry", idx.Manifest.Entries)
	}
	if loads != 2 {
		t.Fatalf("underlying disk loads = %d, want exactly 2 (the failed attempt must not be cached)", loads)
	}
}
