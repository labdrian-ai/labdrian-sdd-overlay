package register

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
)

// opencodeConfigFileName is opencode's own config file name, living directly
// inside its config root (DefaultOpenCodeConfigRoot()-equivalent —
// resolved by the caller, cmd_register.go, not by this package).
const opencodeConfigFileName = "opencode.json"

// opencodeContainerKey is the object opencode v2's MCP servers live under,
// root.mcp.servers (spelled "mcp/servers", see nestedSep).
const opencodeContainerKey = "mcp" + nestedSep + "servers"

// opencodeLegacyContainerKey is where opencode 1.x kept them, root.mcp. This
// writer used to write there; it still recognises and migrates its own entry
// (migrateLegacyOpencode) and never touches anyone else's.
const opencodeLegacyContainerKey = "mcp"

// opencodeTarget is install-state's key for this runtime.
const opencodeTarget = "opencode"

// opencodeEntry is the exact shape longterm-mem writes into
// mcp.servers.longterm-mem (R-017, S12): `{"type":"local","command":
// ["<bin>","mcp"],"codemode":true}` — opencode v2's own shape: the whole
// argv (binary included) as one array, and `codemode` (its default, written
// explicitly) exposing the server's tools through Code Mode.
type opencodeEntry struct {
	Type     string   `json:"type"`
	Command  []string `json:"command"`
	Codemode bool     `json:"codemode"`
}

// opencodeLegacyEntry is the opencode 1.x shape this writer used to write
// into mcp.longterm-mem. It exists only to recognise that entry.
type opencodeLegacyEntry struct {
	Type    string   `json:"type"`
	Command []string `json:"command"`
	Enabled bool     `json:"enabled"`
}

// RegisterOpencode installs (or reinstalls) the ownership-tagged
// longterm-mem MCP entry into opencode's configuration at
// configRoot/opencode.json (R-017), recording the entry's fingerprint under
// stateDir/install-state.json. See jsonInstall for the shared D9 decision
// flow every JSON-backed writer follows. A 1.x entry this writer wrote
// earlier is removed once the v2 entry is in place.
func (w Writer) RegisterOpencode(configRoot, stateDir, binary string) error {
	configPath := filepath.Join(configRoot, opencodeConfigFileName)
	entry, err := json.Marshal(opencodeEntry{Type: "local", Command: []string{binary, "mcp"}, Codemode: true})
	if err != nil {
		return fmt.Errorf("register: %s: marshal entry: %w", opencodeTarget, err)
	}
	recorded, err := w.recordedFingerprint(stateDir)
	if err != nil {
		return fmt.Errorf("register: %s: %w", opencodeTarget, err)
	}
	if err := w.jsonInstall(opencodeTarget, configPath, stateDir, opencodeContainerKey, "longterm-mem", entry); err != nil {
		return err
	}
	legacy, err := json.Marshal(opencodeLegacyEntry{Type: "local", Command: []string{binary, "mcp"}, Enabled: true})
	if err != nil {
		return fmt.Errorf("register: %s: marshal entry: %w", opencodeTarget, err)
	}
	return w.migrateLegacyOpencode(configPath, recorded, ownershipFingerprintJSON(legacy))
}

// recordedFingerprint is the fingerprint install-state holds for opencode
// before this call changes it ("" when there is none).
func (w Writer) recordedFingerprint(stateDir string) (string, error) {
	if w.State == nil {
		return "", errNoInstallState
	}
	state, err := w.State.Load(stateDir)
	if err != nil {
		return "", err
	}
	record, _ := state.Get(opencodeTarget)
	return record.Fingerprint, nil
}

// migrateLegacyOpencode removes opencode 1.x's mcp.longterm-mem when it is
// ours: its exact bytes are what install-state last recorded, or (no
// record, or a record of another path) it canonically equals the 1.x entry
// for the binary being registered. Any other entry under that name is
// someone else's and is left byte-identical.
func (w Writer) migrateLegacyOpencode(configPath, recordedFingerprint, ownedLegacyFingerprint string) error {
	current, present, err := readMember(configPath, opencodeLegacyContainerKey, "longterm-mem")
	if err != nil {
		return fmt.Errorf("register: %s: %w", opencodeTarget, err)
	}
	if !present || current == nil {
		return nil
	}
	ours := (recordedFingerprint != "" && Fingerprint(current) == recordedFingerprint) ||
		(ownedLegacyFingerprint != "" && ownershipFingerprintJSON(current) == ownedLegacyFingerprint)
	if !ours {
		return nil
	}
	if err := RemoveMember(configPath, opencodeLegacyContainerKey, "longterm-mem"); err != nil {
		return fmt.Errorf("register: %s: remove the 1.x entry: %w", opencodeTarget, err)
	}
	return nil
}

// unregisterLegacyOpencode is migrateLegacyOpencode's unregister half: before
// the v2 entry is removed and the record cleared, drop a 1.x entry whose
// bytes are exactly what install-state recorded. Without a record there is
// no proof it is ours, so it stays.
func (w Writer) unregisterLegacyOpencode(configPath, stateDir string) error {
	recorded, err := w.recordedFingerprint(stateDir)
	if err != nil {
		if errors.Is(err, errNoInstallState) {
			return nil // jsonUninstall reports it with the right prefix
		}
		return fmt.Errorf("unregister: %s: %w", opencodeTarget, err)
	}
	current, present, err := readMember(configPath, opencodeLegacyContainerKey, "longterm-mem")
	if err != nil {
		return fmt.Errorf("unregister: %s: %w", opencodeTarget, err)
	}
	if !present || current == nil || recorded == "" || Fingerprint(current) != recorded {
		return nil
	}
	if err := RemoveMember(configPath, opencodeLegacyContainerKey, "longterm-mem"); err != nil {
		return fmt.Errorf("unregister: %s: remove the 1.x entry: %w", opencodeTarget, err)
	}
	return nil
}
