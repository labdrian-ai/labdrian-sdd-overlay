// Package filelog is the file-backed workflow.EventLog: the adapter that keeps a
// workflow's append-only event log in a JSONL file under the user's state home.
//
// What the bytes of a log mean (workflow.ClassifyLog) and when an event may extend
// one (workflow.AdmitAppend) are the domain's, and pure. This package is the rest:
// where the file lives, how it is read without following a symlink, the lock that
// serializes appenders, and the atomic publish of the new file. An append is
// always the same four steps, in this order:
//
//  1. make the store directories (private, no symlink at any component);
//  2. take the workflow's append lock, refusing at once if another holds it;
//  3. read the log and classify it (workflow.ClassifyLog), then admit the event
//     (workflow.AdmitAppend), which is pure and returns the line to add;
//  4. publish the old bytes plus that line by writing a new file and renaming it
//     into place.
//
// The on-disk format and layout are a contract with every earlier version of the
// program, and they do not change here: <state home>/labdrian/workflows/
// <project_id>/<workflow_id>.jsonl (the log, 0600) and <workflow_id>.lock (the
// append lock, 0600, never removed) in directories of mode 0700; one canonical
// JSON line per event (workflow.WorkflowEvent.MarshalLine), each ending in a
// newline. testdata holds a log recorded from the version that kept this code in
// the workflow package, and the tests read it, extend it, and write it again, byte
// for byte.
//
// It is built on engine/statestore (the state home, the directory chain, the
// no-follow read), engine/filelock (the append lock) and engine/atomicfile (the
// publish). Its error messages are the ones the store printed when it lived in the
// workflow package ("workflow store: ...").
package filelog

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/atomicfile"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/statestore"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// Store is a workflow.EventLog.
var _ workflow.EventLog = Store{}

// ErrStoreNotInitialized is returned by every Store method when called on the
// zero Store value instead of one built with NewStore.
var ErrStoreNotInitialized = errors.New("workflow store: store is not initialized; use NewStore")

// ErrUnsupportedPlatform: the running platform has no store support.
var ErrUnsupportedPlatform = errors.New("workflow store: unsupported platform")

// storeComponents are the fixed directories under the state home.
var storeComponents = []string{"labdrian", "workflows"}

// Store holds one workflow's append-only event log at
// <state home>/labdrian/workflows/<project_id>/<workflow_id>.jsonl, outside
// every worktree. The state home is the directory NewStore was given: the
// composition root resolves it ($XDG_STATE_HOME, or $HOME/.local/state when
// XDG_STATE_HOME is unset; see statestore.Home), and this package reads no
// environment variable. Directories are created with mode 0700 and files with
// mode 0600, as the other file-backed stores do (roles/filechain, the shaper's
// clearance store, projection/fsstore), all on engine/statestore.
//
// Unlike the two stores that hold one immutable record per file (roles/filechain,
// keyed by seq, and the shaper's clearance store, keyed by content hash, both
// published via hardlink so a concurrent write can never silently replace an
// existing record), a workflow's log is one mutable, growing file: each Append
// rewrites the whole file with the new event appended. That rewrite-and-replace
// shape is why Store needs an explicit append lock (see acquireLock) where those
// two do not: their writers race for a name that at most one can ever claim,
// while two of this store's writers would otherwise both read the same prefix and
// each publish a "next" event, silently discarding one of them.
//
// Each Append therefore reads the whole existing log and rewrites it plus
// the new event to a temporary file before renaming it into place (see
// publish): that is what makes the publish atomic (a single rename can never
// leave a reader with a half-written file), at the cost of making one Append's
// I/O cost O(n) in the number of events already recorded. This is bounded and
// acceptable: workflow.MaxLogBytes caps a workflow's whole log at 16 MiB, so
// the worst-case rewrite is a bounded, fast, in-memory copy, never an unbounded
// scan.
//
// Store supports linux and darwin only (see NewStore).
type Store struct {
	stateHome string
}

