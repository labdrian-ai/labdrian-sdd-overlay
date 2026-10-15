package main

// The 'manifest routes' subcommand: what the overlay does with each row of overlay.manifest for one
// target. The overlay script asks it where it used to decide for itself with route_resolve and
// grep and awk (Phase 9, B1); the decision is the skills domain's (skills.ResolveRoutes), the
// manifest is read through the file reader the program hands the use case, and this file is the
// command line and the words.

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills/app"
)

// runManifest implements the 'manifest' subcommand over the files of the machine.
func runManifest(p process, args []string) {
	runManifestCore(app.RoutesPorts{Manifest: os.ReadFile}, args, p.stdout, p.stderr, p.exit)
}

// The flags of 'manifest routes'.
const (
	flagOverlayRoot = "--overlay-root"
	flagTarget      = "--target"
	flagSkillsRoot  = "--skills-root"
	flagAgentsRoot  = "--agents-root"
	flagManaged     = "--managed"
)

// manifestRoutesArgs is the command line of 'manifest routes', read.
type manifestRoutesArgs struct {
	overlayRoot string
	manifest    string
	target      string
	skillsRoot  string
	agentsRoot  string
	managedOnly bool
}

// parseManifestRoutesArgs reads the arguments after the verb. A flag nobody named, a flag that has
// no value, and a flag that is not given when it is needed are refused, saying which.
func parseManifestRoutesArgs(args []string) (manifestRoutesArgs, error) {
	var out manifestRoutesArgs
	values := map[string]*string{
		flagOverlayRoot: &out.overlayRoot,
		flagManifest:    &out.manifest,
		flagTarget:      &out.target,
		flagSkillsRoot:  &out.skillsRoot,
		flagAgentsRoot:  &out.agentsRoot,
	}
	for i := 0; i < len(args); i++ {
		switch value, isValue := values[args[i]]; {
		case args[i] == flagManaged:
			out.managedOnly = true
		case isValue:
			if i+1 >= len(args) {
				return manifestRoutesArgs{}, fmt.Errorf("manifest routes: flag %q requires a value", args[i])
			}
			i++
			*value = args[i]
		default:
			return manifestRoutesArgs{}, fmt.Errorf("manifest routes: unknown argument %q", args[i])
		}
	}
	if out.overlayRoot == "" || out.target == "" || out.skillsRoot == "" {
		return manifestRoutesArgs{}, fmt.Errorf("manifest routes requires %s, %s, and %s", flagOverlayRoot, flagTarget, flagSkillsRoot)
	}
	if out.manifest == "" {
		out.manifest = filepath.Join(out.overlayRoot, "overlay.manifest")
	}
	return out, nil
}

// runManifestCore is the testable core of the manifest subcommand: it names the verb, reads the
// command line, asks the use case and tells the rows, one a line, five columns separated by a
// tab (see the usage text). A manifest that is not there is a manifest with no row, said in a
// warning on stderr. Exit 0 on success; 1 on a command line it refuses, a manifest that is there and
// cannot be read, and a row it cannot place, with nothing on stdout.
func runManifestCore(ports app.RoutesPorts, args []string, stdout, stderr io.Writer, exit func(int)) {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "error: manifest requires a verb: routes")
		exit(1)
		return
	}
	if args[0] != "routes" {
		fmt.Fprintf(stderr, "error: unknown manifest verb %q; expected routes\n", args[0])
		exit(1)
		return
	}
	parsed, err := parseManifestRoutesArgs(args[1:])
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		exit(1)
		return
	}
	result, err := app.ManifestRoutes(ports, app.RoutesInput{
		ManifestPath: parsed.manifest,
		OverlayRoot:  parsed.overlayRoot,
		Destinations: skills.RouteDestinations{Target: parsed.target, SkillsRoot: parsed.skillsRoot, AgentsRoot: parsed.agentsRoot},
		ManagedOnly:  parsed.managedOnly,
	})
	if err != nil {
		refuseManifestRoutes(err, stderr)
		exit(1)
		return
	}
	if result.ManifestMissing {
		fmt.Fprintf(stderr, "warning: manifest routes: no manifest at %q: it has no row to route\n", parsed.manifest)
	}
	for _, route := range result.Routes {
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\t%s\n", route.Path, route.Route, route.RepoRel, route.RepoSource, route.Dest)
	}
	exit(0)
}

// refuseManifestRoutes says why the routes could not be told. A row that cannot be placed is said
// in the words the overlay script has always used for it, without the prefix of an error, which
// people and the pins of the script read.
func refuseManifestRoutes(err error, stderr io.Writer) {
	var unrouted *skills.UnroutedRowError
	if errors.As(err, &unrouted) {
		fmt.Fprintln(stderr, err)
		return
	}
	fmt.Fprintf(stderr, "error: manifest routes: %v\n", err)
}
