package projection

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// Classification is the closed vocabulary Load reports for the on-disk state
// of one repository's binding. Only ClassificationAbsent and
// ClassificationOwned accept a write; every other value is preserved exactly as
// found and reported through the named refusal errors of Bind and Unbind. The
// vocabulary is the one engine/workflow uses for a workflow log, without the
// drifted value: a binding has no hash chain to drift.
type Classification string

const (
	// ClassificationAbsent means no binding file exists for the repository.
	ClassificationAbsent Classification = "absent"
	// ClassificationOwned means the file parses strictly as a Binding v1 and
	// its repo_key equals the key in its file name. Loaded.Binding is set.
	ClassificationOwned Classification = "owned"
	// ClassificationForeign means the file is valid JSON that is not ours: it
	// does not parse strictly as a Binding v1 (another schema or version, an
	// unknown or duplicated field, a field of the wrong shape, a document that
	// is not an object), or it is a valid Binding that names another
	// repository than its file name does. A foreign file is never overwritten
	// or removed.
	ClassificationForeign Classification = "foreign"
	// ClassificationMalformed means the file cannot be read as one JSON
	// document at all: it is empty, larger than MaxBindingBytes, not valid
	// UTF-8, not valid JSON, or valid JSON followed by more data. A malformed
	// file is never overwritten or removed.
	ClassificationMalformed Classification = "malformed"
	// ClassificationUnavailable means the state could not be read: a
	// permission error, a state home or store directory that is not a plain
	// directory (a symlink, or a file), or a binding path that is not a
	// regular file (a symlink, a directory, a FIFO). Nothing about the content
	// is known, so it is never overwritten or removed.
	ClassificationUnavailable Classification = "unavailable"
)

// Loaded is the result of Load: the classification, the parsed Binding only
// when the classification is ClassificationOwned, and, for every
// classification other than absent and owned, a human-readable Detail saying
// why.
type Loaded struct {
	Classification Classification
	Binding        Binding
	Detail         string
}

// Sentinel errors returned by the Store. Wrap with %w so callers can tell the
// failures apart with errors.Is.
var (
	// ErrStoreNotInitialized is returned by every Store method called on the
	// zero Store value instead of one built with NewStore.
	ErrStoreNotInitialized = errors.New("projection store: store is not initialized; use NewStore")
	// ErrUnsupportedPlatform: the running platform has no store support.
	ErrUnsupportedPlatform = errors.New("projection store: unsupported platform")
	// ErrRefuseForeignBinding is returned by Bind and Unbind when the on-disk
	// binding is foreign; the file is left byte-for-byte unchanged.
	ErrRefuseForeignBinding = errors.New("projection store: refusing to change the binding: the on-disk file is foreign")
	// ErrRefuseMalformedBinding is returned by Bind and Unbind when the
	// on-disk binding is malformed; the file is left byte-for-byte unchanged.
	ErrRefuseMalformedBinding = errors.New("projection store: refusing to change the binding: the on-disk file is malformed")
	// ErrBindingUnavailable is returned by Bind and Unbind when the on-disk
	// state could not be read; nothing is written or removed.
	ErrBindingUnavailable = errors.New("projection store: refusing to change the binding: the on-disk state is unavailable")
	// ErrAlreadyBound is returned by Bind when the repository is owned by a
	// binding to a different workflow and the caller did not ask to replace it.
	// The message names the workflow it is bound to.
	ErrAlreadyBound = errors.New("projection store: the repository is already bound to a different workflow")
	// ErrBindingBusy is returned by Bind and Unbind when the repository's lock
	// stays taken past lockWait. Nothing was changed; the call can be retried.
	ErrBindingBusy = errors.New("projection store: another bind or unbind is in progress for this repository; retry")
)

// lockWait is how long Bind and Unbind wait for the repository's lock before
// they give up with ErrBindingBusy. It is a variable so a test can shorten it.
var lockWait = 2 * time.Second

// Store keeps one binding per repository at
// <state home>/labdrian/bindings/<repo_key>.json, outside every worktree. The
// state home is workflow.StateHome: $XDG_STATE_HOME, or $HOME/.local/state when
// XDG_STATE_HOME is unset, the same resolution engine/workflow's Store uses.
// Directories are created with mode 0700 and the file with mode 0600, as that
// store does.
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
//     it up to lockWait, then fail with ErrBindingBusy. Load never takes it.
//   - The refusal to touch a foreign, malformed, or unavailable file is a
//     check followed by an action, not one atomic step: the file is classified
//     first and renamed over or removed afterwards, so a foreign file created
//     in between would be replaced. The window is the time between two system
//     calls, and what is lost is a pointer that can be recreated.
//   - Bind with replace is not a compare-and-swap. A caller that reads a
//     binding, judges it stale, and then replaces it (as workflow bind does)
//     can overwrite a live binding another process made in between: the lock
//     does not span the caller's decision.
//   - Removing a binding does not sync the directory. If the machine crashes
//     right after Unbind, the binding may reappear, which is a valid state: the
//     one the repository had a moment ago.
//
// The Store supports linux and darwin only (see platformSupported).
type Store struct {
	stateHome string
}

