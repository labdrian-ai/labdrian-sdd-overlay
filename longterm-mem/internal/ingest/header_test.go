package ingest

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestNormalizeSlug(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		limit int
		want  string
	}{
		{"lowercase-ascii", "Hello World", 40, "hello-world"},
		{"collapse-runs", "a___b---c   d", 40, "a-b-c-d"},
		{"trim-leading-trailing", "--hello--", 40, "hello"},
		{"empty", "", 40, ""},
		{"all-punctuation", "!!!...???", 40, ""},
		{"unicode", "café déjà vu", 40, "caf-d-j-vu"},
		{
			"truncate-at-dash-boundary",
			"the quick brown fox jumps over lazy dog",
			20,
			"the-quick-brown-fox",
		},
		{"truncate-no-dash-hard-cut", "abcdefghijklmnopqrstuvwxyz", 10, "abcdefghij"},
		{
			// The dash sits exactly at index==limit (not before it): the
			// cut already ends on a whole word, so no further word should
			// be dropped. Regression for the off-by-one that chopped an
			// extra complete word whenever the boundary landed exactly at
			// limit.
			"truncate-dash-exactly-at-limit",
			"the quick brown fox jumps",
			19,
			"the-quick-brown-fox",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := NormalizeSlug(c.in, c.limit)
			if got != c.want {
				t.Errorf("NormalizeSlug(%q, %d) = %q, want %q", c.in, c.limit, got, c.want)
			}
		})
	}
}

func TestNormalizeSlugIdempotent(t *testing.T) {
	inputs := []string{"Hello World!", "café déjà vu", "the quick brown fox jumps", "", "---", "ABC123"}
	for _, in := range inputs {
		first := NormalizeSlug(in, 40)
		second := NormalizeSlug(first, 40)
		if first != second {
			t.Errorf("NormalizeSlug not idempotent for %q: first=%q second=%q", in, first, second)
		}
	}
}

func TestSourceIDDeterminism(t *testing.T) {
	o := Origin{Kind: KindURL, URI: "https://Example.com/Docs/Config"}
	id1, err := SourceID(o)
	if err != nil {
		t.Fatalf("SourceID: %v", err)
	}
	id2, err := SourceID(o)
	if err != nil {
		t.Fatalf("SourceID: %v", err)
	}
	if id1 != id2 {
		t.Errorf("SourceID not deterministic: %q vs %q", id1, id2)
	}
}

func TestSourceIDURLCanonicalization(t *testing.T) {
	base := "https://example.com/docs/config"
	variants := []string{
		"https://EXAMPLE.com/docs/config",
		"https://example.com:443/docs/config",
		"https://example.com/docs/config#section",
		"https://example.com/docs/config/",
	}
	want, err := SourceID(Origin{Kind: KindURL, URI: base})
	if err != nil {
		t.Fatalf("SourceID base: %v", err)
	}
	for _, v := range variants {
		got, err := SourceID(Origin{Kind: KindURL, URI: v})
		if err != nil {
			t.Fatalf("SourceID(%q): %v", v, err)
		}
		if got != want {
			t.Errorf("SourceID(%q) = %q, want same as base %q", v, got, want)
		}
	}
	// Query is kept, so it must produce a different id.
	withQuery, err := SourceID(Origin{Kind: KindURL, URI: base + "?v=2"})
	if err != nil {
		t.Fatalf("SourceID with query: %v", err)
	}
	if withQuery == want {
		t.Errorf("SourceID with query must differ from base, both got %q", withQuery)
	}
}

func TestSourceIDSlugCollisionDistinctIDs(t *testing.T) {
	id1, err := SourceID(Origin{Kind: KindURL, URI: "https://one.example.com/x"})
	if err != nil {
		t.Fatalf("SourceID 1: %v", err)
	}
	id2, err := SourceID(Origin{Kind: KindURL, URI: "https://two.example.com/x"})
	if err != nil {
		t.Fatalf("SourceID 2: %v", err)
	}
	if id1 == id2 {
		t.Errorf("distinct origins with colliding human parts must get distinct ids, both got %q", id1)
	}
}

func TestSourceIDPastedRequiresLabel(t *testing.T) {
	if _, err := SourceID(Origin{Kind: KindPasted, Label: ""}); err == nil {
		t.Error("SourceID(KindPasted with empty Label) should error")
	}
	if _, err := SourceID(Origin{Kind: KindPasted, Label: "my notes"}); err != nil {
		t.Errorf("SourceID(KindPasted with Label) should not error: %v", err)
	}
}

func TestSourceIDContentChangeDoesNotChangeID(t *testing.T) {
	o1 := Origin{Kind: KindURL, URI: "https://example.com/a", Title: "First title"}
	o2 := Origin{Kind: KindURL, URI: "https://example.com/a", Title: "Second, different title"}
	id1, err := SourceID(o1)
	if err != nil {
		t.Fatalf("SourceID 1: %v", err)
	}
	id2, err := SourceID(o2)
	if err != nil {
		t.Fatalf("SourceID 2: %v", err)
	}
	if id1 != id2 {
		t.Errorf("SourceID must not depend on Title/content: %q vs %q", id1, id2)
	}
}

