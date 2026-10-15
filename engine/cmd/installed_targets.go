package main

// The 'installed-targets' subcommand: the record of the runtimes the overlay has installed
// longterm-mem for. longterm-mem is one binary shared by every runtime it is registered with, and
// this record is the only thing that says whether a runtime still depends on it: the binary goes
// when the record is empty, and not before. The overlay script used to read and write the record
// itself, in bash, behind a lock made of a directory (Phase 9, B3); the rule is the domain's
// (engine/runtime/ownership), the use case reads and writes through ports
// (engine/runtime/ownership/app), the adapter is the file system and an advisory file lock
// (engine/runtime/ownership/fsstore), and this file is the command line and the words.
//
// The words are the overlay script's, kept as they were because the script's output is pinned and
// read by people: a refusal is "ERROR: longterm-mem: ...", a warning "WARN: longterm-mem: ..." and an
// act "==> ...", where the commands of the engine otherwise say "error: ...". The causes the script
// left to the tools of the machine (mkdir, mktemp, rm) are said here, after the sentence.

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capability"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/ownership"
	ownershipapp "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/ownership/app"
	ownershipfs "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/ownership/fsstore"
)

const (
	flagStateDir         = "--state-dir"
	flagBinary           = "--binary"
	flagPurge            = "--purge"
	flagUnregisterFailed = "--unregister-failed"
)

// exitKept is what 'assess' ends with when the registration record and the shared binary are kept; the
// reason is said on the stream the script said it on. Exit 0 means they may go, and 1 is an error.
const exitKept = 3

// ownershipWiring builds the use case for a state directory, and says where its files are.
type ownershipWiring func(stateDir string) (*ownershipapp.Service, ownershipfs.Layout)

// runInstalledTargets implements the 'installed-targets' subcommand over the files of the machine.
func runInstalledTargets(p process, args []string) {
	runInstalledTargetsCore(productionOwnership, args, p.stdout, p.stderr, p.exit)
}

// productionOwnership wires the use case to the file system: the record under the state directory,
// the lock around it, and the shared binary. The runtimes it can record are the ones the engine knows.
func productionOwnership(stateDir string) (*ownershipapp.Service, ownershipfs.Layout) {
	store := ownershipfs.New(stateDir)
	return ownershipapp.New(store, ownershipfs.Binaries{}, capability.IsTarget), store.Layout
}

// targetsVerb is what a verb takes: the flags that carry a value (the first of the required ones are
// named in the refusal), the flags that carry none.
type targetsVerb struct {
	required []string
	optional []string
	switches []string
}

var installedTargetsVerbs = map[string]targetsVerb{
	"record":  {required: []string{flagStateDir, flagTarget}},
	"release": {required: []string{flagStateDir, flagTarget}},
	"list":    {required: []string{flagStateDir}},
	"assess":  {required: []string{flagStateDir}, switches: []string{flagPurge, flagUnregisterFailed}},
	"retire":  {required: []string{flagStateDir, flagBinary}, switches: []string{flagPurge}},
}

// targetsArgs is the command line of a verb, read.
type targetsArgs struct {
	values   map[string]string
	switches map[string]bool
}

// parseInstalledTargetsArgs reads the arguments after the verb, left to right and strictly. A flag
// nobody named for the verb, a flag that has no value (none, an empty one, or another flag of the verb
// in its place) and a flag the verb requires that is not given are refused, saying which. A flag given
// twice takes its last value.
func parseInstalledTargetsArgs(verb string, spec targetsVerb, args []string) (targetsArgs, error) {
	out := targetsArgs{values: map[string]string{}, switches: map[string]bool{}}
	takesValue := map[string]bool{}
	for _, flag := range append(append([]string(nil), spec.required...), spec.optional...) {
		takesValue[flag] = true
	}
	isFlag := func(arg string) bool {
		if takesValue[arg] {
			return true
		}
		for _, name := range spec.switches {
			if arg == name {
				return true
			}
		}
		return false
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case takesValue[arg]:
			if i+1 >= len(args) || args[i+1] == "" || isFlag(args[i+1]) {
				return out, fmt.Errorf("installed-targets %s: flag %q requires a value", verb, arg)
			}
			i++
			out.values[arg] = args[i]
		case isFlag(arg):
			out.switches[arg] = true
		default:
			return out, fmt.Errorf("installed-targets %s: unknown argument %q", verb, arg)
		}
	}
	for _, flag := range spec.required {
		if out.values[flag] == "" {
			return out, fmt.Errorf("installed-targets %s requires %s", verb, strings.Join(spec.required, " and "))
		}
	}
	return out, nil
}

