package register

import (
	"encoding/json"
	"fmt"
	"path/filepath"
)

// claudeConfigFileName is Claude Code's MCP server registry file name,
// ~/.claude.json — a sibling of ~/.claude/, not a file inside it (that
// directory holds settings.json's hooks, a different config this package
// never touches).
const claudeConfigFileName = ".claude.json"

// claudeContainerKey is the top-level object claude's MCP servers live
// under.
const claudeContainerKey = "mcpServers"

// claudeTarget is install-state's key for this runtime.
const claudeTarget = "claude"

// claudeEntry is the exact shape longterm-mem writes into
// mcpServers.longterm-mem (R-016): `{"type":"stdio","command":"<bin>",
// "args":["mcp"]}`. Field order matches json.Marshal's struct-field order,
// which is what the golden fixtures in testdata/claude were generated
// against.
type claudeEntry struct {
	Type    string   `json:"type"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// RegisterClaude installs (or reinstalls) the ownership-tagged longterm-mem
// MCP server entry into Claude Code's configuration at
// configRoot/.claude.json (R-016), recording the entry's fingerprint under
// stateDir/install-state.json. See jsonInstall for the shared D9 decision
// flow every JSON-backed writer follows: unrelated entries are left byte-
// identical, a reinstall replaces the existing tagged entry in place, and
// an untagged same-named entry is refused with ErrConflict rather than
// overwritten.
func RegisterClaude(configRoot, stateDir, binary string) error {
	configPath := filepath.Join(configRoot, claudeConfigFileName)
	entry, err := json.Marshal(claudeEntry{Type: "stdio", Command: binary, Args: []string{"mcp"}})
	if err != nil {
		return fmt.Errorf("register: %s: marshal entry: %w", claudeTarget, err)
	}
	return jsonInstall(claudeTarget, configPath, stateDir, claudeContainerKey, "longterm-mem", entry)
}
