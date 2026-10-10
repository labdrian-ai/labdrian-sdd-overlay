package main

// The 'runtime <action>' subcommand: status, install, update and uninstall of the runtime adapters and of the longterm-mem component.
// (capabilities and probe are runtime_capabilities.go and runtime_probe.go.)

import (
	"fmt"
	"io"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/execrunner"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/pipkg"
	runtimepkg "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
	runtimecore "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings/settingsfile"
)

// runRuntime implements the 'runtime <action>' subcommand.
// Supported actions: status, install, update, uninstall.
func runRuntime(p process, d deps, args []string) {
	runRuntimeCore(d, execrunner.New(), newPipkgSource(d.environment()), args, p.stdout, p.stderr, p.exit)
}

// componentRuntimeParity and componentLongtermMem are the two values
// --component accepts (D4). componentRuntimeParity is the default and
// preserves every pre-existing --target-based behavior unchanged (10a.7).
const (
	componentRuntimeParity = "runtime-parity"
	componentLongtermMem   = "longterm-mem"
)

// runRuntimeCore is the testable core for the 'runtime' subcommand. commands is how the Pi
// adapter starts the `pi` CLI: the process adapter in the program, a fake in a test, so no test of
// the command can reach a real `pi`. source is how the Pi package builder asks git about the
// overlay: the git of the machine in the program, pipkg.NoRepository in a test.
func runRuntimeCore(d deps, commands runtimepkg.CommandRunner, source pipkg.SourceRepo, args []string, stdout io.Writer, stderr io.Writer, exit func(int)) {
	// capabilities is declarative and read-only: it never constructs an
	// adapter, resolves a config root, or reads HOME, so it is dispatched
	// before the lifecycle flags are parsed and shares none of their
	// defaults (its --target defaults to all, not opencode).
	if len(args) > 0 && args[0] == "capabilities" {
		runRuntimeCapabilities(args[1:], stdout, stderr, exit)
		return
	}
	// probe is read-only too: it stats the presence signals under the process's
	// home and PATH and shares none of the lifecycle flags or defaults.
	if len(args) > 0 && args[0] == "probe" {
		runRuntimeProbe(d, args[1:], stdout, stderr, exit)
		return
	}

	registry, err := newRuntimeRegistry(newWarningRegistryRepository(stderr), settingsfile.Installer{}, commands, source, pipkgOptionsFromEnv(d.env))
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		exit(1)
		return
	}

	opts, err := parseRuntimeArgs(args, registry)
	if err != nil {
		fmt.Fprintln(stderr, err)
		usage(stderr)
		exit(1)
		return
	}

	cfg := runtimeConfigFromEnv(d.env, d.homeDir)

	if opts.Component == componentLongtermMem {
		runLongtermMemComponent(opts, cfg, stdout, stderr, exit)
		return
	}

	cfg.ConfigRoot = opts.ConfigRoot
	targets := registry.Expand(opts.Target)

	// Every adapter is built before the first one acts, so a target the registry cannot build
	// stops the command before anything has been done, never half way through `all`.
	adapters, err := buildRuntimeAdapters(registry, targets, cfg)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		exit(1)
		return
	}

	// Each target acts and is reported before the next one acts. Pi has a real Status(), so it is
	// reported and aggregated like every other target: an honestly unsupported Pi fails
	// `status --target all` just as an honestly unsupported claude, opencode or codex would
	// (W-03 -- there is no "Pi is exempt" status-only carve-out). What fails a run is
	// runtimecore.AggregateStatus's.
	results := make([]runtimecore.LifecycleResult, 0, len(adapters))
	for _, adapter := range adapters {
		result := runtimecore.Perform(adapter, opts.Action)
		fmt.Fprintln(stdout, result.String())
		results = append(results, result)
	}

	if runtimecore.AggregateStatus(opts.Action, results) {
		exit(1)
		return
	}
	exit(0)
}