// runInstalledTargetsCore is the testable core of the subcommand: it names the verb, reads the command
// line, builds the use case for the state directory, runs the verb and tells the outcome. A verb that
// fails says why on stderr and ends with exit 1, the record as it was; 'assess' ends with 3 when what it
// is asked about is kept.
func runInstalledTargetsCore(wire ownershipWiring, args []string, stdout, stderr io.Writer, exit func(int)) {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "error: installed-targets requires a verb: record, release, list, assess, retire")
		exit(1)
		return
	}
	spec, known := installedTargetsVerbs[args[0]]
	if !known {
		fmt.Fprintf(stderr, "error: unknown installed-targets verb %q; expected record, release, list, assess or retire\n", args[0])
		exit(1)
		return
	}
	parsed, err := parseInstalledTargetsArgs(args[0], spec, args[1:])
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		exit(1)
		return
	}

	svc, layout := wire(parsed.values[flagStateDir])
	c := installedTargets{svc: svc, layout: layout, stdout: stdout, stderr: stderr}
	var code int
	switch args[0] {
	case "record":
		code = c.record(parsed.values[flagTarget])
	case "release":
		code = c.release(parsed.values[flagTarget])
	case "list":
		code = c.list()
	case "assess":
		code = c.assess(ownership.Intent{Purge: parsed.switches[flagPurge], UnregisterFailed: parsed.switches[flagUnregisterFailed]})
	case "retire":
		code = c.retire(ownershipapp.RetireRequest{Binary: parsed.values[flagBinary], Purge: parsed.switches[flagPurge]})
	}
	if code != 0 {
		exit(code)
	}
}

// installedTargets is a verb being run: the use case, where its files are, and where it speaks.
type installedTargets struct {
	svc    *ownershipapp.Service
	layout ownershipfs.Layout
	stdout io.Writer
	stderr io.Writer
}

func (c installedTargets) record(target string) int {
	err := c.svc.Record(target)
	return c.refuse(err, target, fmt.Sprintf("could not read %s; refusing to record %s against install state that cannot be inspected", c.layout.Record(), target))
}

func (c installedTargets) release(target string) int {
	err := c.svc.Release(target)
	return c.refuse(err, target, fmt.Sprintf("could not read %s; refusing to rewrite it from a file that could not be inspected", c.layout.Record()))
}

// list prints the names the record vouches for, one to a line. A record that cannot be read is said,
// with no name, and ends with exit 1: "I could not look" is not "there is nothing".
func (c installedTargets) list() int {
	obs := c.svc.Tracked()
	if obs.State == ownership.Unreadable {
		c.readerWarning()
		return 1
	}
	for _, name := range obs.Targets.Names() {
		fmt.Fprintln(c.stdout, name)
	}
	return 0
}

// assess says what an uninstall comes to: exit 0 and nothing said when the registration record and the
// shared binary may go, exit 3 and the reason when they are kept.
func (c installedTargets) assess(in ownership.Intent) int {
	a := c.svc.Assess(in)
	if a.Observation.State == ownership.Unreadable {
		c.readerWarning()
	}
	switch a.Verdict {
	case ownership.FailedUnregister:
		c.warn("at least one target failed to unregister; leaving the registration record and the shared binary untouched (re-run uninstall for the failed target(s) once resolved, or pass --purge to force removal)")
	case ownership.UnreadableRecord:
		c.warn("the install tracking file at " + c.layout.Record() + " exists but could not be read, so whether any runtime still depends on the shared binary is unknown; leaving the registration record and the shared binary untouched. Fix its permissions and re-run uninstall, or pass --purge to remove them anyway.")
	case ownership.NoRecord:
		c.warn("no install tracking found at " + c.layout.Record() + "; leaving the registration record and the shared binary untouched. Re-run with --purge to remove them anyway.")
	case ownership.StillInstalled:
		fmt.Fprintf(c.stdout, "  longterm-mem: unregistered the requested target(s); still installed for: %s (shared binary and engine-owned tracking left untouched -- re-run uninstall for the remaining target(s), or pass --purge, to remove them)\n",
			strings.Join(a.Observation.Targets.Names(), "\n"))
	default:
		return 0
	}
	return exitKept
}

