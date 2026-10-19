package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runReceiptVerb runs 'review-receipt <args>' as the program, with stdin, and returns its exit
// code and both streams.
func runReceiptVerb(t *testing.T, w *receiptWorld, stdin string, args ...string) (int, string, string) {
	t.Helper()
	cmd := exec.Command(w.bin, append([]string{"review-receipt"}, args...)...)
	cmd.Dir = w.reader
	cmd.Env = goldenEnvironment()
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			t.Fatalf("run review-receipt %v: %v", args, err)
		}
		return exit.ExitCode(), stdout.String(), stderr.String()
	}
	return 0, stdout.String(), stderr.String()
}

// The owner's requirement (C23 M5d): "`review-receipt capture --feature consolidation-phase9` ->
// odd/review-receipts/consolidation-phase9/review-939c22285467c2b0.review-state.json;
// acknowledge-approved ... -> allowed (persisted)".
func TestCaptureIntoAFeatureLetsTheAcknowledgementThroughWithSeveralActiveChanges(t *testing.T) {
	w := newReceiptWorld(t)
	root := w.repo("<ROOT>", false)
	w.change(root, "change-a")
	w.change(root, "change-b")
	state := lifecycleState("review-939c22285467c2b0", approved)
	w.putState(filepath.Join(root, ".git"), "review-939c22285467c2b0", state)
	hookInput := acknowledgeHookJSON(acknowledgeCommand)

	if code, _, stderr := runReceiptVerb(t, w, hookInput, "hook", "--cwd", root); code != 2 || !strings.Contains(stderr, "review-receipt capture --feature <name>") {
		t.Fatalf("hook before the capture = %d, %q, want a denial naming --feature", code, stderr)
	}

	code, stdout, stderr := runReceiptVerb(t, w, "", "capture", "--cwd", root, "--feature", "consolidation-phase9")
	if code != 0 {
		t.Fatalf("capture --feature = %d, %q, %q", code, stdout, stderr)
	}
	persisted := filepath.Join(root, "odd", "review-receipts", "consolidation-phase9", "review-939c22285467c2b0.review-state.json")
	if got, err := os.ReadFile(persisted); err != nil || string(got) != state {
		t.Fatalf("persisted %q, %v, want the exact bytes of the state", got, err)
	}
	if code, _, stderr := runReceiptVerb(t, w, hookInput, "hook", "--cwd", root); code != 0 || stderr != "" {
		t.Errorf("hook after the capture = %d, %q, want an allow", code, stderr)
	}

	// Idempotent: the same bytes are left alone; other bytes are never replaced.
	if code, _, stderr := runReceiptVerb(t, w, "", "capture", "--cwd", root, "--feature", "consolidation-phase9"); code != 0 {
		t.Errorf("second capture --feature = %d, %q, want success", code, stderr)
	}
	if err := os.WriteFile(persisted, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runReceiptVerb(t, w, "", "capture", "--cwd", root, "--feature", "consolidation-phase9"); code != 1 || !strings.Contains(stderr, "already exists with different content") {
		t.Errorf("capture over different bytes = %d, %q, want a refusal", code, stderr)
	}
	if code, _, _ := runReceiptVerb(t, w, hookInput, "hook", "--cwd", root); code != 2 {
		t.Errorf("hook with different bytes in the feature = %d, want a denial", code)
	}
}

// --change and --feature are two destinations; both at once is refused and writes nothing, and
// a feature name that is not one path component is refused by name.
func TestCaptureRefusesTwoDestinationsAndAnUnsafeFeatureName(t *testing.T) {
	w := newReceiptWorld(t)
	root := w.repo("<ROOT>", false)
	w.change(root, "the-change")
	w.putReceipt(filepath.Join(root, ".git"), "review-one", legacyReceipt("review-one", receiptSchemaV2, approved))

	code, _, stderr := runReceiptVerb(t, w, "", "capture", "--cwd", root, "--change", "the-change", "--feature", "f")
	if code != 1 || stderr != "error: review-receipt capture: --change and --feature cannot be given together\n" {
		t.Errorf("both = %d, %q, want the refusal", code, stderr)
	}
	code, _, stderr = runReceiptVerb(t, w, "", "capture", "--cwd", root, "--feature", "../escaped")
	if code != 1 || !strings.Contains(stderr, `feature name "../escaped" is not a safe path component`) {
		t.Errorf("unsafe = %d, %q, want the refusal naming the feature name", code, stderr)
	}
	for _, dir := range []string{filepath.Join(root, "odd"), filepath.Join(root, "escaped"), filepath.Join(root, "openspec", "changes", "the-change", "review-receipts")} {
		if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("a refused capture created %s", dir)
		}
	}
}
