package settings

import "fmt"

// OutputStyleKey is the top-level settings key that names the output style of a Claude Code session,
// and OutputStyleName is the one value the overlay owns: the labdrian style that output-styles/labdrian.md
// deploys to ~/.claude/output-styles (T27). Ownership is by value alone, as for the main-session agent:
// any other value ("Gentleman" is the one the owner had) belongs to someone else, so Merge leaves it
// alone, Remove keeps it, and status reports the conflict (L57).
const (
	OutputStyleKey  = "outputStyle"
	OutputStyleName = "labdrian"
)

// outputStyleKey is the key as the shared ownership rules (ownedKey) read it.
var outputStyleKey = ownedKey{key: OutputStyleKey, name: OutputStyleName}

// OutputStyleState is what the settings say about the output style key.
type OutputStyleState = KeyState

const (
	// OutputStyleMissing: the key is absent.
	OutputStyleMissing = KeyMissing
	// OutputStyleOwned: the key holds exactly OutputStyleName.
	OutputStyleOwned = KeyOwned
	// OutputStyleConflict: the key holds another value, which is not ours to change.
	OutputStyleConflict = KeyConflict
)

// OutputStyle reports the state of the output style key in root.
func OutputStyle(root map[string]interface{}) OutputStyleState { return outputStyleKey.state(root) }

// OutputStyleValueJSON is the value of the key in root as JSON, for a message that names it: a string
// in quotes, anything else as it would be written in the file. It is empty when the key is absent.
func OutputStyleValueJSON(root map[string]interface{}) string { return outputStyleKey.valueJSON(root) }

// ForeignOutputStyleNotice is the sentence an install says when the key at location (the settings file)
// holds the foreign value valueJSON: it was left as it is, so the labdrian output style was not enabled.
func ForeignOutputStyleNotice(location, valueJSON string) string {
	return fmt.Sprintf("%s holds \"%s\": %s; it was left as it is, so the labdrian output style was not enabled (set it to %q or remove the key, then run install-hooks again)",
		location, OutputStyleKey, valueJSON, OutputStyleName)
}

// addOutputStyle sets the key to OutputStyleName when it is absent and reports whether it did.
func addOutputStyle(root map[string]interface{}) bool { return outputStyleKey.add(root) }

// removeOutputStyle deletes the key only when it holds exactly OutputStyleName and reports whether it did.
func removeOutputStyle(root map[string]interface{}) bool { return outputStyleKey.remove(root) }
