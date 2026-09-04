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

// TestGateOnTheRealRepositoryWithTheCommittedLedger is what CI runs: with every
// declared gap waived, the committed ledger keeps the run green.
//
// It asserts the EXIT CODE and nothing about which reports are non-compliant.
// The assertions it used to carry — that the repository fails without the
// ledger, and that at least one "known gap" is printed — both pinned the
// repository's current state rather than the gate's behaviour, so closing the
// declared gap would have turned them red with no ledger edit able to fix it.
// Those properties are now tested against fixtures, where they belong:
// TestGateFailsAnUndeclaredNonCompliantReport and
// TestDeclaredGapIsReportedNotSilenced.
func TestGateOnTheRealRepositoryWithTheCommittedLedger(t *testing.T) {
	code, stdout, stderr := runGate(t, "--repo", repoRoot(t), "--known-gaps", "known-gaps.txt")
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d; stdout=%q stderr=%q", code, exitOK, stdout, stderr)
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

// --- Hash classification: field name, never hash length -------------------
//
// Every fixture above abbreviates the landing commit to seven characters. That
// hid the gate's real classification rule: it told a commit from a tree by
// LENGTH, so the tested case worked and its twin did not.

// TestScanArchiveAcceptsAFullShaLandingCommit is the reported case. Recording
// the landing commit in full is this repository's own habit
// (engine/skills/testdata records 40-character commits), and a fully compliant
// report that did so was failed with "records no landing commit" — the exact
// opposite of its defect.
func TestScanArchiveAcceptsAFullShaLandingCommit(t *testing.T) {
	root, commit, tree := fixtureRepo(t)
	if len(commit) != 40 {
		t.Fatalf("fixture commit %q is not a full SHA; this test would prove nothing", commit)
	}
	writeArchiveReport(t, root, "2026-09-10-full-sha-change", fmt.Sprintf(
		"# Archive Report\n\n## Cycle Timestamps\n\n| Anchor | Value |\n|---|---|\n| landing_commit | `%s` |\n| approved_tree | `%s` |\n\nAnchor outcome: verified.\n",
		commit, tree))

	reports, findings, err := ScanArchive(root, ConventionDate)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("a full-SHA landing commit produced findings: %v", findings)
	}
	if len(reports) != 1 || reports[0].Outcome != AnchorVerified {
		t.Fatalf("reports = %v, want one %q report", reports, AnchorVerified)
	}
	if reports[0].Anchor == nil || reports[0].Anchor.LandingCommit != commit {
		t.Fatalf("landing commit = %#v, want %q — the 40-character hash was not read as the commit", reports[0].Anchor, commit)
	}
}

// TestScanArchiveAcceptsAnAbbreviatedApprovedTree is the mirror twin: the tree
// is the short hash and the commit is the long one. A length rule reads BOTH
// directions backwards, so repairing only the long-commit case repairs half
// the defect.
func TestScanArchiveAcceptsAnAbbreviatedApprovedTree(t *testing.T) {
	root, commit, tree := fixtureRepo(t)
	writeArchiveReport(t, root, "2026-09-10-short-tree-change", fmt.Sprintf(
		"# Archive Report\n\n## Cycle Timestamps\n\n| landing_commit | `%s` |\n| approved_tree | `%s` |\n\nAnchor outcome: verified.\n",
		commit[:7], tree[:12]))

	reports, findings, err := ScanArchive(root, ConventionDate)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("an abbreviated approved_tree produced findings: %v", findings)
	}
	if len(reports) != 1 || reports[0].Outcome != AnchorVerified {
		t.Fatalf("reports = %v, want one %q report", reports, AnchorVerified)
	}
	if reports[0].Anchor == nil || reports[0].Anchor.ApprovedTree != tree[:12] {
		t.Fatalf("approved tree = %#v, want %q — the 12-character hash was not read as the tree", reports[0].Anchor, tree[:12])
	}
}

// TestScanArchiveRejectsAFullShaTreeMismatch proves the misclassification also
// SILENCED the gate: with the commit read as a tree, the tree-mismatch and
// fabricated-verification branches were unreachable for a full-SHA record.
func TestScanArchiveRejectsAFullShaTreeMismatch(t *testing.T) {
	root, commit, _ := fixtureRepo(t)
	writeArchiveReport(t, root, "2026-09-10-full-sha-mismatch", fmt.Sprintf(
		"# Archive Report\n\n## Cycle Timestamps\n\n| landing_commit | `%s` |\n| approved_tree | `0000000000000000000000000000000000000000` |\n",
		commit))

	_, findings, err := ScanArchive(root, ConventionDate)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("want exactly one finding, got %v", findings)
	}
	if !strings.Contains(findings[0].Reason, "rejected, not trusted") {
		t.Errorf("finding %q does not name the rejection rule — the mismatch branch was bypassed", findings[0].Reason)
	}
}

