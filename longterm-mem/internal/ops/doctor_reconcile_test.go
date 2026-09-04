package ops

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/ops/testdata"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/promote"
)

// TestDoctorAndReconcile_CloseTheLoopOnARevisionZeroPage is the whole point
// of the reconcile command, asserted end to end rather than one call at a
// time.
//
// doctor's precedence-sidecar check names a wedged page as exactly the state
// "nothing in the promotion path can repair", and reconcile is the only
// advertised repair. So the two have to meet: whatever doctor names,
// reconcile must adopt, and doctor must then go quiet about it. A
// revision-0 page is the case that fell through the gap -- an ordinary
// promoted page (eligibility admits a pinned observation with revision_count
// 0), named by doctor, and refused by the one remedy.
func TestDoctorAndReconcile_CloseTheLoopOnARevisionZeroPage(t *testing.T) {
	const address = "c-000610"
	const project = "labdrian-sdd-overlay"
	vaultRoot := t.TempDir()

	obs := engram.Observation{ID: 610, Type: "decision", Title: "Never Revised", Content: "V1 body.", Project: project, RevisionCount: 0, Pinned: true}
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
	testdata.WriteAddressMap(t, vaultRoot, map[string]string{page.Path: address})
	testdata.WritePrecedenceEntry(t, vaultRoot, page)
	testdata.RegisterPage(t, vaultRoot, address, "Never Revised")

	// The divergence the sidecar never caught up with.
	obs.Content = "V1 body, republished."
	diverged, err := promote.EmitPage(obs, address, nil)
	if err != nil {
		t.Fatalf("EmitPage (diverged): %v", err)
	}
	if err := os.WriteFile(full, []byte(diverged.Frontmatter+diverged.Body), 0o644); err != nil {
		t.Fatalf("simulate the diverged page: %v", err)
	}

	deps := DoctorDeps{VaultRoot: vaultRoot, PrerequisitePresent: func(string) bool { return true }}

	report, err := Doctor(context.Background(), deps, project)
	if err != nil {
		t.Fatalf("Doctor (before): %v", err)
	}
	got := checkStatus(t, report.Checks, CheckPrecedenceSidecarConsistency)
	if got.Status != CheckFailed || !strings.Contains(got.Detail, address) {
		t.Fatalf("precedence-sidecar-consistency = %+v, want it FAILed naming %s: the fixture is not wedged", got, address)
	}

	if _, err := promote.Reconcile(vaultRoot, project, address); err != nil {
		t.Fatalf("Reconcile on the page doctor just named: %v -- the only advertised repair declined the state doctor calls unrepairable", err)
	}

	report, err = Doctor(context.Background(), deps, project)
	if err != nil {
		t.Fatalf("Doctor (after): %v", err)
	}
	if got := checkStatus(t, report.Checks, CheckPrecedenceSidecarConsistency); got.Status != CheckPassed {
		t.Fatalf("precedence-sidecar-consistency = %+v after reconcile, want PASSed -- the repair did not leave the vault clean", got)
	}
}
