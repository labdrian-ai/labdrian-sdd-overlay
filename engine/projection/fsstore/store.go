// Package fsstore is the file-backed projection.BindingStore: the adapter that keeps
// one session binding per repository as a small JSON file under the user's state
// home.
//
// What the bytes of a binding file mean (projection.ClassifyBinding) and when a
// binding may be created, replaced or removed (projection.AdmitBind, AdmitReplace,
// AdmitUnbind, AdmitRemoval) are the domain's, and pure. This package is the rest:
// where the files live, how they are read without following a symlink, the lock
// that serializes writers, and the atomic publish of a new file. The state home is
// handed in by the composition root, which resolves it from the environment once
// (see engine/statestore.Home); this package reads no environment variable.
//
// The on-disk format and layout are a contract with every earlier version of the
// program, and they do not change here: <state home>/labdrian/bindings/<repo_key>.json
// (the binding, 0600) and <repo_key>.lock (the lock, 0600, never removed) in
// directories of mode 0700. testdata holds a sequence of states recorded from the
// version that kept this code in the projection package, and the tests read them,
// extend them, and write them again, byte for byte.
//
// It is built on engine/statestore (the directory chain and the no-follow read),
// engine/filelock (the lock) and engine/atomicfile (the publish). Its error messages
// are the ones the store printed when it lived in the projection package
// ("projection store: ...").
package fsstore

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/atomicfile"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/filelock"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/statestore"
)

// Store is a projection.BindingStore.
var _ projection.BindingStore = Store{}

var (
	// ErrStoreNotInitialized is returned by every Store method called on the
	// zero Store value instead of one built with NewStore.
	ErrStoreNotInitialized = errors.New("projection store: store is not initialized; use NewStore")
	// ErrUnsupportedPlatform: the running platform has no store support.
	ErrUnsupportedPlatform = errors.New("projection store: unsupported platform")
)

// DefaultLockWait is how long Bind and Unbind wait for the repository's lock before
// they give up with projection.ErrBindingBusy.
const DefaultLockWait = 2 * time.Second

// Store keeps one binding per repository at
// <state home>/labdrian/bindings/<repo_key>.json, outside every worktree. The
// state home is the directory NewStore was given. Directories are created with mode
// 0700 and the file with mode 0600, as engine/workflow/filelog's Store does.
//
// A binding is a pointer, not a log. Replacing one loses nothing the workflow's
// own log does not hold, so the store needs no append history:
//
//   - Every write is an atomic rename of a fully written, synced temporary file
//     in the same directory, followed by a sync of the directory. A reader sees
//     the old binding or the new one, never a partial file.
//   - Bind and Unbind serialize per repository through an advisory flock on
//     <repo_key>.lock in the same directory (mode 0600, never removed), held
//     for their whole load, classify, and write or remove sequence, so neither
//     acts on a binding another Bind or Unbind has since replaced. They wait for
//     it up to the store's lock wait (DefaultLockWait unless WithLockWait says
//     otherwise), then fail with projection.ErrBindingBusy. Load never takes it.
//   - The refusal to touch a foreign, malformed, or unavailable file is a
//     check followed by an action, not one atomic step: the file is classified
//     first and renamed over or removed afterwards, so a foreign file created
//     in between would be replaced. The window is the time between two system
//     calls, and what is lost is a pointer that can be recreated. A symlink
//     created in that window is refused by the publish, never replaced.
//   - Bind with replace overwrites whatever owned binding is there. A caller
//     that reads a binding, judges it stale, and then replaces it (as workflow
//     bind does) must use BindIfUnchanged instead, which replaces it only if it
//     is still exactly the binding that was read. The lock spans that
//     comparison and the write, so a live binding another process made in
//     between is never overwritten. UnbindIfUnchanged is the same for a caller
//     that removes a binding it read (as the projection hook does for a closed
//     workflow): it never removes a binding made after the one it saw.
//   - Removing a binding does not sync the directory. If the machine crashes
//     right after Unbind, the binding may reappear, which is a valid state: the
//     one the repository had a moment ago.
//
// The Store supports linux and darwin only (see NewStore).
type Store struct {
	stateHome string
	lockWait  time.Duration
}

