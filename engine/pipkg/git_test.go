package pipkg_test

import (
	"os"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/execrunner"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/pipkg"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/pipkg/gitsource"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// gitEnvironment is the whole environment of every git these tests start, and of the ones the
// fixtures run: the PATH to find the helpers of git, a committer, and nothing else. No variable
// of the developer's reaches it, so a GIT_DIR left by a hook cannot point a test at another
// repository, and the git configuration of the machine (the global file, the system one) is
// out of reach. The tests make their repositories in temporary directories and ask only those.
func gitEnvironment() []string {
	return []string{
		"PATH=" + os.Getenv("PATH"),
		"GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_CONFIG_SYSTEM=" + os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.com",
	}
}

// packagesOf is the package builder of a test: the registry reader it names, the real git of the
// machine under gitEnvironment (asked about temporary directories only), and the Options given.
func packagesOf(registries skills.RegistryRepository, options ...pipkg.Options) pipkg.Packages {
	packages := pipkg.Packages{
		Registries: registries,
		Source:     gitsource.New(execrunner.New(), gitsource.Options{Env: gitEnvironment()}),
	}
	if len(options) > 0 {
		packages.Options = options[0]
	}
	return packages
}
