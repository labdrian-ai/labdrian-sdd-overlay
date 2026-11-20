package runtime

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
)

// claudeOwnerRules is the owner rules half of the Claude runtime: the block between the owner-rules
// markers of <root>/CLAUDE.md, written from rules. It never opens settings.json. ClaudeAdapter keeps
// the block through it next to the hooks, and ClaudeOwnerRulesAdapter keeps the block alone.
//
// Each verb answers a clause, the words that say what was done to the block, and the status the
// block leaves the action in; the caller joins the clause to its own message.
type claudeOwnerRules struct {
	root  string
	rules OwnerRulesSource
}

// path is <root>/CLAUDE.md, the file the owner rules block is written in.
func (c claudeOwnerRules) path() string { return filepath.Join(c.root, "CLAUDE.md") }

// write writes the owner rules block: the clause says what it did, and the status is partial when
// the rules could not be read or the file could not be written (CLAUDE.md stays as it was), else
// restart_required. No checkout is not a failure: the clause says nothing was installed.
func (c claudeOwnerRules) write() (string, core.CapabilityStatus) {
	path := c.path()
	body, err := c.rules.OwnerRules()
	if errors.Is(err, ErrNoOverlayCheckout) {
		return "owner rules not installed: " + err.Error(), core.CapabilityRestartRequired
	}
	if err != nil {
		return "owner rules could not be read: " + err.Error(), core.CapabilityPartial
	}
	change, err := OwnerRulesBlock.WriteFile(path, body)
	if err != nil {
		return "owner rules could not be written: " + err.Error(), core.CapabilityPartial
	}
	switch change {
	case BlockAdded:
		return "owner rules written to " + path, core.CapabilityRestartRequired
	case BlockReplaced:
		return "owner rules updated in " + path, core.CapabilityRestartRequired
	}
	return "owner rules already current in " + path, core.CapabilityRestartRequired
}

// remove takes the owner rules block out of CLAUDE.md. The clause is empty when there was no block
// to remove; the status is partial when the file could not be rewritten.
func (c claudeOwnerRules) remove() (string, core.CapabilityStatus) {
	removed, err := OwnerRulesBlock.RemoveFile(c.path())
	if err != nil {
		return "owner rules could not be removed: " + err.Error(), core.CapabilityPartial
	}
	if removed {
		return "owner rules removed from " + c.path(), core.CapabilityRestartRequired
	}
	return "", core.CapabilityRestartRequired
}

// status is the status message of a block that is not current: partial, naming the file and the
// verb that fixes it. ok is false when the block is current or no checkout was found; note then
// says which, for the supported message.
func (c claudeOwnerRules) status() (message string, ok bool, note string) {
	path := c.path()
	body, err := c.rules.OwnerRules()
	if errors.Is(err, ErrNoOverlayCheckout) {
		return "", false, "owner rules not checked: " + err.Error()
	}
	if err != nil {
		return "Claude owner rules could not be read: " + err.Error(), true, ""
	}
	state, err := OwnerRulesBlock.StateOfFile(path, body)
	if err != nil {
		return "Claude owner rules block could not be checked: " + err.Error(), true, ""
	}
	switch state {
	case BlockMissing:
		return "Claude owner rules block is missing in " + path + "; run 'engine runtime install --target claude'", true, ""
	case BlockStale:
		return "Claude owner rules block is stale in " + path + "; run 'engine runtime update --target claude'", true, ""
	}
	return "", false, "owner rules block is current in " + path
}

// ClaudeOwnerRulesAdapter is the Claude runtime reduced to the owner rules block of CLAUDE.md. It is
// built with no hook installer, so no action of it can reach settings.json: that file is the hook
// installer's alone. Install and update write the block, uninstall removes it, status reads it.
type ClaudeOwnerRulesAdapter struct {
	rules claudeOwnerRules
}

// NewClaudeOwnerRulesAdapter builds the adapter over root, the Claude directory CLAUDE.md is in
// (see Config.ClaudeRoot), and the source of the rules.
func NewClaudeOwnerRulesAdapter(root string, rules OwnerRulesSource) ClaudeOwnerRulesAdapter {
	return ClaudeOwnerRulesAdapter{rules: claudeOwnerRules{root: root, rules: rules}}
}

func (a ClaudeOwnerRulesAdapter) Target() core.Target         { return core.TargetClaude }
func (a ClaudeOwnerRulesAdapter) Apply() core.LifecycleResult { return a.Install() }
func (a ClaudeOwnerRulesAdapter) Install() core.LifecycleResult {
	return a.write(core.ActionInstall)
}
func (a ClaudeOwnerRulesAdapter) Update() core.LifecycleResult {
	return a.write(core.ActionUpdate)
}
func (a ClaudeOwnerRulesAdapter) Rollback() core.LifecycleResult { return a.Uninstall() }
func (a ClaudeOwnerRulesAdapter) Uninstall() core.LifecycleResult {
	if err := a.usable(); err != nil {
		return a.result(core.ActionUninstall, core.CapabilityUnsupported, err.Error())
	}
	clause, status := a.rules.remove()
	return a.result(core.ActionUninstall, status, clause)
}
func (a ClaudeOwnerRulesAdapter) SyncCheck() core.LifecycleResult { return a.Status() }
func (a ClaudeOwnerRulesAdapter) Status() core.LifecycleResult {
	if err := a.usable(); err != nil {
		return a.result(core.ActionStatus, core.CapabilityUnsupported, err.Error())
	}
	message, partial, note := a.rules.status()
	switch {
	case partial:
		return a.result(core.ActionStatus, core.CapabilityPartial, message)
	case strings.HasPrefix(note, "owner rules not checked"):
		return a.result(core.ActionStatus, core.CapabilityUnsupported, note)
	}
	return a.result(core.ActionStatus, core.CapabilitySupported, note)
}

func (a ClaudeOwnerRulesAdapter) write(action core.Action) core.LifecycleResult {
	if err := a.usable(); err != nil {
		return a.result(action, core.CapabilityUnsupported, err.Error())
	}
	clause, status := a.rules.write()
	return a.result(action, status, clause)
}

// usable says whether the adapter has a Claude root and a source of rules to work with.
func (a ClaudeOwnerRulesAdapter) usable() error {
	if a.rules.rules == nil {
		return errors.New("Claude owner rules source is not set; the adapter was built without one")
	}
	if strings.TrimSpace(a.rules.root) == "" {
		return errors.New("Claude config root could not be resolved; set HOME")
	}
	if !filepath.IsAbs(a.rules.root) {
		return fmt.Errorf("Claude config root must be absolute, got %q", a.rules.root)
	}
	return nil
}

func (a ClaudeOwnerRulesAdapter) result(action core.Action, status core.CapabilityStatus, message string) core.LifecycleResult {
	return core.NewLifecycleResult(core.TargetClaude, action, status, message, nil)
}