// TestScanArchiveReadsAnUnlabelledFullShaAsTheLandingCommit closes the same
// hole in its unlabelled form. A hash no field name governs cannot be told
// apart by length either; the anchor contract's primary field is the landing
// commit, so that is the slot an unnamed hash fills.
func TestScanArchiveReadsAnUnlabelledFullShaAsTheLandingCommit(t *testing.T) {
	root, commit, _ := fixtureRepo(t)
	writeArchiveReport(t, root, "2026-09-10-unlabelled-change", fmt.Sprintf(
		"# Archive Report\n\n## Cycle Timestamps\n\nMerged to main as `%s`; no review ran for this candidate.\n",
		commit))

	reports, findings, err := ScanArchive(root, ConventionDate)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("an unlabelled full SHA produced findings: %v", findings)
	}
	if len(reports) != 1 || reports[0].Outcome != AnchorSelfAsserted {
		t.Fatalf("reports = %v, want one %q report", reports, AnchorSelfAsserted)
	}
}

// TestRealArchiveReportsStillClassifyAsBefore is the regression fence for the
// classification change. These two reports are the only in-scope records this
// repository carries, they use prose and table forms no fixture imitates, and
// their outcomes must not move when the rule changes.
func TestRealArchiveReportsStillClassifyAsBefore(t *testing.T) {
	reports, _, err := ScanArchive(repoRoot(t), ConventionDate)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	want := map[string]AnchorOutcome{
		"openspec/changes/archive/2026-08-19-sdd-cycle-timestamp-instrumentation/archive-report.md": AnchorVerified,
		"openspec/changes/archive/2026-08-24-tui-self-update-offer/archive-report.md":               AnchorSelfAsserted,
	}
	got := map[string]AnchorOutcome{}
	for _, report := range reports {
		got[report.RelPath] = report.Outcome
	}
	for relPath, outcome := range want {
		if got[relPath] != outcome {
			t.Errorf("%s classified %q, want %q", relPath, got[relPath], outcome)
		}
	}
}

// --- Verification claims: a disclaimer is not a claim ----------------------

// TestSelfAssertedIsNotFailedForDisclaimingVerification covers the negations.
// A substring probe for "verified" also fires on "unverified" and "not
// verified", so a report that correctly labelled its anchor self-asserted was
// failed for claiming the verification it had just disclaimed.
func TestSelfAssertedIsNotFailedForDisclaimingVerification(t *testing.T) {
	disclaimers := []string{
		"Anchor outcome: self-asserted. The landing commit is unverified: no review receipt exists.",
		"Anchor outcome: self-asserted. No approved_tree was recorded, so the anchor was not verified.",
		"Anchor outcome: self-asserted. With no receipt this anchor can never be verified.",
		"Anchor outcome: self-asserted. There is no independent tree, so the commit cannot be verified.",
		"Anchor outcome: self-asserted. The tree was not independently verified.",
	}
	for i, disclaimer := range disclaimers {
		t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
			root, commit, _ := fixtureRepo(t)
			writeArchiveReport(t, root, "2026-09-10-disclaimed-change", fmt.Sprintf(
				"# Archive Report\n\n## Cycle Timestamps\n\n| landing_commit | `%s` |\n\n%s\n", commit[:7], disclaimer))

			_, findings, err := ScanArchive(root, ConventionDate)
			if err != nil {
				t.Fatalf("scan: %v", err)
			}
			if len(findings) != 0 {
				t.Fatalf("a disclaimed verification produced findings: %v", findings)
			}
		})
	}
}

