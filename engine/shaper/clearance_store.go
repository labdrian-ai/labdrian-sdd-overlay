package shaper

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// FileStore holds clearance records at
// <state home>/labdrian/shaper-clearance/<project_id>/<goal_id>/<handoff_sha256>.json,
// outside every worktree and outside Engram. The state home is
// $XDG_STATE_HOME, or $HOME/.local/state when XDG_STATE_HOME is unset.
//
// Records are immutable: storing identical bytes again is idempotent, and
// storing different bytes under an existing key is refused. Store
// directories it creates are 0700, record files are 0600, and a symlink at
// any store component is refused. File modes and deny guards only keep the
// model out; the store is not a signature, and any process running as the
// same OS user can still write a record.
type FileStore struct {
	stateHome string
}

// storeComponents are the fixed directories under the state home.
var storeComponents = []string{"labdrian", "shaper-clearance"}

// NewFileStore resolves the store from the environment. A set but relative
// XDG_STATE_HOME, and an unset, empty, or relative HOME fallback, are
// refused.
func NewFileStore() (FileStore, error) {
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		if !filepath.IsAbs(xdg) {
			return FileStore{}, fmt.Errorf("clearance store: XDG_STATE_HOME %q is not absolute", xdg)
		}
		return FileStore{stateHome: filepath.Clean(xdg)}, nil
	}
	home := os.Getenv("HOME")
	if home == "" || !filepath.IsAbs(home) {
		return FileStore{}, fmt.Errorf("clearance store: XDG_STATE_HOME is unset and HOME %q is not an absolute path", home)
	}
	return FileStore{stateHome: filepath.Join(home, ".local", "state")}, nil
}

// Path returns the record path for one key without touching the filesystem.
// projectID and goalID must each be one safe path component, and
// handoffSHA256 must be 64 lowercase hex characters.
func (s FileStore) Path(projectID, goalID, handoffSHA256 string) (string, error) {
	dir, err := s.dirComponents(projectID, goalID)
	if err != nil {
		return "", err
	}
	if !sha256HexPattern.MatchString(handoffSHA256) {
		return "", fmt.Errorf("clearance store: handoff sha256 %q is not 64 lowercase hex characters", handoffSHA256)
	}
	return filepath.Join(append(dir, handoffSHA256+".json")...), nil
}

// dirComponents returns the state home followed by every store directory
// component for one key.
func (s FileStore) dirComponents(projectID, goalID string) ([]string, error) {
	if s.stateHome == "" {
		return nil, fmt.Errorf("clearance store: store is not initialized; use NewFileStore")
	}
	for _, c := range []struct{ name, value string }{{"project_id", projectID}, {"goal_id", goalID}} {
		if err := checkStoreComponent(c.name, c.value); err != nil {
			return nil, err
		}
	}
	parts := append([]string{s.stateHome}, storeComponents...)
	return append(parts, projectID, goalID), nil
}

