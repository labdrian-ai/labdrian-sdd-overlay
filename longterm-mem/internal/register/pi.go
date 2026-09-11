package register

import (
	"encoding/json"
	"fmt"
	"path/filepath"
)

// piConfigFileName is the MCP config file package.json's "pi":{"mcp":...}
// key points at inside the built labdrian-pi package (A1, R-005) —
// pi-mcp-adapter reads mcp.json's own top-level mcpServers object.
const piConfigFileName = "mcp.json"

// piContainerKey is the top-level object Pi's MCP servers live under.
const piContainerKey = "mcpServers"

// piTarget is install-state's key for this runtime.
const piTarget = "pi"

// piEntry mirrors claudeEntry's shape exactly: pi-mcp-adapter reads the
// same stdio {type, command, args} object claude does (A1). The key this
// writer targets inside mcpServers stays the bare "longterm-mem", matching
// every other writer's memberKey and jsonInstall's own D9 ownership
// fingerprinting — pi-mcp-adapter prefixes the loaded server with the
// package's own sanitized name (research C5) at LOAD time, not something
// this writer does to the key it writes.
type piEntry struct {
	Type    string   `json:"type"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// RegisterPi installs (or reinstalls) the ownership-tagged longterm-mem
// MCP entry into the built labdrian-pi package's mcp.json
// (configRoot/mcp.json — configRoot IS the package directory for this
// target, not a user config root, C3), recording the entry's fingerprint
// under stateDir/install-state.json. See jsonInstall for the shared D9
// decision flow every JSON-backed writer follows.
func RegisterPi(configRoot, stateDir, binary string) error {
	configPath := filepath.Join(configRoot, piConfigFileName)
	entry, err := json.Marshal(piEntry{Type: "stdio", Command: binary, Args: []string{"mcp"}})
	if err != nil {
		return fmt.Errorf("register: %s: marshal entry: %w", piTarget, err)
	}
	return jsonInstall(piTarget, configPath, stateDir, piContainerKey, "longterm-mem", entry)
}
