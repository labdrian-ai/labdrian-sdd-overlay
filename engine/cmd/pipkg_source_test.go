package main

import (
	"errors"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"
	"time"
)

// decidedRepositoryLocatingVariables is the owner's decision of 2026-10-09 (batch 20, decision 1)
// written out a second time, by hand, on purpose: the production map repositoryLocatingVariables
// must hold exactly these names, so changing that map without changing this list fails a test and
// makes the change a visible decision.
var decidedRepositoryLocatingVariables = []string{
	"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY",
	"GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_COMMON_DIR", "GIT_NAMESPACE", "GIT_PREFIX",
}

func TestTheScrubbedListIsTheOneTheOwnerDecided(t *testing.T) {
	if len(repositoryLocatingVariables) != len(decidedRepositoryLocatingVariables) {
		t.Errorf("the scrubbed list has %d names, the decision names %d", len(repositoryLocatingVariables), len(decidedRepositoryLocatingVariables))
	}
	for _, name := range decidedRepositoryLocatingVariables {
		if !repositoryLocatingVariables[name] {
			t.Errorf("%s is in the decision but not scrubbed", name)
		}
	}
}

func TestGitDoesNotInheritTheVariablesThatLocateARepository(t *testing.T) {
	var environ []string
	for _, name := range decidedRepositoryLocatingVariables {
		environ = append(environ, name+"=/somewhere/else")
	}
	environ = append(environ, "GIT_DIRECTORY_LOOKALIKE=kept")
	got := pipkgGitOptions(environ, pipkgGitTimeout).Env
	for _, entry := range got {
		name, _, _ := strings.Cut(entry, "=")
		for _, banned := range decidedRepositoryLocatingVariables {
			if name == banned {
				t.Errorf("git inherits %s", entry)
			}
		}
	}
	if len(got) != 1 || got[0] != "GIT_DIRECTORY_LOOKALIKE=kept" {
		t.Errorf("git runs under %v, want only the variable whose name merely starts like a banned one", got)
	}
}

func TestGitKeepsTheRestOfTheProcessEnvironment(t *testing.T) {
	environ := []string{"PATH=/usr/bin", "HOME=/home/someone", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_DIR=/x/.git", "LANG=C", "GIT_SSH_COMMAND=ssh -v"}
	want := "PATH=/usr/bin,HOME=/home/someone,GIT_CONFIG_GLOBAL=/dev/null,LANG=C,GIT_SSH_COMMAND=ssh -v"
	if got := strings.Join(pipkgGitOptions(environ, pipkgGitTimeout).Env, ","); got != want {
		t.Errorf("git runs under %q, want %q: everything but the repository-locating variables, in order", got, want)
	}
	// An empty environment stays an empty one: a nil Env would mean "the environment of the
	// process", which is the thing being filtered.
	if got := pipkgGitOptions(nil, pipkgGitTimeout).Env; got == nil || len(got) != 0 {
		t.Errorf("an empty environment became %#v, want an empty non-nil slice", got)
	}
}

// The variable is matched by name, not by prefix, and a value with an equals sign survives.
func TestGitEnvironmentIsFilteredByName(t *testing.T) {
	environ := []string{"GIT_DIR", "GIT_DIRX=1", "XGIT_DIR=1", "GIT_DIR=a=b"}
	if got := strings.Join(pipkgGitOptions(environ, pipkgGitTimeout).Env, ","); got != "GIT_DIRX=1,XGIT_DIR=1" {
		t.Errorf("git runs under %q, want only the names that are not the banned ones (a bare name counts as the variable)", got)
	}
}

// Git runs under a deadline of two minutes per call (owner decision 2 of batch 20, 2026-10-09):
// every call is local and takes milliseconds, the heaviest (status and archive) seconds, so a git
// that runs past it is stuck, and it is reported as a git that could not answer.
func TestGitRunsUnderTheDecidedDeadline(t *testing.T) {
	if pipkgGitTimeout != 2*time.Minute {
		t.Errorf("git runs under a deadline of %v, want 2m0s", pipkgGitTimeout)
	}
	if got := pipkgGitOptions([]string{"PATH=/usr/bin"}, pipkgGitTimeout).Timeout; got != pipkgGitTimeout {
		t.Errorf("the options carry a deadline of %v, want the one handed in (%v)", got, pipkgGitTimeout)
	}
}

func TestTheGitOfThePackageBuilderIsBoundedByPipkgGitMaxOutput(t *testing.T) {
	got := pipkgGitRunner().MaxOutput()
	if got <= 0 {
		t.Fatalf("pipkgGitRunner().MaxOutput() = %d: zero or less is no bound, and this test exists to prove git is bounded", got)
	}
	if got != pipkgGitMaxOutput {
		t.Errorf("pipkgGitRunner().MaxOutput() = %d, want pipkgGitMaxOutput (%d)", got, pipkgGitMaxOutput)
	}
}

// The deadline the program hands the package builder reaches the process that runs git: a git
// that hangs is stopped at it and reported as a git that could not answer, not waited for and not
// read as a "no". The git here is a script on a PATH of the test's own that sleeps for thirty
// seconds, and the deadline is a short one handed to newPipkgSourceWithin, the constructor
// newPipkgSource calls with pipkgGitTimeout.
func TestAHungGitIsStoppedByTheDeadlineThePackageBuilderHandsIt(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("the fake git is a shell script")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte("#!/bin/sh\nexec /bin/sleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	source := newPipkgSourceWithin([]string{"PATH=" + dir}, 200*time.Millisecond)

	started := time.Now()
	answer, err := source.IsWorkTree(t.TempDir())
	elapsed := time.Since(started)

	var unavailable interface{ Unavailable() bool }
	if !errors.As(err, &unavailable) || !unavailable.Unavailable() {
		t.Fatalf("IsWorkTree = %v, %v, want an error that says git could not answer", answer, err)
	}
	if answer {
		t.Error("a git that never answered was read as a yes")
	}
	if elapsed > 10*time.Second {
		t.Errorf("the call took %v: the deadline of 200ms did not stop git", elapsed)
	}
}
