package main

// The 'pipkg build|check' subcommand.

import (
	"fmt"
	"io"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/pipkg"
	runtimepkg "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
)

// runPipkg implements the 'pipkg build|check' subcommand.
func runPipkg(p process, d deps, args []string) {
	runPipkgCore(d, newPipkgSource(d.environment()), args, p.stdout, p.stderr, p.exit)
}

// runPipkgCore is the testable core of the pipkg subcommand: 'build' writes
// the labdrian-pi package tree and retires the overlay-owned link, in
// ~/.pi/agent/agents, of every agent the tree no longer ships; 'check' reports
// drift against it and changes nothing. Requires
// --overlay-root, --registry, and --dest-dir. Fails LOUD on a missing verb
// or missing flag (ADR-4). source is how the builder asks git about the overlay: the git of the
// machine in the program, a fake or pipkg.NoRepository in a test.
func runPipkgCore(d deps, source pipkg.SourceRepo, args []string, stdout, stderr io.Writer, exit func(int)) {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "error: pipkg requires a verb: build, check")
		exit(1)
		return
	}
	verb := args[0]
	if verb != "build" && verb != "check" {
		fmt.Fprintf(stderr, "error: unknown pipkg verb %q; expected build or check\n", verb)
		exit(1)
		return
	}

	var overlayRoot, registryPath, destDir string
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--overlay-root":
			i++
			if i >= len(args) {
				fmt.Fprintln(stderr, "error: --overlay-root requires a value")
				exit(1)
				return
			}
			overlayRoot = args[i]
		case "--registry":
			i++
			if i >= len(args) {
				fmt.Fprintln(stderr, "error: --registry requires a value")
				exit(1)
				return
			}
			registryPath = args[i]
		case "--dest-dir":
			i++
			if i >= len(args) {
				fmt.Fprintln(stderr, "error: --dest-dir requires a value")
				exit(1)
				return
			}
			destDir = args[i]
		default:
			fmt.Fprintf(stderr, "error: unknown option %q\n", args[i])
			exit(1)
			return
		}
	}
	if overlayRoot == "" || registryPath == "" || destDir == "" {
		fmt.Fprintln(stderr, "error: pipkg requires --overlay-root, --registry, and --dest-dir")
		exit(1)
		return
	}

	packages := pipkg.Packages{
		Registries: newWarningRegistryRepository(stderr),
		Source:     source,
		Options:    pipkgOptionsFromEnv(d.env),
	}
	if verb == "build" {
		if err := packages.Build(overlayRoot, registryPath, destDir); err != nil {
			fmt.Fprintf(stderr, "pipkg build: %v\n", err)
			exit(1)
			return
		}
		fmt.Fprintf(stdout, "pipkg build: labdrian-pi package written to %s\n", destDir)
		// The package is swapped in whole, so an agent the overlay dropped is gone from it and the link
		// an earlier deploy made for it points at nothing. The deploy runs this build first and `pi
		// install` only when a `pi` is on the PATH, so the build is the one step every run goes through:
		// it retires such a link, in the home the Pi runtime works in, and tells it.
		for _, note := range runtimepkg.RetireDroppedPiAgentLinks(runtimeConfigFromEnv(d.env, d.homeDir).Home, destDir) {
			fmt.Fprintf(stdout, "pipkg build: %s\n", note)
		}
		exit(0)
		return
	}

	disclosure, err := packages.Check(overlayRoot, registryPath, destDir)
	if disclosure != "" {
		fmt.Fprintf(stdout, "pipkg check: %s\n", disclosure)
	}
	if err != nil {
		fmt.Fprintf(stderr, "pipkg check: %v\n", err)
		exit(1)
		return
	}
	fmt.Fprintln(stdout, "pipkg check: OK (built package matches the current manifest)")
	exit(0)
}
