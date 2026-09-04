package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ConventionDate is the day the landing-commit anchor convention shipped
// (openspec/changes/archive/2026-08-19-sdd-cycle-timestamp-instrumentation).
// Reports archived before it predate the convention and are out of scope: a
// gate that retroactively invalidates history teaches nothing and gets waived.
const ConventionDate = "2026-08-19"

const cycleTimestampsHeading = "## Cycle Timestamps"

var (
	archiveDirDatePattern = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})-`)
	// A hash written as an identity claim: backticked, at least 7 hex chars,
	// optionally abbreviated with a trailing ellipsis. Bare hex in prose is
	// deliberately NOT matched — a decimal-looking word or a version string
	// must never be mistaken for a SHA.
	recordedHashPattern = regexp.MustCompile("`([0-9a-fA-F]{7,40})(?:…|\\.\\.\\.)?`")

	// The FIELD NAMES a record uses to say which git object a hash is. These
	// are what classifies a hash — see parseAnchorSection for why length
	// cannot. `commit`/`tree` are matched as whole words so "committer" and
	// "Tree-based" behave: the first is not a commit label, the second is a
	// tree label.
	landingCommitLabel = regexp.MustCompile(`(?i)\b(?:landing[_ ]commit|commit)\b`)
	approvedTreeLabel  = regexp.MustCompile(`(?i)\b(?:approved[_ ]tree|candidate[_ ]tree|final_candidate_tree|tree)\b`)

	// verifiedClaimPattern and rejectedClaimPattern match the outcome word as a
	// WHOLE word, so "unverified" is not an occurrence of "verified" at all.
	verifiedClaimPattern = regexp.MustCompile(`(?i)\bverified\b`)
	rejectedClaimPattern = regexp.MustCompile(`(?i)\brejected\b`)

	// negatedClaimPattern matches a negator immediately governing an outcome
	// word, allowing at most two intervening plain words ("not independently
	// verified"). The filler admits no punctuation on purpose: it stops the
	// pattern reaching across a clause boundary and mistaking an unrelated
	// "no" — as in "no findings; tree verified" — for a disclaimer.
	negatedClaimPattern = regexp.MustCompile(`(?i)\b(?:not|never|no|none|nothing|cannot|can't|couldn't|wasn't|isn't|without)\s+(?:[a-z']+\s+){0,2}(?:verified|rejected)\b`)
)

// absentDeclarations are the ways a report may state, in prose, that no landing
// commit exists to anchor on. Each asserts the fact positively; silence is not
// one of them, which is the whole point of the gate.
var absentDeclarations = []string{
	"predates the anchor convention",
	"no landing commit",
	"not yet merged",
	"never merged",
	"landing_commit is absent",
}

// Finding is one non-compliance, named by the report it was found in.
type Finding struct {
	Report string
	Reason string
}

func (f Finding) String() string { return f.Report + ": " + f.Reason }

// Report is one archive report the gate considered.
type Report struct {
	RelPath string
	Date    string
	Outcome AnchorOutcome
	Anchor  *RecordedAnchor
}

// ScanArchive walks every archived change's archive-report.md, keeps the ones
// dated on or after `since`, and checks each against the anchor contract.
//
// The contract, stated once so the failure messages can cite it:
//
//  1. The report carries a `## Cycle Timestamps` section.
//  2. If that section records a landing commit, the commit resolves in this
//     repository.
//  3. If it also records a tree hash, the landing commit's own tree matches it
//     (outcome `verified`); a mismatch is only legal when the section says the
//     anchor was `rejected`.
//  4. If it records a tree hash but the section claims no verification, or
//     records NO tree while claiming the anchor is `verified`, that is a
//     fabricated assurance and fails.
//  5. If it records no landing commit, the section states the absent outcome
//     explicitly.
func ScanArchive(repoRoot, since string) ([]Report, []Finding, error) {
	archiveRoot := filepath.Join(repoRoot, "openspec", "changes", "archive")
	entries, err := os.ReadDir(archiveRoot)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// A project that has archived nothing — or keeps its artifacts in
			// engram, or in no store at all — has no archive directory. There
			// are no records to judge, and no record is non-compliant. Every
			// non-zero exit from this gate is a documented hard stop, so
			// reporting absence as an I/O failure permanently blocked closure
			// for those projects.
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("reading %s: %w", archiveRoot, err)
	}
	sinceTime, err := time.Parse("2006-01-02", since)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing --since %q: %w", since, err)
	}

	var reports []Report
	var findings []Finding
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		match := archiveDirDatePattern.FindStringSubmatch(entry.Name())
		if match == nil {
			continue
		}
		archived, parseErr := time.Parse("2006-01-02", match[1])
		if parseErr != nil || archived.Before(sinceTime) {
			continue
		}
		relPath := filepath.ToSlash(filepath.Join("openspec", "changes", "archive", entry.Name(), "archive-report.md"))
		body, readErr := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(relPath)))
		if readErr != nil {
			findings = append(findings, Finding{relPath, fmt.Sprintf("archived on %s but carries no readable archive-report.md (%v)", match[1], readErr)})
			continue
		}
		report, reportFindings := checkReport(repoRoot, relPath, match[1], string(body))
		reports = append(reports, report)
		findings = append(findings, reportFindings...)
	}
	sort.Slice(reports, func(i, j int) bool { return reports[i].RelPath < reports[j].RelPath })
	sort.Slice(findings, func(i, j int) bool { return findings[i].Report < findings[j].Report })
	return reports, findings, nil
}

