package register

import (
	"encoding/json"
	"fmt"
	"path/filepath"
)

// opencodeConfigFileName is opencode's own config file name, living directly
// inside its config root (DefaultOpenCodeConfigRoot()-equivalent —
// resolved by the caller, cmd_register.go, not by this package).
const opencodeConfigFileName = "opencode.json"

// opencodeContainerKey is the top-level object opencode's MCP entries live
// under.
const opencodeContainerKey = "mcp"

// opencodeTarget is install-state's key for this runtime.
const opencodeTarget = "opencode"

// opencodeEntry is the exact shape longterm-mem writes into
// mcp.longterm-mem (R-017): `{"type":"local","command":["<bin>","mcp"],
// "enabled":true}` — opencode's own single-argument-list command shape
// (the whole argv, including the binary itself, as one array), unlike
// claude's separate command/args fields.
type opencodeEntry struct {
	Type    string   `json:"type"`
	Command []string `json:"command"`
	Enabled bool     `json:"enabled"`
}

// RegisterOpencode installs (or reinstalls) the ownership-tagged
// longterm-mem MCP entry into opencode's configuration at
// configRoot/opencode.json (R-017), recording the entry's fingerprint under
// stateDir/install-state.json. See jsonInstall for the shared D9 decision
// flow every JSON-backed writer follows.
func RegisterOpencode(configRoot, stateDir, binary string) error {
	configPath := filepath.Join(configRoot, opencodeConfigFileName)
	entry, err := json.Marshal(opencodeEntry{Type: "local", Command: []string{binary, "mcp"}, Enabled: true})
	if err != nil {
		return fmt.Errorf("register: %s: marshal entry: %w", opencodeTarget, err)
	}
	return jsonInstall(opencodeTarget, configPath, stateDir, opencodeContainerKey, "longterm-mem", entry)
}