// TestUndisclosedMismatchIsNotExcusedByANegatedRejection is the same defect on
// the sibling claim. "not rejected" contains "rejected", so a substring probe
// let a report that DENIES the rejection satisfy the disclosure a rejected
// anchor owes.
func TestUndisclosedMismatchIsNotExcusedByANegatedRejection(t *testing.T) {
	root, commit, _ := fixtureRepo(t)
	writeArchiveReport(t, root, "2026-09-10-negated-rejection", fmt.Sprintf(
		"# Archive Report\n\n## Cycle Timestamps\n\n| landing_commit | `%s` |\n| approved_tree | `0000000000000000000000000000000000000000` |\n\nThe trees agree, so the anchor was not rejected.\n",
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

// --- An empty archive is not a failure -------------------------------------

// TestScanArchivePassesWithoutAnArchiveDirectory covers the projects that have
// no openspec/changes/archive at all — engram-only and artifact-store `none`.
// The gate's every non-zero exit is a documented hard stop, so reporting a
// missing directory as an I/O failure permanently blocked closure for them.
func TestScanArchivePassesWithoutAnArchiveDirectory(t *testing.T) {
	reports, findings, err := ScanArchive(t.TempDir(), ConventionDate)
	if err != nil {
		t.Fatalf("an absent archive directory is not a failure: %v", err)
	}
	if len(reports) != 0 || len(findings) != 0 {
		t.Fatalf("reports=%v findings=%v, want nothing checked and nothing found", reports, findings)
	}
}

// TestGateExitsCleanWithoutAnArchiveDirectory is the same property where the
// CI job observes it: the exit code.
func TestGateExitsCleanWithoutAnArchiveDirectory(t *testing.T) {
	code, stdout, stderr := runGate(t, "--repo", t.TempDir())
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d; stdout=%q stderr=%q", code, exitOK, stdout, stderr)
	}
}

// TestScanArchiveStillFailsOnAnUnreadableArchivePath is the twin that keeps the
// fix honest: only ABSENCE is a clean pass. An archive path that exists but
// cannot be listed is still a real I/O failure and must not be swallowed.
func TestScanArchiveStillFailsOnAnUnreadableArchivePath(t *testing.T) {
	root := t.TempDir()
	changes := filepath.Join(root, "openspec", "changes")
	if err := os.MkdirAll(changes, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(changes, "archive"), []byte("not a directory\n"), 0o600); err != nil {
		t.Fatalf("write archive placeholder: %v", err)
	}
	if _, _, err := ScanArchive(root, ConventionDate); err == nil {
		t.Fatal("an archive path that is not a directory must surface as an error, not be swallowed as an empty archive")
	}
}

// --- Ledger mechanism, tested against fixtures rather than the repository ---

// TestGateFailsAnUndeclaredNonCompliantReport is the honest half of the ledger
// mechanism. It replaces a test that asserted THIS REPOSITORY is non-compliant
// today: that assertion made the gate's own success — someone finally writing
// the missing anchor section — turn a test red, with no ledger edit able to fix
// it. A gate's tests must test the gate.
func TestGateFailsAnUndeclaredNonCompliantReport(t *testing.T) {
	root, _, _ := fixtureRepo(t)
	writeArchiveReport(t, root, "2026-09-10-undeclared-change",
		"# Archive Report\n\n**Delivery**: PRs staged, not yet merged (no landing commit)\n\n## Overview\n\nText.\n")

	code, _, stderr := runGate(t, "--repo", root)
	if code != exitFinding {
		t.Fatalf("exit code = %d, want %d; stderr=%q", code, exitFinding, stderr)
	}
	if !strings.Contains(stderr, "2026-09-10-undeclared-change") {
		t.Errorf("stderr %q does not name the non-compliant report", stderr)
	}
}

// TestDeclaredGapIsReportedNotSilenced is the other half: a waived report is
// still PRINTED as a known gap, so the ledger can never silence a record.
func TestDeclaredGapIsReportedNotSilenced(t *testing.T) {
	root, _, _ := fixtureRepo(t)
	writeArchiveReport(t, root, "2026-09-10-declared-change",
		"# Archive Report\n\n## Overview\n\nNo anchor section.\n")
	relPath := "openspec/changes/archive/2026-09-10-declared-change/archive-report.md"
	ledger := filepath.Join(t.TempDir(), "known-gaps.txt")
	if err := os.WriteFile(ledger, []byte(relPath+"\n"), 0o600); err != nil {
		t.Fatalf("write ledger: %v", err)
	}

	code, stdout, stderr := runGate(t, "--repo", root, "--known-gaps", ledger)
	if code != exitOK {
		t.Fatalf("exit code = %d, want %d; stdout=%q stderr=%q", code, exitOK, stdout, stderr)
	}
	if !strings.Contains(stdout, "known gap "+relPath) {
		t.Errorf("stdout %q does not report the declared gap — a silent waiver is the failure mode this ledger exists to avoid", stdout)
	}
}

// TestAbsenceDeclarationSurvivesAnUnrelatedHash is the twin of the unlabelled
// promotion rule. Promoting ANY unlabelled hash into the landing-commit slot
// made the absent branch unreachable, because checkReport consults
// absentDeclarations only when that slot is empty. A report that correctly
// declares its absent outcome AND quotes a receipt or content digest — which
// these reports routinely do — was then failed for "records landing commit ...
// which does not resolve": a hard stop on a compliant record, the opposite of
// its content. Absence is declared in prose, so the declaration must be read
// BEFORE an unnamed hash is promoted into the field it says does not exist.
func TestAbsenceDeclarationSurvivesAnUnrelatedHash(t *testing.T) {
	root, _, _ := fixtureRepo(t)
	writeArchiveReport(t, root, "2026-09-10-absent-with-digest",
		"# Archive Report\n\n## Cycle Timestamps\n\nNo landing commit: 82 PRs staged, not yet merged.\n\nReview receipt digest `0123456789abcdef0123456789abcdef01234567`.\n")

	reports, findings, err := ScanArchive(root, ConventionDate)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("a declared absence carrying an unrelated digest produced findings: %v", findings)
	}
	if len(reports) != 1 || reports[0].Outcome != AnchorAbsent {
		t.Fatalf("reports = %v, want one %q report", reports, AnchorAbsent)
	}
}

// TestLabelledCommitIsStillReadWhenAbsenceProseIsPresent is that fix's own
// twin: reading the declaration first must not make a LABELLED landing commit
// invisible. A report that names its commit and also carries absence-shaped
// prose still records an anchor, and that anchor is still resolved and checked.
func TestLabelledCommitIsStillReadWhenAbsenceProseIsPresent(t *testing.T) {
	root, commit, tree := fixtureRepo(t)
	writeArchiveReport(t, root, "2026-09-10-labelled-with-absence-prose", fmt.Sprintf(
		"# Archive Report\n\n## Cycle Timestamps\n\n| landing_commit | `%s` |\n| approved_tree | `%s` |\n\nThe predecessor change was never merged; this one was, and the tree is verified.\n",
		commit, tree))

	reports, findings, err := ScanArchive(root, ConventionDate)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("a labelled, verified anchor produced findings: %v", findings)
	}
	if len(reports) != 1 || reports[0].Anchor == nil || reports[0].Anchor.LandingCommit != commit {
		t.Fatalf("reports = %v, want the labelled landing commit %s", reports, commit)
	}
}

