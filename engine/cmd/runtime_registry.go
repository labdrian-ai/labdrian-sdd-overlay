package main

import (
	"fmt"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/pipkg"
	runtimepkg "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// newRuntimeRegistry is the one place the runtimes the program ships are named. The order is the
// order `--target all` acts on them in. Pi reaches its CLI through commands and builds its package
// from the skills registry that registries reads, asking git about the overlay through source,
// under the pipkg options of the run.
func newRuntimeRegistry(registries skills.RegistryRepository, commands runtimepkg.CommandRunner, source pipkg.SourceRepo, packages pipkg.Options) (*runtimepkg.Registry, error) {
	r := runtimepkg.NewRegistry()
	for _, register := range []func(*runtimepkg.Registry) error{
		runtimepkg.RegisterClaude,
		runtimepkg.RegisterOpenCode,
		runtimepkg.RegisterCodex,
		func(r *runtimepkg.Registry) error {
			return runtimepkg.RegisterPi(r, runtimepkg.PiPorts{
				Commands: commands,
				Packages: pipkg.Packages{Registries: registries, Source: source, Options: packages},
			})
		},
	} {
		if err := register(r); err != nil {
			return nil, fmt.Errorf("registering the runtimes: %w", err)
		}
	}
	return r, nil
}

// buildRuntimeAdapters builds the adapter of every target, or none: the first target the registry
// cannot build is the error.
func buildRuntimeAdapters(registry *runtimepkg.Registry, targets []runtimepkg.Target, cfg runtimepkg.Config) ([]runtimepkg.Adapter, error) {
	adapters := make([]runtimepkg.Adapter, 0, len(targets))
	for _, target := range targets {
		adapter, err := registry.New(target, cfg)
		if err != nil {
			return nil, err
		}
		adapters = append(adapters, adapter)
	}
	return adapters, nil
}

// piSkipSubagentsVariable turns off the probe and the install of the Pi Subagents extension, for
// environments that manage it separately (README, "GADU as a real Pi subagent").
const piSkipSubagentsVariable = "LABDRIAN_PI_SKIP_SUBAGENTS"

// runtimeConfigFromEnv reads the environment the runtime adapters depend on, once, and hands it
// down as a value. The home is $HOME without surrounding space, else the one the system names
// for the user, else empty: an adapter never guesses where a home is. The caller sets ConfigRoot
// when the command line gave a --config-root.
func runtimeConfigFromEnv(getenv func(string) string, userHomeDir func() (string, error)) runtimepkg.Config {
	home := strings.TrimSpace(getenv("HOME"))
	if home == "" {
		if dir, err := userHomeDir(); err == nil {
			home = dir
		}
	}
	return runtimepkg.Config{
		Home:          home,
		XDGConfigHome: getenv("XDG_CONFIG_HOME"),
		CodexHome:     getenv("CODEX_HOME"),
		OverlayDir:    getenv("OVERLAY_DIR"),
		StateDir:      getenv("STATE_DIR"),
		// Only the exact value 1 turns the probe off, as it always has.
		PiSkipSubagents: getenv(piSkipSubagentsVariable) == "1",
	}
}
