package main

import (
	"io"
)

// The commands do not reach for the machine: they are handed what they run on, in two values that
// main builds once from the process (main.go). Nothing else in this package reads the arguments,
// the streams, the exit, the environment or the working directory of the process
// (process_boundary_test.go keeps it so), and nothing in it is a variable a test changes: a test
// builds the value it needs, with the field it wants other than the program's, and hands it to the
// command.
//
// process is where a command writes and reads, how it ends and where it is: the five things a
// verb needs from the process around it, which its core takes as plain parameters (stdin, stdout,
// stderr, exit) so that a test can capture them.
//
// deps is what the verbs are built over: the environment, resolved once, and the choices that make
// a verb the program's (which store, which policy, which wait). A field of deps is the program's
// own in productionDeps; the field a test wants otherwise is the only one it replaces.

// process is the process a command runs in, as main hands it over.
type process struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	// exit ends the process with a code. A command calls it and returns right after, because a
	// test hands in an exit that does not end anything.
	exit func(int)
	// getwd is the working directory of the process. A directory that cannot be determined is
	// the command's to treat as none.
	getwd func() (string, error)
}

// deps is what the commands are built over (see the top of this file).
type deps struct {
	// getenv, environ and userHomeDir are the environment of the process. They are functions and
	// not values because a command reads the variable it needs when it needs it, as it always
	// did; main binds them to the process once.
	getenv      func(string) string
	environ     func() []string
	userHomeDir func() (string, error)
}
