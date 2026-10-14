package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/register"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/runtimes"
)

// cmdRegister implements `longterm-mem register --target
// claude|opencode|codex|pi|all [--config-root DIR] [--state-dir DIR]
// [--binary PATH]` (R-016, R-017, R-018). What it does is the registration
// use case's (internal/runtimes: which runtimes a target names, what is
// refused before any is touched, the pi probe, the defaults, the loop over
// the runtimes that attempts every one whatever happened to the others,
// and the verdict among their outcomes), over the per-runtime writers of
// internal/register, which own the D9 decision (insert/replace/refuse/noop)
// and the byte-preserving edit; what is left here is the effectful shell --
// flag parsing, and rendering the report with the exit code of the
// contract. An untagged same-named conflict exits 6, a path that could not
// be resolved from the environment exits 8 (exitPathUnresolvable), any
// other per-target failure exits 1, success exits 0 -- except that a
// --target all expansion skips a runtime whose configuration file is
// absent, since that is how a runtime nobody installed looks from here.
func cmdRegister(args []string) int {
	fs := flag.NewFlagSet("register", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	target := fs.String("target", "", "runtime target: claude|opencode|codex|all (required)")
	configRoot := fs.String("config-root", "", "override the runtime config root directory (single target only)")
	stateDir := fs.String("state-dir", "", "override install-state.json's directory")
	binary := fs.String("binary", "", "override the longterm-mem binary path written into the entry")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	report := machineRuntimes().Register(runtimes.Request{Target: *target, ConfigRoot: *configRoot, StateDir: *stateDir, Binary: *binary})
	return reportRegister(report)
}

// reportRegister says what a run of register did, runtime by runtime, and answers the exit code of it.
//
// Every error the writers return already names the package and the target it was working on ("register: claude:
// ..."), so it is printed alone rather than re-stating both. Re-stating them is how the conflict message came to
// read "longterm-mem: register: claude: register: claude: register: an entry ...".
func reportRegister(report runtimes.Report) int {
	if report.Refusal != nil {
		return refuseRegistration("register", *report.Refusal)
	}
	for _, result := range report.Results {
		switch result.Outcome {
		case runtimes.Done:
			fmt.Printf("longterm-mem: register: %s: ok\n", result.Target)
		case runtimes.Skipped:
			fmt.Printf("longterm-mem: register: %s: skipped (no configuration found at %s)\n", result.Target, result.Root)
		case runtimes.RootUnresolved:
			fmt.Fprintf(os.Stderr, "longterm-mem: register: %s: could not resolve a config root; set HOME or pass --config-root\n", result.Target)
		default: // an entry that is not ours, or a failure: the writer says which, in the words of its error
			fmt.Fprintf(os.Stderr, "longterm-mem: %v\n", result.Err)
		}
	}
	return registrationExitCode(report.Failure())
}

// refuseRegistration says why a request of command (register or unregister) was refused before any runtime was
// touched, and answers the exit code of the refusal.
func refuseRegistration(command string, refusal runtimes.Refusal) int {
	fmt.Fprintf(os.Stderr, "longterm-mem: %s: %s\n", command, refusalWords(refusal))
	switch refusal.Kind {
	case runtimes.StateDirUnresolved, runtimes.BinaryUnresolved:
		return exitPathUnresolvable
	case runtimes.PiNotInstalled:
		return exitInternal
	default:
		return exitUsage
	}
}

// refusalWords is what a refused request is told, with the flags that put it right.
func refusalWords(refusal runtimes.Refusal) string {
	switch refusal.Kind {
	case runtimes.TargetRequired:
		return "--target is required"
	case runtimes.UnknownTarget:
		return fmt.Sprintf("unknown --target %q (want claude|opencode|codex|pi|all)", refusal.Target)
	case runtimes.ConfigRootNeedsOneTarget:
		return `--config-root requires a single --target, not "all"`
	case runtimes.StateDirUnresolved:
		return "could not resolve the install-state directory; set HOME or pass --state-dir"
	case runtimes.BinaryUnresolved:
		return "could not resolve the longterm-mem binary path; set HOME or pass --binary"
	case runtimes.PiNotInstalled:
		return fmt.Sprintf("pi: not installed (no package at %s, or not listed in ~/.pi/agent/settings.json); build it (labdrian-overlay apply --target pi) and run pi install first", refusal.Root)
	default:
		return "the request was refused"
	}
}

// registrationExitCode is the exit code of a run of register or unregister whose request was not refused. A hard
// failure outranks an unresolvable path, which outranks an entry that is not ours (the use case decides which is
// the verdict): a failure means a runtime was attempted and could not be done, an unresolvable path means a runtime
// was never attempted at all and the remedy is the caller's environment, and an entry that is not ours is an
// expected, recoverable outcome the caller resolves by hand.
func registrationExitCode(failure runtimes.Failure) int {
	switch failure {
	case runtimes.SomeTargetFailed:
		return exitInternal
	case runtimes.SomePathUnresolved:
		return exitPathUnresolvable
	case runtimes.SomeEntryNotOurs:
		return exitRegistrationConflict
	default:
		return exitOK
	}
}

// machineRuntimes is the registration use cases over this machine: the writers of the runtimes' configuration, which
// record what they installed in install-state.json, and the machine's own paths (register_paths.go).
func machineRuntimes() runtimes.Runtimes {
	return runtimes.Runtimes{
		Registrar: register.Writer{State: register.FileInstallState{}},
		Machine:   thisMachine{},
	}
}
