package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ReceiptConventionDate is the day the review-receipt-capture convention
// shipped (openspec/changes/pi-package-hardening, receipt-anchor-gate
// slice). Reports archived on or after it may no longer source approved_tree
// from prose alone: a persisted gentle-ai.review-receipt/v2 file is the sole
// verified source, closing the gap that let three prior archives assert a
// tree hash nothing independently checked.
const ReceiptConventionDate = "2026-09-12"

const (
	receiptSchemaName  = "gentle-ai.review-receipt/v2"
	receiptApprovedTag = "approved"
	overrideSchemaName = "labdrian.review-receipt-override/v1"
	overrideFileName   = "override.json"
	// reviewStateSuffix names the second persisted shape (gentle-ai 2.7.0+):
	// the raw review-state.json the lifecycle now leaves behind for an
	// approved-but-unacknowledged lineage, copied byte-for-byte to
	// <lineage>.review-state.json. It never carries a `schema`/
	// `terminal_state` pair -- see persistedReviewState.
	reviewStateSuffix = ".review-state.json"
)

// persistedReceipt is the LEGACY gentle-ai.review-receipt/v2 shape the gate
// reads from openspec/changes/<change>/review-receipts/<lineage>.json.
type persistedReceipt struct {
	Schema             string `json:"schema"`
	LineageID          string `json:"lineage_id"`
	TerminalState      string `json:"terminal_state"`
	FinalCandidateTree string `json:"final_candidate_tree"`
}

// persistedReviewState is the SECOND persisted shape: a raw
// review-state.json, copied verbatim to
// openspec/changes/<change>/review-receipts/<lineage>.review-state.json.
// As of gentle-ai 2.7.0 the lifecycle no longer writes a
// gentle-ai.review-receipt/v2 file at all; an approved-but-unacknowledged
// lineage holds only this shape, so the gate must read it directly rather
// than wait for a receipt file that will never exist.
type persistedReviewState struct {
	State           string   `json:"state"`
	LineageID       string   `json:"lineage_id"`
	SelectedLenses  []string `json:"selected_lenses"`
	RiskLevel       string   `json:"risk_level"`
	CurrentSnapshot struct {
		CandidateTree string `json:"candidate_tree"`
	} `json:"current_snapshot"`
	InitialSnapshot struct {
		BaseTree string `json:"base_tree"`
	} `json:"initial_snapshot"`
}

// receiptOverride is the labdrian.review-receipt-override/v1 payload an
// owner records at openspec/changes/<change>/review-receipts/override.json
// to unblock archive when no receipt was captured.
type receiptOverride struct {
	Schema     string `json:"schema"`
	Owner      string `json:"owner"`
	Reason     string `json:"reason"`
	RecordedAt string `json:"recorded_at"`
}

// requiresReceipt reports whether a report archived on `date` (YYYY-MM-DD)
// falls under the receipt convention and must source approved_tree from a
// persisted receipt rather than prose.
func requiresReceipt(date string) bool {
	return date >= ReceiptConventionDate
}

// loadApprovedTreeFromReceipts scans dir -- a change's review-receipts
// folder -- for an approved persisted receipt, in EITHER shape gentle-ai has
// written across versions, and returns the candidate tree of the one to
// trust. override.json is never a candidate here; it is read separately by
// loadReceiptOverride.
//
//   - legacy `<lineage>.json`: gentle-ai.review-receipt/v2, approved when
//     `terminal_state == "approved"`; the tree is `final_candidate_tree`.
//   - `<lineage>.review-state.json`: the raw review-state gentle-ai 2.7.0+
//     leaves behind for an approved-but-unacknowledged lineage (the
//     lifecycle no longer writes a v2 receipt at all), approved when
//     `state == "approved"`; the tree is `current_snapshot.candidate_tree`.
//
// Neither shape carries a timestamp of its own (see engine/reviewreceipt),
// so when more than one approved file is persisted for the same change the
// gate cannot order them by recency. It deterministically picks the one
// whose filename (the lineage id) sorts LAST across BOTH shapes together,
// and documents the tie-break here rather than leaving it to directory
// iteration order -- a single persisted file, the expected shape, is
// unaffected by this choice.
func loadApprovedTreeFromReceipts(dir string) (tree, lineageID string, found bool, err error) {
	entries, readErr := os.ReadDir(dir)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			return "", "", false, nil
		}
		return "", "", false, fmt.Errorf("reading %s: %w", dir, readErr)
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() || e.Name() == overrideFileName || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	for i := len(names) - 1; i >= 0; i-- {
		path := filepath.Join(dir, names[i])
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return "", "", false, fmt.Errorf("reading %s: %w", path, readErr)
		}
		if strings.HasSuffix(names[i], reviewStateSuffix) {
			if candidateTree, lineage, ok := approvedTreeFromReviewState(data); ok {
				return candidateTree, lineage, true, nil
			}
			continue
		}
		if candidateTree, lineage, ok := approvedTreeFromLegacyReceipt(data); ok {
			return candidateTree, lineage, true, nil
		}
	}
	return "", "", false, nil
}

