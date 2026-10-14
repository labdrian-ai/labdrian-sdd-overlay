// Package runtimes registers longterm-mem's MCP server in the configuration of the coding runtimes of a machine, and
// takes it out again: the use cases behind `longterm-mem register` and `longterm-mem unregister`.
//
// It decides what a request means and what came of it, and touches nothing of the machine. Which runtimes a target
// names (one, or every runtime that keeps a configuration file of its own), what is refused before any of them is
// touched and in which order, where each is acted on when nobody said, what is skipped, and which of the outcomes
// of several runtimes is the verdict of the run: that is this package. Writing a runtime's configuration is the
// Registrar port, which internal/register answers; what the machine looks like (the home of each runtime, the state
// directory, the binary, whether pi is installed) is the Machine port, which the composition root answers. What is
// said of the outcome, in which words and with which exit code, is the command's.
package runtimes

import (
	"errors"
	"io/fs"
)

// The runtimes a target can name, and "all".
const (
	Claude   = "claude"
	Opencode = "opencode"
	Codex    = "codex"
	Pi       = "pi"
	// All names every runtime that keeps a configuration file of its own: not pi, which is a package that is built
	// and installed, not a file that is there.
	All = "all"
)

// Expand is the runtimes a target names, in the order they are attempted, and whether it names any: one runtime by
// its name, or All for every runtime with a configuration file of its own.
func Expand(target string) ([]string, bool) {
	switch target {
	case Claude, Opencode, Codex, Pi:
		return []string{target}, true
	case All:
		return []string{Claude, Opencode, Codex}, true
	default:
		return nil, false
	}
}

// ErrConflict is the refusal to write over an entry the runtime's configuration already has under longterm-mem's name
// and that longterm-mem cannot prove it wrote: it is not ours, whatever it is named, and writing over it would
// destroy someone else's configuration (R-016/R-017 "Untagged same-named entry is refused, not overwritten"). A
// Registrar wraps it, so that it is found with errors.Is.
//
// The text carries the way out, and deliberately does not repeat the "register:" prefix its wrapper already adds.
// This message is the only instruction a user staring at exit 6 gets, and it used to be both tripled
// ("register: claude: register: claude: register: ...", one prefix per layer) and a dead end: it named the situation
// and no action. Its wrapper names the config file, so between them the message says which file, what is wrong,
// and what to do about it.
var ErrConflict = errors.New("an entry with this name already exists and is not owned by longterm-mem; to hand it to longterm-mem, remove that entry by hand and run register again")

// Removal is what unregistering did to one runtime's configuration (R-019).
type Removal int

const (
	// Noop: neither the runtime's entry nor the record of it existed, so there was nothing to do.
	Noop Removal = iota
	// Removed: the entry longterm-mem owned (when it was there) was removed from the runtime's configuration, and
	// its record was cleared.
	Removed
	// Unmanaged: an entry of the name is in the runtime's configuration and longterm-mem has no record of having
	// written it, so it was left in place and reported, never removed.
	Unmanaged
)

// String is the word the command prints for the outcome of one runtime.
func (r Removal) String() string {
	switch r {
	case Noop:
		return "noop"
	case Removed:
		return "removed"
	case Unmanaged:
		return "unmanaged"
	default:
		return "unknown"
	}
}

// Registrar is what the use cases ask of the program's way of writing a runtime's configuration: the writer for
// target puts the entry in, or takes it out, of the configuration it keeps at configRoot, and records what it did
// under stateDir. internal/register answers it.
type Registrar interface {
	// Register writes the entry, which names binary. The error wraps ErrConflict when the runtime's configuration
	// holds an entry of the name that is not ours, and fs.ErrNotExist when there is no configuration to write into.
	Register(target, configRoot, stateDir, binary string) error
	// Unregister takes the entry out, and says what became of it.
	Unregister(target, configRoot, stateDir string) (Removal, error)
}

// Machine is what the use cases ask of the machine they run on. An empty answer is "cannot be told": there is no
// guessing a path from a working directory.
type Machine interface {
	// ConfigRoot is where the runtime keeps its configuration (for pi, the package directory it is built into).
	ConfigRoot(target string) string
	// StateDir is the directory install-state.json is kept in.
	StateDir() string
	// Binary is the path of the longterm-mem binary the entry names.
	Binary() string
	// PiInstalled says whether pi looks installed, and not merely built: the package is there and pi's settings list
	// it.
	PiInstalled(packageDir string) bool
}

// Runtimes are the use cases over a Registrar and a Machine.
type Runtimes struct {
	Registrar Registrar
	Machine   Machine
}

