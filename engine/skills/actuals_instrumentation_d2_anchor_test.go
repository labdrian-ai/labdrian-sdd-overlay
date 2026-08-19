package skills

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestD2AnchorIsVerifiableAndUnambiguous proves design.md D2 (REVISION 3, Phase
// 13 correction): a t1 anchor is recorded as `landing_commit` always, plus
// `approved_tree` ONLY when a review receipt exists for the candidate. The
// outcome resolveD2T1 reports is one of four states, never conflated:
//   - d2AnchorAbsent — no landing_commit was ever recorded; no t1.
//   - d2AnchorSelfAsserted — landing_commit recorded, but no approved_tree (no
//     review ran): t1 STILL resolves, but there is no independent authority
//     to check it against, so it MUST NOT be reported as verified.
//   - d2AnchorVerified — the landing commit's own tree equals the recorded
//     approved_tree: independently checked against a review receipt.
//   - d2AnchorRejected — the landing commit's own tree does NOT equal the
//     recorded approved_tree: t1 omitted, never trusted.
//
// Before Phase 13, `approved_tree` was synthesized from `landing_commit`'s own
// tree whenever no review ran, so the mandated check
// (`git show -s --format=%T <landing_commit>` == `approved_tree`) compared a
// value against itself and could never fail — a no-review anchor was reported
// as "verified" by construction. This file's `self-asserted` outcome and the
// `recorded_anchor_with_no_review_receipt_is_self_asserted_never_verified`
// subtest below exist specifically to make that class of defect impossible to
// reintroduce silently: see that subtest's own mutation-probe comment for the
// RED/GREEN proof.
//
// Also proves: a mismatched tree is REJECTED rather than trusted, and — when a
// tree is all that is available and more than one commit on the default
// branch carries it — the anchor is AMBIGUOUS: t1 is omitted and the
// ambiguity disclosed, never resolved by position. Trees are not unique
// across commits (`main` has 434 commits sharing only 84 distinct duplicated
// tree values, so 205 of 434 commits sit in a colliding tree); "earliest
// carrier" is a guess wearing the costume of a rule, not a disambiguation
// rule — a real three-way collision on this repository's own history (tree
// 2ad8e42e…) resolves earliest-by-time to a commit belonging to a different
// change.
//
// Revision 2's path-scan fallback (a first-parent scan of
// openspec/changes/{change}/) is not merely unused here — resolveD2T1's
// signature below takes an optional (landing_commit, approved_tree) pair and
// nothing else. It cannot accept a change slug or folder path, so it is
// structurally incapable of ever invoking a folder scan. See design.md D2
// revision 3 and its "No heuristic fallback" paragraph.
//
// This lives in a _test.go file — never a production file — because
// engine/skills/'s zero-fetch import allowlist (ADR-15, TestZeroFetchImportAllowlist)
// forbids os/exec, path and time in production code: closure-feedback's real
// git invocations are shell commands the orchestrating agent runs directly
// (skills/inception-pipeline/SKILL.md), never Go library code that shells out.
// This test only PROVES the documented rule is correct and unambiguous; it
// does not implement the runtime path.
func TestD2AnchorIsVerifiableAndUnambiguous(t *testing.T) {
	repoRoot := actualsInstrumentationRepoRoot(t)

	t.Run("recorded_anchor_with_matching_tree_resolves_verified", func(t *testing.T) {
		// deterministic-verification-evidence's own archive-report records, in
		// versioned prose, "merged as `be2c3ca`" — design.md D2 revision 3's
		// own precedent citation. Its real tree (read independently below via
		// `git show`, not asserted here) is `2ad8e42e…`.
		anchor := &d2RecordedAnchor{
			landingCommit: "be2c3ca46ea5f4f740ecab6b51dc2db3f167f37c",
			approvedTree:  "2ad8e42e4f1ef4ff3e46c447d70e6123664f2180",
		}
		t1, outcome, err := resolveD2T1(repoRoot, anchor)
		if err != nil {
			t.Fatalf("resolveD2T1() error = %v", err)
		}
		if outcome != d2AnchorVerified {
			t.Fatalf("resolveD2T1() outcome = %q, want %q for a genuinely matching anchor", outcome, d2AnchorVerified)
		}
		if t1 == nil {
			t.Fatal("resolveD2T1() = nil t1, want the landing commit's committer timestamp")
		}
		wantTS, err := time.Parse(time.RFC3339, "2026-08-05T10:10:53-03:00")
		if err != nil {
			t.Fatalf("parsing expected timestamp: %v", err)
		}
		if !t1.Equal(wantTS) {
			t.Fatalf("resolveD2T1() t1 = %v, want %v", t1, wantTS)
		}
	})

	t.Run("mismatched_tree_is_rejected_not_trusted", func(t *testing.T) {
		// The recorded tree here is a REAL tree object on this repository — the
		// tree of a different commit (79995ea, the merge of an unrelated PR
		// #130) — paired with be2c3ca's SHA. This is exactly the shape
		// design.md's C2 finding warns about: a plausible-looking but wrong
		// anchor must be caught by verification, not trusted.
		anchor := &d2RecordedAnchor{
			landingCommit: "be2c3ca46ea5f4f740ecab6b51dc2db3f167f37c",
			approvedTree:  "b654d2d006d4d151145fd6452817f97d1ddc9ebb",
		}
		t1, outcome, err := resolveD2T1(repoRoot, anchor)
		if err != nil {
			t.Fatalf("resolveD2T1() error = %v", err)
		}
		if outcome != d2AnchorRejected {
			t.Fatalf("resolveD2T1() outcome = %q, want %q — an anchor whose landing commit's tree does not equal the recorded approved_tree must be rejected", outcome, d2AnchorRejected)
		}
		if t1 != nil {
			t.Fatalf("resolveD2T1() t1 = %v, want nil on rejection", t1)
		}
	})

	t.Run("recorded_anchor_with_no_review_receipt_is_self_asserted_never_verified", func(t *testing.T) {
		// Phase 13 (verify round 6, W2): before this fix, `approved_tree` was
		// synthesized from `landing_commit`'s own tree whenever no review ran,
		// which made the verification check compare a value against itself —
		// structurally unfailable, so a no-review anchor was reported as
		// "verified" by construction. This is the binding proof that the fix
		// holds: `approvedTree: ""` here means the archive-report recorded no
		// review receipt for this candidate (the real, current shape — an
		// empty string, never a value copied from landing_commit's own tree).
		//
		// t1 STILL resolves from be2c3ca's own committer timestamp — the
		// measurement is not withheld, only the outcome label changes.
		anchor := &d2RecordedAnchor{
			landingCommit: "be2c3ca46ea5f4f740ecab6b51dc2db3f167f37c",
			approvedTree:  "",
		}
		t1, outcome, err := resolveD2T1(repoRoot, anchor)
		if err != nil {
			t.Fatalf("resolveD2T1() error = %v", err)
		}
		if outcome != d2AnchorSelfAsserted {
			t.Fatalf("resolveD2T1() outcome = %q, want %q — a landing_commit recorded with no independent approved_tree has no authority to be verified against", outcome, d2AnchorSelfAsserted)
		}
		if outcome == d2AnchorVerified {
			t.Fatal("resolveD2T1() must never report a no-review anchor as verified — this is precisely the defect Phase 13 fixes (verify round 6, W2)")
		}
		if t1 == nil {
			t.Fatal("resolveD2T1() = nil t1 for a self-asserted anchor, want the landing commit's committer timestamp — self-asserted is a labelling distinction, not an omit path")
		}
		wantTS, err := time.Parse(time.RFC3339, "2026-08-05T10:10:53-03:00")
		if err != nil {
			t.Fatalf("parsing expected timestamp: %v", err)
		}
		if !t1.Equal(wantTS) {
			t.Fatalf("resolveD2T1() t1 = %v, want %v", t1, wantTS)
		}
		// Mutation-probe evidence (run manually against this exact test, not
		// shipped as a toggle): reverting resolveD2T1's self-asserted branch to
		// synthesize approvedTree from the landing commit's own tree before
		// comparing — the exact abolished behavior — makes this assertion fail
		// with outcome = d2AnchorVerified (RED); restoring the fix returns
		// outcome = d2AnchorSelfAsserted (GREEN). See apply-progress.md Phase
		// 13 section for the full RED/GREEN transcript.
	})

	t.Run("tree_carried_by_more_than_one_commit_is_ambiguous_never_resolved_by_position", func(t *testing.T) {
		// The real three-way collision design.md D2 revision 3 cites by name:
		// tree 2ad8e42e… is carried by be2c3ca (13:10:53Z, the true landing of
		// deterministic-verification-evidence), aa81361 (13:01:31Z, an
		// unrelated PR merge) and 459b48d (11:23:00Z, an unrelated feature
		// commit). Earliest-by-absolute-time would select 459b48d — a commit
		// belonging to a DIFFERENT change — which is exactly why "earliest
		// carrier" is rejected as "a guess wearing the costume of a rule"
		// (SKILL.md) rather than adopted as the disambiguation rule.
		const collidingTree = "2ad8e42e4f1ef4ff3e46c447d70e6123664f2180"

		// The ref is `origin/main`, not the bare name `main`. A CI checkout
		// creates refs/remotes/origin/* plus a local branch only for the ref it
		// checked out, and git does not resolve a bare name through
		// refs/remotes/origin/, so `main` fails to resolve on every branch that
		// is not main — measured in a faithful actions/checkout simulation, where
		// this subtest died on `fatal: ambiguous argument 'main'` even at full
		// fetch depth. `origin/main` resolves in both a CI checkout and a normal
		// clone, and selects the same commits.
		const defaultBranchRef = "origin/main"

		// Confirm the collision is real (counted, not merely asserted) before
		// trusting the production-shaped helper below to report it as ambiguous.
		// This checks the COUNT, not the parsing: both this and the helper share
		// scanCarriersFromGitLog, so a parsing bug would move both together. What
		// it does catch is the fixture going stale — history changing until the
		// tree no longer has exactly three carriers.
		carriers := commitsCarryingTree(t, repoRoot, defaultBranchRef, collidingTree)
		if len(carriers) != 3 {
			t.Fatalf("commits carrying tree %s on %s = %d, want 3 (be2c3ca, aa81361, 459b48d) — either the checkout lacks the history this fixture needs, or history changed and the fixture no longer matches the collision design.md cites", collidingTree, defaultBranchRef, len(carriers))
		}

		sha, ambiguous, err := resolveTreeToCommitOrAmbiguous(repoRoot, defaultBranchRef, collidingTree)
		if err != nil {
			t.Fatalf("resolveTreeToCommitOrAmbiguous() error = %v", err)
		}
		if !ambiguous {
			t.Fatalf("resolveTreeToCommitOrAmbiguous() ambiguous = false, sha = %q, want ambiguous = true — a tree carried by more than one commit must never be resolved by position", sha)
		}
		if sha != "" {
			t.Fatalf("resolveTreeToCommitOrAmbiguous() sha = %q, want empty on ambiguity — no carrier, earliest or otherwise, may be silently chosen", sha)
		}
	})

	t.Run("shipped_skill_prose_matches_the_proven_ambiguity_rule", func(t *testing.T) {
		// Structural binding (design.md D2 revision 3 / verify's C1 class of
		// defect: a prior remediation fixed SKILL.md's prose and left this
		// file's proof asserting the abolished opposite rule, and nothing
		// caught the drift because this file never read SKILL.md). The real
		// collision proven above and the shipped prose it must match are now
		// read and asserted together, in the same test file, so they cannot
		// silently drift apart again.
		skill := readRepoFile(t, repoRoot, inceptionPipelineSkillRelPath)
		for _, want := range []string{
			"**verify** a commit, never to **discover** one",
			"the anchor is ambiguous",
			"never resolved by position",
		} {
			if !strings.Contains(skill, want) {
				t.Fatalf("shipped closure-feedback prose must contain %q — it must match the ambiguity rule the real repository history above proves", want)
			}
		}
		for _, forbidden := range []string{
			"carrying that tree MUST be chosen",
			"earliest commit on the default branch carrying",
		} {
			if strings.Contains(skill, forbidden) {
				t.Fatalf("shipped closure-feedback prose must not contain %q — that is the abolished positional-resolution rule the real 3-way tree collision above disproves", forbidden)
			}
		}
	})

	t.Run("no_recorded_anchor_yields_no_t1_and_attempts_no_folder_scan", func(t *testing.T) {
		// A change predating the anchor convention (design.md D2 revision 3,
		// "No heuristic fallback"): no landing_commit was ever recorded, so
		// resolution must yield nothing — never re-derive one by scanning
		// which commits touched the change's own folder. resolveD2T1's
		// signature (repoRoot, *d2RecordedAnchor) cannot accept a change slug
		// or folder path at all, so passing nil proves the omit path
		// structurally, not merely by convention.
		t1, outcome, err := resolveD2T1(repoRoot, nil)
		if err != nil {
			t.Fatalf("resolveD2T1(nil) error = %v", err)
		}
		if outcome != d2AnchorAbsent {
			t.Fatalf("resolveD2T1(nil) outcome = %q, want %q — there is nothing to reject or verify, only nothing recorded", outcome, d2AnchorAbsent)
		}
		if t1 != nil {
			t.Fatalf("resolveD2T1(nil) t1 = %v, want nil (no recorded anchor -> no t1, never a folder-scan re-derivation)", t1)
		}
	})
}

// d2RecordedAnchor is the versioned-artifact anchor design.md D2 (revision 3)
// records at delivery: the landing commit SHA, always, together with the
// approved candidate tree hash — ONLY when a review receipt exists for the
// candidate (Phase 13 correction: `approvedTree` MUST be the empty string
// when no review ran, never synthesized from `landingCommit`'s own tree, or
// the verification check below would compare a value against itself and
// could never fail). Both values, when both are recorded, are already
// available from the approved receipt at delivery time; only these two
// plain-text values are ever persisted (in the archive-report and the
// actuals record), never the receipt file itself, which lives under the
// unversioned `.git/` common directory.
type d2RecordedAnchor struct {
	landingCommit string
	approvedTree  string
}

// d2AnchorOutcome names how a recorded anchor resolved — never conflating "no
// independent authority checked it" with "an independent authority checked
// it and it matched". See TestD2AnchorIsVerifiableAndUnambiguous's doc
// comment for the full rationale (Phase 13, verify round 6 W2).
type d2AnchorOutcome string

const (
	// d2AnchorAbsent: no landing_commit was ever recorded — a change
	// predating the anchor convention. No t1.
	d2AnchorAbsent d2AnchorOutcome = "absent"
	// d2AnchorSelfAsserted: landing_commit was recorded, but no approved_tree
	// exists (no review ran for this candidate). t1 STILL resolves from
	// landing_commit's committer timestamp — the measurement is not withheld
	// — but there is no independent receipt to check it against, so it MUST
	// NOT be reported as verified.
	d2AnchorSelfAsserted d2AnchorOutcome = "self-asserted"
	// d2AnchorVerified: the landing commit's own tree equals the recorded
	// approved_tree — independently checked against a review receipt.
	d2AnchorVerified d2AnchorOutcome = "verified"
	// d2AnchorRejected: the landing commit's own tree does NOT equal the
	// recorded approved_tree. t1 omitted, never trusted.
	d2AnchorRejected d2AnchorOutcome = "rejected"
)

// resolveD2T1 resolves t1 from an OPTIONAL recorded anchor. It takes no
// change slug or folder path — by construction it can never fall back to
// scanning which commits touched a change's own folder (design.md D2
// revision 3, "No heuristic fallback"; revision 2's path-scan is removed
// outright, not demoted).
//
//   - anchor == nil: no versioned anchor was ever recorded (a change
//     predating the convention). Returns (nil, d2AnchorAbsent, nil) —
//     nothing to reject, nothing to resolve.
//   - anchor.approvedTree == "": no review ran for this candidate, so there
//     is no independent authority to check the SHA against. t1 STILL
//     resolves from landing_commit's own committer timestamp — the
//     measurement is not withheld — but the outcome is self-asserted, never
//     verified: `approved_tree` MUST NOT be synthesized from
//     landing_commit's own tree here, or the comparison below would be
//     trivially true by construction (Phase 13 correction).
//   - anchor's landing commit's own tree != anchor.approvedTree: the anchor
//     is REJECTED, never trusted. Returns (nil, d2AnchorRejected, nil).
//   - anchor's landing commit's own tree == anchor.approvedTree: verified —
//     independently checked against a review receipt. Returns the landing
//     commit's committer timestamp.
func resolveD2T1(repoRoot string, anchor *d2RecordedAnchor) (t1 *time.Time, outcome d2AnchorOutcome, err error) {
	if anchor == nil {
		return nil, d2AnchorAbsent, nil
	}

	out, err := runGitReadOnly("-C", repoRoot, "show", "-s", "--format=%T %cI", anchor.landingCommit)
	if err != nil {
		return nil, "", fmt.Errorf("reading landing commit %s: %w", anchor.landingCommit, err)
	}
	fields := strings.Fields(strings.TrimSpace(out))
	if len(fields) != 2 {
		return nil, "", fmt.Errorf("unexpected `git show` output for %s: %q", anchor.landingCommit, out)
	}
	actualTree, committerDateRaw := fields[0], fields[1]

	if anchor.approvedTree == "" {
		// No review ran for this candidate: there is no independent
		// approved_tree to check actualTree against. t1 still resolves — the
		// measurement is not withheld — but it is self-asserted, not
		// verified. actualTree is deliberately never compared to anything
		// here: comparing it to itself (or to a value derived from it) would
		// be the exact tautology Phase 13 removes.
		ts, err := time.Parse(time.RFC3339, committerDateRaw)
		if err != nil {
			return nil, "", fmt.Errorf("parsing committer date %q for %s: %w", committerDateRaw, anchor.landingCommit, err)
		}
		return &ts, d2AnchorSelfAsserted, nil
	}

	if actualTree != anchor.approvedTree {
		// Verifiable, not merely asserted: the landing commit's OWN tree
		// disagrees with the recorded approved_tree, so the anchor is
		// rejected rather than trusted (design.md D2 revision 3).
		return nil, d2AnchorRejected, nil
	}

	ts, err := time.Parse(time.RFC3339, committerDateRaw)
	if err != nil {
		return nil, "", fmt.Errorf("parsing committer date %q for %s: %w", committerDateRaw, anchor.landingCommit, err)
	}
	return &ts, d2AnchorVerified, nil
}

// scanCarriersFromGitLog parses `git log --format=%H %T` output and returns every
// commit SHA whose own tree equals the given tree. It is the single parsing site
// for that output shape: commitsCarryingTree and resolveTreeToCommitOrAmbiguous
// both go through it, so the two can never disagree because one was updated and
// the other was not. Lines that do not carry exactly two fields are skipped
// rather than treated as an error, so an empty trailing line is harmless.
func scanCarriersFromGitLog(out, tree string) []string {
	var carriers []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) != 2 {
			continue
		}
		if fields[1] == tree {
			carriers = append(carriers, fields[0])
		}
	}
	return carriers
}

