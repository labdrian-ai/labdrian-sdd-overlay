package ops

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/ops/testdata"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/promote"
)

// checkStatus finds check name's Status in checks, failing the test if
// name is not present -- Doctor must always report all four named checks
// (R-011), so a missing entry is itself a test failure, not a skipped
// assertion.
func checkStatus(t *testing.T, checks []Check, name string) Check {
	t.Helper()
	for _, c := range checks {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no check named %q in %+v", name, checks)
	return Check{}
}

// recordPrecedenceRevision backfills revision onto address's existing
// precedence entry, turning the fixture's legacy (hashes-only) entry into
// the fully recorded shape every promotion writes today.
func recordPrecedenceRevision(t *testing.T, vaultRoot, address string, revision int) {
	t.Helper()
	store, err := promote.LoadPrecedenceStore(vaultRoot)
	if err != nil {
		t.Fatalf("LoadPrecedenceStore: %v", err)
	}
	entry, ok := store.Get(address)
	if !ok {
		t.Fatalf("precedence store has no entry for %s to record a revision on", address)
	}
	entry.PromotedRevision = revision
	store.Set(address, entry)
	if err := store.Save(vaultRoot); err != nil {
		t.Fatalf("PrecedenceStore.Save: %v", err)
	}
}

// editPromotedPage appends a human's own line to a promoted page, so its
// bytes no longer match whatever the precedence sidecar recorded.
func editPromotedPage(t *testing.T, vaultRoot, address string) {
	t.Helper()
	pagePath := filepath.Join(vaultRoot, "wiki", "memory", address+".md")
	data, err := os.ReadFile(pagePath)
	if err != nil {
		t.Fatalf("read %s: %v", pagePath, err)
	}
	if err := os.WriteFile(pagePath, append(data, []byte("\nEdited by a human.\n")...), 0o644); err != nil {
		t.Fatalf("write local edit: %v", err)
	}
}

// TestDoctor: R-011's four named-check scenarios, table-driven (8a.4).
// Each case builds a fully consistent baseline vault (a resolvable root,
// one promoted page correctly address-mapped and registered in both the
// catalog and the log, and a present runtime prerequisite) then breaks
// exactly one piece, so the test also proves the other three checks keep
// running and reporting instead of being aborted by the broken one (the
// per-item-failure-must-not-abort-the-run lesson from slice 7's review).
func TestDoctor(t *testing.T) {
	const address = "c-000042"
	const title = "Widget Decision"

	newHealthyDeps := func(t *testing.T) (DoctorDeps, string) {
		t.Helper()
		vaultRoot := t.TempDir()
		page := testdata.WritePromotedPage(t, vaultRoot, address, title)
		testdata.WriteAddressMap(t, vaultRoot, map[string]string{"wiki/memory/" + address + ".md": address})
		testdata.WritePrecedenceEntry(t, vaultRoot, page)
		testdata.RegisterPage(t, vaultRoot, address, title)
		return DoctorDeps{
			VaultRoot:           vaultRoot,
			PrerequisitePresent: func(string) bool { return true },
		}, vaultRoot
	}

	t.Run("Unresolvable vault config is named", func(t *testing.T) {
		deps, vaultRoot := newHealthyDeps(t)
		badRoot := filepath.Join(vaultRoot, "does-not-exist")
		deps.VaultRoot = badRoot

		report, err := Doctor(context.Background(), deps, "labdrian-sdd-overlay")
		if err != nil {
			t.Fatalf("Doctor: %v", err)
		}
		if len(report.Checks) != 5 {
			t.Fatalf("Checks = %+v, want exactly 5 (all checks must run and report)", report.Checks)
		}

		got := checkStatus(t, report.Checks, CheckVaultConfigResolvable)
		if got.Status != CheckFailed {
			t.Fatalf("vault-config-resolvable = %+v, want FAILed", got)
		}
		if !strings.Contains(got.Detail, badRoot) {
			t.Fatalf("vault-config-resolvable detail = %q, want it to name %q", got.Detail, badRoot)
		}

		// The other three checks must still have run and reported, not
		// been skipped because the vault root is unresolvable.
		if got := checkStatus(t, report.Checks, CheckRuntimePrerequisites); got.Status != CheckPassed {
			t.Errorf("runtime-prerequisites = %+v, want PASSed (one broken check must not abort the others)", got)
		}
	})

	t.Run("Corrupted address-map entry is named", func(t *testing.T) {
		deps, vaultRoot := newHealthyDeps(t)
		// Overwrite the manifest so the promoted page's address has no
		// entry at all.
		testdata.WriteAddressMap(t, vaultRoot, map[string]string{})

		report, err := Doctor(context.Background(), deps, "labdrian-sdd-overlay")
		if err != nil {
			t.Fatalf("Doctor: %v", err)
		}

		got := checkStatus(t, report.Checks, CheckAddressMapIntegrity)
		if got.Status != CheckFailed {
			t.Fatalf("address-map-integrity = %+v, want FAILed", got)
		}
		if !strings.Contains(got.Detail, address) {
			t.Fatalf("address-map-integrity detail = %q, want it to name %q", got.Detail, address)
		}

		if got := checkStatus(t, report.Checks, CheckWikiRegistrationConsistency); got.Status != CheckPassed {
			t.Errorf("wiki-registration-consistency = %+v, want PASSed (one broken check must not abort the others)", got)
		}
		if got := checkStatus(t, report.Checks, CheckVaultConfigResolvable); got.Status != CheckPassed {
			t.Errorf("vault-config-resolvable = %+v, want PASSed", got)
		}
	})

	t.Run("Unregistered promoted page is named", func(t *testing.T) {
		deps, vaultRoot := newHealthyDeps(t)
		// Wipe the catalog and log so the promoted page is registered
		// nowhere.
		if err := os.Remove(filepath.Join(vaultRoot, "wiki", "index.md")); err != nil {
			t.Fatalf("remove index.md: %v", err)
		}
		if err := os.Remove(filepath.Join(vaultRoot, "wiki", "log.md")); err != nil {
			t.Fatalf("remove log.md: %v", err)
		}

		report, err := Doctor(context.Background(), deps, "labdrian-sdd-overlay")
		if err != nil {
			t.Fatalf("Doctor: %v", err)
		}

		got := checkStatus(t, report.Checks, CheckWikiRegistrationConsistency)
		if got.Status != CheckFailed {
			t.Fatalf("wiki-registration-consistency = %+v, want FAILed", got)
		}
		if !strings.Contains(got.Detail, address) {
			t.Fatalf("wiki-registration-consistency detail = %q, want it to name %q", got.Detail, address)
		}

		if got := checkStatus(t, report.Checks, CheckAddressMapIntegrity); got.Status != CheckPassed {
			t.Errorf("address-map-integrity = %+v, want PASSed (one broken check must not abort the others)", got)
		}
	})

	t.Run("Promoted page with no precedence-sidecar entry is named", func(t *testing.T) {
		deps, vaultRoot := newHealthyDeps(t)
		// The create path's crash window, as it survives in the field: the
		// page, its address-map entry and its registration all landed, and
		// the precedence sidecar never did. Nothing else in doctor reads
		// that file, so before this check the missing half of the failure
		// was invisible -- doctor reported a healthy vault whenever the
		// catalog and log happened to have been repaired by hand.
		if err := os.Remove(filepath.Join(vaultRoot, ".raw", ".longterm-mem-manifest.json")); err != nil {
			t.Fatalf("remove precedence sidecar: %v", err)
		}

		report, err := Doctor(context.Background(), deps, "labdrian-sdd-overlay")
		if err != nil {
			t.Fatalf("Doctor: %v", err)
		}

		got := checkStatus(t, report.Checks, CheckPrecedenceSidecarConsistency)
		if got.Status != CheckFailed {
			t.Fatalf("precedence-sidecar-consistency = %+v, want FAILed", got)
		}
		if !strings.Contains(got.Detail, address) {
			t.Fatalf("precedence-sidecar-consistency detail = %q, want it to name %q", got.Detail, address)
		}

		if got := checkStatus(t, report.Checks, CheckWikiRegistrationConsistency); got.Status != CheckPassed {
			t.Errorf("wiki-registration-consistency = %+v, want PASSed (one broken check must not abort the others)", got)
		}
		if got := checkStatus(t, report.Checks, CheckAddressMapIntegrity); got.Status != CheckPassed {
			t.Errorf("address-map-integrity = %+v, want PASSed (one broken check must not abort the others)", got)
		}
	})

	t.Run("Locally edited page with a recorded entry is not a precedence-sidecar failure", func(t *testing.T) {
		deps, vaultRoot := newHealthyDeps(t)
		// R-030 makes a hand-edited promoted page a supported, reported
		// state, not a broken vault: its sidecar entry exists, records the
		// revision it published, and simply no longer matches the bytes.
		// Promotion refuses that page and says so on every run, which is
		// R-030 holding the human's edit rather than a wedge to repair --
		// the refusal is standing, not silent, and flagging it here would
		// report every page a human has ever touched as a defect.
		recordPrecedenceRevision(t, vaultRoot, address, 1)
		editPromotedPage(t, vaultRoot, address)

		report, err := Doctor(context.Background(), deps, "labdrian-sdd-overlay")
		if err != nil {
			t.Fatalf("Doctor: %v", err)
		}
		if got := checkStatus(t, report.Checks, CheckPrecedenceSidecarConsistency); got.Status != CheckPassed {
			t.Fatalf("precedence-sidecar-consistency = %+v, want PASSed: a local edit on a fully recorded entry is a supported state (R-030), not a vault defect", got)
		}
	})

	t.Run("Locally edited page whose entry records no revision is named", func(t *testing.T) {
		deps, vaultRoot := newHealthyDeps(t)
		// The wedged residue. An entry that records no promoted revision
		// carries no evidence separating our own unrecorded write from a
		// human's edit, so once its page diverges promotion refuses it --
		// and the refusal is a skip, which suppresses the very store write
		// that would have given the entry a revision. Every later run
		// repeats it. A vault permanently refusing its own page must not
		// report entirely healthy, which is the failure mode this check
		// exists to end.
		editPromotedPage(t, vaultRoot, address)

		report, err := Doctor(context.Background(), deps, "labdrian-sdd-overlay")
		if err != nil {
			t.Fatalf("Doctor: %v", err)
		}
		got := checkStatus(t, report.Checks, CheckPrecedenceSidecarConsistency)
		if got.Status != CheckFailed {
			t.Fatalf("precedence-sidecar-consistency = %+v, want FAILed: a page its own sidecar can never adopt again is a vault defect, not a supported state", got)
		}
		if !strings.Contains(got.Detail, address) {
			t.Fatalf("precedence-sidecar-consistency detail = %q, want it to name %q", got.Detail, address)
		}
	})

	t.Run("Locally edited page whose entry records a negative revision is named", func(t *testing.T) {
		deps, vaultRoot := newHealthyDeps(t)
		// The same wedge, spelled the other way round. Promotion's own
		// guard (update.go's revisionsAllowAdoption) refuses on
		// PromotedRevision <= 0, not just == 0, so a hand-edited or
		// corrupted sidecar recording a negative revision is exactly as
		// unadoptable as one recording none. This check has to spell the
		// predicate the same way, or that page is wedged-but-unreported --
		// the one state this check exists to make impossible.
		recordPrecedenceRevision(t, vaultRoot, address, -1)
		editPromotedPage(t, vaultRoot, address)

		report, err := Doctor(context.Background(), deps, "labdrian-sdd-overlay")
		if err != nil {
			t.Fatalf("Doctor: %v", err)
		}
		got := checkStatus(t, report.Checks, CheckPrecedenceSidecarConsistency)
		if got.Status != CheckFailed {
			t.Fatalf("precedence-sidecar-consistency = %+v, want FAILed: promotion refuses a non-positive recorded revision, so this page is wedged and must not report healthy", got)
		}
		if !strings.Contains(got.Detail, address) {
			t.Fatalf("precedence-sidecar-consistency detail = %q, want it to name %q", got.Detail, address)
		}
	})

	t.Run("Missing runtime prerequisite is named", func(t *testing.T) {
		deps, _ := newHealthyDeps(t)
		deps.PrerequisitePresent = func(string) bool { return false }

		report, err := Doctor(context.Background(), deps, "labdrian-sdd-overlay")
		if err != nil {
			t.Fatalf("Doctor: %v", err)
		}

		got := checkStatus(t, report.Checks, CheckRuntimePrerequisites)
		if got.Status != CheckFailed {
			t.Fatalf("runtime-prerequisites = %+v, want FAILed", got)
		}
		if !strings.Contains(got.Detail, "python3") {
			t.Fatalf("runtime-prerequisites detail = %q, want it to name the missing prerequisite", got.Detail)
		}

		if got := checkStatus(t, report.Checks, CheckAddressMapIntegrity); got.Status != CheckPassed {
			t.Errorf("address-map-integrity = %+v, want PASSed (one broken check must not abort the others)", got)
		}
		if got := checkStatus(t, report.Checks, CheckWikiRegistrationConsistency); got.Status != CheckPassed {
			t.Errorf("wiki-registration-consistency = %+v, want PASSed (one broken check must not abort the others)", got)
		}
	})
}

// TestDoctor_UnreadablePageDoesNotHideEveryOtherPage: one promoted page
// that cannot be read must be reported as its own finding, not swallow the
// scan. Returning the read error out of the page loader made both
// page-walking checks report FAIL carrying only that I/O message, throwing
// away every other page's result -- so a vault with one broken symlink
// would hide a genuinely unregistered page behind it, on every run
// (review finding R3-page-read-error-aborts-two-checks). This is the same
// per-item-failure-aborts-the-run shape the package doc already forbids.
func TestDoctor_UnreadablePageDoesNotHideEveryOtherPage(t *testing.T) {
	vaultRoot := t.TempDir()

	// A genuinely broken page the checks MUST still report: promoted, but
	// absent from both the address map and the catalog.
	const unregistered = "c-000777"
	testdata.WritePromotedPage(t, vaultRoot, unregistered, "Unregistered Page")
	testdata.WriteAddressMap(t, vaultRoot, map[string]string{})

	// An unreadable entry: a dangling symlink is listed by ReadDir as a
	// non-directory but fails ReadFile, deterministically and regardless
	// of the uid the test runs as.
	dangling := filepath.Join(vaultRoot, "wiki", "memory", "c-000888.md")
	if err := os.Symlink(filepath.Join(vaultRoot, "wiki", "memory", "nonexistent-target.md"), dangling); err != nil {
		t.Fatalf("create dangling symlink: %v", err)
	}

	deps := DoctorDeps{
		VaultRoot:           vaultRoot,
		PrerequisitePresent: func(name string) bool { return true },
	}
	report, err := Doctor(context.Background(), deps, "labdrian-sdd-overlay")
	if err != nil {
		t.Fatalf("Doctor: %v", err)
	}

	for _, name := range []string{CheckAddressMapIntegrity, CheckWikiRegistrationConsistency} {
		got := checkStatus(t, report.Checks, name)
		if got.Status != CheckFailed {
			t.Errorf("%s = %+v, want FAILed", name, got)
		}
		if !strings.Contains(got.Detail, "c-000888") {
			t.Errorf("%s detail %q does not report the unreadable page", name, got.Detail)
		}
		if !strings.Contains(got.Detail, unregistered) {
			t.Errorf("%s detail %q lost the other page's finding behind the unreadable one; every page must still be checked", name, got.Detail)
		}
	}
}
