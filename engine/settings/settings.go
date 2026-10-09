// Package settings implements the safe JSON merge for Claude Code settings.json.
//
// Merger provides two operations:
//
//   - Install: adds two hook entries (UserPromptSubmit + PreToolUse) to settings.json.
//     The operation is PRESERVE (all other keys intact), IDEMPOTENT (no duplicates),
//     ATOMIC (write to temp then rename), and creates a .bak before overwrite.
//
//   - Uninstall: removes exactly our two hook entries identified by binary path
//     substring inside hooks[].command, leaving all other keys and hooks intact.
//     Also idempotent.
//
// VERIFIED HOOK ENTRY SHAPE (Claude Code 2.1.185 / docs):
//
//	UserPromptSubmit entry:
//	  {"hooks":[{"type":"command","command":"<bash>"}]}
//
//	PreToolUse entry (with matcher):
//	  {"matcher":"Agent","hooks":[{"type":"command","command":"<bash>"}]}
//
// No outer "type" or "command" keys — those were wrong and are now removed.
// Dedup/uninstall identity uses binary path SUBSTRING inside hooks[].command,
// not an outer "command" key.
//
// SAFETY: neither Install nor Uninstall ever reads or writes the live
// ~/.claude/settings.json; callers must pass the explicit settings path.
// Tests always use t.TempDir() fixtures.
package settings

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/guardmarkers"
)

// ClaudeRuntimeConfigRoot is the default root validator for Claude runtime path
// resolution. The helper intentionally returns the same error language used by the
// runtime adapters so status and mutating commands stay aligned.
const claudeRuntimeRootRequiredMessage = "Claude config root could not be resolved; set HOME"

// Merger performs safe install/uninstall of deterministic-scoping hooks into
// a Claude Code settings.json.
type Merger struct {
	settingsPath string
	hookCommand  string
}

const (
	// Minimalism, design, and sync-trigger identity tokens are exposed to
	// status checks so caller code can assert provable Labdrian ownership
	// without duplicating parsing logic.
	LabdrianMinimalismIdentity = "minimalism-contract.md"
	LabdrianDesignIdentity     = "--embedded-contract " + embeddedDesignName
	// LabdrianSyncTriggerIdentity is both the sync-trigger verb name and the
	// dedup/uninstall identity token for the SessionEnd hook entry — the verb
	// argument itself distinguishes it from every other entry sharing our
	// binary path.
	LabdrianSyncTriggerIdentity = "sync-trigger"
	// LabdrianReviewReceiptIdentity is both the review-receipt verb name and
	// the dedup/uninstall identity token for the PreToolUse/Bash review-receipt
	// hook entry, mirroring LabdrianSyncTriggerIdentity's role for its family.
	LabdrianReviewReceiptIdentity = "review-receipt"
	// LabdrianShaperGuardIdentity is the shaper clearance guard verb and the
	// dedup/uninstall identity token for its PreToolUse entries.
	LabdrianShaperGuardIdentity = "shaper guard-hook"
	// ShaperGuardFileToolMatcher is the PreToolUse matcher of the guard entry
	// that refuses file tools writing into the clearance store.
	ShaperGuardFileToolMatcher = "Write|Edit|MultiEdit|NotebookEdit"
	// ShaperClearanceDenyRule is the permissions.deny backstop for the
	// clearance record entry point. Claude Code documents deny rules as
	// holding in every permission mode, including bypassPermissions. Like
	// the hook, it matches command text only: it is a speed bump, not a
	// security boundary.
	ShaperClearanceDenyRule = "Bash(*" + guardmarkers.Command + "*)"
)

// ValidateClaudeConfigRoot validates that root is non-empty and absolute.
func ValidateClaudeConfigRoot(root string) error {
	if strings.TrimSpace(root) == "" {
		return errors.New(claudeRuntimeRootRequiredMessage)
	}
	if !filepath.IsAbs(root) {
		return fmt.Errorf("Claude config root must be absolute, got %q", root)
	}
	return nil
}