// commitsCarryingTree returns every commit SHA reachable from ref whose own tree
// equals the given tree. The test uses it to check the COLLISION COUNT its
// fixture assumes, so a history change that leaves the tree with a different
// number of carriers fails loudly instead of silently weakening the ambiguity
// case below. It deliberately does NOT claim to check the parsing independently:
// it shares scanCarriersFromGitLog with resolveTreeToCommitOrAmbiguous, and an
// earlier revision of this file duplicated that loop byte-for-byte while
// asserting the two were independent — which they never were.
func commitsCarryingTree(t *testing.T, repoRoot, ref, tree string) []string {
	t.Helper()
	out, err := runGitReadOnly("-C", repoRoot, "log", "--format=%H %T", ref)
	if err != nil {
		t.Fatalf("listing commits on %s: %v", ref, err)
	}
	return scanCarriersFromGitLog(out, tree)
}

// resolveTreeToCommitOrAmbiguous is the production-shaped helper for
// design.md D2 revision 3's disambiguation rule: a tree hash MUST be used to
// verify a commit, never to discover one. WHEN more than one commit reachable
// from ref carries the given tree, resolution is AMBIGUOUS — it returns
// ambiguous=true and no SHA. The caller MUST omit t1 and disclose the
// ambiguity; no carrier, earliest or otherwise, may ever be chosen by
// position. This function is used only for verification and for
// reconstructing historical records; the recorded landing_commit itself
// avoids this search entirely at write time.
func resolveTreeToCommitOrAmbiguous(repoRoot, ref, tree string) (sha string, ambiguous bool, err error) {
	out, err := runGitReadOnly("-C", repoRoot, "log", "--format=%H %T", ref)
	if err != nil {
		return "", false, fmt.Errorf("listing commits on %s: %w", ref, err)
	}

	carriers := scanCarriersFromGitLog(out, tree)
	switch len(carriers) {
	case 0:
		return "", false, fmt.Errorf("no commit reachable from %s carries tree %s", ref, tree)
	case 1:
		return carriers[0], false, nil
	default:
		return "", true, nil
	}
}

// runGitReadOnly executes git with the given arguments and returns stdout. It
// is the sole process-invocation point for this file's D2 anchor proof, so
// the read-only contract (design.md Threat Matrix: only `log`, `show -s`) has
// one place to audit. Test-only: production code in this package must never
// shell out to git (ADR-15, TestZeroFetchImportAllowlist).
func runGitReadOnly(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w (stderr: %s)", strings.Join(args, " "), err, stderr.String())
	}
	return stdout.String(), nil
}
