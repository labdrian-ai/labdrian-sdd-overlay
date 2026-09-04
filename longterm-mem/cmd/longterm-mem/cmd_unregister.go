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
// entry with this name exists that longterm-mem never wrote. A path that
// could not be resolved from the environment exits 8
// (exitPathUnresolvable), never sharing exit 1 with a target that was
// attempted and failed.
//
// One asymmetry with cmdRegister is deliberate and documented at
// register.uninstallCannotDeriveOwnership: register can re-derive
// ownership from an entry's own content when install-state.json is lost,
// and unregister cannot, because it never resolves a binary path and so
// has no entry to compare against. With the record lost, `unregister`
// therefore still reports the entry unmanaged; running `register` first
// restores the record, after which `unregister` removes it normally. See
// that constant's own comment for what the packaged overlay uninstall
// currently does with the exit 6 that reports it, and why this command's
// recovery story does not survive that caller unchanged.
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
		return exitPathUnresolvable
	}

	// Every target in the expanded list is attempted before the exit code
	// is decided, exactly like cmdRegister/cmdDoctor -- one target's
	// outcome never hides another target's.
	unmanaged, failed, unresolvable := false, false, false
	for _, tgt := range targets {
		root := *configRoot
		if root == "" {
			root = defaultRegisterConfigRoot(tgt)
		}
		if root == "" {
			fmt.Fprintf(os.Stderr, "longterm-mem: unregister: %s: could not resolve a config root; set HOME or pass --config-root\n", tgt)
			unresolvable = true
			continue
		}

		// register.Unregister's own errors already name this command and
		// the target it was working on ("unregister: claude: ..."), so
		// this prints the error alone rather than re-stating either.
		// Re-stating them is what produced the stacked prefix on the
		// register side; printing the error alone while the writers still
		// said "register:" is what made unregister report its failures
		// under the other subcommand's name. Exactly one layer names the
		// command, and it is the writer's per-target wrap (writer.go).
		outcome, unregErr := register.Unregister(tgt, root, resolvedStateDir)
		if unregErr != nil {
			fmt.Fprintf(os.Stderr, "longterm-mem: %v\n", unregErr)
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

	// A hard failure outranks an unresolvable path, which outranks an
	// unmanaged report -- cmdRegister's own precedence, for the same
	// reasons: unmanaged is a recoverable outcome the caller resolves by
	// hand, an unresolvable path means the target was never evaluated and
	// the remedy is the caller's environment, and a hard failure means a
	// target was evaluated and broke.
	switch {
	case failed:
		return 1
	case unresolvable:
		return exitPathUnresolvable
	case unmanaged:
		return 6
	default:
		return 0
	}
}
