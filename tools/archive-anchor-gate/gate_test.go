package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	return root
}

// fixtureRepo builds a throwaway git repository with one commit and returns its
// root, the commit SHA, and the commit's tree hash. The gate's whole contract
// is a statement about real git objects, so the tests use real ones.
func fixtureRepo(t *testing.T) (root, commit, tree string) {
	t.Helper()
	root = t.TempDir()
	for _, args := range [][]string{
		{"init", "--quiet", "--initial-branch=main"},
		{"config", "user.email", "gate@example.invalid"},
		{"config", "user.name", "Anchor Gate Fixture"},
	} {
		if _, err := runGitReadOnly(append([]string{"-C", root}, args...)...); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("content\n"), 0o600); err != nil {
		t.Fatalf("write fixture file: %v", err)
	}
	if _, err := runGitReadOnly("-C", root, "add", "file.txt"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if _, err := runGitReadOnly("-C", root, "commit", "--quiet", "-m", "fixture"); err != nil {
		t.Fatalf("git commit: %v", err)
	}
	out, err := runGitReadOnly("-C", root, "show", "-s", "--format=%H %T", "HEAD")
	if err != nil {
		t.Fatalf("git show: %v", err)
	}
	fields := strings.Fields(strings.TrimSpace(out))
	if len(fields) != 2 {
		t.Fatalf("unexpected git show output %q", out)
	}
	return root, fields[0], fields[1]
}

func TestResolveT1ClassifiesEveryOutcome(t *testing.T) {
	root, commit, tree := fixtureRepo(t)

	t.Run("absent", func(t *testing.T) {
		ts, outcome, err := ResolveT1(root, nil)
		if err != nil || outcome != AnchorAbsent || ts != nil {
			t.Fatalf("got (%v, %q, %v), want (nil, %q, nil)", ts, outcome, err, AnchorAbsent)
		}
	})

	t.Run("verified", func(t *testing.T) {
		ts, outcome, err := ResolveT1(root, &RecordedAnchor{LandingCommit: commit, ApprovedTree: tree})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if outcome != AnchorVerified {
			t.Fatalf("outcome = %q, want %q", outcome, AnchorVerified)
		}
		if ts == nil {
			t.Fatal("a verified anchor must resolve t1")
		}
	})

	t.Run("self_asserted_still_resolves_t1", func(t *testing.T) {
		// The measurement is NOT withheld when no review ran; it is labelled.
		ts, outcome, err := ResolveT1(root, &RecordedAnchor{LandingCommit: commit})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if outcome != AnchorSelfAsserted {
			t.Fatalf("outcome = %q, want %q", outcome, AnchorSelfAsserted)
		}
		if ts == nil {
			t.Fatal("a self-asserted anchor still resolves t1 — the measurement is used, just never independently checked")
		}
	})

	t.Run("rejected_omits_t1", func(t *testing.T) {
		ts, outcome, err := ResolveT1(root, &RecordedAnchor{
			LandingCommit: commit,
			ApprovedTree:  "0000000000000000000000000000000000000000",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if outcome != AnchorRejected {
			t.Fatalf("outcome = %q, want %q", outcome, AnchorRejected)
		}
		if ts != nil {
			t.Fatal("a rejected anchor must omit t1, never trust it")
		}
	})

	t.Run("unresolvable_commit_is_an_error_not_an_outcome", func(t *testing.T) {
		if _, _, err := ResolveT1(root, &RecordedAnchor{LandingCommit: "deadbeef"}); err == nil {
			t.Fatal("a landing commit that does not resolve must surface as an error, not be silently classified")
		}
	})
}

// TestSelfAssertedNeverSynthesisesItsOwnTree is the tautology guard. If
// approved_tree were filled in from the landing commit's own tree when no
// review ran, the mandated comparison would compare a value against itself and
// could never fail — and an unfailable check reported as verification is a
// fabricated assurance.
func TestSelfAssertedNeverSynthesisesItsOwnTree(t *testing.T) {
	root, commit, _ := fixtureRepo(t)
	_, outcome, err := ResolveT1(root, &RecordedAnchor{LandingCommit: commit})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome == AnchorVerified {
		t.Fatal("an anchor with no recorded approved_tree was reported as verified — the tree was synthesised from the commit itself")
	}
}

func TestTreeMatchesRejectsTooShortAPrefix(t *testing.T) {
	const actual = "b654d2d006d4d151145fd6452817f97d1ddc9ebb"
	if treeMatches(actual, "b654d2") {
		t.Error("a 6-character prefix distinguishes nothing and must not count as a match")
	}
	if !treeMatches(actual, "b654d2d") {
		t.Error("a 7-character prefix is the documented abbreviation floor and must match")
	}
	if treeMatches(actual, "b654d2e") {
		t.Error("a prefix that disagrees must not match")
	}
}

// writeArchiveReport materialises a fake repository layout with one archived
// change, so the scanner can be exercised on inputs the real tree does not
// contain.
func writeArchiveReport(t *testing.T, root, dirName, body string) {
	t.Helper()
	dir := filepath.Join(root, "openspec", "changes", "archive", dirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "archive-report.md"), []byte(body), 0o600); err != nil {
		t.Fatalf("write report: %v", err)
	}
}

func TestScanArchiveAcceptsAVerifiedAnchor(t *testing.T) {
	root, commit, tree := fixtureRepo(t)
	writeArchiveReport(t, root, "2026-09-10-verified-change", fmt.Sprintf(
		"# Archive Report\n\n## Cycle Timestamps\n\n| Anchor | Value |\n|---|---|\n| landing_commit | `%s` |\n| approved_tree | `%s` |\n\nAnchor outcome: verified.\n",
		commit[:7], tree))

	_, findings, err := ScanArchive(root, ConventionDate)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("verified anchor produced findings: %v", findings)
	}
}

func TestScanArchiveRejectsAMissingCycleTimestampsSection(t *testing.T) {
	root, _, _ := fixtureRepo(t)
	writeArchiveReport(t, root, "2026-09-10-silent-change",
		"# Archive Report\n\n**Delivery**: PRs staged, not yet merged (no landing commit)\n\n## Overview\n\nText.\n")

	_, findings, err := ScanArchive(root, ConventionDate)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("want exactly one finding, got %v", findings)
	}
	if !strings.Contains(findings[0].Reason, cycleTimestampsHeading) {
		t.Errorf("finding %q does not name the missing section", findings[0].Reason)
	}
}

