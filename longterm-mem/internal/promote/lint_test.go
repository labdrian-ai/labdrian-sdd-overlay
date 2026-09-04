package promote

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
)

// writeManifest writes a minimal .raw/.manifest.json carrying addressMap,
// the address_map consistency rule's on-disk source (D6/D7).
func writeManifest(t *testing.T, vaultRoot string, addressMap map[string]string) {
	t.Helper()
	dir := filepath.Join(vaultRoot, ".raw")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	data, err := json.Marshal(map[string]any{"address_map": addressMap})
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".manifest.json"), data, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

// writeIndexWithLink writes a minimal wiki/index.md containing an inbound
// wikilink to address, the inbound-index-link rule's on-disk source.
func writeIndexWithLink(t *testing.T, vaultRoot, address string) {
	t.Helper()
	dir := filepath.Join(vaultRoot, "wiki")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	content := "# Index\n\n- [[" + address + "|Page]]\n"
	if err := os.WriteFile(filepath.Join(dir, "index.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write index.md: %v", err)
	}
}

// TestLintPage_FreshlyPromotedPagePasses: R-027 scenario 4.
func TestLintPage_FreshlyPromotedPagePasses(t *testing.T) {
	fixedNow(t, time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC))
	vaultRoot := t.TempDir()

	obs := engram.Observation{ID: 42, SyncID: "sync-42", Type: "decision", Title: "Widget Rollout", Content: "Ship the widget.", Project: "labdrian-sdd-overlay", RevisionCount: 3}
	page, err := EmitPage(obs, "c-000042", nil)
	if err != nil {
		t.Fatalf("EmitPage: %v", err)
	}

	writeManifest(t, vaultRoot, map[string]string{page.Path: page.Address})
	writeIndexWithLink(t, vaultRoot, page.Address)

	if diags := LintPage(page, vaultRoot); len(diags) != 0 {
		t.Fatalf("LintPage() = %+v, want no diagnostics for a freshly promoted, registered page", diags)
	}
}

// TestLintPage_DanglingWikilinkIsFlagged exercises the
// wikilink-resolvability rule with real links: a related link whose
// target page exists under wiki/memory/ resolves silently, and a
// dangling one yields exactly one wikilink-resolvability diagnostic —
// proving the rule inspects the same directory EmitPage targets and is
// not a no-op (review finding R3-wikilink-rule-unexercised).
func TestLintPage_DanglingWikilinkIsFlagged(t *testing.T) {
	fixedNow(t, time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC))
	vaultRoot := t.TempDir()

	obs := engram.Observation{ID: 44, SyncID: "sync-44", Type: "decision", Title: "Linked", Content: "Body.", Project: "labdrian-sdd-overlay", RevisionCount: 3}
	page, err := EmitPage(obs, "c-000044", []Link{
		{Address: "c-000100", Title: "Resolves"},
		{Address: "c-000999", Title: "Dangling"},
	})
	if err != nil {
		t.Fatalf("EmitPage: %v", err)
	}

	memoryDir := filepath.Join(vaultRoot, "wiki", "memory")
	if err := os.MkdirAll(memoryDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", memoryDir, err)
	}
	if err := os.WriteFile(filepath.Join(memoryDir, "c-000100.md"), []byte("# Resolves\n"), 0o644); err != nil {
		t.Fatalf("write resolving target: %v", err)
	}

	writeManifest(t, vaultRoot, map[string]string{page.Path: page.Address})
	writeIndexWithLink(t, vaultRoot, page.Address)

	var wikilinkDiags []Diagnostic
	for _, d := range LintPage(page, vaultRoot) {
		if d.Rule == "wikilink-resolvability" {
			wikilinkDiags = append(wikilinkDiags, d)
		}
	}
	if len(wikilinkDiags) != 1 {
		t.Fatalf("wikilink-resolvability diagnostics = %+v, want exactly one (for c-000999 only)", wikilinkDiags)
	}
	if !strings.Contains(wikilinkDiags[0].Detail, "c-000999") {
		t.Fatalf("diagnostic %+v does not name the dangling address c-000999", wikilinkDiags[0])
	}
}

// TestLintPage_UnregisteredPageIsFlagged triangulates the pass case: a
// page whose address is absent from an existing (non-empty-schema)
// .raw/.manifest.json, and with no index.md link, must be flagged by the
// address_map-consistency and inbound-index-link rules, proving LintPage
// actually inspects disk state rather than always passing. (A wholly
// absent manifest passes instead -- address allocation is slice 5, not
// yet built -- so the fixture writes an empty address_map, not no file.)
func TestLintPage_UnregisteredPageIsFlagged(t *testing.T) {
	fixedNow(t, time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC))
	vaultRoot := t.TempDir()

	obs := engram.Observation{ID: 43, Type: "decision", Title: "Unregistered", Content: "Body.", Project: "labdrian-sdd-overlay"}
	page, err := EmitPage(obs, "c-000043", nil)
	if err != nil {
		t.Fatalf("EmitPage: %v", err)
	}
	writeManifest(t, vaultRoot, map[string]string{})

	diags := LintPage(page, vaultRoot)
	rules := map[string]bool{}
	for _, d := range diags {
		rules[d.Rule] = true
	}
	if !rules["address-map"] {
		t.Errorf("diagnostics = %+v, want an address-map finding for an unregistered address", diags)
	}
	if !rules["inbound-index-link"] {
		t.Errorf("diagnostics = %+v, want an inbound-index-link finding for a missing wiki/index.md", diags)
	}
}
