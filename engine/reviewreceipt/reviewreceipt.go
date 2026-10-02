// Package reviewreceipt persists a review transaction's receipt before the
// acknowledge-approved invocation burns it, so a change's approved_tree is
// always sourced from a verified receipt -- never self-asserted. This closes
// the bug that archived three prior changes with unverified anchors (R-008).
//
// The package is the rules, and none of the world: it names no file, no process
// and no git. What it asks of the world it asks through four ports (ports.go) --
// where the review transactions are (TransactionStores), what they hold
// (ReceiptSource), where the project keeps what it captured (ReceiptSink), and
// what changes the project has (ChangeCatalog) -- and a Service (service.go) is
// built over them. engine/reviewreceipt/fsstore is the adapter made of git and
// files that the program builds and hands in.
//
// Capture scans every store the project's review transactions live in -- the
// working tree's own (where the active review transaction of a linked worktree
// lives) and the one every working tree shares -- for lineages carrying an
// approved artifact, and persists each byte-for-byte into
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
// Names: the change name and the file name a receipt is persisted under are each
// joined into a path by the sink, so each must be exactly one path component
// (CheckPathComponent: not empty, not a dot segment, no separator, no white space
// at either end, no control character). The lineage id that makes a file name is
// read from a document another program wrote, so the Service checks the name
// before it asks the sink anything, and a refused name is an UnsafeNameError that
// names it.
//
// Fail-closed: Capture never overwrites an existing receipt file with
// different content -- that is an error, never a silent overwrite. The
// fail-closed PreToolUse Bash hook that calls Capture before
// acknowledge-approved runs (Service.RunHook) denies (exit 2) on any capture
// error or on more than one active change, rather than letting the
// acknowledgement proceed without a persisted receipt.
package reviewreceipt

import (
	"fmt"
	"strings"
)

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
	// Path is where the receipt is persisted, as the ReceiptSink names it.
	Path string
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
