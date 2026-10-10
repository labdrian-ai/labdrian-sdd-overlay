package promote

import "testing"

// TestPrecedenceStore_GetAndSet: an entry set under a page address is the one got back, and an address nobody
// set has none. Persisting the store is the repository adapter's concern (internal/vaultfs), not the store's.
func TestPrecedenceStore_GetAndSet(t *testing.T) {
	store := PrecedenceStore{}
	if _, ok := store.Get("c-000042"); ok {
		t.Fatalf("Get on an empty store found an entry, want none")
	}

	want := PrecedenceEntry{BodyHash: "body-hash-1", FrontmatterHash: "fm-hash-1", PromotedRevision: 3}
	store.Set("c-000042", want)
	got, ok := store.Get("c-000042")
	if !ok || got != want {
		t.Fatalf("Get(c-000042) = %+v, %v, want %+v", got, ok, want)
	}
	if _, ok := store.Get("c-000043"); ok {
		t.Fatalf("Get found an entry for an address nobody set")
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