// NewStore resolves the store from the environment (see workflow.StateHome).
// A set but relative XDG_STATE_HOME, and an unset, empty, or relative HOME
// fallback, are refused, as is an unsupported platform.
func NewStore() (Store, error) {
	if !platformSupported {
		return Store{}, fmt.Errorf("%w: %s (supported: linux, darwin)", ErrUnsupportedPlatform, runtime.GOOS)
	}
	stateHome, err := workflow.StateHome()
	if err != nil {
		return Store{}, fmt.Errorf("projection store: %w", err)
	}
	return Store{stateHome: stateHome}, nil
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
	if err := validateRepoKey(repoKey); err != nil {
		return "", fmt.Errorf("projection store: %w", err)
	}
	return filepath.Join(append(s.dirParts(), repoKey+".json")...), nil
}

// lock takes the lock of the binding file at path (see Store), creating the
// store directories and the lock file if missing. It returns the unlock function.
func (s Store) lock(path string) (func(), error) {
	if err := ensureDirs(s.dirParts()); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBindingUnavailable, err)
	}
	return lockFile(strings.TrimSuffix(path, ".json") + ".lock")
}

// Load classifies the on-disk state of one repository's binding (see
// Classification) and, for ClassificationOwned, returns the parsed Binding.
// Load never writes. It returns an error only for a store that was not
// initialized or a repo key that is not 64 lowercase hex digits; every
// problem with the state itself is reported through the classification.
func (s Store) Load(repoKey string) (Loaded, error) {
	path, err := s.path(repoKey)
	if err != nil {
		return Loaded{}, err
	}

	parts := s.dirParts()
	current := parts[0]
	for _, part := range parts[1:] {
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if errors.Is(statErr, os.ErrNotExist) {
			return Loaded{Classification: ClassificationAbsent}, nil
		}
		if statErr != nil {
			return unavailable(statErr.Error()), nil
		}
		if err := requirePlainDir(current, info); err != nil {
			return unavailable(err.Error()), nil
		}
	}

	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return Loaded{Classification: ClassificationAbsent}, nil
	}
	if err != nil {
		return unavailable(err.Error()), nil
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return unavailable(fmt.Sprintf("refusing symlinked binding file %q", path)), nil
	}

	data, err := readBindingFile(path)
	if err != nil {
		return unavailable(err.Error()), nil
	}
	return classifyBinding(repoKey, data), nil
}

// Bind binds the repository to the workflow projectID/workflowID, recording
// now (converted to UTC, without fractions of a second) as when.
//
//   - An absent binding is created.
//   - An owned binding to the same workflow is an idempotent no-op: nothing is
//     rewritten and the original bound_at is kept, whether or not replace is
//     set.
//   - An owned binding to a different workflow is refused with ErrAlreadyBound,
//     whose message names the bound workflow, unless replace is true, in which
//     case it is replaced.
//   - A foreign, malformed, or unavailable file is refused with
//     ErrRefuseForeignBinding, ErrRefuseMalformedBinding, or
//     ErrBindingUnavailable, replace or not, and left byte-for-byte unchanged.
//
// Invalid input (a repo key that is not 64 lowercase hex digits, an invalid
// identifier) is refused before anything is read or created.
func (s Store) Bind(repoKey, projectID, workflowID string, now time.Time, replace bool) error {
	b := Binding{
		Version:    BindingVersion,
		RepoKey:    repoKey,
		ProjectID:  projectID,
		WorkflowID: workflowID,
		BoundAt:    now.UTC().Format(time.RFC3339),
	}
	data, err := b.Marshal()
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
	switch loaded.Classification {
	case ClassificationAbsent:
	case ClassificationOwned:
		bound := loaded.Binding
		if bound.ProjectID == projectID && bound.WorkflowID == workflowID {
			return nil
		}
		if !replace {
			return fmt.Errorf("%w: it is bound to workflow %q of project %q", ErrAlreadyBound, bound.WorkflowID, bound.ProjectID)
		}
	default:
		return refusal(loaded)
	}

	return writeBinding(path, data)
}

// Unbind removes the repository's binding and reports whether it removed one.
// An absent binding is (false, nil): unbinding twice is not an error. An owned
// binding is removed. A foreign, malformed, or unavailable file is refused with
// the same named errors as Bind and left untouched.
func (s Store) Unbind(repoKey string) (removed bool, err error) {
	path, err := s.path(repoKey)
	if err != nil {
		return false, err
	}
	loaded, err := s.Load(repoKey)
	if err != nil {
		return false, err
	}
	if loaded.Classification == ClassificationOwned {
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
	switch loaded.Classification {
	case ClassificationAbsent:
		return false, nil
	case ClassificationOwned:
		if err := os.Remove(path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				// Another process removed it between the load and here: the
				// binding is gone, which is what was asked for.
				return false, nil
			}
			return false, fmt.Errorf("projection store: unbind: %w", err)
		}
		return true, nil
	default:
		return false, refusal(loaded)
	}
}