// NewStore builds the store over stateHome, the directory under which labdrian
// keeps its local state (see statestore.Home); it must be an absolute path. It does
// not check that the directory exists or is usable: that is checked when the store
// is used. A platform without the no-follow read and the lock the store needs
// (statestore.RequirePlatform) is refused with ErrUnsupportedPlatform.
func NewStore(stateHome string) (Store, error) {
	if err := statestore.RequirePlatform(ErrUnsupportedPlatform); err != nil {
		return Store{}, err
	}
	if err := statestore.CheckHome(stateHome); err != nil {
		return Store{}, fmt.Errorf("workflow store: %w", err)
	}
	return Store{stateHome: stateHome}, nil
}

// dirParts returns the state home followed by every store directory
// component up to and including projectID, validating projectID as a safe
// single path component.
func (s Store) dirParts(projectID string) ([]string, error) {
	if s.stateHome == "" {
		return nil, ErrStoreNotInitialized
	}
	if err := workflow.ValidateIdentifier("project_id", projectID); err != nil {
		return nil, fmt.Errorf("workflow store: %w", err)
	}
	parts := append([]string{s.stateHome}, storeComponents...)
	return append(parts, projectID), nil
}

// path returns the workflow log's path without touching the filesystem,
// validating both identifiers.
func (s Store) path(projectID, workflowID string) (string, error) {
	return s.fileIn(projectID, workflowID, ".jsonl")
}

// lockPath returns the append lock's path for one workflow.
func (s Store) lockPath(projectID, workflowID string) (string, error) {
	return s.fileIn(projectID, workflowID, ".lock")
}

func (s Store) fileIn(projectID, workflowID, suffix string) (string, error) {
	dirParts, err := s.dirParts(projectID)
	if err != nil {
		return "", err
	}
	if err := workflow.ValidateIdentifier("workflow_id", workflowID); err != nil {
		return "", fmt.Errorf("workflow store: %w", err)
	}
	return filepath.Join(append(dirParts, workflowID+suffix)...), nil
}

// Load classifies the on-disk state of one workflow (see
// workflow.Classification) and, for ClassificationOwned, returns its parsed
// events and replayed State. Load never writes. It never reads more than
// workflow.MaxLogBytes+1 bytes of a log: a longer one is ClassificationMalformed
// (workflow.OversizedLog) without being read in full.
func (s Store) Load(projectID, workflowID string) (workflow.Loaded, error) {
	loaded, _, err := s.read(projectID, workflowID)
	return loaded, err
}

// read is Load, and also returns the bytes of the log it classified, which an
// append under the lock publishes again with its new line. A log that could not be
// read is ClassificationUnavailable; only an identifier that is not a safe path
// component is an error.
func (s Store) read(projectID, workflowID string) (workflow.Loaded, []byte, error) {
	path, err := s.path(projectID, workflowID)
	if err != nil {
		return workflow.Loaded{}, nil, err
	}
	dirParts, err := s.dirParts(projectID)
	if err != nil {
		return workflow.Loaded{}, nil, err
	}

	if err := statestore.CheckDirs(dirParts); err != nil {
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return workflow.Loaded{Classification: workflow.ClassificationAbsent}, nil, nil
		case errors.Is(err, statestore.ErrSymlink), errors.Is(err, statestore.ErrNotDir):
			return unavailable(fmt.Sprintf("workflow store: %v", err)), nil, nil
		}
		return unavailable(err.Error()), nil, nil
	}

	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return workflow.Loaded{Classification: workflow.ClassificationAbsent}, nil, nil
	}
	if err != nil {
		return unavailable(err.Error()), nil, nil
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return unavailable(fmt.Sprintf("refusing symlinked workflow log %q", path)), nil, nil
	}

	data, size, err := readLog(path)
	if err != nil {
		return unavailable(err.Error()), nil, nil
	}
	if len(data) > workflow.MaxLogBytes {
		// The read stopped one byte past the bound, so data is not the log; the
		// size the opened file reported is the closest to its real one. It is the
		// size of the file that was read, not of whatever the path named when it
		// was first looked at. An oversized log is never handed on: nothing is
		// published over it.
		return workflow.OversizedLog(max(size, int64(len(data)))), nil, nil
	}
	return workflow.ClassifyLog(projectID, workflowID, data), data, nil
}

