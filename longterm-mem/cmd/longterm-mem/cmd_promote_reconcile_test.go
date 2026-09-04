package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/promote"
)

// vaultWithUnrecordedPage writes one promoted page under a fresh vault and
// NO precedence sidecar at all -- the state a create killed between its two
// durable steps leaves, and one of the two states doctor's
// precedence-sidecar check names.
func vaultWithUnrecordedPage(t *testing.T, address string) string {
	t.Helper()
	vaultRoot := t.TempDir()
	page, err := promote.EmitPage(engram.Observation{
		ID: 701, Type: "decision", Title: "Unrecorded", Content: "Body.",
		Project: "reconcile-project", RevisionCount: 3,
	}, address, nil)
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
	return vaultRoot
}

// useVault points every vault-resolving subcommand at vaultRoot.
func useVault(t *testing.T, vaultRoot string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LONGTERM_MEM_VAULT", vaultRoot)
	t.Setenv("LONGTERM_MEM_VAULTS_FILE", filepath.Join(t.TempDir(), "vaults.json"))
}

// TestCmdPromoteReconcile_AdoptsTheNamedAddress: the ordinary path -- one
// address, named by a human, adopted into the precedence store.
func TestCmdPromoteReconcile_AdoptsTheNamedAddress(t *testing.T) {
	const address = "c-000701"
	vaultRoot := vaultWithUnrecordedPage(t, address)
	useVault(t, vaultRoot)

	if exit := runQuietly(t, []string{"promote", "reconcile", "--project", "reconcile-project", address}); exit != 0 {
		t.Fatalf("promote reconcile exited %d, want 0", exit)
	}

	store, err := promote.LoadPrecedenceStore(vaultRoot)
	if err != nil {
		t.Fatalf("LoadPrecedenceStore: %v", err)
	}
	entry, ok := store.Get(address)
	if !ok {
		t.Fatalf("sidecar has no entry for %s after reconcile", address)
	}
	if entry.PromotedRevision != 3 {
		t.Fatalf("entry = %+v, want the revision the page on disk carries (3)", entry)
	}
}

// TestCmdPromoteReconcile_RefusesEveryBulkForm is the design, not a
// convenience check.
//
// The automatic path refuses a wedged page because nothing distinguishes
// longterm-mem's own unrecorded write from a human's edit. The ONLY thing
// reconcile adds is a human naming one address, and that naming is the
// consent. A bulk form -- `--all`, or more addresses than were explicitly
// named -- would reintroduce, behind a flag, exactly the silent
// mass-adoption that ambiguity rules out, with the consent reduced to a
// single keystroke covering pages the operator never looked at.
//
// Both refusals must be explicit errors that say so, not a bare "flag
// provided but not defined": an operator who reaches for --all needs to be
// told why it does not exist.
func TestCmdPromoteReconcile_RefusesEveryBulkForm(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"an --all flag", []string{"promote", "reconcile", "--project", "reconcile-project", "--all"}},
		{"more than one address", []string{"promote", "reconcile", "--project", "reconcile-project", "c-000701", "c-000702"}},
		{"no address at all", []string{"promote", "reconcile", "--project", "reconcile-project"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vaultRoot := vaultWithUnrecordedPage(t, "c-000701")
			useVault(t, vaultRoot)

			stderr := captureStderr(t, func() {
				if exit := run(tc.args); exit != exitUsage {
					t.Fatalf("bulk reconcile exited %d, want %d (usage)", exit, exitUsage)
				}
			})
			if !strings.Contains(stderr, "one address") {
				t.Fatalf("bulk refusal said %q, want it to state that reconcile takes exactly one address", stderr)
			}

			// The refusal has to change nothing: a bulk invocation that
			// adopted even one page would be the mass adoption by
			// instalments.
			store, err := promote.LoadPrecedenceStore(vaultRoot)
			if err != nil {
				t.Fatalf("LoadPrecedenceStore: %v", err)
			}
			if len(store) != 0 {
				t.Fatalf("a refused bulk reconcile still wrote %d sidecar entr(ies)", len(store))
			}
		})
	}
}

// TestCmdPromoteReconcile_UnknownAddressExitsNotFound: an address with no
// page fails cleanly on the contract's not_found code (7), never as a
// generic internal failure and never as a silent success.
func TestCmdPromoteReconcile_UnknownAddressExitsNotFound(t *testing.T) {
	useVault(t, vaultWithUnrecordedPage(t, "c-000701"))

	if exit := runQuietly(t, []string{"promote", "reconcile", "--project", "reconcile-project", "c-999999"}); exit != exitNotFound {
		t.Fatalf("promote reconcile on an unknown address exited %d, want %d (not_found)", exit, exitNotFound)
	}
}
