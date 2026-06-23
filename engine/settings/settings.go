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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Merger performs safe install/uninstall of deterministic-scoping hooks into
// a Claude Code settings.json.
type Merger struct {
	settingsPath string
	hookCommand  string
}

func DefaultClaudeSettingsPath(home string) string {
	return filepath.Join(home, ".claude", "settings.json")
}

func DefaultClaudeContractPath(home string) string {
	return filepath.Join(home, ".claude", "skills", "_shared", "minimalism-contract.md")
}

// NewMerger returns a Merger that will merge hooks into settingsPath using
// hookCommand as the unique identity (binary path substring) for our entries.
func NewMerger(settingsPath, hookCommand string) *Merger {
	return &Merger{
		settingsPath: settingsPath,
		hookCommand:  hookCommand,
	}
}

// Install adds our two hook entries to settings.json if they are not already
// present. It creates the file if absent, and backs up the original to
// settings.json.bak before any overwrite. The write is atomic (temp file +
// rename). Returns an error — and leaves the original untouched — if the
// existing file contains invalid JSON.
func (m *Merger) Install() error {
	root, err := m.loadOrEmpty()
	if err != nil {
		return err
	}

	changed := m.mergeHooks(root)
	if !changed {
		return nil
	}

	return m.writeAtomic(root)
}

// Uninstall removes our two hook entries from settings.json. If the file is
// absent, it is a no-op. Leaves all other keys and hooks intact.
func (m *Merger) Uninstall() error {
	if _, err := os.Stat(m.settingsPath); os.IsNotExist(err) {
		return nil
	}

	root, err := m.loadOrEmpty()
	if err != nil {
		return err
	}

	changed := m.removeHooks(root)
	if !changed {
		return nil
	}

	return m.writeAtomic(root)
}

// loadOrEmpty reads and parses settings.json. If the file does not exist, it
// returns an empty map (which will be written as a new file). If the file
// exists but contains invalid JSON, it returns an error without modifying
// anything.
func (m *Merger) loadOrEmpty() (map[string]interface{}, error) {
	data, err := os.ReadFile(m.settingsPath)
	if os.IsNotExist(err) {
		return map[string]interface{}{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("settings: read %s: %w", m.settingsPath, err)
	}

	var root map[string]interface{}
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("settings: %s contains invalid JSON (not modified): %w", m.settingsPath, err)
	}
	return root, nil
}

// mergeHooks inserts our hook entries if not already present. Returns true if
// any change was made.
func (m *Merger) mergeHooks(root map[string]interface{}) bool {
	hooks := ensureHooksMap(root)
	changed := false

	if !hasCommandInInnerHooks(hooks, "UserPromptSubmit", m.hookCommand) {
		appendHook(hooks, "UserPromptSubmit", m.buildUserPromptSubmitEntry())
		changed = true
	}

	if !hasCommandInInnerHooks(hooks, "PreToolUse", m.hookCommand) {
		appendHook(hooks, "PreToolUse", m.buildPreToolUseEntry())
		changed = true
	}

	root["hooks"] = hooks
	return changed
}

// removeHooks removes our hook entries. Returns true if any change was made.
// Identity uses binary path SUBSTRING inside hooks[].command — matches the
// new entry shape where there is no outer "command" key.
func (m *Merger) removeHooks(root map[string]interface{}) bool {
	hooks, ok := root["hooks"].(map[string]interface{})
	if !ok {
		return false
	}

	changed := false
	for _, key := range []string{"UserPromptSubmit", "PreToolUse"} {
		entries, ok := hooks[key].([]interface{})
		if !ok {
			continue
		}
		var filtered []interface{}
		for _, e := range entries {
			if entryContainsBinary(e, m.hookCommand) {
				changed = true
				continue
			}
			filtered = append(filtered, e)
		}
		if len(filtered) == 0 {
			// Remove the key entirely rather than writing null or an empty
			// array — avoids structural noise in the emitted JSON.
			delete(hooks, key)
		} else {
			hooks[key] = filtered
		}
	}

	root["hooks"] = hooks
	return changed
}