func TestScanArchiveRejectsAFabricatedVerification(t *testing.T) {
	root, commit, _ := fixtureRepo(t)
	writeArchiveReport(t, root, "2026-09-10-fabricated-change", fmt.Sprintf(
		"# Archive Report\n\n## Cycle Timestamps\n\n| landing_commit | `%s` |\n\nAnchor outcome: verified against the review receipt.\n",
		commit[:7]))

	_, findings, err := ScanArchive(root, ConventionDate)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("want exactly one finding, got %v", findings)
	}
	if !strings.Contains(findings[0].Reason, "fabricated assurance") {
		t.Errorf("finding %q does not name the fabricated verification", findings[0].Reason)
	}
}

func TestScanArchiveRejectsAnUndisclosedTreeMismatch(t *testing.T) {
	root, commit, _ := fixtureRepo(t)
	writeArchiveReport(t, root, "2026-09-10-mismatched-change", fmt.Sprintf(
		"# Archive Report\n\n## Cycle Timestamps\n\n| landing_commit | `%s` |\n| approved_tree | `0000000000000000000000000000000000000000` |\n",
		commit[:7]))

	_, findings, err := ScanArchive(root, ConventionDate)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("want exactly one finding, got %v", findings)
	}
	if !strings.Contains(findings[0].Reason, "rejected, not trusted") {
		t.Errorf("finding %q does not name the rejection rule", findings[0].Reason)
	}
}

