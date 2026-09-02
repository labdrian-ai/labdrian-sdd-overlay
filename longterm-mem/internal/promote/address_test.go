package promote

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
)

// allocateAddressFixture is a fake scripts/allocate-address.sh: a real
// shell entrypoint (shebang + exec bit, matching the real script's
// convention) that always allocates c-000042 and exits 0.
const allocateAddressFixture = "#!/bin/sh\necho c-000042\n"

// writeAllocateScript materializes a fixture scripts/allocate-address.sh
// under vaultRoot with body.
func writeAllocateScript(t *testing.T, vaultRoot, body string) {
	t.Helper()
	dir := filepath.Join(vaultRoot, "scripts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "allocate-address.sh"), []byte(body), 0o755); err != nil {
		t.Fatalf("write fixture allocate-address.sh: %v", err)
	}
}

// readAddressMap reads back .raw/.manifest.json's address_map.
func readAddressMap(t *testing.T, vaultRoot string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(vaultRoot, ".raw", ".manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var m struct {
		AddressMap map[string]string `json:"address_map"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	return m.AddressMap
}

// TestAllocate_FirstPromotionAllocatesNewAddress: R-028 scenario 1.
func TestAllocate_FirstPromotionAllocatesNewAddress(t *testing.T) {
	vaultRoot := t.TempDir()
	writeAllocateScript(t, vaultRoot, allocateAddressFixture)

	address, err := Allocate(vaultRoot, "labdrian-sdd-overlay", 101)
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if address != "c-000042" {
		t.Fatalf("address = %q, want %q", address, "c-000042")
	}

	addressMap := readAddressMap(t, vaultRoot)
	wantPath := "wiki/memory/c-000042.md"
	if got := addressMap[wantPath]; got != "c-000042" {
		t.Fatalf("address_map[%q] = %q, want %q (full map: %+v)", wantPath, got, "c-000042", addressMap)
	}
}

// TestAllocate_RePromotionReusesExistingAddress: R-028 scenario 2. No
// scripts/allocate-address.sh fixture exists at all -- if Allocate
// attempted to invoke it, Runner would fail to resolve the script and the
// call would return an error, proving reuse never reaches the script.
func TestAllocate_RePromotionReusesExistingAddress(t *testing.T) {
	vaultRoot := t.TempDir()

	memoryDir := filepath.Join(vaultRoot, "wiki", "memory")
	if err := os.MkdirAll(memoryDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", memoryDir, err)
	}
	obs := engram.Observation{ID: 101, Type: "decision", Title: "Already Promoted", Content: "Body.", Project: "labdrian-sdd-overlay"}
	page, err := EmitPage(obs, "c-000099", nil)
	if err != nil {
		t.Fatalf("EmitPage: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vaultRoot, page.Path), []byte(page.Frontmatter+page.Body), 0o644); err != nil {
		t.Fatalf("write pre-promoted page: %v", err)
	}

	address, err := Allocate(vaultRoot, "labdrian-sdd-overlay", 101)
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if address != "c-000099" {
		t.Fatalf("address = %q, want the reused %q (script must not have been invoked)", address, "c-000099")
	}
}

// TestAllocate_RecordAddressPreservesForeignManifestFields: the manifest
// is wiki-ingest-owned (D7); recordAddress may only touch address_map. A
// producer field this package does not know must survive a fresh
// allocation, and keys absent from the live file must not be fabricated
// (findings R1-manifest-field-drop, R4-manifest-unknown-field-loss).
func TestAllocate_RecordAddressPreservesForeignManifestFields(t *testing.T) {
	vaultRoot := t.TempDir()
	writeAllocateScript(t, vaultRoot, allocateAddressFixture)

	rawDir := filepath.Join(vaultRoot, ".raw")
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", rawDir, err)
	}
	live := `{"version": 7, "ingest_options": {"dedupe": true}, "address_map": {"wiki/memory/c-000001.md": "c-000001"}}`
	if err := os.WriteFile(filepath.Join(rawDir, ".manifest.json"), []byte(live), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	if _, err := Allocate(vaultRoot, "labdrian-sdd-overlay", 101); err != nil {
		t.Fatalf("Allocate: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(rawDir, ".manifest.json"))
	if err != nil {
		t.Fatalf("read manifest back: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse manifest back: %v", err)
	}
	if got := m["version"]; got != float64(7) {
		t.Errorf("version = %v, want 7 (must not be reset)", got)
	}
	if _, ok := m["ingest_options"]; !ok {
		t.Errorf("ingest_options was dropped; foreign fields must survive (manifest: %s)", data)
	}
	for _, key := range []string{"created", "description", "sources"} {
		if _, ok := m[key]; ok {
			t.Errorf("%q was fabricated into a manifest that did not carry it (manifest: %s)", key, data)
		}
	}
	addressMap := readAddressMap(t, vaultRoot)
	if addressMap["wiki/memory/c-000001.md"] != "c-000001" || addressMap["wiki/memory/c-000042.md"] != "c-000042" {
		t.Errorf("address_map = %+v, want the prior entry retained and the new one added", addressMap)
	}
}

// TestAllocate_ReuseWithoutAddressFails: a page matching this engram_id +
// project whose frontmatter carries no address must fail the promotion,
// mirroring the fresh path's "produced no address" guard, never succeed
// with an empty address (findings R2/R3/R4 reuse-empty-address). No
// allocator fixture exists: silently falling through to a fresh
// allocation for an already-promoted page would error too.
func TestAllocate_ReuseWithoutAddressFails(t *testing.T) {
	vaultRoot := t.TempDir()
	memoryDir := filepath.Join(vaultRoot, "wiki", "memory")
	if err := os.MkdirAll(memoryDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", memoryDir, err)
	}
	page := "---\nengram_id: 101\nproject: labdrian-sdd-overlay\n---\n\nBody.\n"
	if err := os.WriteFile(filepath.Join(memoryDir, "c-000099.md"), []byte(page), 0o644); err != nil {
		t.Fatalf("write pre-promoted page: %v", err)
	}

	address, err := Allocate(vaultRoot, "labdrian-sdd-overlay", 101)
	if err == nil {
		t.Fatalf("Allocate = (%q, nil), want an error for a matched page without an address", address)
	}
	if !strings.Contains(err.Error(), "c-000099.md") {
		t.Fatalf("error %q does not name the offending page c-000099.md", err)
	}
}

// TestFindPromotedPage_RevisionRoundTrips: task 7.10 Gap 2 coverage. The
// 7a REFACTOR widened findPromotedPage's return from a bare address to
// promotedPage{Address, Revision}, but address_test.go was never touched
// to exercise Revision directly -- Sync's own tests only ever see it
// indirectly through re-promotion decisions.
func TestFindPromotedPage_RevisionRoundTrips(t *testing.T) {
	vaultRoot := t.TempDir()
	obs := engram.Observation{ID: 201, Type: "decision", Title: "Revisioned", Content: "Body.", Project: "labdrian-sdd-overlay", RevisionCount: 5}
	page, err := EmitPage(obs, "c-000201", nil)
	if err != nil {
		t.Fatalf("EmitPage: %v", err)
	}
	full := filepath.Join(vaultRoot, page.Path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte(page.Frontmatter+page.Body), 0o644); err != nil {
		t.Fatalf("write page: %v", err)
	}

	promoted, ok, err := findPromotedPage(vaultRoot, "labdrian-sdd-overlay", 201)
	if err != nil {
		t.Fatalf("findPromotedPage: %v", err)
	}
	if !ok {
		t.Fatalf("findPromotedPage ok = false, want true")
	}
	if promoted.Address != "c-000201" {
		t.Fatalf("Address = %q, want c-000201", promoted.Address)
	}
	if promoted.Revision != 5 {
		t.Fatalf("Revision = %d, want 5 (the promoted page's own engram_revision)", promoted.Revision)
	}
}

// TestFindPromotedPage_MissingRevisionDefaultsToZero: task 7.10 Gap 2
// coverage. A page with no engram_revision field at all (a page promoted
// before D7's engram_revision field existed, or a hand-authored one)
// must report Revision 0, not error -- distinguishing "never recorded" a
// revision from "recorded an unparseable one".
func TestFindPromotedPage_MissingRevisionDefaultsToZero(t *testing.T) {
	vaultRoot := t.TempDir()
	memoryDir := filepath.Join(vaultRoot, "wiki", "memory")
	if err := os.MkdirAll(memoryDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", memoryDir, err)
	}
	page := "---\nengram_id: 202\nproject: labdrian-sdd-overlay\naddress: c-000202\n---\n\nBody.\n"
	if err := os.WriteFile(filepath.Join(memoryDir, "c-000202.md"), []byte(page), 0o644); err != nil {
		t.Fatalf("write pre-promoted page: %v", err)
	}

	promoted, ok, err := findPromotedPage(vaultRoot, "labdrian-sdd-overlay", 202)
	if err != nil {
		t.Fatalf("findPromotedPage: %v", err)
	}
	if !ok {
		t.Fatalf("findPromotedPage ok = false, want true")
	}
	if promoted.Revision != 0 {
		t.Fatalf("Revision = %d, want 0 for a page with no engram_revision field (not an error)", promoted.Revision)
	}
}

// TestFindPromotedPage_UnparseableRevisionErrors: task 7.10 Gap 2
// coverage. A page whose engram_revision cannot be parsed as an integer
// is corrupted promotion state and must error, naming the offending
// page -- never silently treated as revision 0, which could make Sync
// (R-009) skip re-promoting content that is not actually current.
func TestFindPromotedPage_UnparseableRevisionErrors(t *testing.T) {
	vaultRoot := t.TempDir()
	memoryDir := filepath.Join(vaultRoot, "wiki", "memory")
	if err := os.MkdirAll(memoryDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", memoryDir, err)
	}
	page := "---\nengram_id: 203\nproject: labdrian-sdd-overlay\naddress: c-000203\nengram_revision: not-a-number\n---\n\nBody.\n"
	if err := os.WriteFile(filepath.Join(memoryDir, "c-000203.md"), []byte(page), 0o644); err != nil {
		t.Fatalf("write pre-promoted page: %v", err)
	}

	_, _, err := findPromotedPage(vaultRoot, "labdrian-sdd-overlay", 203)
	if err == nil {
		t.Fatalf("findPromotedPage = nil error, want an error for an unparseable engram_revision")
	}
	if !strings.Contains(err.Error(), "c-000203.md") {
		t.Fatalf("error %q does not name the offending page c-000203.md", err)
	}
}