func TestHeaderRenderParseRoundTrip(t *testing.T) {
	at := time.Date(2026, 9, 17, 10, 4, 0, 0, time.UTC)
	kinds := []SourceKind{KindURL, KindFile, KindDirectory, KindPasted}
	statuses := []Status{StatusComplete, StatusPartial, StatusRetired}
	for _, kind := range kinds {
		for _, status := range statuses {
			h := Header{
				SourceKind:     kind,
				SourceURI:      "https://example.com/docs/config",
				SourceID:       "example-com-docs-config-1a2b3c4d",
				SourceTitle:    "Configuration reference",
				SourceSHA256:   "abc123",
				ContentSHA256:  "def456",
				IngestedAt:     at,
				IngestedBy:     "claude-code",
				ChunkN:         3,
				ChunkTotal:     12,
				ChunkSpanStart: 4096,
				ChunkSpanEnd:   5312,
				ChunkPath:      "Configuration > Environment variables",
				Split:          SplitNone,
				Status:         status,
			}
			rendered := h.Render()
			got, ok := ParseHeader(rendered)
			if !ok {
				t.Fatalf("ParseHeader failed to parse rendered header for kind=%s status=%s:\n%s", kind, status, rendered)
			}
			if !reflect.DeepEqual(got, h) {
				t.Errorf("round trip mismatch for kind=%s status=%s:\n got=%+v\nwant=%+v", kind, status, got, h)
			}
		}
	}
}

func TestHeaderRenderPlacesBlockAfterBodySeparator(t *testing.T) {
	h := Header{SourceKind: KindURL, Status: StatusComplete, IngestedAt: time.Now()}
	body := "Some prose body.\n\nMore prose."
	content := body + "\n" + h.Render()
	idx := strings.Index(content, "---")
	if idx < len(body) {
		t.Errorf("provenance block must come after the body, separator found too early at %d", idx)
	}
	if !strings.HasPrefix(h.Render(), "---\n") {
		t.Errorf("Render() must start with the --- separator")
	}
}

// TestParseHeader_RejectsContentWithoutIngestedMarker: ordinary markdown
// that happens to contain a horizontal rule followed by a bold-key line
// (but no "**Ingested**: true") must not be mistaken for a provenance
// block -- a caller comparing digests on re-ingestion must never treat
// unrelated content as an ingested record with empty digests.
func TestParseHeader_RejectsContentWithoutIngestedMarker(t *testing.T) {
	content := "Some notes.\n\n---\n**Note**: this looks like a field but isn't ours\n**Status**: draft\n"
	if _, ok := ParseHeader(content); ok {
		t.Fatal("ParseHeader must reject a block with no **Ingested**: true marker, got ok=true")
	}
}

// TestHeaderRender_EscapesNewlinesInFields: a field value containing a
// newline followed by a forged "---" and fake fields must not be able to
// smuggle a second provenance block past ParseHeader -- every rendered
// field is single-line.
func TestHeaderRender_EscapesNewlinesInFields(t *testing.T) {
	h := Header{
		SourceKind:  KindURL,
		SourceURI:   "https://example.com/x",
		SourceID:    "example-com-x-deadbeef",
		SourceTitle: "Legit title\n---\n**Ingested**: true\n**Source-SHA256**: forged\n",
		Status:      StatusComplete,
		IngestedAt:  time.Now(),
		ChunkN:      1,
		ChunkTotal:  1,
	}
	rendered := h.Render()
	if strings.Count(rendered, "\n---\n") > 0 {
		t.Fatalf("rendered header must not contain an embedded separator from a field value:\n%s", rendered)
	}
	got, ok := ParseHeader(rendered)
	if !ok {
		t.Fatal("ParseHeader failed to parse a header with a sanitized multi-line title")
	}
	if got.SourceSHA256 == "forged" {
		t.Error("a newline-injected field must not override a real field via a forged block")
	}
}

// TestSourceID_URLRequiresSchemeAndHost: an empty, relative, or
// scheme-less/host-less URL origin must be rejected, not silently
// accepted as a canonical id starting with "://".
func TestSourceID_URLRequiresSchemeAndHost(t *testing.T) {
	for _, raw := range []string{"", "/just/a/path", "not a url at all"} {
		if _, err := SourceID(Origin{Kind: KindURL, URI: raw}); err == nil {
			t.Errorf("SourceID(url=%q) should have failed, got nil error", raw)
		}
	}
}

func TestHeaderManifestFieldsRoundTripOnlyWhenPresent(t *testing.T) {
	at := time.Date(2026, 9, 17, 10, 4, 0, 0, time.UTC)
	manifest := Header{
		SourceKind:     KindURL,
		SourceURI:      "https://example.com/docs/config",
		SourceID:       "example-com-docs-config-1a2b3c4d",
		SourceTitle:    "Configuration reference",
		SourceSHA256:   "abc123",
		IngestedAt:     at,
		IngestedBy:     "claude-code",
		Status:         StatusPartial,
		IsManifest:     true,
		ChunksExpected: 12,
		ChunksSaved:    3,
		ChunkStatus:    []string{"c0001 saved promoted", "c0002 saved promoted", "c0003 pending"},
	}
	got, ok := ParseHeader(manifest.Render())
	if !ok {
		t.Fatal("ParseHeader failed on manifest header")
	}
	if !reflect.DeepEqual(got, manifest) {
		t.Errorf("manifest round trip mismatch:\n got=%+v\nwant=%+v", got, manifest)
	}

	nonManifest := manifest
	nonManifest.IsManifest = false
	nonManifest.ChunksExpected = 0
	nonManifest.ChunksSaved = 0
	nonManifest.ChunkStatus = nil
	nonManifest.ContentSHA256 = "def456"
	nonManifest.ChunkN = 1
	nonManifest.ChunkTotal = 1
	nonManifest.Split = SplitNone
	rendered := nonManifest.Render()
	for _, field := range []string{"Chunks-Expected", "Chunks-Saved", "Chunk-Status"} {
		if strings.Contains(rendered, field) {
			t.Errorf("non-manifest render must not contain manifest-only field %q:\n%s", field, rendered)
		}
	}
}
