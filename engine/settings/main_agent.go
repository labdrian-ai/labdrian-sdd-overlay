package settings

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// MainAgentKey is the top-level settings key that makes a Claude Code main session run as a named
// agent from ~/.claude/agents, and MainAgentName is the one value the overlay owns: the foreman.
// Any other value of the key belongs to someone else: Merge leaves it alone, Remove keeps it, and
// status reports the conflict (S6). Ownership is by value alone: "foreman" names the agent file this
// overlay installs, so an uninstall that removes that agent must not leave the key pointing at it.
const (
	MainAgentKey  = "agent"
	MainAgentName = "foreman"
)

// MainAgentState is what the settings say about the main-session agent key.
type MainAgentState int

const (
	// MainAgentMissing: the key is absent.
	MainAgentMissing MainAgentState = iota
	// MainAgentOwned: the key holds exactly MainAgentName.
	MainAgentOwned
	// MainAgentConflict: the key holds another value, which is not ours to change.
	MainAgentConflict
)

// MainAgentValueJSON is the value of the key in root as JSON, for a message that names it: a string
// in quotes, anything else as it would be written in the file. It is empty when the key is absent.
func MainAgentValueJSON(root map[string]interface{}) string {
	v, ok := root[MainAgentKey]
	if !ok {
		return ""
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return fmt.Sprintf("%v", v)
	}
	return strings.TrimSuffix(buf.String(), "\n")
}

// MainAgent reports the state of the main-session agent key in root.
func MainAgent(root map[string]interface{}) MainAgentState {
	v, ok := root[MainAgentKey]
	if !ok {
		return MainAgentMissing
	}
	if v == MainAgentName {
		return MainAgentOwned
	}
	return MainAgentConflict
}

// addMainAgent sets the key to MainAgentName when it is absent and reports whether it did. A key
// that already holds the name needs nothing, and one that holds another value is never overwritten.
func addMainAgent(root map[string]interface{}) bool {
	if MainAgent(root) != MainAgentMissing {
		return false
	}
	root[MainAgentKey] = MainAgentName
	return true
}

// removeMainAgent deletes the key only when it holds exactly MainAgentName and reports whether it did.
func removeMainAgent(root map[string]interface{}) bool {
	if MainAgent(root) != MainAgentOwned {
		return false
	}
	delete(root, MainAgentKey)
	return true
}
