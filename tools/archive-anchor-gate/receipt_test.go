package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeReceiptFile persists a gentle-ai.review-receipt/v2-shaped JSON file
// under dir/<lineage>.json, exactly as engine/reviewreceipt.Capture would
// have written it.
func writeReceiptFile(t *testing.T, dir, lineage, finalCandidateTree, terminalState string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	payload := map[string]any{
		"schema":               "gentle-ai.review-receipt/v2",
		"lineage_id":           lineage,
		"final_candidate_tree": finalCandidateTree,
		"selected_lenses":      []string{"review-risk"},
		"terminal_state":       terminalState,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal receipt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, lineage+".json"), data, 0o600); err != nil {
		t.Fatalf("write receipt: %v", err)
	}
}

// writeReviewStateFile persists the SECOND persisted shape under
// dir/<lineage>.review-state.json: the real gentle-ai.review-state-record/v2
// wrapper gentle-ai 2.7.0+ leaves behind for an approved-but-unacknowledged
// lineage (it no longer writes a gentle-ai.review-receipt/v2 file at all).
// This mirrors the four real files persisted at
// openspec/changes/pi-package-hardening/review-receipts/ -- a top-level
// `state` OBJECT nesting `lineage_id`, `state`, `current_snapshot` and
// `initial_snapshot`. An earlier revision of this helper wrote a flat
// top-level `state` string instead; that fixture-fidelity gap (WARN-3, and
// the root of CRIT-1) let the gate's own now-deleted flat model pass while
// the real files it needed to read never parsed. See
// TestApprovedTree_FromRealCapturedReceipt for the production-data pin.
func writeReviewStateFile(t *testing.T, dir, lineage, candidateTree, state string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	payload := map[string]any{
		"schema":   "gentle-ai.review-state-record/v2",
		"revision": "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		"state": map[string]any{
			"schema":           "gentle-ai.review-state/v2",
			"lineage_id":       lineage,
			"state":            state,
			"risk_level":       "medium",
			"selected_lenses":  []string{"review-risk"},
			"initial_snapshot": map[string]any{"base_tree": "0000000000000000000000000000000000000000"},
			"current_snapshot": map[string]any{"candidate_tree": candidateTree},
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal review-state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, lineage+".review-state.json"), data, 0o600); err != nil {
		t.Fatalf("write review-state: %v", err)
	}
}

// writeOverrideFile persists a labdrian.review-receipt-override/v1 file.
func writeOverrideFile(t *testing.T, dir, owner, reason, recordedAt string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	payload := map[string]any{
		"schema":      "labdrian.review-receipt-override/v1",
		"owner":       owner,
		"reason":      reason,
		"recorded_at": recordedAt,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal override: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "override.json"), data, 0o600); err != nil {
		t.Fatalf("write override: %v", err)
	}
}

// TestApprovedTree_FromReceipt (task 4.1) asserts that once a change is
// archived on or after ReceiptConventionDate, approved_tree is sourced from
// the persisted review receipt's final_candidate_tree -- via a PREFIX match
// against the landing commit's real tree, exactly like the existing prose
// rule -- rather than from any tree hash written in prose.
func TestApprovedTree_FromReceipt(t *testing.T) {
	root, commit, tree := fixtureRepo(t)
	dirName := "2026-09-12-receipt-verified-change"
	writeArchiveReport(t, root, dirName, "# Archive Report\n\n## Cycle Timestamps\n\n"+
		"| landing_commit | `"+commit+"` |\n\nAnchor outcome: verified against the persisted review receipt.\n")
	receiptsDir := filepath.Join(root, "openspec", "changes", "archive", dirName, "review-receipts")
	// Use only a PREFIX of the real tree, the same abbreviation rule the
	// prose path already honours.
	writeReceiptFile(t, receiptsDir, "review-lineage-1", tree[:7], "approved")

	reports, findings, err := ScanArchive(root, ConventionDate)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("a receipt-verified anchor produced findings: %v", findings)
	}
	if len(reports) != 1 || reports[0].Outcome != AnchorVerified {
		t.Fatalf("reports = %v, want one %q report", reports, AnchorVerified)
	}
	if reports[0].Anchor == nil || reports[0].Anchor.ApprovedTree != tree[:7] {
		t.Fatalf("anchor = %#v, want approved_tree sourced from the receipt (%s)", reports[0].Anchor, tree[:7])
	}
}

