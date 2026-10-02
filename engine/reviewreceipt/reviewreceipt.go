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
// Both are captured when both are present. Parse reads the approved review a
// document of either shape holds (an ApprovedReceipt); it is pure, so a caller
// that holds the bytes, such as the archive anchor gate, needs no git and no
// file access to read one.
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
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// receiptFileName is the legacy receipt file (gentle-ai < 2.7.0) Capture
// looks for inside each lineage directory.
const receiptFileName = "review-receipt.json"

// stateFileName is the lifecycle state file gentle-ai 2.7.0+ writes
// instead of receiptFileName.
const stateFileName = "review-state.json"

// storeRelPath is the transaction store location relative to a git-dir, slash-separated.
const storeRelPath = "gentle-ai/review-transactions/v2"

// activeChangeMarkers are the SDD artifact files whose presence marks a
// directory under openspec/changes/ as a genuine active change, distinct
// from unrelated stray directories. state.yaml is deliberately NOT the sole
// marker: this repository's own openspec/changes/ directories never carry a
// state.yaml (the orchestrator's state persistence is optional and, for
// hybrid/engram-store changes, state lives in Engram instead), so requiring
// it would make active-change detection permanently empty. Each call returns a
// fresh slice, so nothing can change the list for the next one.
func activeChangeMarkers() []string {
	return []string{"tasks.md", "design.md", "proposal.md", "entry.json"}
}

// Captured records one receipt Capture persisted (or confirmed already
// persisted byte-for-byte) this run.
type Captured struct {
	LineageID string
	Path      string
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
	err = eachSurvivingApproved(stores, func(s surviving) error {
		dest := filepath.Join(targetDir, s.Receipt.FileName())
		if err := writeReceiptFile(targetDir, dest, s.Data); err != nil {
			return err
		}
		captured = append(captured, Captured{LineageID: s.Receipt.Lineage, Path: dest})
		return nil
	})
	return captured, err
}

// surviving is one currently-approved receipt, either shape, and the exact
// bytes of the document it was read from.
type surviving struct {
	Receipt ApprovedReceipt
	Data    []byte
}

// seenReceipt is what two surviving receipts must share to be the same one: a
// lineage can hold both shapes, and they are two receipts.
type seenReceipt struct {
	shape   Shape
	lineage string
}

// eachSurvivingApproved calls visit for every currently-approved receipt across
// stores (either shape), once per lineage and shape, in the order the stores
// list them: store by store, lineage directory by lineage directory, the legacy
// receipt before the lifecycle state. It stops at the first error, a visit's
// included, so a caller that writes as it goes has written what came before it.
// A store that does not exist holds nothing; a file that cannot be read is not
// a receipt.
func eachSurvivingApproved(stores []string, visit func(surviving) error) error {
	seen := map[seenReceipt]bool{}
	for _, store := range stores {
		entries, err := os.ReadDir(store)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("reviewreceipt: read %s: %w", store, err)
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			dir := filepath.Join(store, e.Name())
			for _, doc := range []struct {
				shape Shape
				name  string
			}{{ShapeReceipt, receiptFileName}, {ShapeState, stateFileName}} {
				data, err := os.ReadFile(filepath.Join(dir, doc.name))
				if err != nil {
					continue
				}
				r, ok := approvedIn(doc.shape, data)
				key := seenReceipt{doc.shape, r.Lineage}
				if !ok || seen[key] {
					continue
				}
				seen[key] = true
				if err := visit(surviving{Receipt: r, Data: data}); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// AllSurvivingApprovedPersisted reports whether every currently-approved
// receipt is already persisted byte-for-byte under some change in changes
// (the explicit capture remedy already ran); zero surviving receipts is true.
func AllSurvivingApprovedPersisted(repoRoot string, changes []string) (bool, error) {
	stores, err := transactionStores(repoRoot)
	if err != nil {
		return false, err
	}
	var all []surviving
	if err := eachSurvivingApproved(stores, func(s surviving) error {
		all = append(all, s)
		return nil
	}); err != nil {
		return false, err
	}
	for _, s := range all {
		found := false
		for _, change := range changes {
			dest := filepath.Join(repoRoot, "openspec", "changes", change, "review-receipts", s.Receipt.FileName())
			if exists, persisted, err := isPersistedByteForByte(dest, s.Data); err != nil {
				return false, err
			} else if exists && persisted {
				found = true
				break
			}
		}
		if !found {
			return false, nil
		}
	}
	return true, nil
}

// isPersistedByteForByte is the single "already persisted?" check shared by
// writeReceiptFile and AllSurvivingApprovedPersisted.
func isPersistedByteForByte(dest string, data []byte) (exists, persisted bool, err error) {
	existing, err := os.ReadFile(dest)
	if err != nil {
		if os.IsNotExist(err) {
			return false, false, nil
		}
		return false, false, fmt.Errorf("reviewreceipt: read %s: %w", dest, err)
	}
	return true, bytes.Equal(existing, data), nil
}

// writeReceiptFile persists data at dest byte-for-byte, atomically (temp
// file + rename). It is idempotent -- identical existing content is a
// no-op -- but refuses to overwrite an existing file with different content.
func writeReceiptFile(targetDir, dest string, data []byte) error {
	if exists, persisted, err := isPersistedByteForByte(dest, data); err != nil {
		return err
	} else if exists {
		if persisted {
			return nil
		}
		return fmt.Errorf("reviewreceipt: %s already exists with different content", dest)
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

	rel := filepath.FromSlash(storeRelPath)
	stores := []string{filepath.Join(gitDir, rel)}
	if commonDir != gitDir {
		stores = append(stores, filepath.Join(commonDir, rel))
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
	for _, marker := range activeChangeMarkers() {
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
