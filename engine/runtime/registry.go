package runtime

import (
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
)

// Transitional: the Registry and its Factory live in runtime/core. They keep their old names here
// while the adapters and the command are switched; the commit that has switched the last of them
// deletes these three declarations and renames this file register.go.
type Factory = core.Factory

type Registry = core.Registry

func NewRegistry() *Registry { return core.NewRegistry() }

// RegisterClaude registers the Claude runtime.
func RegisterClaude(r *Registry) error {
	return r.Register(TargetClaude, func(cfg Config) Adapter { return NewClaudeAdapter(cfg.ClaudeRoot()) })
}

// RegisterCodex registers the Codex runtime.
func RegisterCodex(r *Registry) error {
	return r.Register(TargetCodex, func(cfg Config) Adapter { return NewCodexAdapter(cfg.CodexRoot()) })
}

// RegisterOpenCode registers the OpenCode runtime.
func RegisterOpenCode(r *Registry) error {
	return r.Register(TargetOpenCode, func(cfg Config) Adapter { return NewOpenCodeAdapter(cfg.OpenCodeRoot()) })
}

// RegisterPi registers the Pi runtime, which reaches the `pi` CLI and the package builder only
// through ports. It refuses ports with a nil member: an adapter built without them would fail at
// the first lifecycle step it runs, long after the program was wired.
func RegisterPi(r *Registry, ports PiPorts) error {
	if ports.Commands == nil {
		return fmt.Errorf("runtime %q registered with no command runner", TargetPi)
	}
	if ports.Packages == nil {
		return fmt.Errorf("runtime %q registered with no package builder", TargetPi)
	}
	return r.Register(TargetPi, func(cfg Config) Adapter {
		return NewPiAdapter(ports,
			PiPaths{Home: cfg.Home, OverlayRoot: cfg.OverlayDir, DestDir: cfg.PiPackageDir()},
			PiOptions{SkipSubagents: cfg.PiSkipSubagents})
	})
}
