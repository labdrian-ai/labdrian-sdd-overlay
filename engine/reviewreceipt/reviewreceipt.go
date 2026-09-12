// Package reviewreceipt persists a review transaction's receipt before the
// acknowledge-approved invocation burns it, so a change's approved_tree is
// always sourced from a verified receipt -- never self-asserted. This closes
// the bug that archived three prior changes with unverified anchors (R-008).
//
// Capture resolves the review-transaction store from BOTH
// `git rev-parse --git-dir` (worktree-private, where the active review
// transaction for a linked worktree lives) and `--git-common-dir` (the
// shared store), scans each for lineage directories carrying an approved
// artifact, and copies each byte-for-byte into
// openspec/changes/<change>/review-receipts/. Two on-disk shapes are
// recognized: the legacy "review-receipt.json" (gentle-ai < 2.7.0),
// persisted as <lineage_id>.json, and the 2.7.0+ lifecycle
// "review-state.json" (an approved-but-unacknowledged lineage directory
// contains ONLY this file), persisted as <lineage_id>.review-state.json.
// Both are captured when both are present.
//
// Single-active-change rule: a review receipt names no change of its own, so
// DetectActiveChange resolves which change a captured receipt belongs to by
// looking at openspec/changes/ -- any non-archive directory that carries at
// least one recognized SDD artifact file is "active". Exactly one active
// change resolves automatically; zero is a pass-through (nothing to attach
// the receipt to); more than one is reported as *MultipleActiveChangesError
// so a caller fails closed instead of guessing which change owns the
// receipt.
//
// Fail-closed: Capture never overwrites an existing receipt file with
// different content -- that is an error, never a silent overwrite. The
// fail-closed PreToolUse Bash hook that calls Capture before
// acknowledge-approved runs (hook.go) denies (exit 2) on any capture error
// or on more than one active change, rather than letting the acknowledgement
// proceed without a persisted receipt.
package reviewreceipt

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// receiptSchema is the only schema Capture accepts.
const receiptSchema = "gentle-ai.review-receipt/v2"

// approvedState is the only terminal_state Capture persists. A surviving
// receipt in the transaction store that is NOT approved (or not yet
// terminal) is never captured -- only an approved receipt is by definition
// un-acknowledged and worth preserving before the burn.
const approvedState = "approved"

// receiptFileName is the legacy receipt file (gentle-ai < 2.7.0) Capture
// looks for inside each lineage directory.
const receiptFileName = "review-receipt.json"

// stateFileName is the lifecycle state file gentle-ai 2.7.0+ writes
// instead of receiptFileName.
const stateFileName = "review-state.json"

// stateSuffix distinguishes a captured review-state.json from a captured
// legacy receipt sharing the same lineage id.
const stateSuffix = ".review-state.json"

// storeRelPath is the transaction store location relative to a git-dir.
var storeRelPath = filepath.Join("gentle-ai", "review-transactions", "v2")

// activeChangeMarkers are the SDD artifact files whose presence marks a
// directory under openspec/changes/ as a genuine active change, distinct
// from unrelated stray directories. state.yaml is deliberately NOT the sole
// marker: this repository's own openspec/changes/ directories never carry a
// state.yaml (the orchestrator's state persistence is optional and, for
// hybrid/engram-store changes, state lives in Engram instead), so requiring
// it would make active-change detection permanently empty.
var activeChangeMarkers = []string{"tasks.md", "design.md", "proposal.md", "entry.json"}

// Captured records one receipt Capture persisted (or confirmed already
// persisted byte-for-byte) this run.
type Captured struct {
	LineageID string
	Path      string
}

// receipt is the subset of gentle-ai.review-receipt/v2 fields Capture and
// ApprovedSummary read.
type receipt struct {
	Schema             string   `json:"schema"`
	LineageID          string   `json:"lineage_id"`
	TerminalState      string   `json:"terminal_state"`
	FinalCandidateTree string   `json:"final_candidate_tree"`
	BaseTree           string   `json:"base_tree"`
	SelectedLenses     []string `json:"selected_lenses"`
	RiskLevel          string   `json:"risk_level"`
}

