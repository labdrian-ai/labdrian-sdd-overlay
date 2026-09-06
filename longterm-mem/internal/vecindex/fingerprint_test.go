package vecindex

import "testing"

// TestFingerprintIgnoresContentBeyondInputLimit pins design's "one fidelity
// trap the port must not fall into" (see design.md's "What this validation
// does NOT establish"): embeddings cover only the first inputLimit
// characters, so a row edited only past that point keeps a valid
// fingerprint (and a valid vector) while its rendered snippet changes
// elsewhere. That is correct under R-066/§2.1, and this test is what would
// catch someone accidentally hashing the full, untruncated content instead.
func TestFingerprintIgnoresContentBeyondInputLimit(t *testing.T) {
	const model = "nomic-embed-text"
	const dim = 768
	const limit = 10

	short := "0123456789"     // exactly at the limit
	long := "0123456789EXTRA" // identical up to the limit, then diverges

	got1 := Fingerprint(model, dim, limit, "title", short)
	got2 := Fingerprint(model, dim, limit, "title", long)

	if got1 != got2 {
		t.Fatalf("Fingerprint should ignore content beyond input_limit=%d, got %q != %q", limit, got1, got2)
	}
}

// TestFingerprintChangesWithContractFields pins the design's "the
// fingerprint puts the embedding contract inside the hash": model,
// dimension, and input_limit are hashed alongside the content, so changing
// any one of them invalidates every fingerprint mechanically.
func TestFingerprintChangesWithContractFields(t *testing.T) {
	base := Fingerprint("model-a", 768, 2000, "title", "content")

	if got := Fingerprint("model-b", 768, 2000, "title", "content"); got == base {
		t.Error("changing model must change the fingerprint")
	}
	if got := Fingerprint("model-a", 1024, 2000, "title", "content"); got == base {
		t.Error("changing dimension must change the fingerprint")
	}
	if got := Fingerprint("model-a", 768, 500, "title", "content"); got == base {
		t.Error("changing input_limit must change the fingerprint")
	}
	if got := Fingerprint("model-a", 768, 2000, "title", "different content"); got == base {
		t.Error("changing content must change the fingerprint")
	}
}