// NewStore builds the store over stateHome, the directory under which labdrian keeps
// its local state (see statestore.Home); it must be an absolute path. It does not
// check that the directory exists or is usable: that is checked when the store is
// used. An unsupported platform is refused.
func NewStore(stateHome string) (Store, error) {
	if !statestore.Supported {
		return Store{}, fmt.Errorf("%w: %s (supported: linux, darwin)", ErrUnsupportedPlatform, runtime.GOOS)
	}
	if !filepath.IsAbs(stateHome) {
		return Store{}, fmt.Errorf("projection store: state home %q is not an absolute path", stateHome)
	}
	return Store{stateHome: stateHome, lockWait: DefaultLockWait}, nil
}

// WithLockWait returns a copy of the store that waits up to wait for a taken lock
// before it gives up with projection.ErrBindingBusy. A wait that is not positive is
// DefaultLockWait.
func (s Store) WithLockWait(wait time.Duration) Store {
	if wait <= 0 {
		wait = DefaultLockWait
	}
	s.lockWait = wait
	return s
}

// dirParts returns the state home followed by every store directory below it.
func (s Store) dirParts() []string {
	return []string{s.stateHome, "labdrian", "bindings"}
}

// path returns the binding file's path without touching the filesystem. The
// key is validated first: a key that is not 64 lowercase hex digits is refused
// before it can shape a path.
func (s Store) path(repoKey string) (string, error) {
	if s.stateHome == "" {
		return "", ErrStoreNotInitialized
	}
	if err := projection.ValidateRepoKey(repoKey); err != nil {
		return "", fmt.Errorf("projection store: %w", err)
	}
	return filepath.Join(append(s.dirParts(), repoKey+".json")...), nil
}

// lock takes the lock of the binding file at path (see Store), creating the
// store directories and the lock file if missing. It returns the unlock function. A
// lock that stays taken past the store's lock wait is projection.ErrBindingBusy.
//
// On a platform without flock it fails closed with ErrUnsupportedPlatform; NewStore
// has refused the platform long before this is reached.
func (s Store) lock(path string) (func(), error) {
	if err := statestore.EnsureDirs(s.dirParts()); err != nil {
		return nil, fmt.Errorf("%w: projection store: %v", projection.ErrBindingUnavailable, err)
	}
	unlock, err := filelock.Acquire(strings.TrimSuffix(path, ".json")+".lock", filelock.Options{Perm: 0o600, Wait: s.lockWait})
	switch {
	case err == nil:
		return unlock, nil
	case errors.Is(err, filelock.ErrBusy):
		return nil, projection.ErrBindingBusy
	case errors.Is(err, filelock.ErrUnsupported):
		return nil, ErrUnsupportedPlatform
	default:
		return nil, fmt.Errorf("projection store: acquire lock: %w", err)
	}
}

// Load classifies the on-disk state of one repository's binding (see
// projection.Classification) and, for ClassificationOwned, returns the parsed
// Binding. Load never writes. It returns an error only for a store that was not
// initialized or a repo key that is not 64 lowercase hex digits; every problem with
// the state itself is reported through the classification.
func (s Store) Load(repoKey string) (projection.Loaded, error) {
	path, err := s.path(repoKey)
	if err != nil {
		return projection.Loaded{}, err
	}

	if err := statestore.CheckDirs(s.dirParts()); err != nil {
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return projection.Loaded{Classification: projection.ClassificationAbsent}, nil
		case errors.Is(err, statestore.ErrSymlink), errors.Is(err, statestore.ErrNotDir):
			return unavailable("projection store: " + err.Error()), nil
		}
		return unavailable(err.Error()), nil
	}

	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return projection.Loaded{Classification: projection.ClassificationAbsent}, nil
	}
	if err != nil {
		return unavailable(err.Error()), nil
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return unavailable(fmt.Sprintf("refusing symlinked binding file %q", path)), nil
	}

	data, err := readBindingFile(path)
	if err != nil {
		return readFailure(err), nil
	}
	return projection.ClassifyBinding(repoKey, data), nil
}

