package runtime

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
)

// ClaudeAdapter is the runtime adapter foundation for Claude CLI.
type ClaudeAdapter struct {
	target core.Target
	root   string
	hooks  HookInstaller
}

// NewClaudeAdapter builds the Claude adapter over root, the directory its settings live in, and
// the hooks port it installs and inspects the hook entries through. An empty root is an adapter
// that reports it cannot work: the caller resolves the default (see Config.ClaudeRoot), the
// adapter does not look for one.
func NewClaudeAdapter(root string, hooks HookInstaller) ClaudeAdapter {
	return ClaudeAdapter{target: core.TargetClaude, root: root, hooks: hooks}
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

	if err := a.hooks.Install(settingsPath, hookCommand); err != nil {
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

	if err := a.hooks.Install(settingsPath, hookCommand); err != nil {
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

	if err := a.hooks.Uninstall(settingsPath, hookCommand); err != nil {
		return a.result(core.ActionUninstall, core.CapabilityPartial, err.Error())
	}

	return a.result(core.ActionUninstall, core.CapabilityRestartRequired, "Claude lifecycle hooks removed; restart Claude to unload stale hook processes")
}

func (a ClaudeAdapter) status() core.LifecycleResult {
	settingsPath, hookCommand, err := a.configPaths()
	if err != nil {
		return a.result(core.ActionStatus, core.CapabilityUnsupported, err.Error())
	}

	found, owned, err := a.hooks.Inspect(settingsPath, hookCommand)
	if err != nil {
		return a.result(core.ActionStatus, core.CapabilityUnsupported, "read settings file "+settingsPath+": "+err.Error())
	}
	if !found {
		return a.result(core.ActionStatus, core.CapabilityUnsupported, "Claude settings file not found at "+settingsPath)
	}

	hookPath := settingsPath
	if !owned {
		return a.result(core.ActionStatus, core.CapabilityPartial,
			"Claude lifecycle hooks are not fully owned/installed in "+hookPath+
				"; run 'labdrian uninstall-hooks' then 'labdrian install-hooks'")
	}

	return a.result(core.ActionStatus, core.CapabilitySupported, "Claude lifecycle hooks are installed and owned")
}

// configPaths resolves the settings file and the hook command from the Claude root: the file is
// <root>/settings.json and the binary its hooks run is <root>/bin/gentle-ai-overlay. A root that
// is blank or not absolute names neither.
func (a ClaudeAdapter) configPaths() (settingsPath, hookCommand string, err error) {
	if strings.TrimSpace(a.root) == "" {
		return "", "", errors.New("Claude config root could not be resolved; set HOME")
	}
	if !filepath.IsAbs(a.root) {
		return "", "", fmt.Errorf("Claude config root must be absolute, got %q", a.root)
	}
	return filepath.Join(a.root, "settings.json"), filepath.Join(a.root, "bin", "gentle-ai-overlay"), nil
}

func (a ClaudeAdapter) result(action core.Action, status core.CapabilityStatus, message string) core.LifecycleResult {
	return core.NewLifecycleResult(a.target, action, status, message, nil)
}