// Request is what a run of register or unregister was asked to do. An empty field was not given.
type Request struct {
	// Target names a runtime, or All.
	Target string
	// ConfigRoot is where to act, for a single runtime; else each runtime's default.
	ConfigRoot string
	// StateDir is where the record of what was installed is kept; else the machine's.
	StateDir string
	// Binary is the path the entry names (register only); else the machine's.
	Binary string
}

// RefusalKind is why a request was refused before any runtime was touched.
type RefusalKind int

const (
	// TargetRequired: no target was named.
	TargetRequired RefusalKind = iota + 1
	// UnknownTarget: the target names no runtime (Refusal.Target).
	UnknownTarget
	// ConfigRootNeedsOneTarget: a config root was given for All, which is more than one place.
	ConfigRootNeedsOneTarget
	// StateDirUnresolved: no state directory was given and the machine cannot tell one.
	StateDirUnresolved
	// BinaryUnresolved: no binary was given and the machine cannot tell one (register only).
	BinaryUnresolved
	// PiNotInstalled: pi was named, and the package at Refusal.Root is not installed (register only).
	PiNotInstalled
)

// Refusal is a request refused before any runtime was touched.
type Refusal struct {
	Kind RefusalKind
	// Target is the name that named no runtime, for UnknownTarget.
	Target string
	// Root is the package directory pi was looked for in, for PiNotInstalled.
	Root string
}

// Outcome is what came of one runtime.
type Outcome int

const (
	// Done: the entry was written, or taken out (Result.Removal says which).
	Done Outcome = iota + 1
	// Skipped: the runtime has no configuration here, in a run that asked for every runtime and so for those that
	// exist. Register only.
	Skipped
	// NotOurs: the runtime's configuration holds an entry of the name that longterm-mem did not write, and it was left
	// as it was (a conflict, for register; unmanaged, for unregister).
	NotOurs
	// Failed: the runtime was attempted and could not be done.
	Failed
	// RootUnresolved: no config root was given and the machine cannot tell one, so the runtime was not attempted.
	RootUnresolved
)

// Result is what came of one runtime.
type Result struct {
	Target string
	// Root is where the runtime was acted on; empty when it could not be told.
	Root    string
	Outcome Outcome
	// Removal is what unregistering did, for a Done outcome of unregister.
	Removal Removal
	// Err is the Registrar's, for NotOurs and Failed.
	Err error
}

// Report is what came of a run: the refusal of the request, or the outcome of each runtime in the order they were
// attempted.
type Report struct {
	// Refusal is set when the request was refused, and then nothing was attempted.
	Refusal *Refusal
	Results []Result
}

// Failure is the verdict of a run, one among the outcomes of its runtimes. The harder outranks the softer: with
// --target all any two can happen in the same run, and reporting the softer would hide the harder behind an exit
// code that reads as "nothing much broke".
type Failure int

const (
	// NoFailure: every runtime was done or skipped.
	NoFailure Failure = iota
	// SomeEntryNotOurs: an expected, recoverable outcome the caller resolves by hand.
	SomeEntryNotOurs
	// SomePathUnresolved: a runtime was never attempted, and the remedy is the caller's environment.
	SomePathUnresolved
	// SomeTargetFailed: a runtime was attempted and could not be done.
	SomeTargetFailed
)

// Failure is the verdict of the run.
func (r Report) Failure() Failure {
	verdict := NoFailure
	for _, result := range r.Results {
		var found Failure
		switch result.Outcome {
		case NotOurs:
			found = SomeEntryNotOurs
		case RootUnresolved:
			found = SomePathUnresolved
		case Failed:
			found = SomeTargetFailed
		}
		if found > verdict {
			verdict = found
		}
	}
	return verdict
}