// approvedTreeFromLegacyReceipt extracts the approved candidate tree from a
// legacy gentle-ai.review-receipt/v2 payload, or reports false when the
// payload does not parse or is not an approved receipt.
func approvedTreeFromLegacyReceipt(data []byte) (tree, lineageID string, ok bool) {
	var r persistedReceipt
	if err := json.Unmarshal(data, &r); err != nil {
		return "", "", false
	}
	if r.Schema != receiptSchemaName || r.TerminalState != receiptApprovedTag || strings.TrimSpace(r.FinalCandidateTree) == "" {
		return "", "", false
	}
	return r.FinalCandidateTree, r.LineageID, true
}

// approvedTreeFromReviewState extracts the approved candidate tree from a
// raw review-state.json payload, or reports false when the payload does not
// parse or is not in the approved state.
func approvedTreeFromReviewState(data []byte) (tree, lineageID string, ok bool) {
	var s persistedReviewState
	if err := json.Unmarshal(data, &s); err != nil {
		return "", "", false
	}
	if s.State != receiptApprovedTag || strings.TrimSpace(s.CurrentSnapshot.CandidateTree) == "" {
		return "", "", false
	}
	return s.CurrentSnapshot.CandidateTree, s.LineageID, true
}

// loadReceiptOverride reads dir/override.json and validates it as a
// labdrian.review-receipt-override/v1 payload naming an owner, a reason, and
// when it was recorded. A file that exists but fails validation is reported
// as an error, never silently treated as absent -- an owner who wrote a
// malformed override intended to unblock archive, and a silent miss would
// instead surface the unrelated "no verified receipt" finding.
func loadReceiptOverride(dir string) (receiptOverride, bool, error) {
	path := filepath.Join(dir, overrideFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return receiptOverride{}, false, nil
		}
		return receiptOverride{}, false, fmt.Errorf("reading %s: %w", path, err)
	}
	var o receiptOverride
	if err := json.Unmarshal(data, &o); err != nil {
		return receiptOverride{}, false, fmt.Errorf("parsing %s: %w", path, err)
	}
	if o.Schema != overrideSchemaName || strings.TrimSpace(o.Owner) == "" || strings.TrimSpace(o.Reason) == "" || strings.TrimSpace(o.RecordedAt) == "" {
		return receiptOverride{}, false, fmt.Errorf("%s is missing a required field (schema/owner/reason/recorded_at)", path)
	}
	return o, true, nil
}

// receiptsDirFor returns the review-receipts folder that sits alongside the
// given archive-report.md's relative path.
func receiptsDirFor(repoRoot, reportRelPath string) string {
	return filepath.Join(repoRoot, filepath.Dir(filepath.FromSlash(reportRelPath)), "review-receipts")
}

// CheckPreArchive runs the receipt-requirement check pre-archive, on the
// live openspec/changes/<change>/ folder (task 4.3 / R-011's `--change`
// exit). It never touches Cycle Timestamps prose -- there is no
// archive-report yet -- so an override file alone is sufficient here; the
// prose-disclosure requirement is enforced later by ScanArchive once the
// change is actually archived.
func CheckPreArchive(repoRoot, change string) (ok bool, message string, err error) {
	dir := filepath.Join(repoRoot, "openspec", "changes", change, "review-receipts")

	tree, _, found, err := loadApprovedTreeFromReceipts(dir)
	if err != nil {
		return false, "", err
	}
	if found {
		return true, fmt.Sprintf("verified review receipt found for %q (approved_tree=%s)", change, tree), nil
	}

	override, hasOverride, err := loadReceiptOverride(dir)
	if err != nil {
		return false, "", err
	}
	if hasOverride {
		return true, fmt.Sprintf(
			"no persisted review receipt for %q, but an owner override is recorded (owner=%s): archive may proceed self-asserted",
			change, override.Owner), nil
	}

	return false, fmt.Sprintf(
		"no verified review receipt for change %q: capture the review receipt before archiving "+
			"(openspec/changes/%s/review-receipts/<lineage>.json or <lineage>.review-state.json), or record an "+
			"explicit owner override at openspec/changes/%s/review-receipts/override.json",
		change, change, change), nil
}