func unavailable(detail string) workflow.Loaded {
	return workflow.Loaded{Classification: workflow.ClassificationUnavailable, Detail: detail}
}

// Append validates next against the current on-disk state and, if legal,
// appends it atomically. The rules are the domain's (workflow.AdmitAppend): it is
// allowed only when Load classifies the current state as ClassificationAbsent
// (next must be the seq-0 created event) or ClassificationOwned (next must be the
// next seq, chain to the last stored event's digest, and pass CheckTransition
// against the replayed State). Every other classification is refused with a named
// error and the file, if any, is left byte-for-byte unchanged. Append serializes
// concurrent writers for the same workflow with a lock file; see acquireLock.
func (s Store) Append(projectID, workflowID string, next workflow.WorkflowEvent) error {
	path, err := s.path(projectID, workflowID)
	if err != nil {
		return err
	}
	lockPath, err := s.lockPath(projectID, workflowID)
	if err != nil {
		return err
	}
	dirParts, err := s.dirParts(projectID)
	if err != nil {
		return err
	}
	if err := statestore.EnsureDirs(dirParts); err != nil {
		return fmt.Errorf("workflow store: %w", err)
	}

	release, err := acquireLock(lockPath)
	if err != nil {
		return err
	}
	defer release()

	loaded, existing, err := s.read(projectID, workflowID)
	if err != nil {
		return fmt.Errorf("workflow store: append: %w", err)
	}
	line, err := workflow.AdmitAppend(projectID, workflowID, loaded, next)
	if err != nil {
		return err
	}
	return publish(path, existing, line)
}

// logOptions is how a log is written: private, flushed to disk, in a hidden
// temporary file of its own. The log is replaced in one rename, so a reader sees
// the old log or the new one, and a replace is never backed up.
func logOptions() atomicfile.Options {
	return atomicfile.Options{Perm: 0o600, Sync: true, TempPattern: ".workflow-*.tmp"}
}

// publish replaces the log at path with existing plus line, atomically: the whole
// new log is written and synced to a temporary file in the target directory and
// renamed into place, then the directory is synced. A rename is right here, where
// the per-record hardlink publish of roles/filechain and of the shaper's clearance
// store (statestore.Publish) is not, because this file is a single mutable, growing
// log rather than one immutable record per name (see Store). A symlink at path is
// refused, never replaced.
func publish(path string, existing, line []byte) error {
	data := make([]byte, 0, len(existing)+len(line))
	data = append(data, existing...)
	data = append(data, line...)
	switch err := atomicfile.WriteFile(path, data, logOptions()); {
	case err == nil:
		return nil
	case errors.Is(err, atomicfile.ErrSymlink):
		return fmt.Errorf("workflow store: refusing symlinked workflow log %q", path)
	default:
		return fmt.Errorf("workflow store: publish: %w", err)
	}
}

// readLog reads path without following a final symlink and requires the opened
// descriptor to be a regular file. It reads at most workflow.MaxLogBytes+1 bytes:
// one past the bound is enough to know the log is too large, and the rest of it is
// never read. It also returns the size the opened file reported, which is what an
// oversized log is reported as.
func readLog(path string) (data []byte, size int64, err error) {
	data, size, err = statestore.ReadFileSized(path, workflow.MaxLogBytes+1)
	switch {
	case err == nil:
		return data, size, nil
	case errors.Is(err, statestore.ErrSymlink):
		return nil, 0, fmt.Errorf("workflow store: refusing symlinked workflow log %q", path)
	case errors.Is(err, statestore.ErrNotRegular):
		return nil, 0, fmt.Errorf("workflow store: %q is not a regular file", path)
	}
	return nil, 0, fmt.Errorf("workflow store: %w", err)
}