// TestApprovedTree_FromReviewStateShape pins the second persisted shape:
// gentle-ai 2.7.0+ no longer writes a gentle-ai.review-receipt/v2 file at
// all, so an approved-but-unacknowledged lineage is captured only as a raw
// review-state.json (persisted here as <lineage>.review-state.json). The
// gate must read `state == "approved"` and
// `current_snapshot.candidate_tree` from THAT shape too.
func TestApprovedTree_FromReviewStateShape(t *testing.T) {
	root, commit, tree := fixtureRepo(t)
	dirName := "2026-09-12-review-state-shape"
	writeArchiveReport(t, root, dirName, "# Archive Report\n\n## Cycle Timestamps\n\n"+
		"| landing_commit | `"+commit+"` |\n\nAnchor outcome: verified against the persisted review receipt.\n")
	receiptsDir := filepath.Join(root, "openspec", "changes", "archive", dirName, "review-receipts")
	writeReviewStateFile(t, receiptsDir, "review-lineage-1", tree, "approved")

	reports, findings, err := ScanArchive(root, ConventionDate)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("a review-state-verified anchor produced findings: %v", findings)
	}
	if len(reports) != 1 || reports[0].Outcome != AnchorVerified {
		t.Fatalf("reports = %v, want one %q report", reports, AnchorVerified)
	}
	if reports[0].Anchor == nil || reports[0].Anchor.ApprovedTree != tree {
		t.Fatalf("anchor = %#v, want approved_tree sourced from the review-state file (%s)", reports[0].Anchor, tree)
	}
}

// TestApprovedTree_NeverReadFromGitTransactionStore (task 4.4) asserts the
// gate reads approved_tree from the persisted, versioned review-receipts
// file -- never from the live, unversioned git transaction store a receipt
// might still be sitting in. A receipt planted only under .git/gentle-ai
// must NOT satisfy the receipt requirement.
func TestApprovedTree_NeverReadFromGitTransactionStore(t *testing.T) {
	root, commit, tree := fixtureRepo(t)
	dirName := "2026-09-12-live-state-not-read"
	writeArchiveReport(t, root, dirName, "# Archive Report\n\n## Cycle Timestamps\n\n"+
		"| landing_commit | `"+commit+"` |\n\nAnchor outcome: verified against the persisted review receipt.\n")
	// Plant a receipt ONLY under the live git transaction store, never under
	// openspec/changes/<change>/review-receipts/.
	liveStore := filepath.Join(root, ".git", "gentle-ai", "review-transactions", "v2", "review-lineage-1")
	writeReceiptFile(t, liveStore, "review-receipt", tree, "approved")

	_, findings, err := ScanArchive(root, ConventionDate)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("want exactly one finding (no verified receipt), got %v", findings)
	}
	if !strings.Contains(findings[0].Reason, "no verified receipt") {
		t.Errorf("finding %q does not name the missing persisted receipt", findings[0].Reason)
	}
}

// TestArchiveBlock_NoReceiptPostConvention (task 4.2) asserts a report
// archived on or after ReceiptConventionDate that records a landing commit
// but no persisted receipt is a hard Finding.
func TestArchiveBlock_NoReceiptPostConvention(t *testing.T) {
	root, commit, _ := fixtureRepo(t)
	writeArchiveReport(t, root, "2026-09-12-no-receipt-change", "# Archive Report\n\n## Cycle Timestamps\n\n"+
		"| landing_commit | `"+commit+"` |\n\nAnchor outcome: self-asserted.\n")

	_, findings, err := ScanArchive(root, ConventionDate)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("want exactly one finding, got %v", findings)
	}
	if !strings.Contains(findings[0].Reason, "no verified receipt") {
		t.Errorf("finding %q does not name the missing receipt", findings[0].Reason)
	}
	if !strings.Contains(findings[0].Reason, ReceiptConventionDate) {
		t.Errorf("finding %q does not cite the receipt convention date", findings[0].Reason)
	}
}

// TestOverride_RecordedSelfAsserted (task 4.2) asserts a report with a
// recorded, valid owner override AND the word "override" disclosed in its
// Cycle Timestamps section passes -- self-asserted, never verified.
func TestOverride_RecordedSelfAsserted(t *testing.T) {
	root, commit, _ := fixtureRepo(t)
	dirName := "2026-09-12-override-change"
	writeArchiveReport(t, root, dirName, "# Archive Report\n\n## Cycle Timestamps\n\n"+
		"| landing_commit | `"+commit+"` |\n\nAnchor outcome: self-asserted with a recorded owner override.\n")
	receiptsDir := filepath.Join(root, "openspec", "changes", "archive", dirName, "review-receipts")
	writeOverrideFile(t, receiptsDir, "alice", "reviewer unavailable, deadline commitment", "2026-09-12T00:00:00Z")

	reports, findings, err := ScanArchive(root, ConventionDate)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("a recorded, disclosed override produced findings: %v", findings)
	}
	if len(reports) != 1 || reports[0].Outcome != AnchorSelfAsserted {
		t.Fatalf("reports = %v, want one %q report", reports, AnchorSelfAsserted)
	}
}

