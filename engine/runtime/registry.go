package runtime

import (
	"fmt"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capability"
)

// Factory builds the adapter of one runtime from the configuration the composition root resolved.
type Factory func(Config) Adapter

// Registry is the set of runtimes a program can act on, each with the Factory that builds its
// adapter. The composition root makes one, registers every runtime it ships, and hands it to
// whoever needs to parse a target or build an adapter; nothing is registered by being imported.
//
// A target is registered under a name of the capability vocabulary (capability.IsTarget), so a
// runtime that declares nothing about itself cannot be added, and an unregistered target is an
// error wherever it is asked for, never a stand-in that reports "unsupported".
type Registry struct {
	factories map[Target]Factory
	order     []Target
}

// NewRegistry returns a registry with no runtime in it.
func NewRegistry() *Registry {
	return &Registry{factories: map[Target]Factory{}}
}

// Register adds the runtime target. It refuses a name outside the capability vocabulary, a nil
// Factory, and a target already registered, so two adapters never answer to one name.
func (r *Registry) Register(target Target, factory Factory) error {
	if !capability.IsTarget(string(target)) {
		return fmt.Errorf("runtime %q is not a declared target (capability.Targets: %s)", target, strings.Join(capability.Targets(), ", "))
	}
	if factory == nil {
		return fmt.Errorf("runtime %q registered with no factory", target)
	}
	if _, taken := r.factories[target]; taken {
		return fmt.Errorf("runtime %q is already registered", target)
	}
	r.factories[target] = factory
	r.order = append(r.order, target)
	return nil
}

// New builds the adapter of target. A target that is not registered, and `all`, which names
// several, are errors; so is a factory that answers with the adapter of another runtime.
func (r *Registry) New(target Target, cfg Config) (Adapter, error) {
	factory, ok := r.factories[target]
	if !ok {
		return nil, fmt.Errorf("runtime %q is not registered", target)
	}
	adapter := factory(cfg)
	if adapter == nil {
		return nil, fmt.Errorf("runtime %q: its factory built no adapter", target)
	}
	if got := adapter.Target(); got != target {
		return nil, fmt.Errorf("runtime %q: its factory built the adapter of %q", target, got)
	}
	return adapter, nil
}

// Targets lists the registered runtimes in the order they were registered.
func (r *Registry) Targets() []Target {
	return append([]Target(nil), r.order...)
}

// Parse reads a --target value: a registered runtime or `all`, ignoring surrounding space.
func (r *Registry) Parse(raw string) (Target, error) {
	target := Target(strings.TrimSpace(raw))
	if target == TargetAll {
		return TargetAll, nil
	}
	if _, ok := r.factories[target]; ok {
		return target, nil
	}
	return "", fmt.Errorf("unknown target %q", raw)
}

// Expand lists the runtimes a target stands for: every registered one, in registration order,
// for `all`, and the target itself otherwise.
func (r *Registry) Expand(target Target) []Target {
	if target == TargetAll {
		return r.Targets()
	}
	return []Target{target}
}

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
