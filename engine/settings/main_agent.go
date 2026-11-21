package settings

import "fmt"

// MainAgentKey is the top-level settings key that makes a Claude Code main session run as a named
// agent from ~/.claude/agents, and MainAgentName is the one value the overlay owns: the foreman.
// Any other value of the key belongs to someone else: Merge leaves it alone, Remove keeps it, and
// status reports the conflict (S6). Ownership is by value alone: "foreman" names the agent file this
// overlay installs, so an uninstall that removes that agent must not leave the key pointing at it.
const (
	MainAgentKey  = "agent"
	MainAgentName = "foreman"
)

// mainAgentKey is the key as the shared ownership rules (ownedKey) read it.
var mainAgentKey = ownedKey{key: MainAgentKey, name: MainAgentName}

// MainAgentState is what the settings say about the main-session agent key.
type MainAgentState = KeyState

const (
	// MainAgentMissing: the key is absent.
	MainAgentMissing = KeyMissing
	// MainAgentOwned: the key holds exactly MainAgentName.
	MainAgentOwned = KeyOwned
	// MainAgentConflict: the key holds another value, which is not ours to change.
	MainAgentConflict = KeyConflict
)

// MainAgentValueJSON is the value of the key in root as JSON, for a message that names it: a string
// in quotes, anything else as it would be written in the file. It is empty when the key is absent.
func MainAgentValueJSON(root map[string]interface{}) string { return mainAgentKey.valueJSON(root) }

// ForeignMainAgentNotice is the sentence an install says when the key at location (the settings file) holds
// the foreign value valueJSON: it was left as it is, so the foreman main session was not enabled. The
// install verb of the command line and the Claude runtime adapter both say it, from here.
func ForeignMainAgentNotice(location, valueJSON string) string {
	return fmt.Sprintf("%s holds \"%s\": %s; it was left as it is, so the foreman main session was not enabled (set it to %q or remove the key, then run install-hooks again)",
		location, MainAgentKey, valueJSON, MainAgentName)
}

// MainAgent reports the state of the main-session agent key in root.
func MainAgent(root map[string]interface{}) MainAgentState { return mainAgentKey.state(root) }

// addMainAgent sets the key to MainAgentName when it is absent and reports whether it did. A key
// that already holds the name needs nothing, and one that holds another value is never overwritten.
func addMainAgent(root map[string]interface{}) bool { return mainAgentKey.add(root) }

// removeMainAgent deletes the key only when it holds exactly MainAgentName and reports whether it did.
func removeMainAgent(root map[string]interface{}) bool { return mainAgentKey.remove(root) }
