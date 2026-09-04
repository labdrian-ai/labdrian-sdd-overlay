package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/promote"
)

// TestCmdPromoteReconcile_ExitCodesAreTheContract pins every refusal
// reconcile maps by sentinel onto its EXACT published code.
//
// Nothing held them there. Replacing the ErrInvalidAddress and
// ErrNotThatPage cases with an unreachable branch left this package green
// while a mistyped address silently became exit 1 (internal) -- the exact
// miscategorisation the codes exist to prevent: a caller reading 1 cannot
// tell "you typed the address wrong" from "longterm-mem broke". The codes
// are a contract precisely because a caller acts on them without reading
// stderr, so each one is asserted by number here.
func TestCmdPromoteReconcile_ExitCodesAreTheContract(t *testing.T) {
	const project = "reconcile-project"

	t.Run("a malformed address is a usage error", func(t *testing.T) {
		useVault(t, vaultWithUnrecordedPage(t, "c-000701"))
		if exit := runQuietly(t, []string{"promote", "reconcile", "--project", project, "../../secret"}); exit != exitUsage {
			t.Fatalf("promote reconcile on a traversal address exited %d, want %d (usage) -- the operator fixes what they typed, and exit %d would report longterm-mem's own fault", exit, exitUsage, exitInternal)
		}
	})

	t.Run("another project's page is a registration conflict", func(t *testing.T) {
		const address = "c-000702"
		vaultRoot := t.TempDir()
		page, err := promote.EmitPage(engram.Observation{
			ID: 702, Type: "decision", Title: "Other Project", Content: "Body.",
			Project: "someone-elses-project", RevisionCount: 3,
		}, address, nil)
		if err != nil {
			t.Fatalf("EmitPage: %v", err)
		}
		writePageInto(t, vaultRoot, page.Path, page.Frontmatter+page.Body)
		useVault(t, vaultRoot)

		if exit := runQuietly(t, []string{"promote", "reconcile", "--project", project, address}); exit != exitRegistrationConflict {
			t.Fatalf("promote reconcile on another project's page exited %d, want %d (registration_conflict) -- an artifact occupies the target and nothing was written", exit, exitRegistrationConflict)
		}
	})

	t.Run("a page with no readable revision is a registration conflict", func(t *testing.T) {
		const address = "c-000703"
		vaultRoot := t.TempDir()
		page, err := promote.EmitPage(engram.Observation{
			ID: 703, Type: "decision", Title: "Unreadable", Content: "Body.",
			Project: project, RevisionCount: 3,
		}, address, nil)
		if err != nil {
			t.Fatalf("EmitPage: %v", err)
		}
		stripped := strings.Replace(page.Frontmatter, "engram_revision: 3\n", "", 1)
		if stripped == page.Frontmatter {
			t.Fatalf("fixture did not strip the revision line; frontmatter was:\n%s", page.Frontmatter)
		}
		writePageInto(t, vaultRoot, page.Path, stripped+page.Body)
		useVault(t, vaultRoot)

		if exit := runQuietly(t, []string{"promote", "reconcile", "--project", project, address}); exit != exitRegistrationConflict {
			t.Fatalf("promote reconcile on a page with no readable revision exited %d, want %d (registration_conflict) -- doctor names exactly such a page by address, so answering exit %d (internal) tells the operator to report a bug instead of repairing the page", exit, exitRegistrationConflict, exitInternal)
		}
	})
}

// writePageInto writes one page body at rel under vaultRoot.
func writePageInto(t *testing.T, vaultRoot, rel, content string) {
	t.Helper()
	full := filepath.Join(vaultRoot, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
}