// runLongtermMemComponent runs the action of 'runtime --component longterm-mem': one component
// that spans claude, opencode and codex, acted on as a whole and not as a target of the registry.
func runLongtermMemComponent(opts runtimeOptions, cfg runtimecore.Config, stdout, stderr io.Writer, exit func(int)) {
	// D4 parse-time refusal: update is rejected here, BEFORE any
	// LongtermMemAdapter is even constructed — never after running one
	// and reporting a failing status. "rollback" needs no separate
	// guard: it is not a recognized action at all (see the action-name
	// validation in parseRuntimeArgs below), so it is already rejected
	// at the exact same point, before any adapter call.
	if opts.Action == runtimecore.ActionUpdate {
		fmt.Fprintln(stderr, "error: longterm-mem does not support the 'update' action; reinstall instead (--component longterm-mem install)")
		exit(1)
		return
	}
	// The binary path is DERIVED from --state-dir, never resolved
	// independently from HOME: the overlay entrypoint deploys the
	// binary at "$STATE_DIR/bin/longterm-mem" and registers MCP
	// entries naming that exact path, so an adapter that resolved it
	// from HOME under an overridden state dir reported a deployed
	// binary as missing and a genuinely owned entry as unmanaged. An
	// empty stateDir yields an empty binary path here, which
	// NewLongtermMemAdapter fills in with the same default it fills
	// stateDir with — so the un-overridden case is unchanged.
	adapter := runtimepkg.NewLongtermMemAdapter(cfg, opts.StateDir, runtimepkg.LongtermMemBinaryPathForStateDir(opts.StateDir))
	result := runtimecore.Perform(adapter, opts.Action)
	fmt.Fprintln(stdout, result.String())
	if runtimecore.ComponentFailed(opts.Action, result) {
		exit(1)
		return
	}
	exit(0)
}

// runtimeOptions is what the command line of 'runtime <action>' asks for. The zero value of a
// field means the flag was not given; Target and Component carry their defaults after parsing.
type runtimeOptions struct {
	// Action is one of status, install, update and uninstall.
	Action runtimecore.Action
	// Target is the runtime acted on, or all of them (--component runtime-parity only).
	Target runtimecore.Target
	// ConfigRoot, when it is not empty, is the one directory every runtime works in (--config-root).
	ConfigRoot string
	// Component is componentRuntimeParity or componentLongtermMem.
	Component string
	// StateDir is the state directory of the longterm-mem component (--state-dir).
	StateDir string
}

// parseRuntimeArgs reads the command line of 'runtime <action>' into its options. The flags are
// read, and the first one that cannot be used ends the parse, before the action is judged. On an
// error the options are empty.
func parseRuntimeArgs(args []string, registry *runtimecore.Registry) (runtimeOptions, error) {
	if len(args) == 0 {
		return runtimeOptions{}, fmt.Errorf("error: runtime requires an action")
	}
	action := args[0]
	if strings.HasPrefix(action, "-") {
		return runtimeOptions{}, fmt.Errorf("error: runtime requires an action: status | install | update | uninstall | capabilities")
	}

	opts := runtimeOptions{Target: runtimecore.TargetOpenCode, Component: componentRuntimeParity}
	for i := 1; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--target":
			i++
			if i >= len(args) {
				return runtimeOptions{}, fmt.Errorf("error: --target requires a value")
			}
			target, err := registry.Parse(args[i])
			if err != nil {
				return runtimeOptions{}, err
			}
			opts.Target = target
		case "--config-root":
			i++
			if i >= len(args) {
				return runtimeOptions{}, fmt.Errorf("error: --config-root requires a value")
			}
			opts.ConfigRoot = args[i]
		case "--component":
			i++
			if i >= len(args) {
				return runtimeOptions{}, fmt.Errorf("error: --component requires a value")
			}
			switch args[i] {
			case componentRuntimeParity, componentLongtermMem:
				opts.Component = args[i]
			default:
				return runtimeOptions{}, fmt.Errorf("error: unknown --component %q (expected %q or %q)", args[i], componentRuntimeParity, componentLongtermMem)
			}
		case "--state-dir":
			i++
			if i >= len(args) {
				return runtimeOptions{}, fmt.Errorf("error: --state-dir requires a value")
			}
			opts.StateDir = args[i]
		default:
			if strings.HasPrefix(a, "--") {
				return runtimeOptions{}, fmt.Errorf("error: unknown flag %q", a)
			}
			return runtimeOptions{}, fmt.Errorf("error: unexpected runtime argument %q", a)
		}
	}

	switch runtimecore.Action(action) {
	case runtimecore.ActionStatus, runtimecore.ActionInstall, runtimecore.ActionUpdate, runtimecore.ActionUninstall:
		opts.Action = runtimecore.Action(action)
	default:
		return runtimeOptions{}, fmt.Errorf("error: unknown runtime action %q", action)
	}
	return opts, nil
}
