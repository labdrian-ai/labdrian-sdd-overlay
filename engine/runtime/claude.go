package runtime

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings"
)

// ClaudeAdapter is the runtime adapter foundation for Claude CLI.
type ClaudeAdapter struct {
	target core.Target
	root   string
	hooks  HookInstaller
	rules  OwnerRulesSource
}

// NewClaudeAdapter builds the Claude adapter over root, the directory its settings live in, and
// the hooks port it installs and inspects the hook entries through. An empty root is an adapter
// that reports it cannot work: the caller resolves the default (see Config.ClaudeRoot), the
// adapter does not look for one.
func NewClaudeAdapter(root string, hooks HookInstaller) ClaudeAdapter {
	return ClaudeAdapter{target: core.TargetClaude, root: root, hooks: hooks}
}

// WithOwnerRules is the adapter that also keeps the owner rules block in CLAUDE.md, with the rules from rules.
func (a ClaudeAdapter) WithOwnerRules(rules OwnerRulesSource) ClaudeAdapter {
	a.rules = rules
	return a
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

	message := "Claude lifecycle hooks installed; restart Claude to load hook changes" + a.foreignKeyNotices(settingsPath)
	return a.withOwnerRules(core.ActionInstall, message)
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

	message := "Claude lifecycle hooks updated; restart Claude to load hook changes" + a.foreignKeyNotices(settingsPath)
	return a.withOwnerRules(core.ActionUpdate, message)
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

	message := "Claude lifecycle hooks removed; restart Claude to unload stale hook processes"
	if a.rules == nil {
		return a.result(core.ActionUninstall, core.CapabilityRestartRequired, message)
	}
	clause, status := a.ownerRules().remove()
	if clause != "" {
		message += "; " + clause
	}
	return a.result(core.ActionUninstall, status, message)
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

	if r, ok := a.mainAgentStatus(settingsPath); ok {
		return r
	}
	if r, ok := a.outputStyleStatus(settingsPath); ok {
		return r
	}
	message := "Claude lifecycle hooks are installed and owned"
	if a.rules != nil {
		partial, ok, note := a.ownerRules().status()
		if ok {
			return a.result(core.ActionStatus, core.CapabilityPartial, partial)
		}
		message += "; " + note
	}
	return a.result(core.ActionStatus, core.CapabilitySupported, message)
}

// mainAgentStatus is the status result for the top-level "agent" key when it is not the foreman's: partial,
// naming the key (missing) or the foreign value and that the foreman main session is not enabled. ok is
// false when the key is the foreman's.
func (a ClaudeAdapter) mainAgentStatus(settingsPath string) (core.LifecycleResult, bool) {
	state, value, err := a.hooks.InspectMainAgent(settingsPath)
	if err != nil {
		return a.result(core.ActionStatus, core.CapabilityUnsupported, "read settings file "+settingsPath+": "+err.Error()), true
	}
	switch state {
	case settings.MainAgentMissing:
		return a.result(core.ActionStatus, core.CapabilityPartial,
			"Claude settings \"agent\" key is missing in "+settingsPath+"; the foreman main session is not enabled; run 'labdrian install-hooks'"), true
	case settings.MainAgentConflict:
		return a.result(core.ActionStatus, core.CapabilityPartial,
			"Claude settings \"agent\" is "+value+" in "+settingsPath+", not \""+settings.MainAgentName+"\"; the foreman main session is not enabled (set it to \""+settings.MainAgentName+"\" or remove the key, then run 'labdrian install-hooks')"), true
	}
	return core.LifecycleResult{}, false
}

// outputStyleStatus is the status result for the top-level "outputStyle" key when it is not ours: partial,
// naming the key (missing) or the foreign value and that the labdrian output style is not enabled. ok is
// false when the key is "labdrian".
func (a ClaudeAdapter) outputStyleStatus(settingsPath string) (core.LifecycleResult, bool) {
	state, value, err := a.hooks.InspectOutputStyle(settingsPath)
	if err != nil {
		return a.result(core.ActionStatus, core.CapabilityUnsupported, "read settings file "+settingsPath+": "+err.Error()), true
	}
	switch state {
	case settings.OutputStyleMissing:
		return a.result(core.ActionStatus, core.CapabilityPartial,
			"Claude settings \""+settings.OutputStyleKey+"\" key is missing in "+settingsPath+"; the labdrian output style is not enabled; run 'labdrian install-hooks'"), true
	case settings.OutputStyleConflict:
		return a.result(core.ActionStatus, core.CapabilityPartial,
			"Claude settings \""+settings.OutputStyleKey+"\" is "+value+" in "+settingsPath+", not \""+settings.OutputStyleName+"\"; the labdrian output style is not enabled (set it to \""+settings.OutputStyleName+"\" or remove the key, then run 'labdrian install-hooks')"), true
	}
	return core.LifecycleResult{}, false
}

// foreignKeyNotices is "; " and the notice of the install verb for each of the keys the overlay owns by
// value (the main-session agent, then the output style) that the installer finds holding a foreign value,
// and nothing when none does.
func (a ClaudeAdapter) foreignKeyNotices(settingsPath string) string {
	var notices string
	if state, value, err := a.hooks.InspectMainAgent(settingsPath); err == nil && state == settings.MainAgentConflict {
		notices += "; " + settings.ForeignMainAgentNotice(settingsPath, value)
	}
	if state, value, err := a.hooks.InspectOutputStyle(settingsPath); err == nil && state == settings.OutputStyleConflict {
		notices += "; " + settings.ForeignOutputStyleNotice(settingsPath, value)
	}
	return notices
}

// configPaths resolves the settings file and the hook command from the Claude root: the file is
// <root>/settings.json and the binary its hooks run is <root>/bin/gentle-ai-overlay. A root that
// is blank or not absolute names neither, and an adapter built without its hook installer cannot
// work at all.
func (a ClaudeAdapter) configPaths() (settingsPath, hookCommand string, err error) {
	if missing(a.hooks) {
		return "", "", errors.New("Claude hook installer is not set; the adapter was built without one")
	}
	if strings.TrimSpace(a.root) == "" {
		return "", "", errors.New("Claude config root could not be resolved; set HOME")
	}
	if !filepath.IsAbs(a.root) {
		return "", "", fmt.Errorf("Claude config root must be absolute, got %q", a.root)
	}
	return filepath.Join(a.root, "settings.json"), filepath.Join(a.root, "bin", "gentle-ai-overlay"), nil
}

// ownerRules is the owner rules half of the adapter, over the same root and the source it was given.
func (a ClaudeAdapter) ownerRules() claudeOwnerRules {
	return claudeOwnerRules{root: a.root, rules: a.rules}
}

// withOwnerRules is the result of an install or update whose hooks are in place: it writes the owner
// rules block and adds what it did to message, or reports partial when the rules could not be read
// or the file could not be written (the hooks stay installed; CLAUDE.md stays as it was). An adapter
// without a source writes nothing and reports the hooks alone.
func (a ClaudeAdapter) withOwnerRules(action core.Action, message string) core.LifecycleResult {
	if a.rules == nil {
		return a.result(action, core.CapabilityRestartRequired, message)
	}
	clause, status := a.ownerRules().write()
	return a.result(action, status, message+"; "+clause)
}

func (a ClaudeAdapter) result(action core.Action, status core.CapabilityStatus, message string) core.LifecycleResult {
	return core.NewLifecycleResult(a.target, action, status, message, nil)
}
