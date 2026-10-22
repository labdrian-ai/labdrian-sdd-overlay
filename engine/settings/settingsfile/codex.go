package settingsfile

import (
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings/codexhooks"
)

// CodexHooks edits the caller-named hooks.json using File's atomic backup conventions. The
// directory must already exist; installation never provisions a Codex home or touches trust state.
type CodexHooks struct{}

// Install replaces the owned SessionStart, PreToolUse and PostToolUse entries, writing nothing
// on a second identical merge.
func (CodexHooks) Install(path, binary string) error {
	return editCodex(path, binary, codexhooks.Merge)
}

// Uninstall removes owned handlers only; an absent file remains absent.
func (CodexHooks) Uninstall(path, binary string) error {
	return editCodex(path, binary, codexhooks.Remove)
}

func editCodex(path, binary string, edit func([]byte, string) ([]byte, bool, error)) error {
	file := File{Path: path}
	target, err := file.resolve()
	if err != nil {
		return err
	}
	data, _, err := (File{Path: target}).Bytes()
	if err != nil {
		return err
	}
	updated, changed, err := edit(data, binary)
	if err != nil {
		return fmt.Errorf("%s: %w (not modified)", path, err)
	}
	if !changed {
		return nil
	}
	return file.WriteBytes(updated)
}