// refusal maps a classification that must not be written over to its named
// error, keeping the detail that explains it.
func refusal(loaded Loaded) error {
	switch loaded.Classification {
	case ClassificationForeign:
		return fmt.Errorf("%w: %s", ErrRefuseForeignBinding, loaded.Detail)
	case ClassificationMalformed:
		return fmt.Errorf("%w: %s", ErrRefuseMalformedBinding, loaded.Detail)
	case ClassificationUnavailable:
		return fmt.Errorf("%w: %s", ErrBindingUnavailable, loaded.Detail)
	default:
		return fmt.Errorf("projection store: unknown classification %q", loaded.Classification)
	}
}

func unavailable(detail string) Loaded {
	return Loaded{Classification: ClassificationUnavailable, Detail: detail}
}

// classifyBinding classifies the bytes of the binding file of repoKey. It is
// pure: see Classification for the rules. The order matters. What cannot be
// read as one JSON document is malformed. What can, but is not a Binding v1 for
// this key, is foreign.
func classifyBinding(repoKey string, data []byte) Loaded {
	switch {
	case len(data) == 0:
		return Loaded{Classification: ClassificationMalformed, Detail: "binding file is empty"}
	case len(data) > MaxBindingBytes:
		return Loaded{Classification: ClassificationMalformed, Detail: fmt.Sprintf("binding file exceeds the maximum of %d bytes", MaxBindingBytes)}
	case !utf8.Valid(data):
		return Loaded{Classification: ClassificationMalformed, Detail: "binding file is not valid UTF-8"}
	case !json.Valid(data):
		// json.Valid is false for invalid syntax and for a document followed
		// by more data alike.
		return Loaded{Classification: ClassificationMalformed, Detail: "binding file is not one valid JSON document (invalid syntax, or data after the document)"}
	}
	b, err := ParseBinding(data)
	if err != nil {
		return Loaded{Classification: ClassificationForeign, Detail: fmt.Sprintf("binding file is not a binding we recognize: %v", err)}
	}
	if b.RepoKey != repoKey {
		return Loaded{Classification: ClassificationForeign, Detail: fmt.Sprintf("binding file names repo_key %q, but its file name says %q", b.RepoKey, repoKey)}
	}
	return Loaded{Classification: ClassificationOwned, Binding: b}
}

// readBindingFile reads at most MaxBindingBytes+1 bytes of path without
// following a final symlink and without blocking, and requires the opened
// descriptor to be a regular file. Reading one byte past the cap is enough for
// classifyBinding to see that the file is too large, without reading the rest.
func readBindingFile(path string) ([]byte, error) {
	f, err := openNoFollow(path)
	if err != nil {
		if isSymlinkRefusal(err) {
			return nil, fmt.Errorf("refusing symlinked binding file %q", path)
		}
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("binding path %q is not a regular file", path)
	}
	return io.ReadAll(io.LimitReader(f, MaxBindingBytes+1))
}

// ensureDirs makes sure the state home exists, then walks every store
// component below it, creating missing ones with mode 0700 and refusing any
// that is a symlink or not a directory. It mirrors engine/workflow's
// ensureWorkflowStoreDirs.
func ensureDirs(parts []string) error {
	home := parts[0]
	if _, err := os.Stat(home); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(home, 0o700); err != nil {
			return fmt.Errorf("projection store: create state home: %w", err)
		}
	}
	if info, err := os.Stat(home); err != nil {
		return fmt.Errorf("projection store: %w", err)
	} else if !info.IsDir() {
		return fmt.Errorf("projection store: state home %q is not a directory", home)
	}

	current := home
	for _, part := range parts[1:] {
		current = filepath.Join(current, part)
		if err := os.Mkdir(current, 0o700); err == nil {
			if err := os.Chmod(current, 0o700); err != nil {
				return fmt.Errorf("projection store: %w", err)
			}
		} else if !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("projection store: %w", err)
		}
		info, err := os.Lstat(current)
		if err != nil {
			return fmt.Errorf("projection store: %w", err)
		}
		if err := requirePlainDir(current, info); err != nil {
			return err
		}
	}
	return nil
}

func requirePlainDir(path string, info os.FileInfo) error {
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("projection store: refusing symlinked store component %q", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("projection store: store component %q is not a directory", path)
	}
	return nil
}

// writeBinding publishes data at path atomically: written and synced to a
// temporary file in the same directory, then renamed into place, then the
// directory is synced so the rename itself survives a crash. The temporary
// file is removed on every failure path.
func writeBinding(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".binding-*.tmp")
	if err != nil {
		return fmt.Errorf("projection store: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("projection store: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("projection store: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("projection store: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("projection store: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("projection store: publish: %w", err)
	}
	if err := syncDir(dir); err != nil {
		return fmt.Errorf("projection store: %w", err)
	}
	return nil
}

func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
