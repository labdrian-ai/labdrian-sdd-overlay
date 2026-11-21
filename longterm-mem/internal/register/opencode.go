package register

import (
	"encoding/json"
	"fmt"
	"os"
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
// and never touches anyone else's.
const opencodeLegacyContainerKey = "mcp"

// opencodeTarget is install-state's key for this runtime.
const opencodeTarget = "opencode"

// opencodeEntry is the exact shape longterm-mem writes into
// mcp.servers.longterm-mem (R-017, S12): `{"type":"local","command":
// ["<bin>","mcp"],"codemode":true}` — opencode v2's own shape: the whole
// argv (binary included) as one array, and `codemode`, which opencode 2.2.7's
// embedded config schema describes as "Expose this server's tools through Code
// Mode. Defaults to true." (written explicitly so the entry does not depend on
// the default).
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
// configRoot/opencode.json (R-017). See RegisterOpencodeMigrating.
func (w Writer) RegisterOpencode(configRoot, stateDir, binary string) error {
	_, err := w.RegisterOpencodeMigrating(configRoot, stateDir, binary)
	return err
}

// RegisterOpencodeMigrating is RegisterOpencode that also says whether it
// removed the opencode 1.x entry (mcp.longterm-mem) an earlier version wrote.
//
// It follows jsonInstall's D9 decision flow for the v2 entry, and decides the
// 1.x entry in the same breath, BEFORE anything is written: the 1.x entry is
// ours when its exact bytes are what install-state recorded, or when it
// canonically equals the 1.x entry of the binary being registered. A 1.x entry
// that is neither is a stranger's, a conflict exactly as it was before the v2
// shape, and so is an `mcp.servers` that is a 1.x server named "servers"
// rather than the v2 container. The v2 insert and the 1.x removal are applied
// to the same bytes and land in ONE write, so the .bak holds the file as the
// call found it and a failure leaves no half state.
func (w Writer) RegisterOpencodeMigrating(configRoot, stateDir, binary string) (migrated bool, err error) {
	configPath := filepath.Join(configRoot, opencodeConfigFileName)
	entry, err := json.Marshal(opencodeEntry{Type: "local", Command: []string{binary, "mcp"}, Codemode: true})
	if err != nil {
		return false, fmt.Errorf("register: %s: marshal entry: %w", opencodeTarget, err)
	}
	legacyEntry, err := json.Marshal(opencodeLegacyEntry{Type: "local", Command: []string{binary, "mcp"}, Enabled: true})
	if err != nil {
		return false, fmt.Errorf("register: %s: marshal entry: %w", opencodeTarget, err)
	}
	if w.State == nil {
		return false, fmt.Errorf("register: %s: %w", opencodeTarget, errNoInstallState)
	}
	state, err := w.State.Load(stateDir)
	if err != nil {
		return false, fmt.Errorf("register: %s: %w", opencodeTarget, err)
	}
	record, recordPresent := state.Get(opencodeTarget)

	current, entryPresent, err := readMember(configPath, opencodeContainerKey, "longterm-mem")
	if err != nil {
		return false, fmt.Errorf("register: %s: %w", opencodeTarget, err)
	}
	legacy, legacyPresent, err := readMember(configPath, opencodeLegacyContainerKey, "longterm-mem")
	if err != nil {
		return false, fmt.Errorf("register: %s: %w", opencodeTarget, err)
	}
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return false, fmt.Errorf("register: %s: read %s: %w", opencodeTarget, configPath, err)
	}
	conflict := fmt.Errorf("register: %s: %s: %w", opencodeTarget, configPath, ErrConflict)

	if serversIsA1xServer(raw) {
		return false, conflict
	}
	legacyOurs := false
	if legacyPresent {
		ownedLegacy := ownershipFingerprintJSON(legacyEntry)
		legacyOurs = legacy != nil &&
			((recordPresent && Fingerprint(legacy) == record.Fingerprint) ||
				(ownedLegacy != "" && ownershipFingerprintJSON(legacy) == ownedLegacy))
		if !legacyOurs {
			return false, conflict
		}
	}

	ownedFingerprint := ownershipFingerprintJSON(entry)
	entryOwned := entryPresent && current != nil && ownedFingerprint != "" &&
		ownershipFingerprintJSON(current) == ownedFingerprint
	fingerprintMatches := entryPresent && recordPresent && record.Fingerprint == Fingerprint(entry)
	action := Decide(entryPresent, recordPresent, entryOwned, fingerprintMatches)
	if action == ActionRefuse {
		return false, conflict
	}

	writesV2 := action == ActionInsert || action == ActionReplace
	if !writesV2 && !legacyOurs {
		if action == ActionAdopt {
			return false, w.adoptExistingEntry(opencodeTarget, stateDir, state, Fingerprint(entry))
		}
		return false, nil
	}

	next := raw
	if writesV2 {
		if next, err = spliceAny(next, opencodeContainerKey, "longterm-mem", entry); err != nil {
			return false, fmt.Errorf("register: %s: splice %s: %w", opencodeTarget, configPath, err)
		}
	}
	if legacyOurs {
		if next, err = Remove(next, opencodeLegacyContainerKey, "longterm-mem"); err != nil {
			return false, fmt.Errorf("register: %s: remove the 1.x entry from %s: %w", opencodeTarget, configPath, err)
		}
		if jsonMemberPresent(next, opencodeLegacyContainerKey, "longterm-mem") {
			return false, fmt.Errorf("register: %s: removal of the 1.x entry from %s did not remove it, not written", opencodeTarget, configPath)
		}
	}
	if !json.Valid(next) {
		return false, fmt.Errorf("register: %s: the edit of %s would produce invalid JSON, not written", opencodeTarget, configPath)
	}
	err = w.installWithRollback(opencodeTarget, configPath, "longterm-mem", stateDir, state, Fingerprint(entry), func() error {
		return replaceConfig(configPath, raw, next)
	})
	return err == nil && legacyOurs, err
}