func checkReport(repoRoot, relPath, date, body string) (Report, []Finding) {
	report := Report{RelPath: relPath, Date: date, Outcome: AnchorAbsent}

	section, ok := cycleTimestampsSection(body)
	if !ok {
		return report, []Finding{{relPath, fmt.Sprintf(
			"archived %s, on or after the %s anchor convention, but carries no %q section. "+
				"The convention is not satisfied by a delivery note elsewhere in the report: a reader "+
				"looking for the anchor has one place to look, and this report does not answer there",
			date, ConventionDate, cycleTimestampsHeading)}}
	}

	commit, tree, unlabelled := parseAnchorSection(section)
	declaresAbsent := declaresAbsence(strings.ToLower(section))

	// ONE KNOB CANNOT SERVE BOTH DIRECTIONS, so an unnamed hash gets exactly
	// one, and only towards the landing commit.
	//
	// An unlabelled hash is indistinguishable from a review-receipt or content
	// digest, which reports quote routinely. It therefore NEVER fills the
	// approved_tree slot: a digest promoted to a tree disagrees with the
	// landing commit's real tree and hard-stops a truthful report with "a
	// mis-recorded anchor is rejected". Only a hash the report LABELS as a tree
	// is a tree.
	//
	// It may still fill the empty landing-commit slot, because the anchor
	// contract's primary field is the commit and a report that writes its
	// anchor bare has still recorded one — but only when it RESOLVES as a
	// commit. That resolvability test replaces the earlier absence-prose knob,
	// which read the whole section for a phrase and so let an unrelated "not
	// yet merged" — about a sub-PR, say — silence the landing commit the next
	// clause recorded. A hash that does not resolve was not a commit: the
	// report records none, and falls to the absence branch below, which is
	// where a quoted digest belongs.
	var spareHashes []string
	for _, hash := range unlabelled {
		if commit == "" && resolvesAsCommit(repoRoot, hash) {
			commit = hash
			continue
		}
		spareHashes = append(spareHashes, hash)
	}
	if commit == "" {
		report.Outcome = AnchorAbsent
		if declaresAbsent {
			return report, nil
		}
		if len(spareHashes) > 0 {
			// The section DOES quote a hash, so telling the author the record
			// is empty is false, and telling them the only exit is to declare
			// absence instructs them to write "no landing commit" into a
			// report that names one -- after which this gate passes it. A bare
			// unresolvable hash and a LABELLED unresolvable one are two
			// spellings of the same defect and get the same diagnosis; what
			// the bare spelling adds is a second, equally legitimate reading
			// (a receipt or content digest), so both exits are named and the
			// author picks the true one.
			return report, []Finding{{relPath, fmt.Sprintf(
				"quotes %s, which no field name calls a commit and which does not resolve here as a commit, "+
					"so the section records no landing commit the gate can use. A SHA nobody can resolve is an "+
					"assertion, not an anchor: if one of those hashes IS the landing commit, label it "+
					"`landing_commit` and record a SHA that resolves here. If they are receipt or content "+
					"digests and no landing commit exists, state that absent outcome (one of: %s) -- but only "+
					"if it is true",
				quotedHashList(spareHashes), strings.Join(absentDeclarations, "; "))}}
		}
		return report, []Finding{{relPath, fmt.Sprintf(
			"records no landing commit and states no absent outcome. An anchor that is missing must SAY it is "+
				"missing (one of: %s); an empty section is indistinguishable from an unwritten one",
			strings.Join(absentDeclarations, "; "))}}
	}

	anchor := &RecordedAnchor{LandingCommit: commit, ApprovedTree: tree}
	report.Anchor = anchor
	claimsRejected := claimsOutcome(section, rejectedClaimPattern)
	claimsVerified := claimsOutcome(section, verifiedClaimPattern)

	_, outcome, err := ResolveT1(repoRoot, anchor)
	if err != nil {
		return report, []Finding{{relPath, fmt.Sprintf(
			"records landing commit `%s`, which does not resolve here as a commit: %v. A SHA nobody can "+
				"resolve is an assertion, not an anchor", commit, err)}}
	}
	report.Outcome = outcome

	switch outcome {
	case AnchorVerified:
		return report, nil
	case AnchorRejected:
		if claimsRejected {
			return report, nil
		}
		return report, []Finding{{relPath, fmt.Sprintf(
			"records landing commit `%s` and tree `%s`, but `git show -s --format=%%T %s` disagrees. "+
				"A mis-recorded anchor is rejected, not trusted: omit t1 and disclose the mismatch",
			commit, tree, commit)}}
	case AnchorSelfAsserted:
		// A tree the gate cannot read is UNKNOWN, not absent, and accusing a
		// report of fabricating an assurance on evidence the gate declined to
		// read is not conservatism. But "some leftover hex was quoted" is not
		// that evidence: under that key ANY backticked hex word -- a digest, an
		// issue id, an unrelated abbreviation -- silenced the gate's only
		// anti-fabrication check. The suppression therefore needs a hash that
		// really is this landing commit's tree, which is the only shape under
		// which an unlabelled tree was plausibly recorded.
		if claimsVerified && !spareHashIsTheRealTree(repoRoot, commit, spareHashes) {
			return report, []Finding{{relPath, fmt.Sprintf(
				"records landing commit `%s` with no approved_tree, yet the section claims the anchor is "+
					"\"verified\". With no independent tree to check against there is nothing that could have "+
					"failed, and an unfailable check reported as verification is a fabricated assurance: record "+
					"it as self-asserted", commit)}}
		}
		return report, nil
	default:
		return report, []Finding{{relPath, fmt.Sprintf("unclassifiable anchor outcome %q", outcome)}}
	}
}

