package roles

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ChainStore holds role handoff chains at
// <state home>/labdrian/role-chains/<project_id>/<goal_id>/<chain_id>/<seq>.json
// (seq zero-padded to six digits), outside every worktree and outside
// Engram. The state home is $XDG_STATE_HOME, or $HOME/.local/state when
// XDG_STATE_HOME is unset. It mirrors engine/shaper's FileStore: records are
// immutable (storing identical bytes again is idempotent, storing different
// bytes under an existing key is refused), store directories it creates are
// 0700, record files are 0600, and a symlink at any store component is
// refused. This store grants no execution authority; it only persists and
// verifies data.
type ChainStore struct {
	stateHome string
}

// chainStoreComponents are the fixed directories under the state home.
var chainStoreComponents = []string{"labdrian", "role-chains"}

// NewChainStore resolves the store from the environment, exactly as
// shaper.NewFileStore does. A set but relative XDG_STATE_HOME, and an unset,
// empty, or relative HOME fallback, are refused.
func NewChainStore() (ChainStore, error) {
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		if !filepath.IsAbs(xdg) {
			return ChainStore{}, fmt.Errorf("role chain store: XDG_STATE_HOME %q is not absolute", xdg)
		}
		return ChainStore{stateHome: filepath.Clean(xdg)}, nil
	}
	home := os.Getenv("HOME")
	if home == "" || !filepath.IsAbs(home) {
		return ChainStore{}, fmt.Errorf("role chain store: XDG_STATE_HOME is unset and HOME %q is not an absolute path", home)
	}
	return ChainStore{stateHome: filepath.Join(home, ".local", "state")}, nil
}

// chainDirParts returns the state home followed by every store directory
// component for one chain key, validating projectID, goalID, and chainID as
// safe single path components.
func (s ChainStore) chainDirParts(projectID, goalID, chainID string) ([]string, error) {
	if s.stateHome == "" {
		return nil, fmt.Errorf("role chain store: store is not initialized; use NewChainStore")
	}
	for _, c := range []struct{ name, value string }{
		{"project_id", projectID}, {"goal_id", goalID}, {"chain_id", chainID},
	} {
		if err := checkChainStoreComponent(c.name, c.value); err != nil {
			return nil, err
		}
	}
	parts := append([]string{s.stateHome}, chainStoreComponents...)
	return append(parts, projectID, goalID, chainID), nil
}

// chainDir returns the chain's directory path without touching the
// filesystem.
func (s ChainStore) chainDir(projectID, goalID, chainID string) (string, error) {
	parts, err := s.chainDirParts(projectID, goalID, chainID)
	if err != nil {
		return "", err
	}
	return filepath.Join(parts...), nil
}

