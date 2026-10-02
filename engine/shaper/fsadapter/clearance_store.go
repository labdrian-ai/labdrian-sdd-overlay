package fsadapter

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/shaper"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/statestore"
)

// ClearanceStore is a shaper.ClearanceStore.
var _ shaper.ClearanceStore = ClearanceStore{}

// storeComponents are the fixed directories under the state home. They are the path
// shaper.GuardStoreMarker names, and a test keeps the two equal.
var storeComponents = []string{"labdrian", "shaper-clearance"}

// ErrUnsupportedPlatform: the running platform has no store support.
var ErrUnsupportedPlatform = errors.New("clearance store: unsupported platform")

// ClearanceStore holds clearance records at
// <state home>/labdrian/shaper-clearance/<project_id>/<goal_id>/<handoff_sha256>.json,
// outside every worktree and outside Engram. The state home is the directory
// NewClearanceStore was given.
//
// Records are immutable: storing identical bytes again is idempotent, and storing
// different bytes under an existing key is refused. Store directories it creates are
// 0700, record files are 0600, and a symlink at any store component is refused. File
// modes and deny guards only keep the model out; the store is not a signature, and any
// process running as the same OS user can still write a record.
type ClearanceStore struct {
	stateHome string
}

// NewClearanceStore builds the store over stateHome, the directory under which labdrian
// keeps its local state (see statestore.Home); it must be an absolute path. It does not
// check that the directory exists or is usable: that is checked when the store is used.
// A platform without a no-follow open (statestore.RequireStore) is refused with
// ErrUnsupportedPlatform.
func NewClearanceStore(stateHome string) (ClearanceStore, error) {
	if err := statestore.RequireStore("clearance store", ErrUnsupportedPlatform, stateHome); err != nil {
		return ClearanceStore{}, err
	}
	return ClearanceStore{stateHome: stateHome}, nil
}

// Path returns the record path for one key without touching the file system. projectID and
// goalID must each be one safe path component, and handoffSHA256 must be 64 lowercase hex
// characters.
func (s ClearanceStore) Path(projectID, goalID, handoffSHA256 string) (string, error) {
	dir, err := s.dirParts(projectID, goalID)
	if err != nil {
		return "", err
	}
	if !shaper.IsSHA256Hex(handoffSHA256) {
		return "", fmt.Errorf("clearance store: handoff sha256 %q is not 64 lowercase hex characters", handoffSHA256)
	}
	return filepath.Join(append(dir, handoffSHA256+".json")...), nil
}

// dirParts returns the state home followed by every store directory component for one
// key, validating projectID and goalID as safe single path components.
func (s ClearanceStore) dirParts(projectID, goalID string) ([]string, error) {
	if s.stateHome == "" {
		return nil, errors.New("clearance store: store is not initialized; use NewClearanceStore")
	}
	for _, c := range []struct{ name, value string }{{"project_id", projectID}, {"goal_id", goalID}} {
		if err := checkComponent(c.name, c.value); err != nil {
			return nil, err
		}
	}
	parts := append([]string{s.stateHome}, storeComponents...)
	return append(parts, projectID, goalID), nil
}

