package main

import (
	"strings"
	"testing"
)

// The program hands git the environment of the process and no deadline, as git ran before the
// package builder had a port for it. Changing either is a behavior change the owner decides
// (odd/tasks/consolidation-phase9.md, batch 20): a test that fails here is a reminder of that,
// not of a bug.
func TestGitRunsUnderTheProcessEnvironmentAndNoDeadline(t *testing.T) {
	environ := []string{"PATH=/usr/bin", "GIT_DIR=/somewhere/.git", "HOME=/home/someone"}
	options := pipkgGitOptions(environ)
	if strings.Join(options.Env, ",") != strings.Join(environ, ",") {
		t.Errorf("git runs under %v, want the environment of the process, whole", options.Env)
	}
	if options.Timeout != 0 {
		t.Errorf("git runs under a deadline of %v, want none", options.Timeout)
	}
}
