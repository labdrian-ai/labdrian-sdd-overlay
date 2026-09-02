package promote

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
)

var update = flag.Bool("update", false, "update golden files")

// fixedNow swaps nowFunc for the duration of the test so EmitPage's
// created/updated timestamps -- and hence golden comparisons -- are
// deterministic.
func fixedNow(t *testing.T, at time.Time) {
	t.Helper()
	original := nowFunc
	nowFunc = func() time.Time { return at }
	t.Cleanup(func() { nowFunc = original })
}

// TestEmitPage_TypeMappedOntoVaultEnum: R-027 scenario 1.
// TestQuoteYAML_EscapesBackslashes proves quoteYAML emits a valid YAML
// double-quoted scalar for titles carrying backslashes: an embedded
// backslash must be escaped (`\\`) and a trailing backslash must not
// swallow the closing quote.
func TestQuoteYAML_EscapesBackslashes(t *testing.T) {
	cases := map[string]string{
		`path C:\temp\x`: `"path C:\\temp\\x"`,
		`trailing\`:      `"trailing\\"`,
		`quote " and \`:  `"quote \" and \\"`,
	}
	for in, want := range cases {
		if got := quoteYAML(in); got != want {
			t.Fatalf("quoteYAML(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestEmitPage_TypeMappedOntoVaultEnum(t *testing.T) {
	fixedNow(t, time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC))

	obs := engram.Observation{
		ID: 101, SyncID: "sync-101", Type: "decision", Title: "Widget Decision",
		Content: "We decided to ship the widget.", Project: "labdrian-sdd-overlay",
		RevisionCount: 1, Pinned: true,
	}

	page, err := EmitPage(obs, "c-000101", nil)
	if err != nil {
		t.Fatalf("EmitPage: %v", err)
	}

	if !strings.Contains(page.Frontmatter, "\ntype: concept\n") {
		t.Fatalf("frontmatter type is not mapped onto the vault enum; got:\n%s", page.Frontmatter)
	}
	for _, extra := range []string{"engram_type: decision\n", "engram_id: 101\n", "project: labdrian-sdd-overlay\n"} {
		if !strings.Contains(page.Frontmatter, extra) {
			t.Fatalf("frontmatter missing extra %q; got:\n%s", extra, page.Frontmatter)
		}
	}
}

// TestEmitPage_RelatedLinksResolve: R-027 scenario 2.
func TestEmitPage_RelatedLinksResolve(t *testing.T) {
	fixedNow(t, time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC))
	vaultRoot := t.TempDir()

	otherDir := filepath.Join(vaultRoot, "wiki", "memory")
	if err := os.MkdirAll(otherDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", otherDir, err)
	}
	otherPath := filepath.Join(otherDir, "c-000099.md")
	if err := os.WriteFile(otherPath, []byte("already promoted"), 0o644); err != nil {
		t.Fatalf("write %s: %v", otherPath, err)
	}

	obs := engram.Observation{ID: 102, Type: "discovery", Title: "Follow-up Discovery", Content: "Details.", Project: "labdrian-sdd-overlay", RevisionCount: 3}
	related := []Link{{Address: "c-000099", Title: "Other Page"}}

	page, err := EmitPage(obs, "c-000102", related)
	if err != nil {
		t.Fatalf("EmitPage: %v", err)
	}

	wantLink := "[[c-000099|Other Page]]"
	if !strings.Contains(page.Frontmatter, wantLink) {
		t.Fatalf("frontmatter related does not include %q; got:\n%s", wantLink, page.Frontmatter)
	}
	if _, err := os.Stat(otherPath); err != nil {
		t.Fatalf("related link target does not resolve to an existing file: %v", err)
	}
}

// TestEmitPage_FilenameSurvivesRetitle: R-027 scenario 3.
func TestEmitPage_FilenameSurvivesRetitle(t *testing.T) {
	fixedNow(t, time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC))

	obs := engram.Observation{ID: 103, Type: "pattern", Title: "Original Title", Content: "Body.", Project: "labdrian-sdd-overlay"}
	first, err := EmitPage(obs, "c-000103", nil)
	if err != nil {
		t.Fatalf("EmitPage (original title): %v", err)
	}

	obs.Title = "Renamed Title"
	second, err := EmitPage(obs, "c-000103", nil)
	if err != nil {
		t.Fatalf("EmitPage (retitled): %v", err)
	}

	if first.Path != second.Path {
		t.Fatalf("Path changed across a retitle: %q -> %q", first.Path, second.Path)
	}
	if first.Path != "wiki/memory/c-000103.md" {
		t.Fatalf("Path = %q, want address-derived wiki/memory/c-000103.md", first.Path)
	}
}

// TestEmitPage_MatchesGolden locks the fixed frontmatter field order (D7)
// and body shape byte-for-byte (go-testing golden pattern, task 4.9).
func TestEmitPage_MatchesGolden(t *testing.T) {
	fixedNow(t, time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC))

	obs := engram.Observation{
		ID: 200, SyncID: "sync-200", Type: "architecture", Title: "Read-Only Store",
		Content: "The store opens Engram read-only.", Project: "labdrian-sdd-overlay",
		RevisionCount: 2, Pinned: false,
	}
	related := []Link{{Address: "c-000099", Title: "Other Page"}}

	page, err := EmitPage(obs, "c-000200", related)
	if err != nil {
		t.Fatalf("EmitPage: %v", err)
	}
	got := page.Frontmatter + page.Body

	goldenPath := filepath.Join("testdata", "pages", "architecture.golden.md")
	if *update {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(goldenPath), err)
		}
		if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden %s: %v", goldenPath, err)
		}
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %s: %v", goldenPath, err)
	}
	if got != string(want) {
		t.Fatalf("emitted page does not match golden %s\n--- got ---\n%s\n--- want ---\n%s", goldenPath, got, want)
	}
}
