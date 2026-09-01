package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/register"
)

// cmdUnregister implements `longterm-mem unregister --target
// claude|opencode|codex|all [--config-root DIR] [--state-dir DIR]`
// (R-019). It mirrors cmdRegister's own flag parsing, per-target dispatch,
// and exit-code precedence, reusing register_paths.go's
// registerExpandTarget/defaultRegisterConfigRoot/defaultRegisterStateDir
// unchanged -- --target expands the same way for both directions. There is
// no --binary flag here: unregister writes nothing, so there is no entry
// shape to resolve a binary path for. There is also no --target all "skip
// a missing config" special case the way cmdRegister needs one (11b.7/
// 12a.6): a runtime whose config file does not exist is exactly
// register.Unregister's own UnregisterNoop outcome, never an error (see
// jsonUninstall/tomlUninstall's own doc comments, writer.go) -- so there is
// nothing for --target all to skip around.
//
// An untagged same-named entry (register.UnregisterUnmanaged, R-019
// "Untagged entry is preserved and reported, not removed") exits 6 --
// registration_conflict, the same code cmdRegister's own ErrConflict
// refusal uses, since both name the exact same underlying situation: an
// entry with this name exists that longterm-mem never wrote.
func cmdUnregister(args []string) int {
	fs := flag.NewFlagSet("unregister", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	target := fs.String("target", "", "runtime target: claude|opencode|codex|all (required)")
	configRoot := fs.String("config-root", "", "override the runtime config root directory (single target only)")
	stateDir := fs.String("state-dir", "", "override install-state.json's directory")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *target == "" {
		fmt.Fprintln(os.Stderr, "longterm-mem: unregister: --target is required")
		return 2
	}

	targets, err := registerExpandTarget(*target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "longterm-mem: unregister: %v\n", err)
		return 2
	}
	if *configRoot != "" && len(targets) > 1 {
		fmt.Fprintln(os.Stderr, "longterm-mem: unregister: --config-root requires a single --target, not \"all\"")
		return 2
	}

	resolvedStateDir := *stateDir
	if resolvedStateDir == "" {
		resolvedStateDir = defaultRegisterStateDir()
	}
	// See cmdRegister's identical guard: an unresolvable state dir must
	// never degrade into a silent fallback that reads/writes
	// install-state.json wherever the process working directory happens
	// to be.
	if resolvedStateDir == "" {
		fmt.Fprintln(os.Stderr, "longterm-mem: unregister: could not resolve the install-state directory; set HOME or pass --state-dir")
		return 1
	}

	// Every target in the expanded list is attempted before the exit code
	// is decided, exactly like cmdRegister/cmdDoctor -- one target's
	// outcome never hides another target's.
	unmanaged, failed := false, false
	for _, tgt := range targets {
		root := *configRoot
		if root == "" {
			root = defaultRegisterConfigRoot(tgt)
		}
		if root == "" {
			fmt.Fprintf(os.Stderr, "longterm-mem: unregister: %s: could not resolve a config root; set HOME or pass --config-root\n", tgt)
			failed = true
			continue
		}

		outcome, unregErr := register.Unregister(tgt, root, resolvedStateDir)
		if unregErr != nil {
			fmt.Fprintf(os.Stderr, "longterm-mem: unregister: %s: %v\n", tgt, unregErr)
			failed = true
			continue
		}
		if outcome == register.UnregisterUnmanaged {
			fmt.Printf("longterm-mem: unregister: %s: unmanaged (an entry exists that longterm-mem does not own; left untouched)\n", tgt)
			unmanaged = true
			continue
		}
		fmt.Printf("longterm-mem: unregister: %s: %s\n", tgt, outcome)
	}

	// A hard failure outranks an unmanaged report, mirroring cmdRegister's
	// own "hard failure outranks conflict" precedence: unmanaged is a
	// recoverable, expected outcome the caller resolves by hand, while a
	// hard failure means a target could not even be evaluated.
	switch {
	case failed:
		return 1
	case unmanaged:
		return 6
	default:
		return 0
	}
}