func checkChainStoreComponent(name, value string) error {
	switch {
	case value == "", value == ".", value == "..":
		return fmt.Errorf("role chain store: %s %q is not a usable path component", name, value)
	case strings.ContainsAny(value, `/\`+string(filepath.Separator)+"\x00"):
		return fmt.Errorf("role chain store: %s %q contains a path separator or NUL", name, value)
	}
	return nil
}

// recordFileName is the zero-padded record filename for seq.
func recordFileName(seq int) string {
	return fmt.Sprintf("%06d.json", seq)
}

// RecordPath returns the record path for one (projectID, goalID, chainID,
// seq) key without touching the filesystem.
func (s ChainStore) RecordPath(projectID, goalID, chainID string, seq int) (string, error) {
	if seq < 1 {
		return "", fmt.Errorf("role chain store: seq must be >= 1, got %d", seq)
	}
	dir, err := s.chainDir(projectID, goalID, chainID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, recordFileName(seq)), nil
}

// LoadChain reads every record of one chain, in seq order, verifies the
// whole chain with VerifyChain, and returns it. A chain that does not exist
// yet returns an empty, nil-error slice: appending to it is simply the first
// record. It refuses a symlink at any store component, a record file that is
// not a regular file, and a record whose filename does not match its own
// declared seq.
func (s ChainStore) LoadChain(projectID, goalID, chainID string) ([]ChainRecord, error) {
	parts, err := s.chainDirParts(projectID, goalID, chainID)
	if err != nil {
		return nil, err
	}
	current := parts[0]
	for _, part := range parts[1:] {
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if errors.Is(statErr, os.ErrNotExist) {
			return nil, nil
		}
		if statErr != nil {
			return nil, fmt.Errorf("role chain store: %w", statErr)
		}
		if err := requirePlainDir(current, info); err != nil {
			return nil, err
		}
	}
	dirPath := current

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("role chain store: %w", err)
	}
	var names []string
	for _, e := range entries {
		if e.Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("role chain store: refusing symlinked record %q", filepath.Join(dirPath, e.Name()))
		}
		if !e.Type().IsRegular() {
			continue
		}
		if strings.HasSuffix(e.Name(), ".json") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	records := make([]ChainRecord, 0, len(names))
	for _, name := range names {
		data, err := readStoredChainRecord(filepath.Join(dirPath, name))
		if err != nil {
			return nil, err
		}
		h, err := ParseRoleHandoff(data)
		if err != nil {
			return nil, fmt.Errorf("role chain store: record %q: %w", name, err)
		}
		if want := recordFileName(h.Seq); name != want {
			return nil, fmt.Errorf("role chain store: record %q declares seq %d, want filename %q", name, h.Seq, want)
		}
		records = append(records, ChainRecord{Raw: data, Handoff: h})
	}
	if err := VerifyChain(records); err != nil {
		return nil, fmt.Errorf("role chain store: %w", err)
	}
	return records, nil
}

// Append strictly parses data as a RoleHandoff record, loads and verifies
// the existing chain it declares, validates that appending data keeps the
// chain well-formed, and stores it. It returns the record path. Identical
// bytes already stored at the same seq are idempotent; different bytes are
// refused and the stored record is left unchanged. The record is published
// atomically: written and synced to a temporary file in the target
// directory, then hard-linked into place, which, unlike a rename, can never
// replace an existing record.
func (s ChainStore) Append(data []byte) (string, error) {
	h, err := ParseRoleHandoff(data)
	if err != nil {
		return "", fmt.Errorf("role chain store: append: %w", err)
	}
	records, err := s.LoadChain(h.ProjectID, h.GoalID, h.ChainID)
	if err != nil {
		return "", fmt.Errorf("role chain store: append: %w", err)
	}
	candidate := ChainRecord{Raw: data, Handoff: h}
	switch {
	case h.Seq >= 1 && h.Seq <= len(records):
		// Re-appending an already-stored seq. The stored chain was already
		// verified by LoadChain; idempotency (identical bytes) or refusal
		// (different bytes) is decided below, at the write step.
	case h.Seq == len(records)+1:
		extended := make([]ChainRecord, 0, len(records)+1)
		extended = append(extended, records...)
		extended = append(extended, candidate)
		if err := VerifyChain(extended); err != nil {
			return "", fmt.Errorf("role chain store: append: %w", err)
		}
	default:
		return "", fmt.Errorf("role chain store: append: seq %d is neither the next seq (%d) nor an existing one (chain has %d records)", h.Seq, len(records)+1, len(records))
	}

	path, err := s.RecordPath(h.ProjectID, h.GoalID, h.ChainID, h.Seq)
	if err != nil {
		return "", err
	}
	dirParts, _ := s.chainDirParts(h.ProjectID, h.GoalID, h.ChainID)
	if err := ensureChainDirs(dirParts); err != nil {
		return "", err
	}

	if _, err := os.Lstat(path); err == nil {
		return path, compareStoredChainRecord(path, data)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("role chain store: %w", err)
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".role-handoff-*.tmp")
	if err != nil {
		return "", fmt.Errorf("role chain store: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := writeAndCloseChainRecord(tmp, data); err != nil {
		return "", fmt.Errorf("role chain store: write temporary record: %w", err)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return "", fmt.Errorf("role chain store: %w", err)
	}
	if err := os.Link(tmpName, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return path, compareStoredChainRecord(path, data)
		}
		return "", fmt.Errorf("role chain store: publish record: %w", err)
	}
	if err := syncChainDir(dir); err != nil {
		return "", fmt.Errorf("role chain store: %w", err)
	}
	return path, nil
}

// ensureChainDirs makes sure the state home exists, then walks every store
// component below it, creating missing ones with mode 0700 and refusing any
// that is a symlink or not a directory.
func ensureChainDirs(parts []string) error {
	home := parts[0]
	if _, err := os.Stat(home); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(home, 0o700); err != nil {
			return fmt.Errorf("role chain store: create state home: %w", err)
		}
	}
	if info, err := os.Stat(home); err != nil {
		return fmt.Errorf("role chain store: %w", err)
	} else if !info.IsDir() {
		return fmt.Errorf("role chain store: state home %q is not a directory", home)
	}

	current := home
	for _, part := range parts[1:] {
		current = filepath.Join(current, part)
		if err := os.Mkdir(current, 0o700); err == nil {
			if err := os.Chmod(current, 0o700); err != nil {
				return fmt.Errorf("role chain store: %w", err)
			}
		} else if !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("role chain store: %w", err)
		}
		info, err := os.Lstat(current)
		if err != nil {
			return fmt.Errorf("role chain store: %w", err)
		}
		if err := requirePlainDir(current, info); err != nil {
			return err
		}
	}
	return nil
}

func requirePlainDir(path string, info os.FileInfo) error {
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("role chain store: refusing symlinked store component %q", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("role chain store: store component %q is not a directory", path)
	}
	return nil
}

// compareStoredChainRecord succeeds only when the record at path holds
// exactly data.
func compareStoredChainRecord(path string, data []byte) error {
	existing, err := readStoredChainRecord(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(existing, data) {
		return fmt.Errorf("role chain store: refusing to replace immutable record %q with different bytes", path)
	}
	return nil
}

// readStoredChainRecord reads path without following a final symlink and
// requires the opened descriptor to be a regular file.
func readStoredChainRecord(path string) ([]byte, error) {
	f, err := openNoFollow(path)
	if err != nil {
		if isSymlinkRefusal(err) {
			return nil, fmt.Errorf("role chain store: refusing symlinked record %q", path)
		}
		return nil, fmt.Errorf("role chain store: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("role chain store: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("role chain store: record %q is not a regular file", path)
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("role chain store: %w", err)
	}
	return data, nil
}

func writeAndCloseChainRecord(f *os.File, data []byte) error {
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

func syncChainDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
