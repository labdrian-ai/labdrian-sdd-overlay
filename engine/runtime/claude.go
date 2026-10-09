package runtime

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings"
)

// ClaudeAdapter is the runtime adapter foundation for Claude CLI.
type ClaudeAdapter struct {
	target core.Target
	root   string
}

// NewClaudeAdapter builds the Claude adapter over root, the directory its settings live in. An
// empty root is an adapter that reports it cannot work: the caller resolves the default (see
// Config.ClaudeRoot), the adapter does not look for one.
func NewClaudeAdapter(root string) ClaudeAdapter {
	return ClaudeAdapter{target: core.TargetClaude, root: root}
}

func (a ClaudeAdapter) Target() core.Target         { return a.target }
func (a ClaudeAdapter) Apply() core.LifecycleResult { return a.Install() }
func (a ClaudeAdapter) Install() core.LifecycleResult {
	settingsPath, hookCommand, err := a.configPaths()
	if err != nil {
		return a.result(core.ActionInstall, core.CapabilityUnsupported, err.Error())
	}
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		return a.result(core.ActionInstall, core.CapabilityUnsupported, err.Error())
	}

	merger := settings.NewMerger(settingsPath, hookCommand)
	if err := merger.Install(); err != nil {
		return a.result(core.ActionInstall, core.CapabilityPartial, err.Error())
	}

	return a.result(core.ActionInstall, core.CapabilityRestartRequired, "Claude lifecycle hooks installed; restart Claude to load hook changes")
}
func (a ClaudeAdapter) Status() core.LifecycleResult    { return a.status() }
func (a ClaudeAdapter) SyncCheck() core.LifecycleResult { return a.Status() }
func (a ClaudeAdapter) Update() core.LifecycleResult {
	settingsPath, hookCommand, err := a.configPaths()
	if err != nil {
		return a.result(core.ActionUpdate, core.CapabilityUnsupported, err.Error())
	}
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		return a.result(core.ActionUpdate, core.CapabilityUnsupported, err.Error())
	}

	merger := settings.NewMerger(settingsPath, hookCommand)
	if err := merger.Install(); err != nil {
		return a.result(core.ActionUpdate, core.CapabilityPartial, err.Error())
	}

	return a.result(core.ActionUpdate, core.CapabilityRestartRequired, "Claude lifecycle hooks updated; restart Claude to load hook changes")
}
func (a ClaudeAdapter) Rollback() core.LifecycleResult { return a.Uninstall() }
func (a ClaudeAdapter) Uninstall() core.LifecycleResult {
	settingsPath, hookCommand, err := a.configPaths()
	if err != nil {
		return a.result(core.ActionUninstall, core.CapabilityUnsupported, err.Error())
	}
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		return a.result(core.ActionUninstall, core.CapabilityUnsupported, err.Error())
	}

	merger := settings.NewMerger(settingsPath, hookCommand)
	if err := merger.Uninstall(); err != nil {
		return a.result(core.ActionUninstall, core.CapabilityPartial, err.Error())
	}

	return a.result(core.ActionUninstall, core.CapabilityRestartRequired, "Claude lifecycle hooks removed; restart Claude to unload stale hook processes")
}

func (a ClaudeAdapter) status() core.LifecycleResult {
	settingsPath, hookCommand, err := a.configPaths()
	if err != nil {
		return a.result(core.ActionStatus, core.CapabilityUnsupported, err.Error())
	}

	// readJSONObjectOrNil (10a.9) is the same "read file, tolerate absence,
	// decode to a generic object" shape LongtermMemAdapter needs for its own
	// claude/opencode config inspection — genuinely shared, unlike
	// OpenCodeAdapter's own strongly-typed readConfig, which validates a
	// bespoke plugin schema this component has nothing to do with.
	root, err := readJSONObjectOrNil(settingsPath)
	if err != nil {
		return a.result(core.ActionStatus, core.CapabilityUnsupported, "read settings file "+settingsPath+": "+err.Error())
	}
	if root == nil {
		return a.result(core.ActionStatus, core.CapabilityUnsupported, "Claude settings file not found at "+settingsPath)
	}

	hookPath := settingsPath
	if !settings.HasSupportedClaudeLifecycleState(root, hookCommand) {
		return a.result(core.ActionStatus, core.CapabilityPartial,
			"Claude lifecycle hooks are not fully owned/installed in "+hookPath+
				"; run 'labdrian uninstall-hooks' then 'labdrian install-hooks'")
	}

	return a.result(core.ActionStatus, core.CapabilitySupported, "Claude lifecycle hooks are installed and owned")
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

func (a ClaudeAdapter) result(action core.Action, status core.CapabilityStatus, message string) core.LifecycleResult {
	return core.NewLifecycleResult(a.target, action, status, message, nil)
}
