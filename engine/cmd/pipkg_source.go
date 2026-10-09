package main

import (
	"strings"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/execrunner"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/pipkg"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/pipkg/gitsource"
)

// repositoryLocatingVariables are the variables that tell git which repository to act on. A hook,
// a wrapper or a script that runs in some repository sets them, and `git -C <overlay>` would then
// read that repository instead of the overlay's own and compare a package against the wrong
// tree. The owner decided (2026-10-09, batch 20) that the git the package builder runs does not
// inherit them and sees everything else of the process environment.
var repositoryLocatingVariables = map[string]bool{
	"GIT_DIR": true, "GIT_WORK_TREE": true, "GIT_INDEX_FILE": true, "GIT_OBJECT_DIRECTORY": true,
	"GIT_ALTERNATE_OBJECT_DIRECTORIES": true, "GIT_COMMON_DIR": true, "GIT_NAMESPACE": true,
	"GIT_PREFIX": true,
}

// pipkgGitOptions are the two choices the program makes for the git the package builder asks
// about its overlay. Git runs under the environment of the process without the variables that
// locate a repository (repositoryLocatingVariables), in the order the process had it, and under
// pipkgGitTimeout per call. Both live here, in one place, so that changing either is a change to
// this function and to nothing the builder does. environ is the process environment as the entry point read
// it; an empty one stays empty, never nil, because the adapter reads nil as "the environment of
// the process".
func pipkgGitOptions(environ []string) gitsource.Options {
	kept := make([]string, 0, len(environ))
	for _, entry := range environ {
		name, _, _ := strings.Cut(entry, "=")
		if !repositoryLocatingVariables[name] {
			kept = append(kept, entry)
		}
	}
	return gitsource.Options{Env: kept, Timeout: pipkgGitTimeout}
}

// pipkgGitTimeout stops one git call that runs longer (owner decision 2 of batch 20, 2026-10-09).
// Every call is local and takes milliseconds, the heaviest (status and archive of a large tree)
// seconds; a git past two minutes is stuck, and the builder reports it as a git that could not
// answer instead of hanging.
const pipkgGitTimeout = 2 * time.Minute

// pipkgGitMaxOutput bounds what the program holds of one stream of one git call. The only large
// answer is the archive of the sources, about 1 MiB for this overlay; 256 MiB leaves it room to
// grow a hundredfold and stops a runaway before it fills memory.
const pipkgGitMaxOutput = 256 << 20

// pipkgGitRunner is the process adapter git is started through, bounded by pipkgGitMaxOutput.
func pipkgGitRunner() execrunner.Runner {
	return execrunner.New().WithMaxOutput(pipkgGitMaxOutput)
}

// newPipkgSource is the git of the machine as the SourceRepo of the package builder: the
// process adapter starts it, the adapter of the port asks it, under pipkgGitOptions.
func newPipkgSource(environ []string) pipkg.SourceRepo {
	return gitsource.New(pipkgGitRunner(), pipkgGitOptions(environ))
}