// reviewState is the subset of review-state.json's nested "state" object
// that Capture and ApprovedSummary read.
type reviewState struct {
	State struct {
		LineageID       string   `json:"lineage_id"`
		State           string   `json:"state"`
		RiskLevel       string   `json:"risk_level"`
		SelectedLenses  []string `json:"selected_lenses"`
		InitialSnapshot struct {
			BaseTree string `json:"base_tree"`
		} `json:"initial_snapshot"`
		CurrentSnapshot struct {
			CandidateTree string `json:"candidate_tree"`
		} `json:"current_snapshot"`
	} `json:"state"`
}

// Capture scans both the worktree-private and common git transaction stores
// for approved review receipts and persists each into
// openspec/changes/<change>/review-receipts/<lineage_id>.json. It returns
// the receipts captured (or already present byte-for-byte) this run.
func Capture(repoRoot, change string) ([]Captured, error) {
	if strings.TrimSpace(change) == "" {
		return nil, errors.New("reviewreceipt: change name is required")
	}

	stores, err := transactionStores(repoRoot)
	if err != nil {
		return nil, err
	}

	targetDir := filepath.Join(repoRoot, "openspec", "changes", change, "review-receipts")

	var captured []Captured
	seenReceipt := map[string]bool{}
	seenState := map[string]bool{}
	for _, store := range stores {
		entries, err := os.ReadDir(store)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return captured, fmt.Errorf("reviewreceipt: read %s: %w", store, err)
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			lineageDir := filepath.Join(store, e.Name())

			if data, err := os.ReadFile(filepath.Join(lineageDir, receiptFileName)); err == nil {
				var r receipt
				if json.Unmarshal(data, &r) == nil && r.Schema == receiptSchema &&
					r.TerminalState == approvedState && r.LineageID != "" && !seenReceipt[r.LineageID] {
					seenReceipt[r.LineageID] = true
					dest := filepath.Join(targetDir, r.LineageID+".json")
					if err := writeReceiptFile(targetDir, dest, data); err != nil {
						return captured, err
					}
					captured = append(captured, Captured{LineageID: r.LineageID, Path: dest})
				}
			}
			if data, err := os.ReadFile(filepath.Join(lineageDir, stateFileName)); err == nil {
				var s reviewState
				if json.Unmarshal(data, &s) == nil && s.State.State == approvedState &&
					s.State.LineageID != "" && !seenState[s.State.LineageID] {
					seenState[s.State.LineageID] = true
					dest := filepath.Join(targetDir, s.State.LineageID+stateSuffix)
					if err := writeReceiptFile(targetDir, dest, data); err != nil {
						return captured, err
					}
					captured = append(captured, Captured{LineageID: s.State.LineageID, Path: dest})
				}
			}
		}
	}
	return captured, nil
}

// ApprovedSummary reads an approved review artifact at path -- either
// shape Capture persists -- and returns the tuple a caller needs to
// verify an approved_tree anchor. Errors if path is unreadable, unparsable
// as either shape, or not approved.
func ApprovedSummary(path string) (lineage, finalCandidateTree, baseTree string, lenses []string, riskLevel string, err error) {
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		return "", "", "", nil, "", fmt.Errorf("reviewreceipt: read %s: %w", path, readErr)
	}

	var r receipt
	if json.Unmarshal(data, &r) == nil && r.Schema == receiptSchema {
		if r.TerminalState != approvedState {
			return "", "", "", nil, "", fmt.Errorf("reviewreceipt: %s is not approved (terminal_state=%q)", path, r.TerminalState)
		}
		return r.LineageID, r.FinalCandidateTree, r.BaseTree, r.SelectedLenses, r.RiskLevel, nil
	}

	var s reviewState
	if err := json.Unmarshal(data, &s); err != nil {
		return "", "", "", nil, "", fmt.Errorf("reviewreceipt: %s is neither a recognized receipt nor review-state file: %w", path, err)
	}
	if s.State.LineageID == "" {
		return "", "", "", nil, "", fmt.Errorf("reviewreceipt: %s is neither a recognized receipt nor review-state file", path)
	}
	if s.State.State != approvedState {
		return "", "", "", nil, "", fmt.Errorf("reviewreceipt: %s is not approved (state=%q)", path, s.State.State)
	}
	return s.State.LineageID, s.State.CurrentSnapshot.CandidateTree, s.State.InitialSnapshot.BaseTree, s.State.SelectedLenses, s.State.RiskLevel, nil
}