func checkComponent(name, value string) error {
	switch {
	case value == "", value == ".", value == "..":
		return fmt.Errorf("clearance store: %s %q is not a usable path component", name, value)
	case strings.ContainsAny(value, `/\`+string(filepath.Separator)+"\x00"):
		return fmt.Errorf("clearance store: %s %q contains a path separator or NUL", name, value)
	}
	return nil
}

// Put strictly parses data as a clearance record and stores it under the key its subject
// names. It returns the record path. Identical bytes already stored are idempotent;
// different bytes are refused and the stored record is left unchanged. The record is
// published atomically: it is written and synced to a temporary file in the target
// directory and then hard-linked into place, which, unlike a rename, can never replace an
// existing record (statestore.Publish).
func (s ClearanceStore) Put(data []byte) (string, error) {
	r, err := shaper.ParseRecord(data)
	if err != nil {
		return "", fmt.Errorf("clearance store: %w", err)
	}
	path, err := s.Path(r.Subject.ProjectID, r.Subject.GoalID, r.Subject.HandoffSHA256)
	if err != nil {
		return "", err
	}
	parts, err := s.dirParts(r.Subject.ProjectID, r.Subject.GoalID)
	if err != nil {
		return "", err
	}
	if err := statestore.EnsureDirs(parts); err != nil {
		return "", fmt.Errorf("clearance store: %w", err)
	}
	if err := statestore.Publish(path, data); err != nil {
		return "", recordError(path, err)
	}
	return path, nil
}

// Get returns the stored record bytes for one key. It refuses a symlink at any store
// component and a record that is not a regular file. A key with no record is
// shaper.ErrClearanceNotFound. It returns raw bytes; callers verify them with
// shaper.Verify.
//
// The read is bounded by shaper.MaxRecordBytes. Put refuses a larger record (the parse it
// starts with does), and the bound is the largest record the program ever accepted, so no
// record this program stored is out of reach; a larger file is something else left in the
// store, and it is refused after one byte past the bound has been read, naming the size
// the opened file reported, instead of being read whole.
func (s ClearanceStore) Get(projectID, goalID, handoffSHA256 string) ([]byte, error) {
	path, err := s.Path(projectID, goalID, handoffSHA256)
	if err != nil {
		return nil, err
	}
	parts, err := s.dirParts(projectID, goalID)
	if err != nil {
		return nil, err
	}
	if err := statestore.CheckDirs(parts); err != nil {
		return nil, absent(fmt.Errorf("clearance store: %w", err))
	}
	// One byte past the bound is enough to know the record is too large; the rest of it is
	// never read.
	data, size, err := statestore.ReadFileSized(path, shaper.MaxRecordBytes+1)
	if err != nil {
		return nil, absent(recordError(path, err))
	}
	if len(data) > shaper.MaxRecordBytes {
		return nil, recordTooLarge(path, size, len(data))
	}
	return data, nil
}

// recordTooLarge is the refusal of a record file over the bound. reported is the size the
// opened file reported and read how many bytes of it were read, at most one past the bound.
// It names the larger of the two: they differ when the file grew between the open and the
// read, and then the descriptor's size alone would make the refusal say that a file of a
// few bytes exceeds the maximum.
func recordTooLarge(path string, reported int64, read int) error {
	return fmt.Errorf("clearance store: record %q is %d bytes, exceeding the maximum of %d", path, max(reported, int64(read)), shaper.MaxRecordBytes)
}

// recordError gives a failure to read or publish the record at path the wording this store
// has always used for it. A refusal to replace a stored record with different bytes
// (statestore.ErrImmutable) already reads as it always did; it only gains the store's name.
func recordError(path string, err error) error {
	switch {
	case errors.Is(err, statestore.ErrSymlink):
		return fmt.Errorf("clearance store: refusing symlinked record %q", path)
	case errors.Is(err, statestore.ErrNotRegular):
		return fmt.Errorf("clearance store: record %q is not a regular file", path)
	}
	return fmt.Errorf("clearance store: %w", err)
}

// notFound is the error for a key with no record. Its text is the file system's own,
// unchanged, and it is shaper.ErrClearanceNotFound for a caller of the port as well as
// fs.ErrNotExist.
type notFound struct{ err error }

func (e notFound) Error() string   { return e.err.Error() }
func (e notFound) Unwrap() []error { return []error{shaper.ErrClearanceNotFound, e.err} }

// absent marks err as a missing record when the file system said something was not there,
// and leaves every other failure as it is.
func absent(err error) error {
	if errors.Is(err, fs.ErrNotExist) {
		return notFound{err}
	}
	return err
}
