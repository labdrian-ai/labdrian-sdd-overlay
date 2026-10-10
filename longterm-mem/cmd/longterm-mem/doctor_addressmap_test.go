package main

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/ops"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/ops/testdata"
)

// doctorCheck runs the doctor command over the vault and returns the named check of its report.
func doctorCheck(t *testing.T, vaultRoot, name string) ops.Check {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LONGTERM_MEM_VAULT", vaultRoot)
	stdout := captureStdout(t, func() {
		run([]string{"doctor", "--project", "labdrian-sdd-overlay", "--json"})
	})
	var report ops.DoctorReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("decode the doctor's output: %v\n%s", err, stdout)
	}
	for _, check := range report.Checks {
		if check.Name == name {
			return check
		}
	}
	t.Fatalf("the doctor's report has no %s check:\n%s", name, stdout)
	return ops.Check{}
}

// The doctor command reads the vault's address map through the adapter it wires: a healthy vault passes the
// address-map check, and a page its map names under another path is a finding, in the words the doctor has
// always used. A command that forgot to wire the reader would fail the check of every vault with a page in it.
func TestCmdDoctor_ChecksThePagesAgainstTheVaultsAddressMap(t *testing.T) {
	const (
		address = "c-000042"
		title   = "Widget Decision"
	)
	vaultRoot := realTempDir(t)
	page := testdata.WritePromotedPage(t, vaultRoot, address, title)
	testdata.WriteAddressMap(t, vaultRoot, map[string]string{page.Path: address})
	testdata.WritePrecedenceEntry(t, vaultRoot, page)
	testdata.RegisterPage(t, vaultRoot, address, title)

	if got := doctorCheck(t, vaultRoot, ops.CheckAddressMapIntegrity); got.Status != ops.CheckPassed {
		t.Fatalf("address-map-integrity = %+v over a healthy vault, want PASS", got)
	}

	testdata.WriteAddressMap(t, vaultRoot, map[string]string{filepath.ToSlash(filepath.Join("wiki", "memory", "elsewhere.md")): address})
	got := doctorCheck(t, vaultRoot, ops.CheckAddressMapIntegrity)
	if want := "address c-000042 maps to wiki/memory/elsewhere.md, not wiki/memory/c-000042.md"; got.Status != ops.CheckFailed || got.Detail != want {
		t.Errorf("address-map-integrity = %+v, want FAIL with %q", got, want)
	}
}