// writeReceiptFile persists data at dest byte-for-byte, atomically (temp
// file + rename). It is idempotent -- identical existing content is a
// no-op -- but refuses to overwrite an existing file with different content.
func writeReceiptFile(targetDir, dest string, data []byte) error {
	if existing, err := os.ReadFile(dest); err == nil {
		if bytes.Equal(existing, data) {
			return nil
		}
		return fmt.Errorf("reviewreceipt: %s already exists with different content", dest)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("reviewreceipt: read %s: %w", dest, err)
	}

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("reviewreceipt: create %s: %w", targetDir, err)
	}

	tmp, err := os.CreateTemp(targetDir, ".review-receipt-*.json.tmp")
	if err != nil {
		return fmt.Errorf("reviewreceipt: create temp in %s: %w", targetDir, err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("reviewreceipt: write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("reviewreceipt: close temp: %w", err)
	}
	if err := os.Rename(tmpPath, dest); err != nil {
		return fmt.Errorf("reviewreceipt: rename to %s: %w", dest, err)
	}
	return nil
}

// transactionStores returns the deduplicated absolute review-transaction
// store directories for repoRoot: the worktree-private store
// (`git rev-parse --git-dir`) and the shared store
// (`git rev-parse --git-common-dir`). A plain repository (no worktrees)
// resolves both to the same directory; a linked worktree resolves them to
// different directories, and Capture must scan both because an active
// review transaction for the current worktree lives under the
// worktree-private git-dir.
func transactionStores(repoRoot string) ([]string, error) {
	gitDir, err := resolveGitPath(repoRoot, "--git-dir")
	if err != nil {
		return nil, err
	}
	commonDir, err := resolveGitPath(repoRoot, "--git-common-dir")
	if err != nil {
		return nil, err
	}

	stores := []string{filepath.Join(gitDir, storeRelPath)}
	if commonDir != gitDir {
		stores = append(stores, filepath.Join(commonDir, storeRelPath))
	}
	return stores, nil
}

// resolveGitPath runs `git -C repoRoot rev-parse <flag>` and resolves a
// relative result against repoRoot -- git prints a path relative to the
// caller's cwd for a non-bare, non-worktree repo.
func resolveGitPath(repoRoot, flag string) (string, error) {
	out, err := exec.Command("git", "-C", repoRoot, "rev-parse", flag).Output()
	if err != nil {
		return "", fmt.Errorf("reviewreceipt: git rev-parse %s: %w", flag, err)
	}
	p := strings.TrimSpace(string(out))
	if !filepath.IsAbs(p) {
		p = filepath.Join(repoRoot, p)
	}
	return p, nil
}

// DetectActiveChange returns the single non-archive directory under
// openspec/changes/ that carries at least one recognized SDD artifact file
// (see activeChangeMarkers). It returns ("", nil) when there is no
// openspec/changes/ directory or no active change -- callers treat that as
// pass-through, not an error. More than one active change is reported as
// *MultipleActiveChangesError so callers can fail closed instead of
// guessing which change a captured receipt belongs to.
func DetectActiveChange(repoRoot string) (string, error) {
	changesDir := filepath.Join(repoRoot, "openspec", "changes")
	entries, err := os.ReadDir(changesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("reviewreceipt: read %s: %w", changesDir, err)
	}

	var active []string
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "archive" {
			continue
		}
		if hasActiveChangeMarker(filepath.Join(changesDir, e.Name())) {
			active = append(active, e.Name())
		}
	}

	switch len(active) {
	case 0:
		return "", nil
	case 1:
		return active[0], nil
	default:
		sort.Strings(active)
		return "", &MultipleActiveChangesError{Changes: active}
	}
}

// hasActiveChangeMarker reports whether dir contains at least one file from
// activeChangeMarkers.
func hasActiveChangeMarker(dir string) bool {
	for _, marker := range activeChangeMarkers {
		if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
			return true
		}
	}
	return false
}

// MultipleActiveChangesError reports that more than one active change
// exists under openspec/changes/, so the caller cannot determine which
// change a captured receipt belongs to without explicit direction.
type MultipleActiveChangesError struct {
	Changes []string
}

func (e *MultipleActiveChangesError) Error() string {
	return fmt.Sprintf(
		"reviewreceipt: multiple active changes (%s); run `review-receipt capture --change <name>` before acknowledging",
		strings.Join(e.Changes, ", "),
	)
}