// Register writes the entry into the runtimes the request names. The request is refused, in this order, when no
// target is named, when it names none that exists, when a config root is given for more than one runtime, when the
// state directory or the binary cannot be told, and when pi is named and not installed; otherwise every runtime is
// attempted whatever happened to the ones before it, so one runtime's failure never hides another's success or
// failure.
//
// Pi is not part of All for the reason it is not a file that is there: All adds it, last, only when it looks
// installed, and silently otherwise, the same "this machine does not have it" story the other runtimes get from a
// missing configuration. Pi named explicitly gets the opposite: it fails loudly when the probe says no, rather
// than let the writer discover the missing package on its own with a less specific error.
func (r Runtimes) Register(req Request) Report {
	targets, refusal := r.targetsOf(req)
	if refusal != nil {
		return Report{Refusal: refusal}
	}
	stateDir, binary := orElse(req.StateDir, r.Machine.StateDir()), orElse(req.Binary, r.Machine.Binary())
	// An unresolvable default path must never degrade into a silent fallback. An empty binary path installs a
	// structurally valid entry that can never start a server, and an empty state dir writes install-state.json into
	// whatever the process working directory happens to be, losing the ownership record the next run needs to tell
	// its own write from a hand edit. Both are refused before any target is touched, rather than written and
	// reported as ok.
	switch {
	case stateDir == "":
		return Report{Refusal: &Refusal{Kind: StateDirUnresolved}}
	case binary == "":
		return Report{Refusal: &Refusal{Kind: BinaryUnresolved}}
	}

	targets, refusal = r.withPi(req, targets)
	if refusal != nil {
		return Report{Refusal: refusal}
	}

	// expandedAll distinguishes "register everything this machine has" from "register this one runtime, which I am
	// telling you is there". Only the first may skip a runtime whose config is absent.
	expandedAll := len(targets) > 1
	return Report{Results: r.each(req, targets, func(target, root string) Result {
		err := r.Registrar.Register(target, root, stateDir, binary)
		switch {
		case err == nil:
			return Result{Target: target, Root: root, Outcome: Done}
		case errors.Is(err, ErrConflict):
			return Result{Target: target, Root: root, Outcome: NotOurs, Err: err}
		case expandedAll && errors.Is(err, fs.ErrNotExist):
			// --target all means "every runtime this machine has", not "every runtime that exists" -- few machines
			// have all of them. A missing config is how a runtime that is not installed looks from here, and
			// failing the whole run over it would report a failure for a run in which the runtimes that ARE
			// installed were registered correctly. Asking for one runtime by name is a different statement, and
			// still fails.
			return Result{Target: target, Root: root, Outcome: Skipped}
		default:
			return Result{Target: target, Root: root, Outcome: Failed, Err: err}
		}
	})}
}

// Unregister takes the entry out of the runtimes the request names. It asks fewer questions than Register: it
// writes no entry, so it needs no binary, and a runtime whose configuration is not there is a no-op for the
// Registrar to say, not something to skip, so All has nothing to skip around and pi is not part of it.
func (r Runtimes) Unregister(req Request) Report {
	targets, refusal := r.targetsOf(req)
	if refusal != nil {
		return Report{Refusal: refusal}
	}
	stateDir := orElse(req.StateDir, r.Machine.StateDir())
	if stateDir == "" {
		return Report{Refusal: &Refusal{Kind: StateDirUnresolved}}
	}

	return Report{Results: r.each(req, targets, func(target, root string) Result {
		removal, err := r.Registrar.Unregister(target, root, stateDir)
		switch {
		case err != nil:
			return Result{Target: target, Root: root, Outcome: Failed, Err: err}
		case removal == Unmanaged:
			return Result{Target: target, Root: root, Outcome: NotOurs, Removal: removal}
		default:
			return Result{Target: target, Root: root, Outcome: Done, Removal: removal}
		}
	})}
}

// targetsOf is the runtimes a request names, or the refusal of a request that names none, or names them in a way
// that cannot be done.
func (r Runtimes) targetsOf(req Request) ([]string, *Refusal) {
	if req.Target == "" {
		return nil, &Refusal{Kind: TargetRequired}
	}
	targets, known := Expand(req.Target)
	if !known {
		return nil, &Refusal{Kind: UnknownTarget, Target: req.Target}
	}
	if req.ConfigRoot != "" && len(targets) > 1 {
		return nil, &Refusal{Kind: ConfigRootNeedsOneTarget}
	}
	return targets, nil
}

// withPi adds pi to the runtimes of All when it looks installed, and refuses a request that names pi where it does
// not.
func (r Runtimes) withPi(req Request, targets []string) ([]string, *Refusal) {
	if req.Target != All && req.Target != Pi {
		return targets, nil
	}
	root := orElse(req.ConfigRoot, r.Machine.ConfigRoot(Pi))
	installed := r.Machine.PiInstalled(root)
	switch {
	case req.Target == Pi && !installed:
		return nil, &Refusal{Kind: PiNotInstalled, Root: root}
	case req.Target == All && installed:
		return append(targets, Pi), nil
	default:
		return targets, nil
	}
}

// each does act for every target, in order, whatever happened to the ones before it, at the root the request gave or
// the machine tells for the runtime; a runtime whose root cannot be told is not attempted.
func (r Runtimes) each(req Request, targets []string, act func(target, root string) Result) []Result {
	results := make([]Result, 0, len(targets))
	for _, target := range targets {
		root := orElse(req.ConfigRoot, r.Machine.ConfigRoot(target))
		if root == "" {
			results = append(results, Result{Target: target, Outcome: RootUnresolved})
			continue
		}
		results = append(results, act(target, root))
	}
	return results
}

// orElse is given, or fallback when nothing was given.
func orElse(given, fallback string) string {
	if given != "" {
		return given
	}
	return fallback
}
