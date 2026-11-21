package runtime

import (
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
)

// RegisterClaudeWithOwnerRules registers the Claude runtime like RegisterClaude and has it keep the
// owner rules block in CLAUDE.md: the rules are read from the overlay checkout the config names
// (Config.LabdrianOverlayDir), or the nearest one above the working directory. The program wires
// this one; RegisterClaude stays for a caller that wants the hooks alone.
func RegisterClaudeWithOwnerRules(r *core.Registry, hooks HookInstaller) error {
	if missing(hooks) {
		return fmt.Errorf("runtime %q registered with no hook installer", core.TargetClaude)
	}
	return r.Register(core.TargetClaude, func(cfg core.Config) core.Adapter {
		return NewClaudeAdapter(cfg.ClaudeRoot(), hooks).WithOwnerRules(OverlayOwnerRules{OverlayDir: cfg.LabdrianOverlayDir})
	})
}