// retire removes the shared binary and the record when the record, read again under the lock, says
// that nothing depends on the binary, or when --purge forces it.
func (c installedTargets) retire(req ownershipapp.RetireRequest) int {
	got, err := c.svc.Retire(req)
	if err != nil {
		return c.refuse(err, "", "")
	}
	if got.Observation.State == ownership.Unreadable {
		c.readerWarning()
	}
	switch {
	case got.Disposal == ownership.HoldBinaryUnreadable:
		c.warn("the install tracking file could not be read, so whether any runtime still depends on the shared binary is unknown; leaving " + req.Binary + " in place. Fix its permissions and re-run uninstall, or pass --purge to remove it anyway.")
	case got.Binary == ownershipapp.BinaryRemoved:
		fmt.Fprintf(c.stdout, "==> longterm-mem binary removed: %s\n", req.Binary)
	case got.Binary == ownershipapp.BinaryAbsent:
		fmt.Fprintf(c.stdout, "==> longterm-mem binary already absent: %s\n", req.Binary)
	}
	return 0
}

// readerWarning is what the script's reader said of a record that is there and cannot be read, before
// whatever else it said.
func (c installedTargets) readerWarning() {
	c.warn(c.layout.Record() + " exists but cannot be read; refusing to read it as empty install state")
}

func (c installedTargets) warn(text string) { fmt.Fprintf(c.stderr, "WARN: longterm-mem: %s\n", text) }

func (c installedTargets) die(text string) int {
	fmt.Fprintf(c.stderr, "ERROR: longterm-mem: %s\n", text)
	return 1
}

// cause is why a step failed, as the system said it: the error of a path or a link without the operation and
// the path it repeats, since the sentence it ends names the file already ("not a directory", "permission
// denied"). An error that is neither is said as it is.
func cause(err error) error {
	var path *fs.PathError
	if errors.As(err, &path) {
		return path.Err
	}
	var link *os.LinkError
	if errors.As(err, &link) {
		return link.Err
	}
	return err
}

// refuse says why a verb could not do what it was asked, in the script's words, and returns the exit
// code it ends with: 0 for no error, 1 for any. unreadable is the sentence for a record that cannot be
// read, which differs by verb; target names the target an append failed for.
func (c installedTargets) refuse(err error, target, unreadable string) int {
	if err == nil {
		return 0
	}
	var (
		unknown   *ownership.UnknownTargetError
		recordDir *ownershipapp.RecordDirError
		stateDir  *ownershipapp.StateDirError
		lock      *ownershipapp.LockError
		noRead    *ownershipapp.UnreadableError
		appendErr *ownershipapp.AppendError
		replace   *ownershipapp.ReplaceError
		binary    *ownershipapp.BinaryRemoveError
		record    *ownershipapp.RecordRemoveError
	)
	switch {
	case errors.As(err, &unknown):
		return c.die(unknown.Error())
	case errors.As(err, &recordDir):
		return c.die(fmt.Sprintf("could not create %s: %v", c.layout.RecordDir(), cause(recordDir.Err)))
	case errors.As(err, &stateDir):
		return c.die(fmt.Sprintf("could not create %s: %v", c.layout.StateDir, cause(stateDir.Err)))
	case errors.As(err, &lock):
		if errors.Is(err, ownershipapp.ErrBusy) {
			return c.die("could not acquire the install-tracking lock at " + c.layout.Lock() + "; another overlay invocation is updating install state. Nothing was changed — re-run once it finishes.")
		}
		return c.die(fmt.Sprintf("could not take the install-tracking lock at %s: %v", c.layout.Lock(), cause(lock.Err)))
	case errors.As(err, &noRead):
		c.readerWarning()
		return c.die(unreadable)
	case errors.As(err, &appendErr):
		return c.die(fmt.Sprintf("could not record %s in %s: %v", target, c.layout.Record(), cause(appendErr.Err)))
	case errors.As(err, &replace):
		switch replace.Stage {
		case ownershipapp.StageCreate:
			return c.die(fmt.Sprintf("could not create a temporary file in %s: %v", c.layout.RecordDir(), cause(replace.Err)))
		case ownershipapp.StageWrite:
			return c.die(fmt.Sprintf("could not rewrite %s: %v", c.layout.Record(), cause(replace.Err)))
		default:
			return c.die(fmt.Sprintf("could not update %s: %v", c.layout.Record(), cause(replace.Err)))
		}
	case errors.As(err, &binary):
		return c.die(fmt.Sprintf("could not remove %s: %v", binary.Path, cause(binary.Err)))
	case errors.As(err, &record):
		return c.die(fmt.Sprintf("could not remove %s: %v", c.layout.Record(), cause(record.Err)))
	}
	return c.die(err.Error())
}
