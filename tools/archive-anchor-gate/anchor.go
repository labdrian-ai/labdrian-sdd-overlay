package main

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// AnchorOutcome is the four-way resolution vocabulary the closure contract
// requires a record to name, so no reader can mistake a self-asserted anchor
// (used, but never independently checked) for a verified one.
type AnchorOutcome string

const (
	// AnchorAbsent: no landing_commit was ever recorded — a change predating
	// the anchor convention, or one that has not landed. No t1.
	AnchorAbsent AnchorOutcome = "absent"
	// AnchorSelfAsserted: landing_commit was recorded, but no approved_tree
	// exists (no review ran for this candidate). t1 STILL resolves from the
	// landing commit's committer timestamp — the measurement is not withheld —
	// but there is no independent receipt to check it against, so it MUST NOT
	// be reported as verified.
	AnchorSelfAsserted AnchorOutcome = "self-asserted"
	// AnchorVerified: the landing commit's own tree equals the recorded
	// approved_tree — independently checked against a review receipt.
	AnchorVerified AnchorOutcome = "verified"
	// AnchorRejected: the landing commit's own tree does NOT equal the
	// recorded approved_tree. t1 omitted, never trusted.
	AnchorRejected AnchorOutcome = "rejected"
)

// RecordedAnchor is the versioned anchor a change records at delivery.
// approvedTree is empty when no review ran for the candidate; it is NEVER
// synthesised from the landing commit's own tree, because a check comparing a
// value against itself can never fail, and an unfailable check reported as
// verification is a fabricated assurance.
type RecordedAnchor struct {
	LandingCommit string
	ApprovedTree  string
}

// ResolveT1 resolves t1 from an OPTIONAL recorded anchor.
//
// This is the production home of the resolution rule. It previously existed
// only inside engine/skills/actuals_instrumentation_d2_anchor_test.go with zero
// non-test callers, which meant the rule the closure contract mandates was
// implemented nowhere an executor could invoke it: the behaviour was tested,
// and then performed by hand.
//
// It takes no change slug and no folder path. By construction it therefore
// cannot fall back to scanning which commits touched a change's own folder —
// a scan that lands on an unrelated PR's merge commit is not hypothetical in
// this repository's history.
//
//   - anchor == nil: nothing was ever recorded. (nil, AnchorAbsent, nil).
//   - anchor.ApprovedTree == "": no independent authority exists. t1 resolves
//     from the landing commit's committer timestamp, outcome AnchorSelfAsserted.
//   - tree mismatch: (nil, AnchorRejected, nil) — never trusted.
//   - tree match: the landing commit's committer timestamp, AnchorVerified.
//
// A tree hash is used to VERIFY a commit, never to DISCOVER one: trees are not
// unique across commits, so searching for "the commit carrying this tree"
// returns commits belonging to other changes.
func ResolveT1(repoRoot string, anchor *RecordedAnchor) (*time.Time, AnchorOutcome, error) {
	if anchor == nil {
		return nil, AnchorAbsent, nil
	}
	if strings.TrimSpace(anchor.LandingCommit) == "" {
		return nil, AnchorAbsent, nil
	}

	out, err := runGitReadOnly("-C", repoRoot, "show", "-s", "--format=%T %cI", anchor.LandingCommit)
	if err != nil {
		return nil, "", fmt.Errorf("reading landing commit %s: %w", anchor.LandingCommit, err)
	}
	fields := strings.Fields(strings.TrimSpace(out))
	if len(fields) != 2 {
		return nil, "", fmt.Errorf("unexpected `git show` output for %s: %q", anchor.LandingCommit, out)
	}
	actualTree, committerDateRaw := fields[0], fields[1]

	if anchor.ApprovedTree == "" {
		// actualTree is deliberately compared to nothing here. Comparing it to
		// itself, or to a value derived from it, is the exact tautology this
		// contract removes.
		ts, parseErr := time.Parse(time.RFC3339, committerDateRaw)
		if parseErr != nil {
			return nil, "", fmt.Errorf("parsing committer date %q for %s: %w", committerDateRaw, anchor.LandingCommit, parseErr)
		}
		return &ts, AnchorSelfAsserted, nil
	}

	if !treeMatches(actualTree, anchor.ApprovedTree) {
		return nil, AnchorRejected, nil
	}

	ts, err := time.Parse(time.RFC3339, committerDateRaw)
	if err != nil {
		return nil, "", fmt.Errorf("parsing committer date %q for %s: %w", committerDateRaw, anchor.LandingCommit, err)
	}
	return &ts, AnchorVerified, nil
}

// minRecordedHashPrefix is the shortest abbreviation this gate will treat as an
// identity claim. Shorter than this a hash is not distinguishing anything, and
// accepting it would let a near-miss pass as a match.
const minRecordedHashPrefix = 7

// treeMatches compares a recorded tree against the landing commit's actual
// tree. Records abbreviate hashes in prose, so the recorded value is matched as
// a PREFIX of the actual one — a full hash still compares in full, and an
// abbreviation still fails when it disagrees anywhere it is written.
func treeMatches(actualTree, recordedTree string) bool {
	recorded := strings.ToLower(strings.TrimSpace(recordedTree))
	if len(recorded) < minRecordedHashPrefix {
		return false
	}
	return strings.HasPrefix(strings.ToLower(actualTree), recorded)
}

func runGitReadOnly(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}
