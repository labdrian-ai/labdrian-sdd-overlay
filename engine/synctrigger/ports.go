package synctrigger

import (
	"io"
	"os"
	"time"
)

// Runner is the part of the machine the trigger starts programs with: the one it re-executes as
// the detached child, and the longterm-mem sync it waits for. The trigger decides what to start,
// with which arguments and what an ending means; a Runner starts it, and knows nothing of events,
// logs or outcomes. The adapter for the operating system is engine/synctrigger/osproc.
type Runner interface {
	// Executable is the path of the program that is running, which Run starts again as the child
	// when Options.Self is empty.
	Executable() (string, error)

	// Detach starts program with args and returns once it has started, without waiting for it. The
	// program outlives the caller: it runs in a session of its own, with no standard input, and
	// its standard output and standard error are the file log itself, not a copy the caller would
	// have to keep making (which is why log is a file and not a writer).
	Detach(program string, args []string, log *os.File) error

	// Run starts program with args in dir and waits for it, with what it prints on its standard
	// error copied to stderr. When it has not ended after timeout, it and everything it started
	// are killed and the Result says TimedOut. A program that ended is a Result, whatever its exit
	// code; an error means it could not be run to an end.
	Run(program string, args []string, dir string, timeout time.Duration, stderr io.Writer) (Result, error)
}

// Result is how a program run by Runner.Run ended.
type Result struct {
	// Exit is the exit code of a program that ended by itself; -1 for one a signal ended.
	Exit int
	// TimedOut says that the program was killed because it did not end in time. Exit means
	// nothing then.
	TimedOut bool
}
