package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/runtimes"
)

// cmdUnregister implements `longterm-mem unregister --target
// claude|opencode|codex|pi|all [--config-root DIR] [--state-dir DIR]` (R-019).
// It mirrors cmdRegister: the same use case family (internal/runtimes) decides
// the targets, the refusals and the verdict, and the same rendering and exit
// codes apply -- --target expands the same way for both directions. There is
// no --binary flag here: unregister writes nothing, so there is no entry shape
// to resolve a binary path for. There is also no --target all "skip a missing
// config" special case the way register needs one (11b.7/12a.6): a runtime
// whose config file does not exist is exactly register.Unregister's own
// UnregisterNoop outcome, never an error (see jsonUninstall/tomlUninstall's
// own doc comments, writer.go) -- so there is nothing for --target all to skip
// around.
//
// An untagged same-named entry (runtimes.Unmanaged, R-019 "Untagged entry is
// preserved and reported, not removed") exits 6 -- registration_conflict, the
// same code register's own ErrConflict refusal uses, since both name the exact
// same underlying situation: an entry with this name exists that longterm-mem
// never wrote. A path that could not be resolved from the environment exits 8
// (exitPathUnresolvable), never sharing exit 1 with a target that was
// attempted and failed.
//
// One asymmetry with register is deliberate and documented at
// register.uninstallCannotDeriveOwnership: register can re-derive ownership
// from an entry's own content when install-state.json is lost, and unregister
// cannot, because it never resolves a binary path and so has no entry to
// compare against. With the record lost, `unregister` therefore still reports
// the entry unmanaged; running `register` first restores the record, after
// which `unregister` removes it normally. See that constant's own comment for
// what the packaged overlay uninstall currently does with the exit 6 that
// reports it, and why this command's recovery story does not survive that
// caller unchanged.
func cmdUnregister(args []string) int {
	fs := flag.NewFlagSet("unregister", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	target := fs.String("target", "", "runtime target: claude|opencode|codex|all (required)")
	configRoot := fs.String("config-root", "", "override the runtime config root directory (single target only)")
	stateDir := fs.String("state-dir", "", "override install-state.json's directory")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	report := machineRuntimes().Unregister(runtimes.Request{Target: *target, ConfigRoot: *configRoot, StateDir: *stateDir})
	return reportUnregister(report)
}

// reportUnregister says what a run of unregister did, runtime by runtime, and answers the exit code of it.
//
// The writers' own errors already name this command and the target it was working on ("unregister: claude: ..."),
// so a failure is printed alone rather than re-stating either. Re-stating them is what produced the stacked prefix
// on the register side; printing the error alone while the writers still said "register:" is what made unregister
// report its failures under the other subcommand's name. Exactly one layer names the command, and it is the
// writer's per-target wrap (writer.go).
func reportUnregister(report runtimes.Report) int {
	if report.Refusal != nil {
		return refuseRegistration("unregister", *report.Refusal)
	}
	for _, result := range report.Results {
		switch result.Outcome {
		case runtimes.Done:
			fmt.Printf("longterm-mem: unregister: %s: %s\n", result.Target, result.Removal)
		case runtimes.NotOurs:
			fmt.Printf("longterm-mem: unregister: %s: unmanaged (an entry exists that longterm-mem does not own; left untouched)\n", result.Target)
		case runtimes.RootUnresolved:
			fmt.Fprintf(os.Stderr, "longterm-mem: unregister: %s: could not resolve a config root; set HOME or pass --config-root\n", result.Target)
		default:
			fmt.Fprintf(os.Stderr, "longterm-mem: %v\n", result.Err)
		}
	}
	return registrationExitCode(report.Failure())
}
