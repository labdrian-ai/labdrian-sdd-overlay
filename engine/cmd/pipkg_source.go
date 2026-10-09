package main

import (
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/execrunner"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/pipkg"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/pipkg/gitsource"
)

// pipkgGitOptions are the two choices the program makes for the git the package builder asks
// about its overlay, both the ones that held before the builder had a port for git: git runs
// under the environment of the process, handed down whole, and under no deadline. They are
// decisions of the composition root and live here, in one place, so that changing either is a
// change to this function (a cleaner environment, a time limit) and to nothing the builder does.
// environ is the process environment as the entry point read it.
func pipkgGitOptions(environ []string) gitsource.Options {
	return gitsource.Options{Env: environ}
}

// newPipkgSource is the git of the machine as the SourceRepo of the package builder: the
// process adapter starts it, the adapter of the port asks it, under pipkgGitOptions.
func newPipkgSource(environ []string) pipkg.SourceRepo {
	return gitsource.New(execrunner.New(), pipkgGitOptions(environ))
}
