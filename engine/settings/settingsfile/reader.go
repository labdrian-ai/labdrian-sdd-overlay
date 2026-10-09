package settingsfile

import (
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings"
)

// Reader reads a settings.json for the status check, which only looks at it. It is not File.Read:
// it names no file in what it reports, because the caller knows which file it asked for and says
// so, and it never refuses a file for being a link or for the way it was written.
type Reader struct{}

// Settings reads the settings.json at path. A file that does not exist is the document with
// nothing in it, as is one that holds the JSON value null: neither is an error, hooks are simply
// absent. A file that cannot be read is the error of the system, and one whose content is not a
// JSON object is an error that begins "invalid JSON: ". A symbolic link is read through.
func (Reader) Settings(path string) (settings.Document, error) {
	data, found, err := File{Path: path}.Bytes()
	if err != nil {
		return settings.Document{}, err
	}
	if !found {
		return settings.Document{}, nil
	}
	doc, err := settings.Parse(data)
	if err != nil {
		return settings.Document{}, fmt.Errorf("invalid JSON: %w", err)
	}
	return doc, nil
}
