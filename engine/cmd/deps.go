package main

import (
	"errors"
	"io"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capability/presence"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
)

// The commands do not reach for the machine: they are handed what they run on, in two values that
// main builds once from the process (main.go). Nothing else in this package reads the arguments,
// the streams, the exit, the environment or the working directory of the process
// (process_boundary_test.go keeps it so), and nothing in it is a variable a test changes: a test
// builds the value it needs, with the field it wants other than the program's, and hands it to the
// command.
//
// process is where a command writes and reads and how it ends: the four things a verb needs from
// the process around it, which its core takes as plain parameters (stdin, stdout, stderr, exit)
// so that a test can capture them.
//
// deps is what the verbs are built over: the environment of the process (its variables, its home
// and its working directory), the facts main resolves from it once, and the choices that make a verb
// the program's (which store, which policy, which prober, which wait). A field of deps is the
// program's own in productionDeps; the field a test wants otherwise is the only one it replaces.
//
// What deps and process do not cover is not the process: reading a file the command was named, or
// the file-backed adapters a use case is built over, is the command's own work and is tested with
// files in a temporary directory, where the guard (process_boundary_test.go) looks only at the
// process.

// process is the process a command runs in, as main hands it over.
type process struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	// exit ends the process with a code. A command calls it and returns right after, because a
	// test hands in an exit that does not end anything.
	exit func(int)
}

// deps is what the commands are built over (see the top of this file).
type deps struct {
	// getwd is the working directory of the process. A directory that cannot be determined is
	// the command's to treat as none.
	getwd func() (string, error)
	// getenv, environ and userHomeDir are the environment of the process. They are functions and
	// not values because a command reads the variable it needs when it needs it, as it always
	// did; main binds them to the process once.
	getenv      func(string) string
	environ     func() []string
	userHomeDir func() (string, error)
	// agentChild says that the process runs inside a gentle-pi agent child, where no human
	// answers a dialog. main resolves it once from the environment (agentChildVariable), so
	// that no command reads that variable deep inside itself.
	agentChild bool

	// probeFS is the stat access of 'runtime probe' (and of the dependency prober the workflow
	// verbs record observations with); nil means the operating system's. probeTimeout bounds one
	// probe run, so that a hung filesystem cannot hang the command.
	probeFS      presence.StatFS
	probeTimeout time.Duration

	// openBindings opens the session binding store of a run: the file-backed one over the state
	// home in the program. A test that wants to run code at a moment of the store decorates it.
	openBindings func() (projection.BindingStore, error)
	// gate decides whether a tool call is allowed against the workflow a repository is bound to
	// (projection.Gate in the program). A test that wants the decision to fail replaces it.
	gate func(projection.GateInput) projection.GateResult
	// workflowProber returns the DependencyProber the workflow verbs record observations with.
	// Nil, as in the program, is the presence prober over the home and PATH of these deps and
	// their probeFS (dependencyProber): it looks at paths with stat and at PATH entries by name,
	// and never opens a file, runs a program, or names a path (see engine/capability/presence),
	// so an "available" observation only says something is present and states the limit. The
	// lifecycle bounds every probe with its own deadline (workflow's dependencyProbeTimeout), so
	// a hung filesystem cannot hang a verb whichever prober is used. A test installs
	// workflow.UnavailableProber, the safe default that confirms nothing.
	workflowProber func() workflow.DependencyProber

	// skillsLockWait is how long a skills verb waits for a taken lock before it gives up with
	// exit 2. Zero means filelock.DefaultWait (2 s), the bound the workflow binding store uses.
	skillsLockWait time.Duration
}

// A field of deps that is nil is never called. For the environment it means there is none: no
// variable is set, there is no home or working directory to find, and the process has no
// variables to hand on (the methods below say so). For the choices that make a verb the
// program's it means the program's own: the file-backed binding store, the domain's gate, the
// presence prober over this deps' environment, and the default timeout of a probe. So a deps
// built with fewer fields than productionDeps sets is still a deps that works.

// env is the value of the variable name, empty when it is not set or the deps have no environment.
func (d deps) env(name string) string {
	if d.getenv == nil {
		return ""
	}
	return d.getenv(name)
}

// environment is the variables a child process is started with, none when the deps have none.
func (d deps) environment() []string {
	if d.environ == nil {
		return nil
	}
	return d.environ()
}

// homeDir is the home directory of the user, or an error when the deps cannot find one.
func (d deps) homeDir() (string, error) {
	if d.userHomeDir == nil {
		return "", errNoEnvironment
	}
	return d.userHomeDir()
}

// workingDir is the working directory of the process, or an error when the deps cannot say.
func (d deps) workingDir() (string, error) {
	if d.getwd == nil {
		return "", errNoEnvironment
	}
	return d.getwd()
}

// errNoEnvironment is why a deps without a function of the environment cannot answer a question
// about it.
var errNoEnvironment = errors.New("no environment was given")

// dependencyProber is the prober the workflow verbs record observations with: the deps' own,
// or the presence prober over the home and PATH of the deps and their stat access.
func (d deps) dependencyProber() workflow.DependencyProber {
	if d.workflowProber != nil {
		return d.workflowProber()
	}
	home, path := runtimeProbeEnv(d)
	return presence.Prober{Home: home, Path: path, ProbeFS: d.probeFS}
}

// probeBound is how long one probe run may take: the deps' own timeout when it is positive, and
// otherwise the default, so that a deps with no timeout is bound by the default and not by a
// deadline that has already passed.
func (d deps) probeBound() time.Duration {
	if d.probeTimeout <= 0 {
		return defaultProbeTimeout
	}
	return d.probeTimeout
}

// openBinding opens the binding store of a run: the deps' own, or the file-backed one.
func (d deps) openBinding() (projection.BindingStore, error) {
	if d.openBindings != nil {
		return d.openBindings()
	}
	return newBindingStore()
}

// agentChildVariable is the environment variable a gentle-pi agent child runs with, set to
// agentChildValue.
const (
	agentChildVariable = "GENTLE_PI_AGENTS_CHILD"
	agentChildValue    = "1"
)
