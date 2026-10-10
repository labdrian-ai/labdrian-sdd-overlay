package main

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
)

// This file is what the tests of the commands build their world from. A test never changes a
// variable of the program: it asks for the process and the deps of a run, and replaces the one
// field it wants otherwise.

// capturedProcess is a process that runs nowhere: its streams are buffers, its exit only records
// the codes, and its working directory is the one the test names.
type capturedProcess struct {
	process
	out, err bytes.Buffer
	exits    []int
}

// newCapturedProcess is a process with an empty stdin, buffers for its streams, and the working
// directory cwd (an empty cwd is a working directory that cannot be determined).
func newCapturedProcess(cwd string) *capturedProcess {
	c := &capturedProcess{}
	c.process = process{
		stdin:  strings.NewReader(""),
		stdout: &c.out,
		stderr: &c.err,
		exit:   func(code int) { c.exits = append(c.exits, code) },
		getwd: func() (string, error) {
			if cwd == "" {
				return "", errors.New("no working directory")
			}
			return cwd, nil
		},
	}
	return c
}

// testDeps is the deps of the program, as main builds them from the process the tests run in:
// its environment is the one TestMain pinned under a temporary directory.
func testDeps() deps { return productionDeps() }

// environmentOf is a getenv that answers from a map and is empty for anything else.
func environmentOf(vars map[string]string) func(string) string {
	return func(name string) string { return vars[name] }
}

func TestTestDepsIsTheDepsOfTheProgram(t *testing.T) {
	d := testDeps()
	if d.getenv == nil || d.environ == nil || d.userHomeDir == nil {
		t.Fatalf("deps %+v has a field main did not set", d)
	}
	t.Setenv("DEPS_TEST_VARIABLE", "set")
	if got := d.getenv("DEPS_TEST_VARIABLE"); got != "set" {
		t.Errorf("getenv = %q, want the process's own environment, read when asked", got)
	}
	if home, err := d.userHomeDir(); err != nil || home != os.Getenv("HOME") {
		t.Errorf("userHomeDir = %q, %v; want HOME %q", home, err, os.Getenv("HOME"))
	}
}