// --- Only a LABELLED hash may fill the tree slot ---------------------------
//
// An unlabelled hash is indistinguishable from a receipt or content digest, so
// it can neither BE the approved tree nor prove that none was recorded. The
// four shapes below are the cross-product the gate has to keep straight:
// labelled commit + labelled tree (above), labelled commit + bare digest,
// absence declaration + bare digest (above), and labelled commit + a bare hash
// that happens to equal the real tree.

// TestLabelledCommitWithABareReceiptDigestIsNotRejected is the reported live
// false hard stop. Quoting a review receipt digest in prose is the routine,
// documented-compliant form; promoting that digest into the approved_tree slot
// made it disagree with the landing commit's real tree and failed a truthful
// report with "a mis-recorded anchor is rejected".
//
// The `claiming_verified` case is where the false hard stop ENDS. Quoting a
// digest still may not promote it into the tree slot, so the record stays
// self-asserted -- but a report that then claims "verified" is claiming an
// assurance nothing in it could have produced, and that finding is the gate's
// only anti-fabrication check. It used to be silenced by the mere presence of
// a leftover hex token, which made "quote any hex word" a complete bypass; the
// suppression now requires a hash that really is the landing commit's tree
// (see TestBareHashEqualToTheRealTreeIsNeitherVerifiedNorFabricated).
func TestLabelledCommitWithABareReceiptDigestIsNotRejected(t *testing.T) {
	for _, tc := range []struct {
		name, prose     string
		wantFabrication bool
	}{
		{name: "plain", prose: "Review receipt digest `0123456789abcdef0123456789abcdef01234567`.\n"},
		{name: "claiming_verified", prose: "Review receipt digest `0123456789abcdef0123456789abcdef01234567`; the anchor is verified.\n", wantFabrication: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, commit, _ := fixtureRepo(t)
			writeArchiveReport(t, root, "2026-09-10-receipt-digest", fmt.Sprintf(
				"# Archive Report\n\n## Cycle Timestamps\n\n| landing_commit | `%s` |\n\n%s", commit, tc.prose))

			reports, findings, err := ScanArchive(root, ConventionDate)
			if err != nil {
				t.Fatalf("scan: %v", err)
			}
			for _, finding := range findings {
				if strings.Contains(finding.Reason, "mis-recorded anchor is rejected") {
					t.Fatalf("a bare receipt digest was promoted to the tree slot and rejected: %v", finding)
				}
			}
			switch {
			case tc.wantFabrication:
				if len(findings) != 1 || !strings.Contains(findings[0].Reason, "fabricated assurance") {
					t.Fatalf("findings = %v, want exactly the fabricated-assurance finding", findings)
				}
			case len(findings) != 0:
				t.Fatalf("a bare receipt digest produced findings: %v", findings)
			}
			if len(reports) != 1 || reports[0].Outcome != AnchorSelfAsserted {
				t.Fatalf("reports = %v, want one %q report", reports, AnchorSelfAsserted)
			}
			if reports[0].Anchor == nil || reports[0].Anchor.ApprovedTree != "" {
				t.Fatalf("anchor = %#v, want an empty approved_tree — an unlabelled digest is not a tree", reports[0].Anchor)
			}
		})
	}
}