// ResolveClaudeSettingsPath returns the settings.json path for a Claude root after
// validating the root.
func ResolveClaudeSettingsPath(configRoot string) (string, error) {
	if err := ValidateClaudeConfigRoot(configRoot); err != nil {
		return "", err
	}
	return filepath.Join(configRoot, "settings.json"), nil
}

// ResolveClaudeHookCommandPath returns the on-disk hook command location for a
// Claude runtime root after validating the root.
func ResolveClaudeHookCommandPath(configRoot string) (string, error) {
	if err := ValidateClaudeConfigRoot(configRoot); err != nil {
		return "", err
	}
	return filepath.Join(configRoot, "bin", "gentle-ai-overlay"), nil
}

// NewMerger returns a Merger that will merge hooks into settingsPath using
// hookCommand as the unique identity (binary path substring) for our entries.
func NewMerger(settingsPath, hookCommand string) *Merger {
	return &Merger{
		settingsPath: settingsPath,
		hookCommand:  hookCommand,
	}
}

// Install adds our hook entries to settings.json if they are not already
// present. It creates the file if absent, and backs up the original to
// settings.json.bak before any overwrite. The write is atomic (temp file +
// rename). Returns an error — and leaves the original untouched — if the
// existing file contains invalid JSON.
func (m *Merger) Install() error {
	if m.hookCommand == "" {
		return ErrEmptyHookCommand
	}
	doc, err := m.loadOrEmpty()
	if err != nil {
		return err
	}

	changed, err := doc.Merge(m.hookCommand)
	if err != nil {
		return fmt.Errorf("settings: %s contains invalid JSON (not modified): %w", m.settingsPath, err)
	}
	if !changed {
		return nil
	}

	return m.writeAtomic(doc)
}

// Uninstall removes our hook entries from settings.json. If the file is
// absent, it is a no-op. Leaves all other keys and hooks intact.
func (m *Merger) Uninstall() error {
	if m.hookCommand == "" {
		return ErrEmptyHookCommand
	}
	if _, err := os.Stat(m.settingsPath); os.IsNotExist(err) {
		return nil
	}

	doc, err := m.loadOrEmpty()
	if err != nil {
		return err
	}

	changed, err := doc.Remove(m.hookCommand)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}

	return m.writeAtomic(doc)
}

// loadOrEmpty reads and parses settings.json. If the file does not exist, it
// returns an empty document (which will be written as a new file). If the file
// exists but contains invalid JSON, it returns an error without modifying
// anything.
func (m *Merger) loadOrEmpty() (Document, error) {
	data, err := os.ReadFile(m.settingsPath)
	if os.IsNotExist(err) {
		return Empty(), nil
	}
	if err != nil {
		return Document{}, fmt.Errorf("settings: read %s: %w", m.settingsPath, err)
	}

	doc, err := Parse(data)
	if err != nil {
		return Document{}, fmt.Errorf("settings: %s contains invalid JSON (not modified): %w", m.settingsPath, err)
	}
	return doc, nil
}

// writeAtomic serializes doc to a temp file, copies the original to .bak if it
// exists, then renames the temp into place.
func (m *Merger) writeAtomic(doc Document) error {
	data, err := doc.Bytes()
	if err != nil {
		return err
	}

	dir := filepath.Dir(m.settingsPath)
	tmp, err := os.CreateTemp(dir, ".settings-*.json.tmp")
	if err != nil {
		return fmt.Errorf("settings: create temp: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		// Clean up temp file on any error path.
		os.Remove(tmpPath)
	}()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("settings: write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("settings: close temp: %w", err)
	}

	// Backup existing file before rename.
	if _, statErr := os.Stat(m.settingsPath); statErr == nil {
		bakPath := m.settingsPath + ".bak"
		if err := copyFile(m.settingsPath, bakPath); err != nil {
			return fmt.Errorf("settings: backup to %s: %w", bakPath, err)
		}
	}

	if err := os.Rename(tmpPath, m.settingsPath); err != nil {
		return fmt.Errorf("settings: rename temp to %s: %w", m.settingsPath, err)
	}
	return nil
}

// copyFile copies src to dst, creating dst if it does not exist.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}