func TestScanArchiveAcceptsADisclosedRejection(t *testing.T) {
	root, commit, _ := fixtureRepo(t)
	writeArchiveReport(t, root, "2026-09-10-disclosed-change", fmt.Sprintf(
		"# Archive Report\n\n## Cycle Timestamps\n\n| landing_commit | `%s` |\n| approved_tree | `0000000000000000000000000000000000000000` |\n\nAnchor outcome: rejected — the trees disagree, so t1 is omitted and the mismatch is disclosed.\n",
		commit[:7]))

	_, findings, err := ScanArchive(root, ConventionDate)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("a disclosed rejection is compliant, got findings %v", findings)
	}
}

func TestScanArchiveIgnoresReportsPredatingTheConvention(t *testing.T) {
	root, _, _ := fixtureRepo(t)
	writeArchiveReport(t, root, "2026-06-20-old-change", "# Archive Report\n\nNo anchors here.\n")

	reports, findings, err := ScanArchive(root, ConventionDate)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(reports) != 0 || len(findings) != 0 {
		t.Fatalf("a pre-convention report must be out of scope, got reports=%v findings=%v", reports, findings)
	}
}

func runGate(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// TestGateFailsOnTheRealRepositoryWithoutTheLedger is the honest half of the
// ledger mechanism: without a waiver the gate genuinely fails on this
// repository today, and it names the one report responsible.
func TestGateFailsOnTheRealRepositoryWithoutTheLedger(t *testing.T) {
	code, _, stderr := runGate(t, "--repo", repoRoot(t))
	if code != exitFinding {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, exitFinding, stderr)
	}
	if !strings.Contains(stderr, "2026-09-02-longterm-mem") {
		t.Errorf("stderr %q does not name the non-compliant report", stderr)
	}
}

// TestGateOnTheRealRepositoryWithTheCommittedLedger is what CI runs. Every gap
// is declared, so the run is green; the moment a NEW report misses its anchor
// section it is not on the list and the run goes red.
func TestGateOnTheRealRepositoryWithTheCommittedLedger(t *testing.T) {
	code, stdout, stderr := runGate(t, "--repo", repoRoot(t), "--known-gaps", "known-gaps.txt")
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d; stdout=%q stderr=%q", code, exitOK, stdout, stderr)
	}
	if !strings.Contains(stdout, "known gap") {
		t.Errorf("stdout %q does not report the declared gap — a silent waiver is the failure mode this ledger exists to avoid", stdout)
	}
}

// TestStaleWaiverFailsTheRun keeps the ledger from rotting into a permanent
// exemption: an entry that no longer describes a real gap fails the run until
// it is deleted.
func TestStaleWaiverFailsTheRun(t *testing.T) {
	root, commit, tree := fixtureRepo(t)
	writeArchiveReport(t, root, "2026-09-10-compliant-change", fmt.Sprintf(
		"# Archive Report\n\n## Cycle Timestamps\n\n| landing_commit | `%s` |\n| approved_tree | `%s` |\n",
		commit[:7], tree))
	ledger := filepath.Join(t.TempDir(), "known-gaps.txt")
	if err := os.WriteFile(ledger, []byte("openspec/changes/archive/2026-09-10-compliant-change/archive-report.md\n"), 0o600); err != nil {
		t.Fatalf("write ledger: %v", err)
	}

	code, _, stderr := runGate(t, "--repo", root, "--known-gaps", ledger)
	if code != exitFinding {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, exitFinding, stderr)
	}
	if !strings.Contains(stderr, "stale waiver") {
		t.Errorf("stderr %q does not name the stale waiver", stderr)
	}
}

func TestUsageErrors(t *testing.T) {
	if code, _, _ := runGate(t); code != exitUsage {
		t.Errorf("missing --repo: exit code = %d, want %d", code, exitUsage)
	}
	if code, _, _ := runGate(t, "--repo", repoRoot(t), "--known-gaps", filepath.Join(t.TempDir(), "absent.txt")); code != exitIO {
		t.Errorf("absent ledger: exit code = %d, want %d", code, exitIO)
	}
}

func TestGitIsAvailable(t *testing.T) {
	// The whole gate is a claim about git objects; if git is missing, every
	// other test in this file would report a vacuous pass.
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatalf("git is required to exercise this gate: %v", err)
	}
}