// cycleTimestampsSection returns the body of the Cycle Timestamps section, up
// to the next `##`-or-shallower heading.
func cycleTimestampsSection(body string) (string, bool) {
	lines := strings.Split(body, "\n")
	start := -1
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), cycleTimestampsHeading) {
			start = i + 1
			break
		}
	}
	if start == -1 {
		return "", false
	}
	for i := start; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "## ") || strings.HasPrefix(trimmed, "# ") {
			return strings.Join(lines[start:i], "\n"), true
		}
	}
	return strings.Join(lines[start:], "\n"), true
}

// claimsOutcome reports whether the section ASSERTS the outcome word, rather
// than merely containing its letters.
//
// A substring probe for "verified" also fires on "unverified" and on "not
// verified", so a report that correctly labelled its anchor self-asserted, and
// said so by disclaiming verification, was failed for claiming the assurance it
// had just disclaimed. The sibling probe for "rejected" fails the other way: it
// let "the anchor was not rejected" satisfy the disclosure a genuinely rejected
// anchor owes. One rule fixes both — a negated occurrence is not a claim.
func claimsOutcome(section string, word *regexp.Regexp) bool {
	negated := negatedClaimPattern.FindAllStringIndex(section, -1)
	for _, occurrence := range word.FindAllStringIndex(section, -1) {
		claimed := true
		for _, span := range negated {
			if occurrence[0] >= span[0] && occurrence[1] <= span[1] {
				claimed = false
				break
			}
		}
		if claimed {
			return true
		}
	}
	return false
}

