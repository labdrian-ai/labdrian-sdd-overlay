package settingsfile

import (
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings"
)

// Installer installs and removes the hooks the overlay owns in a settings.json, and says what one
// holds. It has no state: the file is named by each call, so one Installer serves every root.
// It is the adapter of the HookInstaller port the Claude runtime adapter owns.
type Installer struct{}

// Install adds the hook entries the overlay owns for the binary at hookCommand to the settings
// file at path, if they are not all there, creating the file if it is absent and keeping the one
// it replaces as <path>.bak. A file that holds invalid JSON is left as it is and reported.
func (Installer) Install(path, hookCommand string) error {
	if hookCommand == "" {
		return settings.ErrEmptyHookCommand
	}
	file := File{Path: path}
	doc, _, err := file.Read()
	if err != nil {
		return err
	}
	changed, err := doc.Merge(hookCommand)
	if err != nil {
		// Only a null document is refused, and it is a file with nothing to merge into.
		return fmt.Errorf("settings: %s contains invalid JSON (not modified): %w", path, err)
	}
	if !changed {
		return nil
	}
	return file.Write(doc)
}

// Uninstall removes the entries Install adds from the settings file at path. A file that is absent
// is not an error: there is nothing to remove.
func (Installer) Uninstall(path, hookCommand string) error {
	if hookCommand == "" {
		return settings.ErrEmptyHookCommand
	}
	file := File{Path: path}
	doc, _, err := file.Read() // a file that is not there reads as the empty document, which has nothing to remove
	if err != nil {
		return err
	}
	changed, err := doc.Remove(hookCommand)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	return file.Write(doc)
}

// Inspect says what the settings file at path holds for the binary at hookCommand. found is false
// when there is no file, or it holds the JSON value null, which holds nothing. owned is true when
// every hook family the overlay installs is in place and owned by that binary. A file that cannot
// be read or parsed is returned as the system or encoding/json reported it, for the caller to word.
func (Installer) Inspect(path, hookCommand string) (found, owned bool, err error) {
	data, present, err := File{Path: path}.Bytes()
	if err != nil || !present {
		return false, false, err
	}
	doc, err := settings.Parse(data)
	if err != nil {
		return false, false, err
	}
	if doc.Null() {
		return false, false, nil
	}
	return true, settings.HasSupportedClaudeLifecycleState(doc.Root(), hookCommand), nil
}
