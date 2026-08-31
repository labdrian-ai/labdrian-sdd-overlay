package promote

import (
	"os"
	"path/filepath"
	"testing"
)

// TestPrecedenceStore_LoadSaveRoundTrip: task 6.1 (D6). A fresh vault root
// loads an empty store; Set + Save persists body_hash/frontmatter_hash
// keyed by page_address to .raw/.longterm-mem-manifest.json via
// tmp+fsync+rename; a fresh Load on the same vault root sees the same
// entry back.
func TestPrecedenceStore_LoadSaveRoundTrip(t *testing.T) {
	vaultRoot := t.TempDir()

	store, err := LoadPrecedenceStore(vaultRoot)
	if err != nil {
		t.Fatalf("LoadPrecedenceStore (fresh vault): %v", err)
	}
	if len(store) != 0 {
		t.Fatalf("fresh store = %+v, want empty", store)
	}
	if _, ok := store.Get("c-000042"); ok {
		t.Fatalf("Get on a fresh store found an entry, want none")
	}

	store.Set("c-000042", PrecedenceEntry{BodyHash: "body-hash-1", FrontmatterHash: "fm-hash-1"})
	if err := store.Save(vaultRoot); err != nil {
		t.Fatalf("Save: %v", err)
	}

	manifestPath := filepath.Join(vaultRoot, ".raw", ".longterm-mem-manifest.json")
	if _, err := os.Stat(manifestPath); err != nil {
		t.Fatalf("Save did not write %s: %v", manifestPath, err)
	}

	reloaded, err := LoadPrecedenceStore(vaultRoot)
	if err != nil {
		t.Fatalf("LoadPrecedenceStore (after save): %v", err)
	}
	entry, ok := reloaded.Get("c-000042")
	if !ok {
		t.Fatalf("Get(c-000042) not found after reload; store: %+v", reloaded)
	}
	if entry.BodyHash != "body-hash-1" || entry.FrontmatterHash != "fm-hash-1" {
		t.Fatalf("entry = %+v, want body_hash=body-hash-1 frontmatter_hash=fm-hash-1", entry)
	}
}
