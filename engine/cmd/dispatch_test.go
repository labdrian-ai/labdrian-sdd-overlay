package main

import (
	"strings"
	"testing"
)

// run is how the command line reaches a subcommand. These tests pin what it says for a command
// line that names none, or one that is not known; the subcommands themselves are tested where
// they live and run as the program by the golden tests.

func TestRunWithNoArgumentsPrintsTheUsageAndExitsOne(t *testing.T) {
	p := newCapturedProcess("")

	run(p.process, testDeps(), nil)

	if len(p.exits) != 1 || p.exits[0] != 1 {
		t.Errorf("exit calls = %v, want exactly [1]", p.exits)
	}
	if !strings.HasPrefix(p.err.String(), "Usage:\n  engine propagate ") {
		t.Errorf("stderr begins %q, want the usage", firstLines(p.err.String(), 2))
	}
	if p.out.Len() != 0 {
		t.Errorf("stdout = %q, want nothing", p.out.String())
	}
}

func TestRunWithAnUnknownSubcommandSaysSoThenPrintsTheUsage(t *testing.T) {
	p := newCapturedProcess("")

	run(p.process, testDeps(), []string{"frobnicate", "--x"})

	if len(p.exits) != 1 || p.exits[0] != 1 {
		t.Errorf("exit calls = %v, want exactly [1]", p.exits)
	}
	if !strings.HasPrefix(p.err.String(), "error: unknown subcommand \"frobnicate\"\nUsage:\n") {
		t.Errorf("stderr begins %q, want the unknown subcommand, then the usage", firstLines(p.err.String(), 3))
	}
}

func TestRunHandsEachSubcommandItsOwnArguments(t *testing.T) {
	// 'roles' with no verb says what it needs, which only the roles command says; the arguments
	// after its name are the ones it is given.
	p := newCapturedProcess("")

	run(p.process, testDeps(), []string{"roles"})

	if want := "error: roles requires a verb: validate, next, resume, append, match-shaper\n"; p.err.String() != want {
		t.Errorf("stderr = %q, want %q", p.err.String(), want)
	}
	if len(p.exits) != 1 || p.exits[0] != 1 {
		t.Errorf("exit calls = %v, want exactly [1]", p.exits)
	}
}

func firstLines(s string, n int) string {
	lines := strings.SplitAfter(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "")
}