// parseAnchorSection extracts the landing commit and the approved tree from the
// section, telling them apart by the FIELD NAME that governs each hash — the
// nearest `landing_commit`/`commit` or `approved_tree`/`tree` label written
// before it on its own line.
//
// Length classifies nothing, and using it was a real defect: records write both
// objects at both sizes. A 40-character landing commit is this repository's own
// habit, and a report that recorded one had it read as a tree, leaving the
// commit slot empty — so a truthful, fully compliant report failed the blocking
// CI job with "records no landing commit", the opposite of its defect, while
// the tree-mismatch and fabricated-verification branches were bypassed
// entirely. The mirror twin, an abbreviated `approved_tree`, was read as a
// commit for exactly the same reason.
//
// Hashes no field name governs are RETURNED separately, in reading order, and
// never promoted here: the caller decides. An unnamed hash is a candidate for
// the landing-commit slot ONLY, and an unnamed hash is not made a tree by being
// long — see checkReport for why the tree slot takes labelled hashes alone.
func parseAnchorSection(section string) (commit, tree string, unlabelled []string) {
	for _, line := range strings.Split(section, "\n") {
		for _, match := range recordedHashPattern.FindAllStringSubmatchIndex(line, -1) {
			hash := strings.ToLower(line[match[2]:match[3]])
			switch governingLabel(line[:match[0]]) {
			case labelCommit:
				if commit == "" {
					commit = hash
				}
			case labelTree:
				if tree == "" {
					tree = hash
				}
			default:
				unlabelled = append(unlabelled, hash)
			}
		}
	}
	return commit, tree, unlabelled
}

// spareHashIsTheRealTree reports whether any hash the report left unlabelled is
// in fact the landing commit's own tree. That -- and only that -- is evidence
// that a tree WAS recorded without a field name, which is what may excuse a
// report from the fabricated-assurance finding. An unrelated hex token is not
// evidence of anything, and must not buy silence.
func spareHashIsTheRealTree(repoRoot, commit string, spareHashes []string) bool {
	if len(spareHashes) == 0 {
		return false
	}
	out, err := runGitReadOnly("-C", repoRoot, "show", "-s", "--format=%T", commit)
	if err != nil {
		return false
	}
	actualTree := strings.TrimSpace(out)
	for _, hash := range spareHashes {
		if treeMatches(actualTree, hash) {
			return true
		}
	}
	return false
}

// quotedHashList renders hashes back the way a report writes them, so a finding
// names the exact tokens the author must look at.
func quotedHashList(hashes []string) string {
	quoted := make([]string, 0, len(hashes))
	for _, hash := range hashes {
		quoted = append(quoted, "`"+hash+"`")
	}
	return strings.Join(quoted, ", ")
}

// resolvesAsCommit reports whether a hash the report never named resolves, in
// this repository, as a commit. It is the only evidence available that an
// unnamed hash was meant as the landing commit; a digest, a content hash, or a
// hash from another repository fails it, and is then left where it was written
// instead of being hard-stopped as an unresolvable anchor.
func resolvesAsCommit(repoRoot, hash string) bool {
	out, err := runGitReadOnly("-C", repoRoot, "rev-parse", "--verify", "--quiet", hash+"^{commit}")
	return err == nil && strings.TrimSpace(out) != ""
}

// declaresAbsence reports whether the (lowercased) section states positively
// that no landing commit exists.
func declaresAbsence(loweredSection string) bool {
	for _, declaration := range absentDeclarations {
		if strings.Contains(loweredSection, declaration) {
			return true
		}
	}
	return false
}

// anchorLabel names which anchor field a hash belongs to.
type anchorLabel int

const (
	labelNone anchorLabel = iota
	labelCommit
	labelTree
)

// governingLabel returns the field name that governs the hash following the
// given line prefix: the LAST commit-or-tree label written before it, since a
// single table row can name both ("commit `a0374e3` ... verified against
// `final_candidate_tree = 5ab1490…`").
func governingLabel(prefix string) anchorLabel {
	commitAt := lastMatchIndex(landingCommitLabel, prefix)
	treeAt := lastMatchIndex(approvedTreeLabel, prefix)
	switch {
	case commitAt < 0 && treeAt < 0:
		return labelNone
	case treeAt > commitAt:
		return labelTree
	default:
		return labelCommit
	}
}

// lastMatchIndex returns the start offset of the last match of pattern in text,
// or -1 when it never matches.
func lastMatchIndex(pattern *regexp.Regexp, text string) int {
	matches := pattern.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return -1
	}
	return matches[len(matches)-1][0]
}
