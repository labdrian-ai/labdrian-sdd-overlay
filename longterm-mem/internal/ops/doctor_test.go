package ops

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/ops/testdata"
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
		testdata.WritePromotedPage(t, vaultRoot, address, title)
		testdata.WriteAddressMap(t, vaultRoot, map[string]string{"wiki/memory/" + address + ".md": address})
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
		if len(report.Checks) != 4 {
			t.Fatalf("Checks = %+v, want exactly 4 (all checks must run and report)", report.Checks)
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
