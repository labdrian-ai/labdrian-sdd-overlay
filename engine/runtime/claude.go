package runtime

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings"
)

// ClaudeAdapter is the runtime adapter foundation for Claude CLI.
type ClaudeAdapter struct {
	target Target
	root   string
}

func NewClaudeAdapter(root string) ClaudeAdapter {
	if root == "" {
		root = DefaultClaudeConfigRoot()
	}
	return ClaudeAdapter{target: TargetClaude, root: root}
}

func (a ClaudeAdapter) Target() Target         { return a.target }
func (a ClaudeAdapter) Apply() LifecycleResult { return a.Install() }
func (a ClaudeAdapter) Install() LifecycleResult {
	settingsPath, hookCommand, err := a.configPaths()
	if err != nil {
		return a.result(ActionInstall, CapabilityUnsupported, err.Error())
	}
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		return a.result(ActionInstall, CapabilityUnsupported, err.Error())
	}

	merger := settings.NewMerger(settingsPath, hookCommand)
	if err := merger.Install(); err != nil {
		return a.result(ActionInstall, CapabilityPartial, err.Error())
	}

	return a.result(ActionInstall, CapabilityRestartRequired, "Claude lifecycle hooks installed; restart Claude to load hook changes")
}
func (a ClaudeAdapter) Status() LifecycleResult    { return a.status() }
func (a ClaudeAdapter) SyncCheck() LifecycleResult { return a.Status() }
func (a ClaudeAdapter) Update() LifecycleResult {
	settingsPath, hookCommand, err := a.configPaths()
	if err != nil {
		return a.result(ActionUpdate, CapabilityUnsupported, err.Error())
	}
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		return a.result(ActionUpdate, CapabilityUnsupported, err.Error())
	}

	merger := settings.NewMerger(settingsPath, hookCommand)
	if err := merger.Install(); err != nil {
		return a.result(ActionUpdate, CapabilityPartial, err.Error())
	}

	return a.result(ActionUpdate, CapabilityRestartRequired, "Claude lifecycle hooks updated; restart Claude to load hook changes")
}
func (a ClaudeAdapter) Rollback() LifecycleResult { return a.Uninstall() }
func (a ClaudeAdapter) Uninstall() LifecycleResult {
	settingsPath, hookCommand, err := a.configPaths()
	if err != nil {
		return a.result(ActionUninstall, CapabilityUnsupported, err.Error())
	}
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		return a.result(ActionUninstall, CapabilityUnsupported, err.Error())
	}

	merger := settings.NewMerger(settingsPath, hookCommand)
	if err := merger.Uninstall(); err != nil {
		return a.result(ActionUninstall, CapabilityPartial, err.Error())
	}

	return a.result(ActionUninstall, CapabilityRestartRequired, "Claude lifecycle hooks removed; restart Claude to unload stale hook processes")
}

func (a ClaudeAdapter) status() LifecycleResult {
	settingsPath, hookCommand, err := a.configPaths()
	if err != nil {
		return a.result(ActionStatus, CapabilityUnsupported, err.Error())
	}

	// readJSONObjectOrNil (10a.9) is the same "read file, tolerate absence,
	// decode to a generic object" shape LongtermMemAdapter needs for its own
	// claude/opencode config inspection — genuinely shared, unlike
	// OpenCodeAdapter's own strongly-typed readConfig, which validates a
	// bespoke plugin schema this component has nothing to do with.
	root, err := readJSONObjectOrNil(settingsPath)
	if err != nil {
		return a.result(ActionStatus, CapabilityUnsupported, "read settings file "+settingsPath+": "+err.Error())
	}
	if root == nil {
		return a.result(ActionStatus, CapabilityUnsupported, "Claude settings file not found at "+settingsPath)
	}

	hookPath := settingsPath
	if !settings.HasSupportedClaudeLifecycleState(root, hookCommand) {
		return a.result(ActionStatus, CapabilityPartial, "Claude lifecycle hooks are not fully owned/installed in "+hookPath)
	}

	return a.result(ActionStatus, CapabilitySupported, "Claude lifecycle hooks are installed and owned")
}

func (a ClaudeAdapter) configPaths() (string, string, error) {
	if a.root == "" {
		return "", "", fmt.Errorf("Claude config root could not be resolved; set HOME")
	}

	settingsPath, err := settings.ResolveClaudeSettingsPath(a.root)
	if err != nil {
		return "", "", err
	}

	hookPath, err := settings.ResolveClaudeHookCommandPath(a.root)
	if err != nil {
		return "", "", err
	}

	return settingsPath, hookPath, nil
}

func (a ClaudeAdapter) result(action Action, status CapabilityStatus, message string) LifecycleResult {
	return NewLifecycleResult(a.target, action, status, message, nil)
}
