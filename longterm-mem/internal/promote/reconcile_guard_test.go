package promote

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
)

// TestReconcile_ShapeCheckIsTheOnlyThingRefusingATraversalThatNamesItself
// pins the address-shape guard to the ONE case only that guard can refuse.
//
// The sibling test (TestReconcile_RefusesAnAddressOutsideThePagesDirectory)
// does not pin it: every address it passes is caught by something else --
// the frontmatter-identity check catches `../../secret`, plain not-found
// catches `nested/c-000607` and `c-000607.md`, and the empty-string branch
// catches "". Reverting the shape check to the old `address == ""` leaves
// that test green, so the guard could be deleted without a single failure.
//
// The case that isolates it is a hand-made file OUTSIDE wiki/memory whose
// own frontmatter address IS the traversal string. Then the identity check
// agrees, the project agrees, the revision reads, and nothing between the
// join and the store write objects: without the shape check the file is
// read, adopted, and keyed `../../secret` in a shared sidecar no promotion
// will ever look up and nothing removes.
func TestReconcile_ShapeCheckIsTheOnlyThingRefusingATraversalThatNamesItself(t *testing.T) {
	const project = "labdrian-sdd-overlay"
	const traversal = "../../secret"
	vaultRoot := t.TempDir()
	fixedNow(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))

	page, err := EmitPage(engram.Observation{ID: 610, Type: "decision", Title: "Outside", Content: "Body.", Project: project, RevisionCount: 2}, "c-000610", nil)
	if err != nil {
		t.Fatalf("EmitPage: %v", err)
	}
	// The file claims the traversal string as its own address, so the
	// identity check cannot be what refuses it.
	frontmatter := strings.Replace(page.Frontmatter, "address: c-000610", "address: "+traversal, 1)
	if frontmatter == page.Frontmatter {
		t.Fatalf("fixture did not rewrite the address line; frontmatter was:\n%s", page.Frontmatter)
	}
	// filepath.Join(vaultRoot, "wiki/memory", "../../secret.md") lands here.
	outside := filepath.Join(vaultRoot, "secret.md")
	if err := os.WriteFile(outside, []byte(frontmatter+page.Body), 0o644); err != nil {
		t.Fatalf("write the outside file: %v", err)
	}

	if _, err := Reconcile(vaultRoot, project, traversal); !errors.Is(err, ErrInvalidAddress) {
		t.Fatalf("Reconcile(%q) error = %v, want ErrInvalidAddress -- only the address-shape check stands between a self-naming traversal and adoption of a file outside %s", traversal, err, pagePathPrefix)
	}

	store, err := LoadPrecedenceStore(vaultRoot)
	if err != nil {
		t.Fatalf("LoadPrecedenceStore: %v", err)
	}
	if len(store) != 0 {
		t.Fatalf("a refused traversal still wrote %d sidecar entr(ies): %+v", len(store), store)
	}
}

// TestReconcile_FollowsASymlinkedPageBecauseTheVaultMayBeOne pins what the
// address-shape check is NOT.
//
// The check's comment used to claim the validated address means a resolved
// path "can only land directly inside wiki/memory". That was untrue:
// nothing here resolves symlinks or prefix-checks the result, so a symlink
// at a promoted page's path is followed and its target adopted. The claim
// is now narrowed to the address STRING, and this test holds the behaviour
// the narrowing describes, so that a later reader who mistakes the
// narrowing for an oversight and "fixes" it with filepath.EvalSymlinks plus
// a prefix check finds out here: a vault reached through a symlink is an
// ordinary setup, the ordinary promotion path follows the same link, and
// containment would refuse those vaults for no gain. Identity is what keeps
// the adopted key honest, and it still applies to whatever the link
// resolves to (the traversal test above).
func TestReconcile_FollowsASymlinkedPageBecauseTheVaultMayBeOne(t *testing.T) {
	const address = "c-000612"
	const project = "labdrian-sdd-overlay"
	vaultRoot := t.TempDir()
	fixedNow(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))

	page, err := EmitPage(engram.Observation{ID: 612, Type: "decision", Title: "Elsewhere", Content: "Body.", Project: project, RevisionCount: 4}, address, nil)
	if err != nil {
		t.Fatalf("EmitPage: %v", err)
	}
	elsewhere := filepath.Join(t.TempDir(), address+".md")
	if err := os.WriteFile(elsewhere, []byte(page.Frontmatter+page.Body), 0o644); err != nil {
		t.Fatalf("write the real page: %v", err)
	}
	link := filepath.Join(vaultRoot, pagePathPrefix, address+".md")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(link), err)
	}
	if err := os.Symlink(elsewhere, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	outcome, err := Reconcile(vaultRoot, project, address)
	if err != nil {
		t.Fatalf("Reconcile on a symlinked page: %v -- a vault reached through a symlink is an ordinary setup, and refusing it would be a new wedge", err)
	}
	if !outcome.Adopted || outcome.PromotedRevision != 4 {
		t.Fatalf("Reconcile outcome = %+v, want the symlinked page adopted at revision 4", outcome)
	}
}