// TestBareHashEqualToTheRealTreeIsNeitherVerifiedNorFabricated is the mirror.
// The unlabelled hash IS the landing commit's tree, but nothing in the report
// says so, so the gate may not upgrade the record to `verified` — and equally
// may not call the report's own "verified" claim a fabricated assurance,
// because a tree it cannot read is unknown, not absent.
func TestBareHashEqualToTheRealTreeIsNeitherVerifiedNorFabricated(t *testing.T) {
	root, commit, tree := fixtureRepo(t)
	writeArchiveReport(t, root, "2026-09-10-bare-real-tree", fmt.Sprintf(
		"# Archive Report\n\n## Cycle Timestamps\n\n| landing_commit | `%s` |\n\nThe review receipt reads `%s`; the anchor is verified.\n",
		commit, tree))

	reports, findings, err := ScanArchive(root, ConventionDate)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("an unlabelled hash equal to the real tree produced findings: %v", findings)
	}
	if len(reports) != 1 || reports[0].Outcome != AnchorSelfAsserted {
		t.Fatalf("reports = %v, want one %q report — an unnamed hash cannot promote a record to verified", reports, AnchorSelfAsserted)
	}
}

// TestAbsenceProseElsewhereDoesNotSilenceARealLandingCommit is the absence
// rule's own twin. A declaration scoped to something ELSE — a sub-PR that has
// not landed — must not suppress the landing commit the very next clause
// records, which would report a landed change as having no anchor at all.
func TestAbsenceProseElsewhereDoesNotSilenceARealLandingCommit(t *testing.T) {
	root, commit, _ := fixtureRepo(t)
	writeArchiveReport(t, root, "2026-09-10-scoped-absence", fmt.Sprintf(
		"# Archive Report\n\n## Cycle Timestamps\n\nSub-PR #3 is not yet merged. Merged to main as `%s`.\n", commit))

	reports, findings, err := ScanArchive(root, ConventionDate)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("a scoped absence phrase produced findings: %v", findings)
	}
	if len(reports) != 1 || reports[0].Outcome != AnchorSelfAsserted {
		t.Fatalf("reports = %v, want one %q report", reports, AnchorSelfAsserted)
	}
	if reports[0].Anchor == nil || reports[0].Anchor.LandingCommit != commit {
		t.Fatalf("anchor = %#v, want the recorded landing commit %s", reports[0].Anchor, commit)
	}
}

// TestUnresolvableBareHashIsNotAnAnchorAndDoesNotExcuseSilence guards the
// fallback that makes the three tests above safe. A bare hash that does not
// resolve as a commit was never a landing commit, so the report records none —
// and a report that records none while saying nothing about it is still the
// silence this gate exists to catch.
func TestUnresolvableBareHashIsNotAnAnchorAndDoesNotExcuseSilence(t *testing.T) {
	root, _, _ := fixtureRepo(t)
	writeArchiveReport(t, root, "2026-09-10-bare-digest-only",
		"# Archive Report\n\n## Cycle Timestamps\n\nReview receipt digest `0123456789abcdef0123456789abcdef01234567`.\n")

	_, findings, err := ScanArchive(root, ConventionDate)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("want exactly one finding, got %v", findings)
	}
	// The remedy names BOTH exits: label a real commit, or declare the absence
	// if that is the truth. It may not present the declaration as the only one,
	// because for a report that does name a commit that exit is a lie -- see
	// TestBothSpellingsOfAnUnresolvableCommitAgree.
	if !strings.Contains(findings[0].Reason, "records no landing commit the gate can use") {
		t.Errorf("finding %q does not say the section records no usable landing commit", findings[0].Reason)
	}
	if !strings.Contains(findings[0].Reason, "no landing commit exists, state that absent outcome") {
		t.Errorf("finding %q does not offer the absent declaration as an exit", findings[0].Reason)
	}
}

