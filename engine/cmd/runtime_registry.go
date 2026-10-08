package main

import (
	"fmt"
	"strings"

	runtimepkg "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// newRuntimeRegistry is the one place the runtimes the program ships are named. The order is the
// order `--target all` acts on them in. registries is how Pi reads the skills registry its
// package is built from.
func newRuntimeRegistry(registries skills.RegistryRepository) (*runtimepkg.Registry, error) {
	r := runtimepkg.NewRegistry()
	for _, register := range []func(*runtimepkg.Registry) error{
		runtimepkg.RegisterClaude,
		runtimepkg.RegisterOpenCode,
		runtimepkg.RegisterCodex,
		func(r *runtimepkg.Registry) error { return runtimepkg.RegisterPi(r, registries) },
	} {
		if err := register(r); err != nil {
			return nil, fmt.Errorf("registering the runtimes: %w", err)
		}
	}
	return r, nil
}

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
	}
}
