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
		runGateTask(p, d, rest)
	case "merge-settings":
		runMergeSettings(p, rest)
	case "uninstall-hooks":
		runUninstallHooks(p, rest)
	case "merge-codex-hooks":
		runCodexHooks(p, rest, false)
	case "uninstall-codex-hooks":
		runCodexHooks(p, rest, true)
	case "status":
		runStatus(p, d, rest)
	case "prespec":
		runPrespec(p, rest)
	case "runtime":
		runRuntime(p, d, rest)
	case "gadu-generate":
		runGaduGenerate(p, d, rest)
	case "pipkg":
		runPipkg(p, d, rest)
	case "skills":
		runSkills(p, d, rest)
	case "manifest":
		runManifest(p, rest)
	case "registry":
		runRegistry(p, rest)
	case "installed-targets":
		runInstalledTargets(p, rest)
	case "sync-trigger":
		runSyncTrigger(p, d, rest)
	case "review-receipt":
		runReviewReceipt(p, d, rest)
	case "shaper":
		runShaper(p, d, rest)
	case "roles":
		runRoles(p, rest)
	case "memory":
		runMemory(p, d, rest)
	case "workflow":
		runWorkflow(p, d, rest)
	case "projection":
		runProjection(p, d, rest)
	default:
		fmt.Fprintf(p.stderr, "error: unknown subcommand %q\n", args[0])
		usage(p.stderr)
		p.exit(1)
	}
}
