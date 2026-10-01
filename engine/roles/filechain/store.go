// Package filechain is the file-backed roles.ChainLog: the adapter that keeps
// role handoff chains as one immutable JSON file per record under the user's
// state home.
//
// What a chain is and when a record may join one (roles.VerifyChain,
// roles.AdmitRecord) are the domain's, and pure. This package is the rest: where
// the files live, how they are read without following a symlink, and how a record
// is published so that no writer can replace another's. An append is always the
// same steps, in this order:
//
//  1. parse the record strictly (roles.ParseRoleHandoff), which names its chain;
//  2. load that chain and verify it (LoadChain);
//  3. admit the record to it (roles.AdmitRecord), which is pure;
//  4. make the store directories (private, no symlink at any component) and publish
//     the record: written and synced to a temporary file in the chain's directory
//     and hard-linked into place, which, unlike a rename, can never replace an
//     existing record. Storing identical bytes again is a no-op; different bytes
//     under an existing name are refused.
//
// There is no lock: two writers racing for one name each produce a whole file and
// at most one link can win, so none can lose another's record. That is why this
// store needs none where the workflow log, one growing file, needs one.
//
// The on-disk format and layout are a contract with every earlier version of the
// program, and they do not change here:
// <state home>/labdrian/role-chains/<project_id>/<goal_id>/<chain_id>/<seq>.json,
// seq zero-padded to six digits, the record's own bytes exactly as offered, files
// 0600 in directories of mode 0700. testdata holds a chain recorded from the
// version that kept this code in the roles package, and the tests read it, extend
// it, and write it again, byte for byte.
//
// It is built on engine/statestore (the state home, the directory chain, the
// no-follow read, the immutable publish). Its error messages are the ones the store
// printed when it lived in the roles package ("role chain store: ..."). On a
// platform without a no-follow open (anything but linux and darwin) reading a
// record fails closed instead of reading one it cannot vouch for.
package filechain

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/roles"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/statestore"
)

// Store is a roles.ChainLog.
var _ roles.ChainLog = Store{}

// storeComponents are the fixed directories under the state home.
var storeComponents = []string{"labdrian", "role-chains"}

// Store keeps role handoff chains at
// <state home>/labdrian/role-chains/<project_id>/<goal_id>/<chain_id>/<seq>.json
// (seq zero-padded to six digits), outside every worktree and outside Engram. The
// state home is $XDG_STATE_HOME, or $HOME/.local/state when XDG_STATE_HOME is
// unset. It mirrors engine/shaper's FileStore: records are immutable, store
// directories it creates are 0700, record files are 0600, and a symlink at any store
// component is refused. This store grants no execution authority; it only persists
// and verifies data.
type Store struct {
	stateHome string
}

// NewStore resolves the store from the environment (see statestore.Home). A set but
// relative XDG_STATE_HOME, and an unset, empty, or relative HOME fallback, are
// refused.
func NewStore() (Store, error) {
	stateHome, err := statestore.Home()
	if err != nil {
		return Store{}, fmt.Errorf("role chain store: %w", err)
	}
	return Store{stateHome: stateHome}, nil
}

// dirParts returns the state home followed by every store directory component for
// one chain key, validating projectID, goalID, and chainID as safe single path
// components.
func (s Store) dirParts(projectID, goalID, chainID string) ([]string, error) {
	if s.stateHome == "" {
		return nil, errors.New("role chain store: store is not initialized; use NewStore")
	}
	for _, c := range []struct{ name, value string }{
		{"project_id", projectID}, {"goal_id", goalID}, {"chain_id", chainID},
	} {
		if err := checkComponent(c.name, c.value); err != nil {
			return nil, err
		}
	}
	parts := append([]string{s.stateHome}, storeComponents...)
	return append(parts, projectID, goalID, chainID), nil
}

func checkComponent(name, value string) error {
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

// LoadChain reads every record of one chain, in seq order, verifies the whole chain
// with roles.VerifyChain, and returns it. A chain that does not exist yet returns an
// empty, nil-error slice: appending to it is simply the first record. It refuses a
// symlink at any store component, a record file that is not a regular file, and a
// record whose filename does not match its own declared seq.
func (s Store) LoadChain(projectID, goalID, chainID string) ([]roles.ChainRecord, error) {
	parts, err := s.dirParts(projectID, goalID, chainID)
	if err != nil {
		return nil, err
	}
	if err := statestore.CheckDirs(parts); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("role chain store: %w", err)
	}
	dirPath := filepath.Join(parts...)

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("role chain store: %w", err)
	}
	var names []string
	for _, e := range entries {
		if e.Type()&fs.ModeSymlink != 0 {
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

	records := make([]roles.ChainRecord, 0, len(names))
	for _, name := range names {
		data, err := readRecord(filepath.Join(dirPath, name))
		if err != nil {
			return nil, err
		}
		h, err := roles.ParseRoleHandoff(data)
		if err != nil {
			return nil, fmt.Errorf("role chain store: record %q: %w", name, err)
		}
		if want := recordFileName(h.Seq); name != want {
			return nil, fmt.Errorf("role chain store: record %q declares seq %d, want filename %q", name, h.Seq, want)
		}
		records = append(records, roles.ChainRecord{Raw: data, Handoff: h})
	}
	if err := roles.VerifyChain(records); err != nil {
		return nil, fmt.Errorf("role chain store: %w", err)
	}
	return records, nil
}

// Append strictly parses data as a RoleHandoff record, loads and verifies the
// existing chain it declares, admits the record to it (roles.AdmitRecord), and stores
// it. It returns the record path. Identical bytes already stored at the same seq are
// idempotent; different bytes are refused and the stored record is left unchanged.
func (s Store) Append(data []byte) (string, error) {
	h, err := roles.ParseRoleHandoff(data)
	if err != nil {
		return "", fmt.Errorf("role chain store: append: %w", err)
	}
	chain, err := s.LoadChain(h.ProjectID, h.GoalID, h.ChainID)
	if err != nil {
		return "", fmt.Errorf("role chain store: append: %w", err)
	}
	if err := roles.AdmitRecord(chain, roles.ChainRecord{Raw: data, Handoff: h}); err != nil {
		return "", err
	}

	parts, err := s.dirParts(h.ProjectID, h.GoalID, h.ChainID)
	if err != nil {
		return "", err
	}
	path := filepath.Join(filepath.Join(parts...), recordFileName(h.Seq))
	if err := statestore.EnsureDirs(parts); err != nil {
		return "", fmt.Errorf("role chain store: %w", err)
	}
	if err := statestore.Publish(path, data); err != nil {
		return "", recordError(path, err)
	}
	return path, nil
}

// readRecord reads the record at path without following a final symlink and requires
// the opened descriptor to be a regular file.
func readRecord(path string) ([]byte, error) {
	data, err := statestore.ReadFile(path, 0)
	if err != nil {
		return nil, recordError(path, err)
	}
	return data, nil
}

// recordError gives a failure to read or publish the record at path the wording this
// store has always used for it. A refusal to replace a stored record with different
// bytes (statestore.ErrImmutable) already reads as it always did; it only gains the
// store's name.
func recordError(path string, err error) error {
	switch {
	case errors.Is(err, statestore.ErrSymlink):
		return fmt.Errorf("role chain store: refusing symlinked record %q", path)
	case errors.Is(err, statestore.ErrNotRegular):
		return fmt.Errorf("role chain store: record %q is not a regular file", path)
	}
	return fmt.Errorf("role chain store: %w", err)
}
