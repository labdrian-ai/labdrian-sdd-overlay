package settings

// MainAgentKey is the top-level settings key that makes a Claude Code main session run as a named
// agent from ~/.claude/agents, and MainAgentName is the one value the overlay owns: the foreman.
// Any other value of the key belongs to someone else: Merge leaves it alone, Remove keeps it, and
// status reports the conflict (S6).
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
