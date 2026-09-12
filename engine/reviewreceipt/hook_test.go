package reviewreceipt_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewreceipt"
)

const ackHookInput = `{"tool_name":"Bash","tool_input":{"command":"gentle-ai review acknowledge-approved --cwd /repo --lineage review-x"}}`

// TestHook_PassThrough_NoOpenspecChanges asserts the hook allows (exit 0, no
// message) when the repo has no openspec/changes/ directory at all -- there
// is no change to attach a captured receipt to.
func TestHook_PassThrough_NoOpenspecChanges(t *testing.T) {
	repo := gitFixtureRepo(t)
	writeReceipt(t, repo, "review-orphan", "gentle-ai.review-receipt/v2", "approved")

	exitCode, message := reviewreceipt.RunHook([]byte(ackHookInput), repo)
	if exitCode != 0 {
		t.Errorf("expected exit 0, got %d (message: %q)", exitCode, message)
	}
	if message != "" {
		t.Errorf("expected empty message, got %q", message)
	}

	// Nothing should have been captured -- there is no active change.
	if _, err := os.Stat(filepath.Join(repo, "openspec")); !os.IsNotExist(err) {
		t.Errorf("openspec/ should not have been created by the pass-through hook, stat err=%v", err)
	}
}

// TestHook_Deny_MultipleActiveChanges asserts the hook denies (exit 2) with
// a message naming `review-receipt capture --change <name>` when more than
// one active change exists and the surviving approved receipt has NOT been
// explicitly captured anywhere yet -- it must not guess which change owns
// the receipt.
func TestHook_Deny_MultipleActiveChanges(t *testing.T) {
	repo := gitFixtureRepo(t)
	seedActiveChange(t, repo, "change-a")
	seedActiveChange(t, repo, "change-b")
	writeReceipt(t, repo, "review-ambiguous", "gentle-ai.review-receipt/v2", "approved")

	exitCode, message := reviewreceipt.RunHook([]byte(ackHookInput), repo)
	if exitCode != 2 {
		t.Fatalf("expected exit 2, got %d", exitCode)
	}
	if !strings.Contains(message, "review-receipt capture --change <name>") {
		t.Errorf("message %q does not name the remediation command", message)
	}
}

// TestHook_MultiChange_PassesAfterExplicitCapture: deny, then Capture into
// one active change, then a retried acknowledgement passes.
func TestHook_MultiChange_PassesAfterExplicitCapture(t *testing.T) {
	repo := gitFixtureRepo(t)
	seedActiveChange(t, repo, "change-a")
	seedActiveChange(t, repo, "change-b")
	writeReceipt(t, repo, "review-fixable", "gentle-ai.review-receipt/v2", "approved")

	exitCode, _ := reviewreceipt.RunHook([]byte(ackHookInput), repo)
	if exitCode != 2 {
		t.Fatalf("expected first run to deny (exit 2), got %d", exitCode)
	}

	if _, err := reviewreceipt.Capture(repo, "change-a"); err != nil {
		t.Fatalf("Capture: %v", err)
	}

	exitCode, message := reviewreceipt.RunHook([]byte(ackHookInput), repo)
	if exitCode != 0 {
		t.Fatalf("expected second run to pass (exit 0) after explicit capture, got %d (message: %q)", exitCode, message)
	}
}

// TestHook_MultiChange_DeniesWhenAnyUncaptured: capturing one of two
// surviving lineages must not clear the deny for the other, uncaptured one.
func TestHook_MultiChange_DeniesWhenAnyUncaptured(t *testing.T) {
	repo := gitFixtureRepo(t)
	seedActiveChange(t, repo, "change-a")
	seedActiveChange(t, repo, "change-b")
	writeReceipt(t, repo, "review-captured", "gentle-ai.review-receipt/v2", "approved")
	writeReceipt(t, repo, "review-uncaptured", "gentle-ai.review-receipt/v2", "approved")

	if _, err := reviewreceipt.Capture(repo, "change-a"); err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if err := os.Remove(filepath.Join(repo, "openspec", "changes", "change-a", "review-receipts", "review-uncaptured.json")); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	exitCode, message := reviewreceipt.RunHook([]byte(ackHookInput), repo)
	if exitCode != 2 {
		t.Fatalf("expected exit 2 (one lineage still uncaptured), got %d (message: %q)", exitCode, message)
	}
	if !strings.Contains(message, "review-receipt capture --change <name>") {
		t.Errorf("message %q does not name the remediation command", message)
	}
}

// TestHook_Capture_SingleActiveChange asserts the hook allows (exit 0) and
// captures every surviving approved receipt when exactly one active change
// exists.
func TestHook_Capture_SingleActiveChange(t *testing.T) {
	repo := gitFixtureRepo(t)
	seedActiveChange(t, repo, "only-change")
	writeReceipt(t, repo, "review-solo", "gentle-ai.review-receipt/v2", "approved")

	exitCode, message := reviewreceipt.RunHook([]byte(ackHookInput), repo)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d (message: %q)", exitCode, message)
	}

	dest := filepath.Join(repo, "openspec", "changes", "only-change", "review-receipts", "review-solo.json")
	if _, err := os.Stat(dest); err != nil {
		t.Errorf("expected receipt captured at %s: %v", dest, err)
	}
}

// TestHook_PassThrough_CommandDoesNotMatch asserts the hook never inspects
// active changes or captures anything for a command that does not contain
// the acknowledge-approved marker.
func TestHook_PassThrough_CommandDoesNotMatch(t *testing.T) {
	repo := gitFixtureRepo(t)
	seedActiveChange(t, repo, "only-change")
	writeReceipt(t, repo, "review-untouched", "gentle-ai.review-receipt/v2", "approved")

	input := `{"tool_name":"Bash","tool_input":{"command":"git status"}}`
	exitCode, message := reviewreceipt.RunHook([]byte(input), repo)
	if exitCode != 0 || message != "" {
		t.Fatalf("expected (0, \"\"), got (%d, %q)", exitCode, message)
	}
	if _, err := os.Stat(filepath.Join(repo, "openspec", "changes", "only-change", "review-receipts")); !os.IsNotExist(err) {
		t.Errorf("non-matching command must not trigger capture, stat err=%v", err)
	}
}
