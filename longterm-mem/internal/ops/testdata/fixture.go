// Package testdata is internal/ops's shared test-fixture builder (8a.7
// REFACTOR), reused by status_test.go and doctor_test.go: a temp vault
// root's sync-state record, a promoted page, its address-map entry, and
// its catalog/log registration -- the same on-disk shapes Status and
// Doctor each independently read.
package testdata

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/promote"
)

// WriteSyncState writes a minimal sync-state record at vaultRoot's
// contract path (.vault-meta/longterm-mem-sync-state.json, mirroring
// promote.syncStateRelPath), simulating a prior successful sync.
func WriteSyncState(t *testing.T, vaultRoot, completedAt string) {
	t.Helper()
	dir := filepath.Join(vaultRoot, ".vault-meta")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	content := `{"schema":1,"last_sync_completed_at":"` + completedAt + `"}`
	full := filepath.Join(dir, "longterm-mem-sync-state.json")
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
}

// WritePromotedPage renders a minimal promoted page via promote.EmitPage
// and writes it to vaultRoot/wiki/memory/<address>.md, mirroring
// internal/promote's own seedPromotedPage/writePromotedPage convention --
// doctor's address-map/registration checks operate on exactly this
// on-disk shape (a real promote.Page written by Writer.Promote in
// production).
func WritePromotedPage(t *testing.T, vaultRoot, address, title string) promote.Page {
	t.Helper()
	obs := engram.Observation{ID: 1, Type: "decision", Title: title, Content: "Body content.", Project: "labdrian-sdd-overlay", RevisionCount: 1}
	page, err := promote.EmitPage(obs, address, nil)
	if err != nil {
		t.Fatalf("EmitPage: %v", err)
	}
	full := filepath.Join(vaultRoot, page.Path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte(page.Frontmatter+page.Body), 0o644); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
	return page
}

// WriteAddressMap writes .raw/.manifest.json's address_map, the on-disk
// source promote.LintPage's address-map-consistency rule checks against
// (lint.go's checkAddressMap).
func WriteAddressMap(t *testing.T, vaultRoot string, addressMap map[string]string) {
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

// RegisterPage records address/title in the vault's catalog
// (wiki/index.md) and append-only log (wiki/log.md) via the real
// production registrars (register.go), so a "fully registered" fixture
// exercises the exact writers Writer.Promote itself calls (task 7.10).
func RegisterPage(t *testing.T, vaultRoot, address, title string) {
	t.Helper()
	if err := promote.RegisterIndex(filepath.Join(vaultRoot, "wiki", "index.md"), address, title); err != nil {
		t.Fatalf("RegisterIndex: %v", err)
	}
	if err := promote.RegisterLog(filepath.Join(vaultRoot, "wiki", "log.md"), address, title, time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("RegisterLog: %v", err)
	}
}
