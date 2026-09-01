package main

import (
	"errors"
	"flag"
	"fmt"
	iofs "io/fs" // aliased: fs is the FlagSet variable below
	"os"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/register"
)

// cmdRegister implements `longterm-mem register --target
// claude|opencode|codex|all [--config-root DIR] [--state-dir DIR]
// [--binary PATH]` (R-016, R-017, R-018). It drives internal/register's
// per-runtime writers, which own the actual D9 decision
// (insert/replace/refuse/noop) and byte-preserving edit, and it resolves
// targets and default paths through register_paths.go; what is left here
// is the effectful shell -- flag parsing, per-target dispatch, and mapping
// each writer's outcome to a process exit code. An untagged same-named
// conflict (register.ErrConflict) exits 6, any other per-target failure
// exits 1, success exits 0 -- except that a --target all expansion skips a
// runtime whose configuration file is absent, since that is how a runtime
// nobody installed looks from here. Like cmdDoctor, every target in the expanded
// list is attempted before the exit code is decided, so one target's
// failure never hides another target's success or failure.
func cmdRegister(args []string) int {
	fs := flag.NewFlagSet("register", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	target := fs.String("target", "", "runtime target: claude|opencode|codex|all (required)")
	configRoot := fs.String("config-root", "", "override the runtime config root directory (single target only)")
	stateDir := fs.String("state-dir", "", "override install-state.json's directory")
	binary := fs.String("binary", "", "override the longterm-mem binary path written into the entry")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *target == "" {
		fmt.Fprintln(os.Stderr, "longterm-mem: register: --target is required")
		return 2
	}

	targets, err := registerExpandTarget(*target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "longterm-mem: register: %v\n", err)
		return 2
	}
	if *configRoot != "" && len(targets) > 1 {
		fmt.Fprintln(os.Stderr, "longterm-mem: register: --config-root requires a single --target, not \"all\"")
		return 2
	}

	resolvedStateDir := *stateDir
	if resolvedStateDir == "" {
		resolvedStateDir = defaultRegisterStateDir()
	}
	resolvedBinary := *binary
	if resolvedBinary == "" {
		resolvedBinary = defaultRegisterBinaryPath()
	}
	// An unresolvable default path must never degrade into a silent
	// fallback. An empty binary path installs a structurally valid entry
	// that can never start a server, and an empty state dir writes
	// install-state.json into whatever the process working directory
	// happens to be -- losing the ownership record the next run needs to
	// tell its own write from a hand edit. Both are refused before any
	// target is touched, rather than written and reported as ok.
	if resolvedStateDir == "" {
		fmt.Fprintln(os.Stderr, "longterm-mem: register: could not resolve the install-state directory; set HOME or pass --state-dir")
		return 1
	}
	if resolvedBinary == "" {
		fmt.Fprintln(os.Stderr, "longterm-mem: register: could not resolve the longterm-mem binary path; set HOME or pass --binary")
		return 1
	}

	// expandedAll distinguishes "register everything this machine has"
	// from "register this one runtime, which I am telling you is there".
	// Only the first may skip a runtime whose config is absent.
	expandedAll := len(targets) > 1
	conflict, failed := false, false
	for _, tgt := range targets {
		root := *configRoot
		if root == "" {
			root = defaultRegisterConfigRoot(tgt)
		}
		if root == "" {
			fmt.Fprintf(os.Stderr, "longterm-mem: register: %s: could not resolve a config root; set HOME or pass --config-root\n", tgt)
			failed = true
			continue
		}

		regErr := registerTarget(tgt, root, resolvedStateDir, resolvedBinary)
		switch {
		case regErr == nil:
		case errors.Is(regErr, register.ErrConflict):
			fmt.Fprintf(os.Stderr, "longterm-mem: register: %s: %v\n", tgt, regErr)
			conflict = true
			continue
		case expandedAll && errors.Is(regErr, iofs.ErrNotExist):
			// --target all means "every runtime this machine has", not
			// "every runtime that exists" -- few machines have all three.
			// A missing config is how a runtime that is not installed
			// looks from here, and failing the whole run over it would
			// report a failure for a run in which the runtimes that ARE
			// installed were registered correctly. Asking for one runtime
			// by name is a different statement, and still fails below.
			fmt.Printf("longterm-mem: register: %s: skipped (no configuration found at %s)\n", tgt, root)
			continue
		default:
			fmt.Fprintf(os.Stderr, "longterm-mem: register: %s: %v\n", tgt, regErr)
			failed = true
			continue
		}
		fmt.Printf("longterm-mem: register: %s: ok\n", tgt)
	}

	// A hard failure outranks a conflict. A conflict is an expected,
	// recoverable outcome the caller resolves by hand; a hard failure
	// means a target was not registered at all. With --target all the two
	// can happen in the same run, and reporting the softer of them would
	// hide the harder one behind an exit code that reads as "nothing
	// broke".
	switch {
	case failed:
		return 1
	case conflict:
		return 6
	default:
		return 0
	}
}

// registerTarget dispatches to the one runtime writer target names.
func registerTarget(target, configRoot, stateDir, binary string) error {
	switch target {
	case "claude":
		return register.RegisterClaude(configRoot, stateDir, binary)
	case "opencode":
		return register.RegisterOpencode(configRoot, stateDir, binary)
	case "codex":
		return register.RegisterCodex(configRoot, stateDir, binary)
	default:
		return fmt.Errorf("register: %s: unknown target", target)
	}
}