// readFailure classifies the failure to read a binding file that Load has just
// seen exist. Load checks that the file is there and then opens it, and another
// process can remove it in between (Unbind does): the binding is then gone, which
// is absent, not a state nothing can be known about. Any other failure (a
// permission error, a path that stopped being a regular file) is unavailable.
func readFailure(err error) projection.Loaded {
	if errors.Is(err, fs.ErrNotExist) {
		return projection.Loaded{Classification: projection.ClassificationAbsent}
	}
	return unavailable(err.Error())
}

func unavailable(detail string) projection.Loaded {
	return projection.Loaded{Classification: projection.ClassificationUnavailable, Detail: detail}
}

// Bind binds the repository to the workflow projectID/workflowID, recording now
// (converted to UTC, without fractions of a second) as when. What is created, kept,
// replaced or refused is projection.AdmitBind's: an absent binding is created, the
// same workflow again is a no-op that keeps the original bound_at, a different
// workflow is refused with ErrAlreadyBound unless replace is true, and a file that
// is foreign, malformed, or unavailable is refused and left byte-for-byte unchanged.
//
// Invalid input (a repo key that is not 64 lowercase hex digits, an invalid
// identifier) is refused before anything is read or created.
func (s Store) Bind(repoKey, projectID, workflowID string, now time.Time, replace bool) error {
	record := projection.NewBinding(repoKey, projectID, workflowID, now)
	data, err := record.Marshal()
	if err != nil {
		return fmt.Errorf("projection store: bind: %w", err)
	}
	path, err := s.path(repoKey)
	if err != nil {
		return err
	}
	unlock, err := s.lock(path)
	if err != nil {
		return err
	}
	defer unlock()

	loaded, err := s.Load(repoKey)
	if err != nil {
		return err
	}
	write, err := projection.AdmitBind(loaded, record, replace)
	if err != nil || !write {
		return err
	}
	return writeBinding(path, data)
}

// BindIfUnchanged replaces the repository's binding with projectID/workflowID,
// recording now (converted to UTC, without fractions of a second) as when, only
// if the stored binding is still exactly expected: the binding the caller read
// and judged (projection.AdmitReplace). It is the compare-and-swap for a caller
// that decides from a binding and then acts on the decision, as workflow bind does
// when it finds the bound workflow stale; a binding another process made in between
// is never overwritten.
//
// The comparison is made once without the lock, so a stale caller is refused
// without touching the disk, and again under the repository's lock, which is held
// until the write is done. A refusal that finds the binding gone or not ours does not
// even create the store. A lock that stays taken past the lock wait is
// ErrBindingBusy: nothing was compared or written.
//
// Invalid input (a repo key that is not 64 lowercase hex digits, an invalid
// identifier, an expected binding that is invalid or names another repository) is
// refused before anything is read or created, and is not ErrBindingChanged.
func (s Store) BindIfUnchanged(repoKey, projectID, workflowID string, now time.Time, expected projection.Binding) error {
	record := projection.NewBinding(repoKey, projectID, workflowID, now)
	data, err := record.Marshal()
	if err != nil {
		return fmt.Errorf("projection store: bind if unchanged: %w", err)
	}
	if err := projection.CheckExpected("bind if unchanged", repoKey, expected); err != nil {
		return err
	}
	path, err := s.path(repoKey)
	if err != nil {
		return err
	}

	loaded, err := s.Load(repoKey)
	if err != nil {
		return err
	}
	if err := projection.UnchangedSince(loaded, expected); err != nil {
		return err
	}
	unlock, err := s.lock(path)
	if err != nil {
		return err
	}
	defer unlock()
	if loaded, err = s.Load(repoKey); err != nil {
		return err
	}
	write, err := projection.AdmitReplace(loaded, expected, record)
	if err != nil || !write {
		return err
	}
	return writeBinding(path, data)
}

// Unbind removes the repository's binding and reports whether it removed one
// (projection.AdmitUnbind). An absent binding is (false, nil): unbinding twice is not
// an error. An owned binding is removed. A foreign, malformed, or unavailable file is
// refused with the same named errors as Bind and left untouched.
func (s Store) Unbind(repoKey string) (removed bool, err error) {
	path, err := s.path(repoKey)
	if err != nil {
		return false, err
	}
	loaded, err := s.Load(repoKey)
	if err != nil {
		return false, err
	}
	if loaded.Classification == projection.ClassificationOwned {
		// Only a removal needs the lock (taking it creates directories), and what
		// is removed is decided again under it: a Bind may have replaced the file.
		unlock, err := s.lock(path)
		if err != nil {
			return false, err
		}
		defer unlock()
		if loaded, err = s.Load(repoKey); err != nil {
			return false, err
		}
	}
	remove, err := projection.AdmitUnbind(loaded)
	if err != nil || !remove {
		return false, err
	}
	return removeBinding(path, "unbind")
}

