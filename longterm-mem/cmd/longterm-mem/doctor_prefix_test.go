package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/ops"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/ops/testdata"
)

// goldenCheckDetail is the detail the named check carries in the doctor half of an ops golden file.
func goldenCheckDetail(t *testing.T, goldenPath, check string) string {
	t.Helper()
	raw, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read the ops golden file: %v", err)
	}
	doctor, _, found := strings.Cut(strings.TrimPrefix(string(raw), "--- doctor ---\n"), "\n--- status ---\n")
	if !found {
		t.Fatalf("%s has no status half: the golden file's layout changed", goldenPath)
	}
	var report ops.DoctorReport
	if err := json.Unmarshal([]byte(doctor), &report); err != nil {
		t.Fatalf("decode the doctor half of %s: %v", goldenPath, err)
	}
	for _, c := range report.Checks {
		if c.Name == check {
			return c.Detail
		}
	}
	t.Fatalf("%s has no %s check", goldenPath, check)
	return ""
}

// The doctor command names a precedence sidecar it cannot parse the way the ops golden files say it does,
// prefix included. The ops golden is recorded with an adapter that is given the prefix by hand, because the
// ops package cannot import the command that wires it; this test is what holds the two together: the
// command's own wiring, run over the same damaged vault, must print the golden's text, so that neither a
// change to the prefix the command asks for nor an edit of the golden can drift from the other unseen.
func TestCmdDoctor_ReportsAnUnparseableSidecarAsTheOpsGoldenDoes(t *testing.T) {
	const (
		address = "c-000042"
		title   = "Widget Decision"
		golden  = "08-a-sidecar-that-is-not-json"
		check   = "precedence-sidecar-consistency"
	)
	vaultRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve the temporary directory: %v", err)
	}
	testdata.WritePromotedPage(t, vaultRoot, address, title)
	testdata.WriteAddressMap(t, vaultRoot, map[string]string{"wiki/memory/" + address + ".md": address})
	testdata.RegisterPage(t, vaultRoot, address, title)
	sidecar := filepath.Join(vaultRoot, ".raw", ".longterm-mem-manifest.json")
	if err := os.MkdirAll(filepath.Dir(sidecar), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sidecar, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LONGTERM_MEM_VAULT", vaultRoot)

	stdout := captureStdout(t, func() {
		run([]string{"doctor", "--project", "labdrian-sdd-overlay", "--json"})
	})
	var report ops.DoctorReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("decode the doctor's output: %v\n%s", err, stdout)
	}
	var got string
	for _, c := range report.Checks {
		if c.Name == check {
			got = c.Detail
		}
	}

	want := strings.ReplaceAll(goldenCheckDetail(t, filepath.Join("..", "..", "internal", "ops", "testdata", "golden", golden+".golden"), check), "<vault>", vaultRoot)
	if got != want {
		t.Errorf("the doctor command reports the %s check as\n  %q\nwhere the ops golden file %s has\n  %q", check, got, golden, want)
	}
	if !strings.Contains(got, "promote: parse ") {
		t.Errorf("detail %q does not carry the promote prefix the golden pins", got)
	}
}
