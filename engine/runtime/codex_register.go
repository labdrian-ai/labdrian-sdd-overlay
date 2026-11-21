package runtime

import (
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
)

// RegisterCodexWithOwnerRules registers the Codex runtime like RegisterCodex and has it keep the
// owner rules block in ~/AGENTS.md (the config's home), <codex root>/AGENTS.md and
// <codex root>/agents.md: the rules are read from the overlay checkout the config names
// (Config.LabdrianOverlayDir), or the nearest one above the working directory. The program wires
// this one; RegisterCodex stays for a caller that wants the manifest alone.
func RegisterCodexWithOwnerRules(r *core.Registry) error {
	return r.Register(core.TargetCodex, func(cfg core.Config) core.Adapter {
		return NewCodexAdapter(cfg.CodexRoot()).WithOwnerRules(OverlayOwnerRules{OverlayDir: cfg.LabdrianOverlayDir}, cfg.Home)
	})
}
