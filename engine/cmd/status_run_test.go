package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// runStatus asks the deps where the installation is: the HOME of the environment they give and the
// working directory they give, never the process's own.
func TestRunStatusLooksWhereTheDepsSay(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	d := testDeps()
	d.getenv = environmentOf(map[string]string{"HOME": home})
	d.getwd = func() (string, error) { return project, nil }
	p := newCapturedProcess()

	runStatus(p.process, d)

	out := p.out.String()
	for _, want := range []string{
		filepath.Join(home, ".claude", "settings.json"),
		filepath.Join(project, ".atl", "skill-registry.md"),
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the report does not name %s:\n%s", want, out)
		}
	}
	if len(p.exits) != 1 || p.exits[0] != 1 {
		t.Errorf("exit calls = %v, want exactly [1] (an empty home fails its hard checks)", p.exits)
	}
}

// A working directory that cannot be determined is none: the registry is then not looked for.
func TestRunStatusWithoutAWorkingDirectoryDoesNotLookForTheRegistry(t *testing.T) {
	d := testDeps()
	d.getenv = environmentOf(map[string]string{"HOME": t.TempDir()})
	d.getwd = func() (string, error) { return "", errors.New("no working directory") }
	p := newCapturedProcess()

	runStatus(p.process, d)

	if strings.Contains(p.out.String(), "registry:") {
		t.Errorf("the report has a registry line though there is no working directory:\n%s", p.out.String())
	}
}