// TestAResolvableBareHashOutranksAbsenceProse pins the boundary the two rules
// above meet at, because it is the one case where they disagree. A section that
// declares absence AND quotes a hash that really is a commit in this repository
// records an anchor: it is read as self-asserted — used, never independently
// checked — rather than as no anchor at all. The alternative, letting the prose
// win, is what let an absence phrase about something ELSE erase a real landing
// commit; resolvability is evidence, and a phrase is not.
func TestAResolvableBareHashOutranksAbsenceProse(t *testing.T) {
	root, commit, _ := fixtureRepo(t)
	writeArchiveReport(t, root, "2026-09-10-absence-with-real-commit", fmt.Sprintf(
		"# Archive Report\n\n## Cycle Timestamps\n\nNo landing commit was recorded for the tracker; the work landed as `%s`.\n", commit))

	reports, findings, err := ScanArchive(root, ConventionDate)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("a resolvable bare commit produced findings: %v", findings)
	}
	if len(reports) != 1 || reports[0].Outcome != AnchorSelfAsserted {
		t.Fatalf("reports = %v, want one %q report", reports, AnchorSelfAsserted)
	}
}

// TestBothSpellingsOfAnUnresolvableCommitAgree is the regression that made the
// gate's remedy dangerous. A report body that NAMES its landing commit but
// writes the SHA bare fell through to the absence branch, whose remedy told the
// author to state "no landing commit" -- in a report that names one. An author
// who obeys writes a false record, and the gate then passes it with zero
// findings: a hard stop whose only exit is a lie. The LABELLED spelling of the
// same defect always said the true thing, so the two spellings disagreed about
// one defect. They must now give the same diagnosis.
func TestBothSpellingsOfAnUnresolvableCommitAgree(t *testing.T) {
	const unresolvable = "0123456789abcdef0123456789abcdef01234567"
	for _, tc := range []struct{ name, section string }{
		{"bare", "Merged to main as `" + unresolvable + "`.\n"},
		{"labelled", "| landing_commit | `" + unresolvable + "` |\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, _, _ := fixtureRepo(t)
			writeArchiveReport(t, root, "2026-09-10-unresolvable-"+tc.name,
				"# Archive Report\n\n## Cycle Timestamps\n\n"+tc.section)

			_, findings, err := ScanArchive(root, ConventionDate)
			if err != nil {
				t.Fatalf("scan: %v", err)
			}
			if len(findings) != 1 {
				t.Fatalf("want exactly one finding, got %v", findings)
			}
			reason := findings[0].Reason
			if !strings.Contains(reason, "does not resolve here as a commit") {
				t.Errorf("finding %q does not diagnose the unresolvable SHA", reason)
			}
			if !strings.Contains(reason, "assertion, not an anchor") {
				t.Errorf("finding %q does not name why an unresolvable SHA is not an anchor", reason)
			}
			if strings.Contains(reason, "must SAY it is missing") {
				t.Errorf("finding %q offers a false absence declaration as the only remedy", reason)
			}
		})
	}
}

// TestAnUnrelatedHexTokenDoesNotSilenceTheFabricationCheck pins the gate's ONLY
// anti-fabrication check against the cheapest possible bypass. The suppression
// key used to be "the section left some backticked hex over", so quoting any
// hex word at all -- an issue id, an unrelated abbreviation -- bought silence
// for a report claiming an assurance nothing could have produced.
func TestAnUnrelatedHexTokenDoesNotSilenceTheFabricationCheck(t *testing.T) {
	root, commit, _ := fixtureRepo(t)
	writeArchiveReport(t, root, "2026-09-10-unrelated-hex", fmt.Sprintf(
		"# Archive Report\n\n## Cycle Timestamps\n\n| landing_commit | `%s` |\n\nSee `deadbee` for context. Anchor outcome: verified.\n",
		commit))

	_, findings, err := ScanArchive(root, ConventionDate)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("want exactly one finding, got %v", findings)
	}
	if !strings.Contains(findings[0].Reason, "fabricated assurance") {
		t.Errorf("finding %q does not name the fabricated assurance", findings[0].Reason)
	}
}
