package register

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// installStateFileName is install-state.json's file name inside a target's
// stateDir (D9; see installstate.go for the schema itself).
const installStateFileName = "install-state.json"

// ErrConflict is Decide's ActionRefuse outcome (D9): a same-named entry
// already exists in the runtime's own config, but install-state has no
// record for this target — it is not ours, and writing over it would
// destroy someone else's configuration (R-016/R-017 "Untagged same-named
// entry is refused, not overwritten"). cmd_register.go maps this to exit
// code 6. Every jsonInstall caller returns this wrapped with errors.Is
// still resolving it, so callers can branch on it without string matching.
var ErrConflict = errors.New("register: an entry with this name already exists and is not owned by longterm-mem")

// saveInstallState is InstallState.Save, indirected the way promote's
// nowFunc is, so a test can provoke the one failure the filesystem cannot
// provoke on its own: every way to make the state directory unwritable
// also breaks the LoadInstallState that precedes the config write, so
// jsonInstall would return before writing and the rollback below would
// never run.
var saveInstallState = func(s *InstallState, path string) error { return s.Save(path) }

// jsonInstall is the shared JSON-writer install flow every JSON-backed
// runtime target (claude.go, opencode.go) drives through its own
// containerKey/memberKey/entry shape (11b.8): read the current entry (if
// any) from the runtime's own config, decide the action via the shared D9
// semantics table (Decide), and act on it — insert or replace via
// WriteMember plus recording the new fingerprint in install-state.json,
// refuse via ErrConflict, or do nothing. The runtime's own config file is
// never touched on refuse or noop.
func jsonInstall(target, configPath, stateDir, containerKey, memberKey string, entry json.RawMessage) error {
	statePath := filepath.Join(stateDir, installStateFileName)
	state, err := LoadInstallState(statePath)
	if err != nil {
		return fmt.Errorf("register: %s: %w", target, err)
	}
	record, recordPresent := state.Get(target)

	_, entryPresent, err := readMember(configPath, containerKey, memberKey)
	if err != nil {
		return fmt.Errorf("register: %s: %w", target, err)
	}

	// fingerprintMatches compares install-state's recorded fingerprint
	// against the entry this call is ABOUT TO WRITE (not the bytes
	// currently on disk, decide.go's Decide doc comment) — so a reinstall
	// with the same desired entry (e.g. the same resolved binary path) is
	// a true no-op (skips WriteMember, .bak, and install-state.Save
	// entirely), while a reinstall whose desired entry changed (e.g. a
	// different binary path) always replaces in place, matching R-016/
	// R-017's "Reinstall is idempotent" scenario.
	fingerprintMatches := entryPresent && recordPresent && record.Fingerprint == Fingerprint(entry)

	switch Decide(entryPresent, recordPresent, fingerprintMatches) {
	case ActionNoop:
		return nil
	case ActionRefuse:
		return fmt.Errorf("register: %s: %w", target, ErrConflict)
	case ActionInsert, ActionReplace:
		return installWithRollback(target, configPath, memberKey, state, statePath, Fingerprint(entry), func() error {
			return WriteMember(configPath, containerKey, memberKey, entry)
		})
	default:
		return fmt.Errorf("register: %s: internal error: unreachable Decide outcome", target)
	}
}

