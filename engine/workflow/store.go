package workflow

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

// ErrStoreNotInitialized is returned by every Store method when called on the
// zero Store value instead of one built with NewStore.
var ErrStoreNotInitialized = errors.New("workflow store: store is not initialized; use NewStore")

// workflowStoreComponents are the fixed directories under the state home.
var workflowStoreComponents = []string{"labdrian", "workflows"}

// Store holds one workflow's append-only event log at
// <state home>/labdrian/workflows/<project_id>/<workflow_id>.jsonl, outside
// every worktree. The state home is $XDG_STATE_HOME, or $HOME/.local/state
// when XDG_STATE_HOME is unset. It follows the same resolution, directory
// permissions (0700), and file permissions (0600) as engine/roles's
// ChainStore and engine/shaper's FileStore.
//
// Unlike those two stores, which hold one immutable record per file (keyed
// by seq or content hash, published via hardlink so a concurrent write can
// never silently replace an existing record), a workflow's log is one
// mutable, growing file: each Append rewrites the whole file with the new
// event appended. That rewrite-and-replace shape is why Store needs an
// explicit append lock (see acquireLock) where ChainStore and FileStore do
// not: two of their writers race for a name that at most one can ever claim,
// while two of this store's writers would otherwise both read the same
// prefix and each publish a "next" event, silently discarding one of them.
//
// Each Append therefore reads the whole existing log and rewrites it plus
// the new event to a temporary file before renaming it into place (see
// appendWorkflowLog): that is what makes the publish atomic (a single
// rename can never leave a reader with a half-written file), at the cost of
// making one Append's I/O cost O(n) in the number of events already
// recorded. This is bounded and acceptable: MaxLogBytes caps a
// workflow's whole log at 16 MiB, so the worst-case rewrite is a bounded,
// fast, in-memory copy, never an unbounded scan.
//
// Store supports linux and darwin only (see checkPlatform).
type Store struct {
	stateHome string
}

// ErrUnsupportedPlatform: the running platform has no store support.
var ErrUnsupportedPlatform = errors.New("workflow store: unsupported platform")

// checkPlatform accepts only linux and darwin (no-follow read + reclaimable lock).
func checkPlatform(goos string) error {
	if goos == "linux" || goos == "darwin" {
		return nil
	}
	return fmt.Errorf("%w: %s (supported: linux, darwin)", ErrUnsupportedPlatform, goos)
}

// StateHome resolves the directory under which labdrian keeps its local
// state, outside every worktree: $XDG_STATE_HOME when it is set, otherwise
// $HOME/.local/state. A set but relative XDG_STATE_HOME, and an unset, empty,
// or relative HOME fallback, are refused. It reads only the environment: it
// does not check that the directory exists or is usable.
//
// It is the one resolution the stores of this module that live under the
// state home share (Store here and the session binding store in
// engine/projection), so they can never disagree about where "the state
// home" is. Its errors carry no store name; each caller adds its own prefix.
func StateHome() (string, error) {
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		if !filepath.IsAbs(xdg) {
			return "", fmt.Errorf("XDG_STATE_HOME %q is not absolute", xdg)
		}
		return filepath.Clean(xdg), nil
	}
	home := os.Getenv("HOME")
	if home == "" || !filepath.IsAbs(home) {
		return "", fmt.Errorf("XDG_STATE_HOME is unset and HOME %q is not an absolute path", home)
	}
	return filepath.Join(home, ".local", "state"), nil
}

// NewStore resolves the store from the environment, exactly as
// roles.NewChainStore and shaper.NewFileStore do (see StateHome). A set but
// relative XDG_STATE_HOME, and an unset, empty, or relative HOME fallback, are
// refused, as is an unsupported platform (see checkPlatform).
func NewStore() (Store, error) {
	if err := checkPlatform(runtime.GOOS); err != nil {
		return Store{}, err
	}
	stateHome, err := StateHome()
	if err != nil {
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
	if err := ValidateIdentifier("project_id", projectID); err != nil {
		return nil, fmt.Errorf("workflow store: %w", err)
	}
	parts := append([]string{s.stateHome}, workflowStoreComponents...)
	return append(parts, projectID), nil
}

// path returns the workflow log's path without touching the filesystem,
// validating both identifiers.
func (s Store) path(projectID, workflowID string) (string, error) {
	dirParts, err := s.dirParts(projectID)
	if err != nil {
		return "", err
	}
	if err := ValidateIdentifier("workflow_id", workflowID); err != nil {
		return "", fmt.Errorf("workflow store: %w", err)
	}
	return filepath.Join(append(dirParts, workflowID+".jsonl")...), nil
}

// lockPath returns the append lock's path for one workflow.
func (s Store) lockPath(projectID, workflowID string) (string, error) {
	dirParts, err := s.dirParts(projectID)
	if err != nil {
		return "", err
	}
	if err := ValidateIdentifier("workflow_id", workflowID); err != nil {
		return "", fmt.Errorf("workflow store: %w", err)
	}
	return filepath.Join(append(dirParts, workflowID+".lock")...), nil
}

// Load classifies the on-disk state of one workflow (see Classification)
// and, for ClassificationOwned, returns its parsed events and replayed
// State. Load never writes.
func (s Store) Load(projectID, workflowID string) (Loaded, error) {
	path, err := s.path(projectID, workflowID)
	if err != nil {
		return Loaded{}, err
	}
	dirParts, err := s.dirParts(projectID)
	if err != nil {
		return Loaded{}, err
	}

	current := dirParts[0]
	for _, part := range dirParts[1:] {
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if errors.Is(statErr, os.ErrNotExist) {
			return Loaded{Classification: ClassificationAbsent}, nil
		}
		if statErr != nil {
			return Loaded{Classification: ClassificationUnavailable, Detail: statErr.Error()}, nil
		}
		if err := requireWorkflowPlainDir(current, info); err != nil {
			return Loaded{Classification: ClassificationUnavailable, Detail: err.Error()}, nil
		}
	}

	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return Loaded{Classification: ClassificationAbsent}, nil
	}
	if err != nil {
		return Loaded{Classification: ClassificationUnavailable, Detail: err.Error()}, nil
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return Loaded{Classification: ClassificationUnavailable, Detail: fmt.Sprintf("refusing symlinked workflow log %q", path)}, nil
	}

	data, err := readWorkflowLogFile(path)
	if err != nil {
		return Loaded{Classification: ClassificationUnavailable, Detail: err.Error()}, nil
	}
	return ClassifyLog(projectID, workflowID, data), nil
}

