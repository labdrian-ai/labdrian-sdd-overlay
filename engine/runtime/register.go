package runtime

import (
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
)

// The Register functions add one runtime each to a Registry the composition root owns. The
// Registry, its Factory and the Config a factory is handed are the core's (runtime/core); what
// belongs here is knowing which adapter answers to which target.

// RegisterClaude registers the Claude runtime.
func RegisterClaude(r *core.Registry) error {
	return r.Register(core.TargetClaude, func(cfg core.Config) core.Adapter { return NewClaudeAdapter(cfg.ClaudeRoot()) })
}

// RegisterCodex registers the Codex runtime.
func RegisterCodex(r *core.Registry) error {
	return r.Register(core.TargetCodex, func(cfg core.Config) core.Adapter { return NewCodexAdapter(cfg.CodexRoot()) })
}

// RegisterOpenCode registers the OpenCode runtime.
func RegisterOpenCode(r *core.Registry) error {
	return r.Register(core.TargetOpenCode, func(cfg core.Config) core.Adapter { return NewOpenCodeAdapter(cfg.OpenCodeRoot()) })
}

// RegisterPi registers the Pi runtime, which reaches the `pi` CLI and the package builder only
// through ports. It refuses ports with a nil member: an adapter built without them would fail at
// the first lifecycle step it runs, long after the program was wired.
func RegisterPi(r *core.Registry, ports PiPorts) error {
	if ports.Commands == nil {
		return fmt.Errorf("runtime %q registered with no command runner", core.TargetPi)
	}
	if ports.Packages == nil {
		return fmt.Errorf("runtime %q registered with no package builder", core.TargetPi)
	}
	return r.Register(core.TargetPi, func(cfg core.Config) core.Adapter {
		return NewPiAdapter(ports,
			PiPaths{Home: cfg.Home, OverlayRoot: cfg.OverlayDir, DestDir: cfg.PiPackageDir()},
			PiOptions{SkipSubagents: cfg.PiSkipSubagents})
	})
}
