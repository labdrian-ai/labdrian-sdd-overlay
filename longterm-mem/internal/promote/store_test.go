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

// TestPrecedenceEntry_MatchesPage_FailsClosedWithoutFrontmatter pins the
// OUTCOME MatchesPage's doc promises for a file with no parseable
// frontmatter block, including the zero-value entry a hand-truncated sidecar
// (`{"c-000042":{}}`) decodes into. It deliberately does not claim to pin the
// early return itself: deleting that return leaves this test green, because
// the degenerate split hashes the empty string and no entry carries that
// digest -- which is why the comment there says the return states the intent
// rather than enforcing it. What must not change is the answer: false.
func TestPrecedenceEntry_MatchesPage_FailsClosedWithoutFrontmatter(t *testing.T) {
	if (PrecedenceEntry{}).MatchesPage("") {
		t.Fatalf("an entry recording empty hashes matched an empty page; doctor would report a wedged page as healthy")
	}
	entry := PrecedenceEntry{BodyHash: hashText("body"), FrontmatterHash: hashText("---\ntitle: \"T\"\n---\n")}
	if entry.MatchesPage("no frontmatter here, just prose\n") {
		t.Fatalf("an entry matched a page with no parseable frontmatter block")
	}
}
