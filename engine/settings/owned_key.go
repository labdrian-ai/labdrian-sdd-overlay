package settings

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// KeyState is what the settings say about a top-level key the overlay may own.
type KeyState int

const (
	// KeyMissing: the key is absent.
	KeyMissing KeyState = iota
	// KeyOwned: the key holds exactly the one value the overlay owns.
	KeyOwned
	// KeyConflict: the key holds another value, which is not ours to change.
	KeyConflict
)

// ownedKey is a top-level settings key that the overlay owns only while it holds one exact value
// (the main-session agent, the output style). Ownership is by value alone: any other value, null and
// a value that is not a string included, belongs to someone else, so Merge never overwrites it,
// Remove keeps it, and status reports the conflict.
type ownedKey struct {
	key  string
	name string
}

func (k ownedKey) state(root map[string]interface{}) KeyState {
	v, ok := root[k.key]
	if !ok {
		return KeyMissing
	}
	if v == k.name {
		return KeyOwned
	}
	return KeyConflict
}

// valueJSON is the value of the key in root as JSON, for a message that names it: a string in quotes,
// anything else as it would be written in the file. It is empty when the key is absent.
func (k ownedKey) valueJSON(root map[string]interface{}) string {
	v, ok := root[k.key]
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

// add sets the key to the owned value when it is absent and reports whether it did. A key that
// already holds the value needs nothing, and one that holds another value is never overwritten.
func (k ownedKey) add(root map[string]interface{}) bool {
	if k.state(root) != KeyMissing {
		return false
	}
	root[k.key] = k.name
	return true
}

// remove deletes the key only when it holds exactly the owned value and reports whether it did.
func (k ownedKey) remove(root map[string]interface{}) bool {
	if k.state(root) != KeyOwned {
		return false
	}
	delete(root, k.key)
	return true
}