// writeAtomic serializes root to a temp file, validates it parses back,
// copies the original to .bak if it exists, then renames the temp into place.
func (m *Merger) writeAtomic(root map[string]interface{}) error {
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("settings: marshal: %w", err)
	}

	// Validate the serialized output parses cleanly before touching any files.
	var validate interface{}
	if err := json.Unmarshal(data, &validate); err != nil {
		return fmt.Errorf("settings: merged JSON failed validation (not written): %w", err)
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

// ensureHooksMap retrieves or creates the "hooks" map in root.
func ensureHooksMap(root map[string]interface{}) map[string]interface{} {
	if h, ok := root["hooks"].(map[string]interface{}); ok {
		return h
	}
	h := map[string]interface{}{}
	root["hooks"] = h
	return h
}

// hasCommandInInnerHooks reports whether hooks[key] already contains an entry
// whose inner hooks[].command contains hookCommand as a substring.
// This matches the new entry shape: {"hooks":[{"type":"command","command":"..."}]}.
func hasCommandInInnerHooks(hooks map[string]interface{}, key, hookCommand string) bool {
	entries, ok := hooks[key].([]interface{})
	if !ok {
		return false
	}
	for _, e := range entries {
		if entryContainsBinary(e, hookCommand) {
			return true
		}
	}
	return false
}

// entryContainsBinary returns true if the hook entry (an outer object) contains
// hookCommand as a substring in any of its inner hooks[].command strings.
func entryContainsBinary(e interface{}, hookCommand string) bool {
	em, ok := e.(map[string]interface{})
	if !ok {
		return false
	}
	innerHooks, ok := em["hooks"].([]interface{})
	if !ok {
		return false
	}
	for _, ih := range innerHooks {
		ihm, ok := ih.(map[string]interface{})
		if !ok {
			continue
		}
		if cmdStr, ok := ihm["command"].(string); ok {
			if strings.Contains(cmdStr, hookCommand) {
				return true
			}
		}
	}
	return false
}

// appendHook appends entry to hooks[key], creating the slice if absent.
func appendHook(hooks map[string]interface{}, key string, entry map[string]interface{}) {
	var entries []interface{}
	if existing, ok := hooks[key].([]interface{}); ok {
		entries = existing
	}
	hooks[key] = append(entries, entry)
}

// buildUserPromptSubmitEntry returns the hook entry for propagate.
// The hook runs 'gentle-ai-overlay propagate' to ensure the scoped block
// exists in the current project's .atl/skill-registry.md.
//
// VERIFIED SHAPE (Claude Code docs):
//
//	{"hooks":[{"type":"command","command":"<bash>"}]}
//
// Missing-binary safety: the command uses 'bash -c' with a guard:
//
//	command -v <binary> &>/dev/null && <binary> ... || true
//
// This ensures the hook exits 0 even if the binary is absent, so no Agent
// call is ever blocked by a missing binary.
func (m *Merger) buildUserPromptSubmitEntry() map[string]interface{} {
	// CLAUDE_PROJECT_DIR is the env var Claude Code sets for hooks to point at
	// the project root. The :- fallback to "." keeps the command functional
	// when the env var is absent (e.g. local testing), but the primary path
	// is always anchored to the project root when Claude Code fires the hook.
	cmd := fmt.Sprintf(
		`command -v %s &>/dev/null && %s propagate --registry "${CLAUDE_PROJECT_DIR:-.}/.atl/skill-registry.md" --contract-file ~/.claude/skills/_shared/minimalism-contract.md || true`,
		m.hookCommand, m.hookCommand,
	)
	return map[string]interface{}{
		"hooks": []interface{}{map[string]interface{}{
			"type":    "command",
			"command": cmd,
		}},
	}
}

// buildPreToolUseEntry returns the hook entry for gate-task (matcher: Agent tool).
//
// VERIFIED SHAPE (Claude Code 2.1.185):
//
//	{"matcher":"Agent","hooks":[{"type":"command","command":"<bash>"}]}
//
// The matcher MUST be "Agent" (not "Task") — empirically verified: the sub-agent
// spawn tool is named "Agent" in CC 2.1.185. A "Task" matcher fires on nothing.
//
// F-PATH: the --contract-path flag passes the ABSOLUTE path ($HOME-expanded) so
// a sub-agent in any project cwd can read the contract file. The injected bare
// path line in the Agent prompt will be this absolute path.
//
// Missing-binary safety: same guard pattern as UserPromptSubmit.
func (m *Merger) buildPreToolUseEntry() map[string]interface{} {
	cmd := fmt.Sprintf(
		`command -v %s &>/dev/null && %s gate-task --contract-file ~/.claude/skills/_shared/minimalism-contract.md --contract-path "$HOME/.claude/skills/_shared/minimalism-contract.md" || true`,
		m.hookCommand, m.hookCommand,
	)
	return map[string]interface{}{
		"matcher": "Agent",
		"hooks": []interface{}{map[string]interface{}{
			"type":    "command",
			"command": cmd,
		}},
	}
}

// copyFile copies src to dst, creating dst if it does not exist.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}