// TestReconcile_MalformedAddressesCarryTheInvalidAddressSentinel pins the
// sentinel itself, not merely "an error": the command surface maps
// ErrInvalidAddress onto the usage exit code, and an error that merely
// happens to be non-nil lands on the internal one instead.
func TestReconcile_MalformedAddressesCarryTheInvalidAddressSentinel(t *testing.T) {
	const project = "labdrian-sdd-overlay"
	vaultRoot := t.TempDir()

	for _, address := range []string{"../../secret", "../secret", "nested/c-000607", "c-000607.md", "", "c-00060", "C-000607"} {
		if _, err := Reconcile(vaultRoot, project, address); !errors.Is(err, ErrInvalidAddress) {
			t.Fatalf("Reconcile(%q) error = %v, want ErrInvalidAddress", address, err)
		}
	}
}

// TestReconcile_AnUnreadableRevisionCarriesTheUnusablePageSentinel pins the
// refusal for a page whose own engram_revision cannot be read.
//
// It was a bare fmt.Errorf, so the command surface fell through to its
// default branch and answered exit 1 (internal) -- for a vault-data
// condition doctor NAMES by address. The advertised repair answered "a bug
// in longterm-mem" to a broken page, and the state never advanced.
func TestReconcile_AnUnreadableRevisionCarriesTheUnusablePageSentinel(t *testing.T) {
	const project = "labdrian-sdd-overlay"
	fixedNow(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))

	for name, mutate := range map[string]func(string) string{
		"the engram_revision line is stripped": func(fm string) string {
			return strings.Replace(fm, "engram_revision: 2\n", "", 1)
		},
		"the revision is not a number": func(fm string) string {
			return strings.Replace(fm, "engram_revision: 2", "engram_revision: not-a-number", 1)
		},
		"the revision is negative": func(fm string) string {
			return strings.Replace(fm, "engram_revision: 2", "engram_revision: -1", 1)
		},
	} {
		t.Run(name, func(t *testing.T) {
			const address = "c-000611"
			vaultRoot := t.TempDir()
			page, err := EmitPage(engram.Observation{ID: 611, Type: "decision", Title: "Unreadable", Content: "Body.", Project: project, RevisionCount: 2}, address, nil)
			if err != nil {
				t.Fatalf("EmitPage: %v", err)
			}
			mutated := mutate(page.Frontmatter)
			if mutated == page.Frontmatter {
				t.Fatalf("fixture did not change the revision line; frontmatter was:\n%s", page.Frontmatter)
			}
			page.Frontmatter = mutated
			writePromotedPageAt(t, vaultRoot, address, page)

			if _, err := Reconcile(vaultRoot, project, address); !errors.Is(err, ErrUnusablePage) {
				t.Fatalf("Reconcile error = %v, want ErrUnusablePage -- an unreadable page is a vault-data condition, not longterm-mem's own internal fault", err)
			}
		})
	}
}

// TestReconcile_APageWithNoFrontmatterBlockCarriesTheUnusablePageSentinel
// pins the SECOND arm that carries ErrUnusablePage.
//
// Two refusals share that sentinel: an unreadable engram_revision, and a
// file with no parseable frontmatter block at all. Only the first was
// pinned. Unwrapping this one back to a bare fmt.Errorf left BOTH
// ./internal/promote and ./cmd/longterm-mem green, while the refusal fell
// through to the command's default branch and answered exit 1 (internal)
// for a vault-data condition -- the exact miscategorisation the sentinel
// was introduced to remove, surviving in the arm nobody tested.
//
// A page whose frontmatter cannot be parsed is a broken page, not a bug in
// longterm-mem, and the operator is entitled to be told which one it is.
func TestReconcile_APageWithNoFrontmatterBlockCarriesTheUnusablePageSentinel(t *testing.T) {
	const (
		project = "labdrian-sdd-overlay"
		address = "c-000613"
	)
	fixedNow(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))

	vaultRoot := t.TempDir()
	full := filepath.Join(vaultRoot, pagePathPrefix, address+".md")
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
	}
	// No delimiters at all. The file exists at the promoted page's own
	// path, so it is past the not-found branch, and frontmatterBlock finds
	// nothing to parse -- which is the only way to reach this arm.
	if err := os.WriteFile(full, []byte("A note somebody left here, with no frontmatter at all.\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}

	if _, err := Reconcile(vaultRoot, project, address); !errors.Is(err, ErrUnusablePage) {
		t.Fatalf("Reconcile error = %v, want ErrUnusablePage -- a page whose frontmatter cannot be parsed is a vault-data condition, not longterm-mem's own internal fault", err)
	}
}
