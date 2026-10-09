package main

import (
	"fmt"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/pipkg"
	runtimepkg "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
	runtimecore "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// newRuntimeRegistry is the one place the runtimes the program ships are named. The order is the
// order `--target all` acts on them in. Pi reaches its CLI through commands and builds its package
// from the skills registry that registries reads, asking git about the overlay through source,
// under the pipkg options of the run.
func newRuntimeRegistry(registries skills.RegistryRepository, commands runtimepkg.CommandRunner, source pipkg.SourceRepo, packages pipkg.Options) (*runtimecore.Registry, error) {
	r := runtimecore.NewRegistry()
	for _, register := range []func(*runtimecore.Registry) error{
		runtimepkg.RegisterClaude,
		runtimepkg.RegisterOpenCode,
		runtimepkg.RegisterCodex,
		func(r *runtimecore.Registry) error {
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
func buildRuntimeAdapters(registry *runtimecore.Registry, targets []runtimecore.Target, cfg runtimecore.Config) ([]runtimecore.Adapter, error) {
	adapters := make([]runtimecore.Adapter, 0, len(targets))
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
func runtimeConfigFromEnv(getenv func(string) string, userHomeDir func() (string, error)) runtimecore.Config {
	home := strings.TrimSpace(getenv("HOME"))
	if home == "" {
		if dir, err := userHomeDir(); err == nil {
			home = dir
		}
	}
	return runtimecore.Config{
		Home:          home,
		XDGConfigHome: getenv("XDG_CONFIG_HOME"),
		CodexHome:     getenv("CODEX_HOME"),
		OverlayDir:    getenv("OVERLAY_DIR"),
		StateDir:      getenv("STATE_DIR"),
		// The checkout the OpenCode plugin's contracts are read from; the adapter judges it.
		LabdrianOverlayDir: getenv(runtimecore.LabdrianOverlayDirVariable),
		// Only the exact value 1 turns the probe off, as it always has.
		PiSkipSubagents: getenv(piSkipSubagentsVariable) == "1",
	}
}