// TestPreArchiveFlag (task 4.3) asserts the --change flag runs the same
// receipt check pre-archive, on the LIVE change folder, and exits 1 when the
// receipt is missing, 0 when a receipt or a recorded override exists.
func TestPreArchiveFlag(t *testing.T) {
	root, _, tree := fixtureRepo(t)

	t.Run("missing_receipt_blocks", func(t *testing.T) {
		changeDir := filepath.Join(root, "openspec", "changes", "no-receipt-yet")
		if err := os.MkdirAll(changeDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		code, _, stderr := runGate(t, "--repo", root, "--change", "no-receipt-yet")
		if code != exitFinding {
			t.Fatalf("code = %d, want %d (blocked); stderr=%s", code, exitFinding, stderr)
		}
		if !strings.Contains(stderr, "no-receipt-yet") {
			t.Errorf("stderr %q does not name the change", stderr)
		}
	})

	t.Run("receipt_present_passes", func(t *testing.T) {
		changeDir := filepath.Join(root, "openspec", "changes", "has-receipt")
		receiptsDir := filepath.Join(changeDir, "review-receipts")
		writeReceiptFile(t, receiptsDir, "review-lineage-1", tree, "approved")
		code, stdout, stderr := runGate(t, "--repo", root, "--change", "has-receipt")
		if code != exitOK {
			t.Fatalf("code = %d, want %d (ok); stdout=%s stderr=%s", code, exitOK, stdout, stderr)
		}
	})

	t.Run("review_state_shape_present_passes", func(t *testing.T) {
		changeDir := filepath.Join(root, "openspec", "changes", "has-review-state")
		receiptsDir := filepath.Join(changeDir, "review-receipts")
		writeReviewStateFile(t, receiptsDir, "review-lineage-1", tree, "approved")
		code, stdout, stderr := runGate(t, "--repo", root, "--change", "has-review-state")
		if code != exitOK {
			t.Fatalf("code = %d, want %d (ok); stdout=%s stderr=%s", code, exitOK, stdout, stderr)
		}
	})

	t.Run("override_present_passes", func(t *testing.T) {
		changeDir := filepath.Join(root, "openspec", "changes", "has-override")
		receiptsDir := filepath.Join(changeDir, "review-receipts")
		writeOverrideFile(t, receiptsDir, "alice", "reviewer unavailable", "2026-09-12T00:00:00Z")
		code, stdout, stderr := runGate(t, "--repo", root, "--change", "has-override")
		if code != exitOK {
			t.Fatalf("code = %d, want %d (ok); stdout=%s stderr=%s", code, exitOK, stdout, stderr)
		}
	})
}

// TestApprovedTree_FromRealCapturedReceipt (CRIT-1 remediation) loads one of
// the four review-receipt files this change actually persisted under
// openspec/changes/pi-package-hardening/review-receipts/ -- the real
// gentle-ai.review-state-record/v2 wrapper shape, nested one level under
// "state" -- and asserts the gate extracts its true
// state.current_snapshot.candidate_tree. A fabricated fixture (like
// writeReviewStateFile above) cannot catch a gate that only agrees with
// itself; this test is seeded from production data.
func TestApprovedTree_FromRealCapturedReceipt(t *testing.T) {
	repo := repoRoot(t)
	src := filepath.Join(repo, "openspec", "changes", "pi-package-hardening",
		"review-receipts", "review-42353e66bae50ad8.review-state.json")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read real receipt fixture %s: %v", src, err)
	}

	var wrapper struct {
		State struct {
			CurrentSnapshot struct {
				CandidateTree string `json:"candidate_tree"`
			} `json:"current_snapshot"`
		} `json:"state"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		t.Fatalf("parse real receipt fixture: %v", err)
	}
	wantTree := wrapper.State.CurrentSnapshot.CandidateTree
	if wantTree == "" {
		t.Fatalf("real receipt fixture %s has no state.current_snapshot.candidate_tree", src)
	}

	dir := t.TempDir()
	dest := filepath.Join(dir, "review-42353e66bae50ad8.review-state.json")
	if err := os.WriteFile(dest, data, 0o600); err != nil {
		t.Fatalf("copy real receipt fixture: %v", err)
	}

	tree, lineage, found, err := loadApprovedTreeFromReceipts(dir)
	if err != nil {
		t.Fatalf("loadApprovedTreeFromReceipts: %v", err)
	}
	if !found {
		t.Fatalf("a real, approved gentle-ai.review-state-record/v2 receipt was not recognized")
	}
	if tree != wantTree {
		t.Fatalf("tree = %q, want %q (from the real captured receipt)", tree, wantTree)
	}
	if lineage != "review-42353e66bae50ad8" {
		t.Fatalf("lineage = %q, want review-42353e66bae50ad8", lineage)
	}
}
