package main

import (
	"strings"
	"testing"
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
	got := pipkgGitOptions(environ).Env
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
	if got := strings.Join(pipkgGitOptions(environ).Env, ","); got != want {
		t.Errorf("git runs under %q, want %q: everything but the repository-locating variables, in order", got, want)
	}
	// An empty environment stays an empty one: a nil Env would mean "the environment of the
	// process", which is the thing being filtered.
	if got := pipkgGitOptions(nil).Env; got == nil || len(got) != 0 {
		t.Errorf("an empty environment became %#v, want an empty non-nil slice", got)
	}
}

// The variable is matched by name, not by prefix, and a value with an equals sign survives.
func TestGitEnvironmentIsFilteredByName(t *testing.T) {
	environ := []string{"GIT_DIR", "GIT_DIRX=1", "XGIT_DIR=1", "GIT_DIR=a=b"}
	if got := strings.Join(pipkgGitOptions(environ).Env, ","); got != "GIT_DIRX=1,XGIT_DIR=1" {
		t.Errorf("git runs under %q, want only the names that are not the banned ones (a bare name counts as the variable)", got)
	}
}

// Git runs under no deadline, as before the builder had a port for it (decision 2 of batch 20 is
// held by the owner). Changing it is a behavior change: a test that fails here is a reminder of
// that, not of a bug.
func TestGitRunsUnderNoDeadline(t *testing.T) {
	if got := pipkgGitOptions([]string{"PATH=/usr/bin"}).Timeout; got != 0 {
		t.Errorf("git runs under a deadline of %v, want none", got)
	}
}

func TestTheGitOfThePackageBuilderIsBoundedByPipkgGitMaxOutput(t *testing.T) {
	if got := pipkgGitRunner().MaxOutput(); got != pipkgGitMaxOutput {
		t.Errorf("pipkgGitRunner().MaxOutput() = %d, want pipkgGitMaxOutput (%d)", got, pipkgGitMaxOutput)
	}
}
