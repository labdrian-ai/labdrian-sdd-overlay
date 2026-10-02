package shaper

import "errors"

// This file is the shaper's side of the ports it needs from the world. A port is a
// question or an obligation the domain states in its own words; an adapter, in a
// subpackage (engine/shaper/fsadapter), answers it from files. The domain never learns
// where the answer came from.

// ErrClearanceNotFound is what a ClearanceStore reports, wrapped with its own detail,
// when it holds no record for the key it was asked about. It is how a caller tells an
// absent clearance apart from a store that could not be read.
var ErrClearanceNotFound = errors.New("clearance store: no clearance record for that key")

// ClearanceStore is where host-owned clearance records are kept, one per handoff of a
// goal of a project. The domain owns this port; engine/shaper/fsadapter implements it
// over files under the user's state home, outside every worktree and outside Engram.
//
// A record is keyed by the subject it names (project_id, goal_id, handoff_sha256) and is
// immutable: storing identical bytes again is idempotent, and storing different bytes
// under an existing key is refused and leaves the stored record as it was. The store
// holds bytes; whether a stored record clears anything is for Verify, which callers run on
// what Get returns. The store is not a signature, and nothing about it stops a process
// running as the same OS user from forging a record: the runtime deny guards (see
// GuardStoreMarker) only keep the model out.
type ClearanceStore interface {
	// Path returns where the record for one key is, or would be, kept, as text for a
	// person to read in a report, without touching the store. The key is refused if
	// projectID or goalID is not one safe name or handoffSHA256 is not 64 lowercase hex
	// characters (IsSHA256Hex).
	Path(projectID, goalID, handoffSHA256 string) (string, error)

	// Put strictly parses data as a clearance record (ParseRecord), stores it under the
	// key its subject names, and returns where it is kept. Either decision is stored; a
	// record from any channel but a human's TUI is refused by the parse. Identical
	// bytes already stored under the key are idempotent; different bytes are refused.
	Put(data []byte) (string, error)

	// Get returns the stored bytes for one key, unverified. A key with no record is an
	// error that satisfies errors.Is(err, ErrClearanceNotFound); a store that cannot be
	// read, or holds something that is not a plain record, is any other error.
	Get(projectID, goalID, handoffSHA256 string) ([]byte, error)
}