// Append validates next against the current on-disk state and, if legal,
// appends it atomically. The rules are the domain's (see AdmitAppend): it is
// allowed only when Load classifies the current state as ClassificationAbsent
// (next must be the seq-0 created event) or ClassificationOwned (next must be
// the next seq, chain to the last stored event's digest, and pass
// CheckTransition against the replayed State). Every other classification is
// refused with a named error and the file, if any, is left byte-for-byte
// unchanged. Append serializes concurrent writers for the same workflow with a
// lock file; see acquireLock.
func (s Store) Append(projectID, workflowID string, next WorkflowEvent) error {
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
	if err := ensureWorkflowStoreDirs(dirParts); err != nil {
		return err
	}

	release, err := acquireLock(lockPath)
	if err != nil {
		return err
	}
	defer release()

	loaded, err := s.Load(projectID, workflowID)
	if err != nil {
		return fmt.Errorf("workflow store: append: %w", err)
	}
	line, err := AdmitAppend(projectID, workflowID, loaded, next)
	if err != nil {
		return err
	}
	return appendWorkflowLog(path, line)
}

// appendWorkflowLog rewrites the whole workflow log with line appended to
// its current content (empty if the file does not exist yet), publishing
// the result atomically: written and synced to a temporary file in the
// target directory, then renamed into place (unlike engine/roles's and
// engine/shaper's per-record hardlink publish, a rename is correct here
// because this file is a single mutable, growing log rather than one
// immutable record per name; see Store's doc comment).
func appendWorkflowLog(path string, line []byte) error {
	var existing []byte
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("workflow store: refusing symlinked workflow log %q", path)
		}
		existing, err = readWorkflowLogFile(path)
		if err != nil {
			return fmt.Errorf("workflow store: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("workflow store: %w", err)
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".workflow-*.tmp")
	if err != nil {
		return fmt.Errorf("workflow store: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(existing); err != nil {
		tmp.Close()
		return fmt.Errorf("workflow store: %w", err)
	}
	if _, err := tmp.Write(line); err != nil {
		tmp.Close()
		return fmt.Errorf("workflow store: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("workflow store: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("workflow store: %w", err)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return fmt.Errorf("workflow store: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("workflow store: publish: %w", err)
	}
	if err := syncWorkflowDir(dir); err != nil {
		return fmt.Errorf("workflow store: %w", err)
	}
	return nil
}

// ensureWorkflowStoreDirs makes sure the state home exists, then walks every
// store component below it, creating missing ones with mode 0700 and
// refusing any that is a symlink or not a directory. It mirrors
// engine/roles's ensureChainDirs and engine/shaper's ensureStoreDirs.
func ensureWorkflowStoreDirs(parts []string) error {
	home := parts[0]
	if _, err := os.Stat(home); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(home, 0o700); err != nil {
			return fmt.Errorf("workflow store: create state home: %w", err)
		}
	}
	if info, err := os.Stat(home); err != nil {
		return fmt.Errorf("workflow store: %w", err)
	} else if !info.IsDir() {
		return fmt.Errorf("workflow store: state home %q is not a directory", home)
	}

	current := home
	for _, part := range parts[1:] {
		current = filepath.Join(current, part)
		if err := os.Mkdir(current, 0o700); err == nil {
			if err := os.Chmod(current, 0o700); err != nil {
				return fmt.Errorf("workflow store: %w", err)
			}
		} else if !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("workflow store: %w", err)
		}
		info, err := os.Lstat(current)
		if err != nil {
			return fmt.Errorf("workflow store: %w", err)
		}
		if err := requireWorkflowPlainDir(current, info); err != nil {
			return err
		}
	}
	return nil
}

func requireWorkflowPlainDir(path string, info os.FileInfo) error {
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("workflow store: refusing symlinked store component %q", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("workflow store: store component %q is not a directory", path)
	}
	return nil
}

// readWorkflowLogFile reads path without following a final symlink and
// requires the opened descriptor to be a regular file.
func readWorkflowLogFile(path string) ([]byte, error) {
	f, err := openNoFollow(path)
	if err != nil {
		if isSymlinkRefusal(err) {
			return nil, fmt.Errorf("workflow store: refusing symlinked workflow log %q", path)
		}
		return nil, fmt.Errorf("workflow store: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("workflow store: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("workflow store: %q is not a regular file", path)
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("workflow store: %w", err)
	}
	return data, nil
}

func syncWorkflowDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