// tomlInstall is jsonInstall's TOML counterpart (12a): read
// whether tableKey.memberKey already exists in the runtime's own
// config.toml, decide the action via the shared D9 semantics table
// (Decide), and act on it — insert or replace via WriteTOMLSection plus
// recording the new fingerprint in install-state.json, refuse via
// ErrConflict, or do nothing. It shares jsonInstall's own
// installWithRollback helper rather than re-implementing the config-write/
// install-state-write rollback window, so a TOML-specific writer can never
// reopen the exact CRITICAL 11b-1 fixed for JSON (see installWithRollback's
// doc comment).
func tomlInstall(target, configPath, stateDir, tableKey, memberKey, binary string, newSection []byte) error {
	statePath := filepath.Join(stateDir, installStateFileName)
	state, err := LoadInstallState(statePath)
	if err != nil {
		return fmt.Errorf("register: %s: %w", target, err)
	}
	record, recordPresent := state.Get(target)

	entryPresent, err := readTOMLPresence(configPath, tableKey, memberKey)
	if err != nil {
		return fmt.Errorf("register: %s: %w", target, err)
	}

	// See jsonInstall's identical comment: fingerprintMatches compares
	// against the section this call is ABOUT TO WRITE, not the bytes
	// currently on disk.
	fingerprintMatches := entryPresent && recordPresent && record.Fingerprint == Fingerprint(newSection)

	switch Decide(entryPresent, recordPresent, fingerprintMatches) {
	case ActionNoop:
		return nil
	case ActionRefuse:
		return fmt.Errorf("register: %s: %w", target, ErrConflict)
	case ActionInsert, ActionReplace:
		return installWithRollback(target, configPath, memberKey, state, statePath, Fingerprint(newSection), func() error {
			return WriteTOMLSection(configPath, tableKey, memberKey, binary, newSection)
		})
	default:
		return fmt.Errorf("register: %s: internal error: unreachable Decide outcome", target)
	}
}

// installWithRollback performs the two-effect sequence every runtime
// writer's install flow shares (D9), format-agnostic: call writeConfig to
// make the runtime config edit permanent, then record fingerprint in
// install-state.json under target. The config write and the install-state
// write are two separate effects, and the window between them is the one
// failure a re-run cannot recover from: a config carrying a longterm-mem
// entry install-state has no record of is exactly the shape ActionRefuse
// is for, so every later run would refuse over an entry longterm-mem
// itself wrote (11b-1 CRITICAL). If saveInstallState fails, writeConfig's
// effect is therefore rolled back to the bytes configPath had before it
// ran, and the caller is told the entry was withdrawn.
func installWithRollback(target, configPath, label string, state *InstallState, statePath, fingerprint string, writeConfig func() error) error {
	before, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("register: %s: read %s: %w", target, configPath, err)
	}
	mode := os.FileMode(0o600)
	if info, statErr := os.Stat(configPath); statErr == nil {
		mode = info.Mode().Perm()
	}
	if err := writeConfig(); err != nil {
		return fmt.Errorf("register: %s: %w", target, err)
	}
	state.Set(target, TargetRecord{Fingerprint: fingerprint})
	if err := saveInstallState(state, statePath); err != nil {
		if rollbackErr := os.WriteFile(configPath, before, mode); rollbackErr != nil {
			return fmt.Errorf("register: %s: %w (and restoring %s failed: %v — remove the %q entry by hand before re-running)", target, err, configPath, rollbackErr, label)
		}
		return fmt.Errorf("register: %s: %w (the %q entry was withdrawn from %s)", target, err, label, configPath)
	}
	return nil
}

// readMember reads the current value at containerKey.memberKey inside the
// JSON document at path. This is a read-only inspection, not the byte-
// preserving Splice write path — decoding into a generic map is fine here;
// only WRITES must never decode-reencode the whole document (D9's
// byte-identity contract binds Splice/WriteMember, not this read side). A
// config file that does not exist yet, or that has no containerKey or
// memberKey, is not an error — it just means the entry is not present.
func readMember(path, containerKey, memberKey string) (json.RawMessage, bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read %s: %w", path, err)
	}

	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, false, fmt.Errorf("parse %s: %w", path, err)
	}
	containerRaw, ok := doc[containerKey]
	if !ok {
		return nil, false, nil
	}
	var container map[string]json.RawMessage
	if err := json.Unmarshal(containerRaw, &container); err != nil {
		return nil, false, fmt.Errorf("parse %s %q: %w", path, containerKey, err)
	}
	val, ok := container[memberKey]
	return val, ok, nil
}

// readTOMLPresence reports whether a table header matching
// tableKey.memberKey is present in the TOML document at path — the TOML
// analogue of readMember, used only to answer entryPresent for Decide; it
// never needs the section's actual content, mirroring readMember's own
// scope. A config file that does not exist yet is not an error — it just
// means the entry is not present.
func readTOMLPresence(path, tableKey, memberKey string) (bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	return locateTOMLSection(raw, tableKey, memberKey).found, nil
}
