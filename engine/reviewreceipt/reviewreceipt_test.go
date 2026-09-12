package reviewreceipt_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewreceipt"
)

// gitFixtureRepo initializes a real git repository at t.TempDir() so
// Capture can resolve --git-dir/--git-common-dir with the real `git`
// binary. Skipped under -short like the other git-in-t.TempDir suites in
// this repo (engine/pipkg/pipkg_provenance_test.go).
func gitFixtureRepo(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping git-in-TempDir fixture under -short")
	}
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("fixture\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-q", "-m", "initial")
	return dir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// writeReceipt writes a review-receipt.json under repo's git-dir transaction
// store for the given lineage, with the given schema/terminal_state.
func writeReceipt(t *testing.T, repo, lineage, schema, terminalState string) {
	t.Helper()
	lineageDir := filepath.Join(repo, ".git", "gentle-ai", "review-transactions", "v2", lineage)
	if err := os.MkdirAll(lineageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	payload := map[string]interface{}{
		"schema":               schema,
		"lineage_id":           lineage,
		"final_candidate_tree": "deadbeef",
		"selected_lenses":      []string{"review-risk"},
		"terminal_state":       terminalState,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lineageDir, "review-receipt.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
}

// seedActiveChange creates openspec/changes/<change>/tasks.md so
// DetectActiveChange (and Capture's target dir) resolve it as active.
func seedActiveChange(t *testing.T, repo, change string) {
	t.Helper()
	dir := filepath.Join(repo, "openspec", "changes", change)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tasks.md"), []byte("# tasks\n"), 0644); err != nil {
		t.Fatal(err)
	}
}

// TestCapture_SchemaAndTerminalState asserts Capture persists only receipts
// whose schema is exactly gentle-ai.review-receipt/v2 AND whose
// terminal_state is exactly "approved" -- any other schema or a non-terminal
// (or non-approved terminal) state is silently skipped, not an error.
func TestCapture_SchemaAndTerminalState(t *testing.T) {
	repo := gitFixtureRepo(t)
	seedActiveChange(t, repo, "my-change")

	writeReceipt(t, repo, "review-approved1", "gentle-ai.review-receipt/v2", "approved")
	writeReceipt(t, repo, "review-wrongschema", "gentle-ai.review-receipt/v1", "approved")
	writeReceipt(t, repo, "review-notapproved", "gentle-ai.review-receipt/v2", "declined")

	captured, err := reviewreceipt.Capture(repo, "my-change")
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if len(captured) != 1 {
		t.Fatalf("expected exactly 1 captured receipt, got %d: %#v", len(captured), captured)
	}
	if captured[0].LineageID != "review-approved1" {
		t.Errorf("expected lineage review-approved1 captured, got %q", captured[0].LineageID)
	}

	destDir := filepath.Join(repo, "openspec", "changes", "my-change", "review-receipts")
	if _, err := os.Stat(filepath.Join(destDir, "review-approved1.json")); err != nil {
		t.Errorf("expected approved receipt persisted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destDir, "review-wrongschema.json")); !os.IsNotExist(err) {
		t.Errorf("wrong-schema receipt should NOT be persisted, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(destDir, "review-notapproved.json")); !os.IsNotExist(err) {
		t.Errorf("non-approved receipt should NOT be persisted, stat err=%v", err)
	}
}

// TestCapture_AtomicWrite asserts Capture writes a byte-identical copy of
// the source receipt, that re-running Capture on the same source is a
// no-op (idempotent), and that a differing existing destination file is an
// error rather than a silent overwrite.
func TestCapture_AtomicWrite(t *testing.T) {
	repo := gitFixtureRepo(t)
	seedActiveChange(t, repo, "my-change")
	writeReceipt(t, repo, "review-atomic1", "gentle-ai.review-receipt/v2", "approved")

	srcPath := filepath.Join(repo, ".git", "gentle-ai", "review-transactions", "v2", "review-atomic1", "review-receipt.json")
	srcBytes, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := reviewreceipt.Capture(repo, "my-change"); err != nil {
		t.Fatalf("Capture (first run): %v", err)
	}

	destPath := filepath.Join(repo, "openspec", "changes", "my-change", "review-receipts", "review-atomic1.json")
	destBytes, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read captured receipt: %v", err)
	}
	if string(destBytes) != string(srcBytes) {
		t.Errorf("captured receipt is not byte-identical:\nsrc:  %s\ndest: %s", srcBytes, destBytes)
	}

	// Idempotent: capturing again with identical source content is a no-op,
	// not an error.
	if _, err := reviewreceipt.Capture(repo, "my-change"); err != nil {
		t.Fatalf("Capture (second run, idempotent): %v", err)
	}

	// A differing existing destination file is an error, never silently
	// overwritten.
	if err := os.WriteFile(destPath, []byte(`{"different":true}`), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := reviewreceipt.Capture(repo, "my-change"); err == nil {
		t.Error("expected Capture to error on a differing existing destination file, got nil")
	}
}
