package settings

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ErrEmptyHookCommand is returned by Merge and Remove when they are given no binary path. Every
// entry is recognized as ours by that path, so with none there is nothing to tell our entries
// from anyone else's, and acting could rewrite or remove foreign hooks.
var ErrEmptyHookCommand = errors.New("settings: the hook command is empty; without the binary path our entries cannot be told from foreign ones (nothing was changed)")

// ErrNotAnObject is returned by Merge for a settings.json whose JSON is the single value null:
// it parses, but it holds no object to put hooks in. Remove has nothing to remove from it and
// succeeds. Before this error existed, installing into such a file crashed on a write to a nil map.
var ErrNotAnObject = errors.New("JSON null is not a settings object")

// Document is the content of a Claude Code settings.json as the overlay edits it: a JSON object
// in which Merge adds the hook entries the overlay owns and Remove takes them out again, and
// nothing else is touched. It holds no file and does no I/O; reading and writing the file is an
// adapter's (settings/settingsfile).
//
// The object is decoded into generic values and written back with the keys sorted and two-space
// indentation. Numbers go through float64 and strings through UTF-8, so a number that float64
// cannot hold exactly is not kept digit for digit; that is how the file has always been written.
type Document struct {
	// root is nil for a settings.json that holds the JSON value null.
	root map[string]interface{}
}

// Empty is the document of a settings.json that does not exist yet.
func Empty() Document { return Document{root: map[string]interface{}{}} }

// Parse decodes the bytes of a settings.json. Anything that is not a JSON object or null is an
// error, the one encoding/json reports, so the caller can name the file it came from.
func Parse(data []byte) (Document, error) {
	var root map[string]interface{}
	if err := json.Unmarshal(data, &root); err != nil {
		return Document{}, err
	}
	return Document{root: root}, nil
}

// Null reports whether the document came from a settings.json that holds the JSON value null.
func (d Document) Null() bool { return d.root == nil }

// Root is the decoded object, for the helpers that answer what the settings hold (the Has and
// Missing functions). It is nil for a null document. The caller must not change it.
func (d Document) Root() map[string]interface{} { return d.root }

// Merge adds the hook entries and the permissions.deny rule the overlay owns, each only if it is
// not already there, for the binary at hookCommand, and reports whether the document changed.
// Everything else in the document stays as it was. It never changes a document that already holds
// everything, so installing twice writes nothing the second time.
func (d *Document) Merge(hookCommand string) (changed bool, err error) {
	if hookCommand == "" {
		return false, ErrEmptyHookCommand
	}
	if d.root == nil {
		return false, ErrNotAnObject
	}
	return owner{hookCommand}.mergeHooks(d.root), nil
}

// Remove takes out exactly the entries the overlay owns for the binary at hookCommand, and the
// permissions.deny rule, and reports whether the document changed. A foreign entry is never
// removed, even one that runs the same binary with another verb.
func (d *Document) Remove(hookCommand string) (changed bool, err error) {
	if hookCommand == "" {
		return false, ErrEmptyHookCommand
	}
	return owner{hookCommand}.removeHooks(d.root), nil // a null document has nothing to find
}

// Bytes is the document as it is written: the keys sorted, two spaces of indentation, no trailing
// newline. A null document is written as null.
func (d Document) Bytes() ([]byte, error) {
	data, err := json.MarshalIndent(d.root, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("settings: marshal: %w", err)
	}
	return data, nil
}
