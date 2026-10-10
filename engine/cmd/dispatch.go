package main

import "fmt"

// run routes a command line to its subcommand. args are the arguments after the program's name:
// the subcommand, then its own. A command line that names none, or one that is not known, is a
// usage error. Every exit is followed by a return, because a test hands in an exit that ends
// nothing.
func run(p process, d deps, args []string) {
	if len(args) < 1 {
		usage(p.stderr)
		p.exit(1)
		return
	}
	rest := args[1:]
	switch args[0] {
	case "propagate":
		runPropagate(p, rest)
	case "gate-task":
		runGateTask(p, rest)
	case "merge-settings":
		runMergeSettings(p, rest)
	case "uninstall-hooks":
		runUninstallHooks(p, rest)
	case "status":
		runStatus(p, d)
	case "prespec":
		runPrespec(p, rest)
	case "runtime":
		runRuntime(rest)
	case "gadu-generate":
		runGaduGenerate(p, d, rest)
	case "pipkg":
		runPipkg(p, d, rest)
	case "skills":
		runSkills(rest)
	case "sync-trigger":
		runSyncTrigger(p, d, rest)
	case "review-receipt":
		runReviewReceipt(p, d, rest)
	case "shaper":
		runShaper(p, d, rest)
	case "roles":
		runRoles(p, rest)
	case "memory":
		runMemory(p, rest)
	case "workflow":
		runWorkflow(rest)
	case "projection":
		runProjection(rest)
	default:
		fmt.Fprintf(p.stderr, "error: unknown subcommand %q\n", args[0])
		usage(p.stderr)
		p.exit(1)
	}
}