func checkStoreComponent(name, value string) error {
	switch {
	case value == "", value == ".", value == "..":
		return fmt.Errorf("clearance store: %s %q is not a usable path component", name, value)
	case strings.ContainsAny(value, `/\`+string(filepath.Separator)+"\x00"):
		return fmt.Errorf("clearance store: %s %q contains a path separator or NUL", name, value)
	}
	return nil
}

// Put strictly parses data as a clearance record and stores it under the key
// its subject names. It returns the record path. Identical bytes already
// stored are idempotent; different bytes are refused and the stored record
// is left unchanged. The record is published atomically: it is written and
// synced to a temporary file in the target directory and then hard-linked
// into place, which, unlike a rename, can never replace an existing record.
func (s FileStore) Put(data []byte) (string, error) {
	r, err := ParseRecord(data)
	if err != nil {
		return "", fmt.Errorf("clearance store: %w", err)
	}
	path, err := s.Path(r.Subject.ProjectID, r.Subject.GoalID, r.Subject.HandoffSHA256)
	if err != nil {
		return "", err
	}
	dirParts, _ := s.dirComponents(r.Subject.ProjectID, r.Subject.GoalID)
	if err := ensureStoreDirs(dirParts); err != nil {
		return "", err
	}

	if _, err := os.Lstat(path); err == nil {
		return path, compareStored(path, data)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("clearance store: %w", err)
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".clearance-*.tmp")
	if err != nil {
		return "", fmt.Errorf("clearance store: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := writeAndClose(tmp, data); err != nil {
		return "", fmt.Errorf("clearance store: write temporary record: %w", err)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return "", fmt.Errorf("clearance store: %w", err)
	}
	if err := os.Link(tmpName, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return path, compareStored(path, data)
		}
		return "", fmt.Errorf("clearance store: publish record: %w", err)
	}
	if err := syncDir(dir); err != nil {
		return "", fmt.Errorf("clearance store: %w", err)
	}
	return path, nil
}

// Get returns the stored record bytes for one key. It refuses a symlink at
// any store component and a record that is not a regular file. It returns
// raw bytes; callers verify them with Verify.
func (s FileStore) Get(projectID, goalID, handoffSHA256 string) ([]byte, error) {
	path, err := s.Path(projectID, goalID, handoffSHA256)
	if err != nil {
		return nil, err
	}
	dirParts, _ := s.dirComponents(projectID, goalID)
	current := dirParts[0]
	for _, part := range dirParts[1:] {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return nil, fmt.Errorf("clearance store: %w", err)
		}
		if err := requirePlainDir(current, info); err != nil {
			return nil, err
		}
	}
	return readStoredRecord(path)
}

// ensureStoreDirs makes sure the state home exists, then walks every store
// component below it, creating missing ones with mode 0700 and refusing any
// that is a symlink or not a directory. The state home itself may be a
// symlink; the store components below it may not.
func ensureStoreDirs(parts []string) error {
	home := parts[0]
	if _, err := os.Stat(home); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(home, 0o700); err != nil {
			return fmt.Errorf("clearance store: create state home: %w", err)
		}
	}
	if info, err := os.Stat(home); err != nil {
		return fmt.Errorf("clearance store: %w", err)
	} else if !info.IsDir() {
		return fmt.Errorf("clearance store: state home %q is not a directory", home)
	}

	current := home
	for _, part := range parts[1:] {
		current = filepath.Join(current, part)
		if err := os.Mkdir(current, 0o700); err == nil {
			if err := os.Chmod(current, 0o700); err != nil {
				return fmt.Errorf("clearance store: %w", err)
			}
		} else if !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("clearance store: %w", err)
		}
		info, err := os.Lstat(current)
		if err != nil {
			return fmt.Errorf("clearance store: %w", err)
		}
		if err := requirePlainDir(current, info); err != nil {
			return err
		}
	}
	return nil
}

func requirePlainDir(path string, info os.FileInfo) error {
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("clearance store: refusing symlinked store component %q", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("clearance store: store component %q is not a directory", path)
	}
	return nil
}

// compareStored succeeds only when the record at path holds exactly data.
func compareStored(path string, data []byte) error {
	existing, err := readStoredRecord(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(existing, data) {
		return fmt.Errorf("clearance store: refusing to replace immutable record %q with different bytes", path)
	}
	return nil
}

// readStoredRecord reads path without following a final symlink and requires
// the opened descriptor to be a regular file.
func readStoredRecord(path string) ([]byte, error) {
	f, err := openNoFollow(path)
	if err != nil {
		if isSymlinkRefusal(err) {
			return nil, fmt.Errorf("clearance store: refusing symlinked record %q", path)
		}
		return nil, fmt.Errorf("clearance store: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("clearance store: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("clearance store: record %q is not a regular file", path)
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("clearance store: %w", err)
	}
	return data, nil
}

func writeAndClose(f *os.File, data []byte) error {
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