// unregisterOpencode is jsonUninstall for opencode: the v2 entry and, when
// install-state recorded its exact bytes, the 1.x entry are both decided
// first and taken out in ONE write, so a refusal or a failure never leaves one
// removed and the other not, and the .bak holds the file as it was found. A
// 1.x mcp.longterm-mem with no record to show it is ours is the stranger's:
// unmanaged, left in place.
func (w Writer) unregisterOpencode(configRoot, stateDir string) (UnregisterOutcome, error) {
	configPath := filepath.Join(configRoot, opencodeConfigFileName)
	if w.State == nil {
		return 0, fmt.Errorf("unregister: %s: %w", opencodeTarget, errNoInstallState)
	}
	state, err := w.State.Load(stateDir)
	if err != nil {
		return 0, fmt.Errorf("unregister: %s: %w", opencodeTarget, err)
	}
	record, recordPresent := state.Get(opencodeTarget)

	current, entryPresent, err := readMember(configPath, opencodeContainerKey, "longterm-mem")
	if err != nil {
		return 0, fmt.Errorf("unregister: %s: %w", opencodeTarget, err)
	}
	legacy, legacyPresent, err := readMember(configPath, opencodeLegacyContainerKey, "longterm-mem")
	if err != nil {
		return 0, fmt.Errorf("unregister: %s: %w", opencodeTarget, err)
	}
	legacyOurs := legacyPresent && legacy != nil && recordPresent && Fingerprint(legacy) == record.Fingerprint
	fingerprintMatches := entryPresent && recordPresent && current != nil && record.Fingerprint == Fingerprint(current)

	switch Decide(entryPresent, recordPresent, uninstallCannotDeriveOwnership, fingerprintMatches) {
	case ActionInsert:
		if legacyPresent {
			return UnregisterUnmanaged, nil
		}
		return UnregisterNoop, nil
	case ActionRefuse, ActionAdopt:
		return UnregisterUnmanaged, nil
	case ActionNoop, ActionReplace:
		if entryPresent || legacyOurs {
			if err := w.removeOpencodeEntries(configPath, entryPresent, legacyOurs); err != nil {
				return 0, fmt.Errorf("unregister: %s: %w", opencodeTarget, err)
			}
		}
		state.Delete(opencodeTarget)
		if err := w.State.Save(stateDir, state); err != nil {
			return 0, fmt.Errorf("unregister: %s: %w", opencodeTarget, err)
		}
		return UnregisterRemoved, nil
	default:
		return 0, fmt.Errorf("unregister: %s: internal error: unreachable Decide outcome", opencodeTarget)
	}
}

// removeOpencodeEntries takes the v2 entry and/or the 1.x entry out of the
// config in one write, validated before the filesystem is touched.
func (w Writer) removeOpencodeEntries(configPath string, v2, legacy bool) error {
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", configPath, err)
	}
	next := raw
	if v2 {
		if next, err = removeAny(next, opencodeContainerKey, "longterm-mem"); err != nil {
			return fmt.Errorf("remove from %s: %w", configPath, err)
		}
	}
	if legacy {
		if next, err = Remove(next, opencodeLegacyContainerKey, "longterm-mem"); err != nil {
			return fmt.Errorf("remove from %s: %w", configPath, err)
		}
	}
	if !json.Valid(next) {
		return fmt.Errorf("removing the entries from %s would produce invalid JSON, not written", configPath)
	}
	if (v2 && jsonMemberPresent(next, opencodeContainerKey, "longterm-mem")) ||
		(legacy && jsonMemberPresent(next, opencodeLegacyContainerKey, "longterm-mem")) {
		return fmt.Errorf("removal from %s did not remove the entry, not written", configPath)
	}
	return replaceConfig(configPath, raw, next)
}

// serversIsA1xServer reports whether root.mcp.servers is not opencode v2's
// container but an opencode 1.x server that is called "servers": an object
// with a `type` or `command` member that is not itself an object (the v2
// container holds only servers, which are objects).
func serversIsA1xServer(raw []byte) bool {
	var doc, mcp, servers map[string]json.RawMessage
	if json.Unmarshal(emptyDocumentAsObject(raw), &doc) != nil ||
		json.Unmarshal(doc["mcp"], &mcp) != nil ||
		json.Unmarshal(mcp["servers"], &servers) != nil {
		return false
	}
	for _, key := range []string{"type", "command"} {
		if value, ok := servers[key]; ok {
			var asObject map[string]json.RawMessage
			if json.Unmarshal(value, &asObject) != nil {
				return true
			}
		}
	}
	return false
}