// UnbindIfUnchanged removes the repository's binding only if it is still exactly
// expected: the binding the caller read and acted on (projection.AdmitRemoval). It
// is Unbind for a caller that decides from a binding and then removes it, as the
// projection hook does when the bound workflow turns out to be closed: in the gap
// between reading and removing, another process can bind the next workflow, and that
// binding is not the caller's to remove.
//
// A binding that is already gone is (false, nil), as for Unbind: the state the
// caller wanted holds, and nothing is created to say so. Any other binding or file is
// (false, ErrBindingChanged), whose message says what is there now, and nothing is
// removed. As for BindIfUnchanged, the comparison is made once without the lock and
// again under it, which is held until the file is removed, and a lock that stays
// taken past the lock wait is ErrBindingBusy. Invalid input (a repo key that is not
// 64 lowercase hex digits, an expected binding that is invalid or names another
// repository) is refused before anything is read or created, and is not
// ErrBindingChanged.
func (s Store) UnbindIfUnchanged(repoKey string, expected projection.Binding) (removed bool, err error) {
	path, err := s.path(repoKey)
	if err != nil {
		return false, err
	}
	if err := projection.CheckExpected("unbind if unchanged", repoKey, expected); err != nil {
		return false, err
	}

	loaded, err := s.Load(repoKey)
	if err != nil {
		return false, err
	}
	if remove, err := projection.AdmitRemoval(loaded, expected); !remove {
		return false, err
	}
	unlock, err := s.lock(path)
	if err != nil {
		return false, err
	}
	defer unlock()
	if loaded, err = s.Load(repoKey); err != nil {
		return false, err
	}
	if remove, err := projection.AdmitRemoval(loaded, expected); !remove {
		return false, err
	}
	return removeBinding(path, "unbind if unchanged")
}

// removeBinding removes the file at path, and reports whether it did. A file that is
// already gone, removed by a process that does not take the lock, is the state that
// was asked for: (false, nil).
func removeBinding(path, op string) (bool, error) {
	if err := os.Remove(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("projection store: %s: %w", op, err)
	}
	return true, nil
}

// readBindingFile reads at most projection.MaxBindingBytes+1 bytes of path without
// following a final symlink and without blocking, and requires the opened
// descriptor to be a regular file. Reading one byte past the cap is enough for
// projection.ClassifyBinding to see that the file is too large, without reading the
// rest.
func readBindingFile(path string) ([]byte, error) {
	data, err := statestore.ReadFile(path, projection.MaxBindingBytes+1)
	switch {
	case err == nil:
		return data, nil
	case errors.Is(err, statestore.ErrSymlink):
		return nil, fmt.Errorf("refusing symlinked binding file %q", path)
	case errors.Is(err, statestore.ErrNotRegular):
		return nil, fmt.Errorf("binding path %q is not a regular file", path)
	}
	return nil, err
}

// bindingOptions is how a binding is written: private, flushed to disk, in a hidden
// temporary file of its own, replacing the file in one rename without a backup.
func bindingOptions() atomicfile.Options {
	return atomicfile.Options{Perm: 0o600, Sync: true, TempPattern: ".binding-*.tmp"}
}

// writeBinding publishes data at path atomically: written and synced to a
// temporary file in the same directory, then renamed into place, then the
// directory is synced so the rename itself survives a crash. The temporary file is
// removed on every failure path. A symlink at path is refused, never replaced.
func writeBinding(path string, data []byte) error {
	switch err := atomicfile.WriteFile(path, data, bindingOptions()); {
	case err == nil:
		return nil
	case errors.Is(err, atomicfile.ErrSymlink):
		return fmt.Errorf("projection store: refusing symlinked binding file %q", path)
	default:
		return fmt.Errorf("projection store: publish: %w", err)
	}
}
