// Package synctrigger implements the non-blocking sync-trigger runner: a
// parent that detaches a child of itself and returns immediately, and a
// child that runs `longterm-mem sync` under a bounded timeout, classifies
// the outcome, and appends one summary line to an operator-discoverable
// log. Both the parent's pre-spawn failures and the child's sync outcome
// are contract-mapped so the triggering host command (a Claude Code hook,
// or the archive closure-feedback step) never sees a non-zero exit or a
// blocking call.
//
// The trigger starts no program and reads no clock itself. Programs are
// started by the Runner it is given (engine/synctrigger/osproc is the one
// for the operating system), and the time of a log line and the length of a
// sync are read from the clock it is given, so what the trigger decides
// can be seen without a process or a wait.
package synctrigger

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/filelock"
)

// Options configures a sync-trigger run. Run uses it to validate its own
// argv and spawn the detached child; RunChild uses it to locate and invoke
// the longterm-mem binary. Callers (engine/cmd) are responsible for
// resolving defaults that depend on the environment (e.g. the state
// directory) and for handing in the clock and the Runner -- this package
// accepts what it is given and does not consult HOME, the clock or the
// process table itself, which keeps it testable without environment
// coupling.
type Options struct {
	// Event is "session-end" or "archive".
	Event string
	// Cwd is the working directory the child sync runs in -- the project
	// directory for session-end, the project root for archive.
	Cwd string
	// StateDir is the overlay state root (default ~/.labdrian-overlay,
	// resolved by the caller). The log lives at
	// StateDir/logs/sync-trigger.log; the default longterm-mem binary path
	// is StateDir/bin/longterm-mem.
	StateDir string
	// Binary overrides the longterm-mem binary path. Empty means
	// StateDir/bin/longterm-mem.
	Binary string
	// Self overrides the path Run re-execs as the detached child. Empty
	// means the path Runner.Executable gives.
	Self string
	// ChildArgv gives the arguments Run starts Self with, from the event, the
	// absolute cwd and the state directory Run settled on. The command line
	// of the program (what it calls the runner and its flags) is the
	// caller's to know: this package only starts what it is told to and
	// reads none of that grammar. Run only. Nil means there is no child to
	// start, which Run reports as error:spawn.
	ChildArgv func(event, cwd, stateDir string) []string
	// Timeout bounds the child sync invocation. Zero means 60s.
	Timeout time.Duration
	// Stderr is where Run reports best-effort pre-log errors. Nil means
	// os.Stderr.
	Stderr io.Writer
	// Now is the clock: it dates every log line and times the sync. Nil
	// means a clock that always says the zero time, so that a trigger
	// started without one still never reads the machine's.
	Now func() time.Time
	// Runner starts the programs. Nil means none can be started: Run and
	// RunChild report it as error:spawn (error:self when Self is empty and
	// Run has to ask for its own path).
	Runner Runner
}

// Outcome is the classified result of one child sync attempt.
type Outcome struct {
	Kind string
	Exit int
	Dur  time.Duration
}

// The kinds of outcome the trigger reports, in its log and in an Outcome. The exit codes of
// longterm-mem map onto them in classify.
const (
	kindOK                = "ok"
	kindTimeout           = "timeout"
	kindUsage             = "error:usage"
	kindSpawn             = "error:spawn"
	kindSelf              = "error:self"
	kindLog               = "error:log"
	kindNotExecutable     = "error:binary-not-executable"
	kindNoBinary          = "skip:no-binary"
	kindNoProject         = "skip:no-project"
	kindNoVault           = "skip:no-vault"
	kindEngramUnavailable = "failure:engram-unavailable"
	kindVaultSubprocess   = "failure:vault-subprocess"
	kindExitPrefix        = "failure:exit-"
)

const (
	defaultTimeout = 60 * time.Second

	// The names under the state directory: the log, and the binary the sync runs when
	// Options.Binary is empty. syncCommand is the argument longterm-mem is started with.
	logDirName        = "logs"
	logFileName       = "sync-trigger.log"
	binDirName        = "bin"
	defaultBinaryName = "longterm-mem"
	syncCommand       = "sync"

	// logLimitBytes is the size at which the log is rotated when it is next opened, and keptLogs
	// how many rotated files (sync-trigger.log.1 to .3) are kept.
	logLimitBytes = 1 << 20
	keptLogs      = 3

	// stderrKeepBytes is how much of what the sync prints on stderr is kept to classify its exit
	// code with; all of it still goes to the log.
	stderrKeepBytes = 64 << 10

	// notProjectMarker is the exact substring longterm-mem's own
	// resolveProjectFlagWith emits (project_resolve.go) when --project is
	// omitted and the working directory does not resolve to a project.
	// Matching on it is how classify tells a genuine usage error (any
	// other flag.Parse failure) from "this cwd just isn't a project" --
	// both share exit 2.
	notProjectMarker = "could not be resolved from the working directory"
)

var validEvents = map[string]bool{"session-end": true, "archive": true}

// validArgs reports whether event is recognized and cwd is absolute and
// non-empty. Run and RunChild both reject argv failing this before doing
// anything else.
func validArgs(event, cwd string) bool {
	return validEvents[event] && filepath.IsAbs(cwd)
}

// stderr is where the trigger reports before it has a log.
func (o Options) stderr() io.Writer {
	if o.Stderr == nil {
		return os.Stderr
	}
	return o.Stderr
}

// now reads the clock the trigger was given.
func (o Options) now() time.Time {
	if o.Now == nil {
		return time.Time{}
	}
	return o.Now()
}

// runner is the Runner the trigger was given, or the one that fails every call.
func (o Options) runner() Runner {
	if o.Runner == nil {
		return noRunner{}
	}
	return o.Runner
}

// reportUsage says on stderr that the trigger was started with an event or a cwd it cannot use,
// naming both as the caller gave them. It is the one place the line is written, for Run and for
// RunChild, so the two cannot say it differently.
func reportUsage(stderr io.Writer, event, cwd string) {
	fmt.Fprintf(stderr, "sync-trigger: %s event=%q cwd=%q\n", kindUsage, event, cwd)
}

// reportFailure says on stderr that something the trigger needed failed, before there was a log
// to say it in.
func reportFailure(stderr io.Writer, kind string, detail any) {
	fmt.Fprintf(stderr, "sync-trigger: %s: %v\n", kind, detail)
}

// absoluteCwd is cwd made absolute against the working directory of the process, which is how a
// relative --cwd (from a "${CLAUDE_PROJECT_DIR:-$PWD}" hook fallback whose $PWD was itself
// relative, or any other caller) never becomes a silent error:usage. An empty cwd stays empty,
// because it must never inherit the caller's directory, and one that cannot be made absolute
// stays as it is: validArgs then refuses it.
func absoluteCwd(cwd string) string {
	if cwd == "" || filepath.IsAbs(cwd) {
		return cwd
	}
	if abs, err := filepath.Abs(cwd); err == nil {
		return abs
	}
	return cwd
}

// Run is the parent entrypoint. It validates its own argv, opens the log,
// locates itself, and detaches a re-exec of itself, with the arguments
// Options.ChildArgv gives (the child that runs RunChild), through the Runner,
// which starts it in a session of its own so that it survives the caller's
// process-group teardown.
// It always returns 0: every failure in this path -- bad argv, an
// unwritable log, a missing/rebuilding self, a spawn failure -- is
// reported best-effort and swallowed, because the triggering host command
// must never observe a non-zero exit or block on this runner (R-003).
func Run(o Options) int {
	stderr := o.stderr()

	cwd := absoluteCwd(o.Cwd)
	if !validArgs(o.Event, cwd) {
		reportUsage(stderr, o.Event, o.Cwd)
		return 0
	}
	o.Cwd = cwd

	logFile, err := openLog(o.StateDir)
	if err != nil {
		reportFailure(stderr, kindLog, err)
		return 0
	}
	defer logFile.Close()

	fail := func(kind string) { appendLog(logFile, o.now(), o.Event, o.Cwd, kind, 0, 0) }
	runner := o.runner()

	self := o.Self
	if self == "" {
		self, err = runner.Executable()
		if err != nil {
			reportFailure(stderr, kindSelf, err)
			fail(kindSelf)
			return 0
		}
	}

	if o.ChildArgv == nil {
		reportFailure(stderr, kindSpawn, "no command line was given for the child")
		fail(kindSpawn)
		return 0
	}
	if err := runner.Detach(self, o.ChildArgv(o.Event, o.Cwd, o.StateDir), logFile); err != nil {
		reportFailure(stderr, kindSpawn, err)
		fail(kindSpawn)
	}
	return 0
}

// RunChild runs longterm-mem sync to completion (or until it times out),
// classifies the outcome, appends one summary log line, and returns that
// Outcome. It is invoked by the detached "--child" re-exec that Run
// starts -- never by Run itself -- so it always runs in its own process,
// already free of the caller's process group.
func RunChild(o Options) Outcome {
	start := o.now()

	logFile, logErr := openLog(o.StateDir)
	if logErr == nil {
		defer logFile.Close()
	}

	finish := func(kind string, exitCode int) Outcome {
		end := o.now()
		outcome := Outcome{Kind: kind, Exit: exitCode, Dur: end.Sub(start)}
		if logFile != nil {
			appendLog(logFile, end, o.Event, o.Cwd, outcome.Kind, outcome.Exit, outcome.Dur)
		}
		return outcome
	}

	if !validArgs(o.Event, o.Cwd) {
		reportUsage(o.stderr(), o.Event, o.Cwd)
		return finish(kindUsage, 0)
	}

	binary := o.Binary
	if binary == "" {
		// A state directory that is not absolute would name a binary under whatever directory
		// the child runs in, so there is no default binary to run.
		if !filepath.IsAbs(o.StateDir) {
			return finish(kindNoBinary, 0)
		}
		binary = filepath.Join(o.StateDir, binDirName, defaultBinaryName)
	}

	info, statErr := os.Stat(binary)
	if statErr != nil || info.IsDir() {
		return finish(kindNoBinary, 0)
	}
	if info.Mode()&0o111 == 0 {
		return finish(kindNotExecutable, 0)
	}

	timeout := o.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	stderrBuf := &boundedBuffer{limit: stderrKeepBytes}
	var stderr io.Writer = stderrBuf
	if logFile != nil {
		stderr = io.MultiWriter(stderrBuf, logFile)
	}

	result, err := o.runner().Run(binary, []string{syncCommand}, o.Cwd, timeout, stderr)
	// What the Result says the program did comes before the error, which is how it always was: a
	// program killed at its time is a timeout, whatever it reported as it was stopped, and one
	// that ended with an exit code is classified by it (an *exec.ExitError was never a failure to
	// start). Only an error with nothing the program did is a failure to run it.
	switch {
	case result.TimedOut:
		return finish(kindTimeout, 0)
	case result.Exit != 0:
		return finish(classify(result.Exit, stderrBuf.String()), result.Exit)
	case err != nil:
		return finish(kindSpawn, 0)
	default:
		return finish(kindOK, 0)
	}
}

// classify maps a longterm-mem sync exit code (and, for the ambiguous
// usage code, its stderr) onto the sync-trigger outcome contract. The
// exit codes it matches are longterm-mem's own published contract
// (longterm-mem/cmd/longterm-mem/exit_codes.go).
func classify(exitCode int, stderr string) string {
	switch exitCode {
	case 0:
		return kindOK
	case 2:
		if strings.Contains(stderr, notProjectMarker) {
			return kindNoProject
		}
		return kindUsage
	case 3:
		return kindNoVault
	case 4:
		return kindEngramUnavailable
	case 5:
		return kindVaultSubprocess
	default:
		return kindExitPrefix + strconv.Itoa(exitCode)
	}
}

// openLog opens (creating and appending) the sync-trigger log under
// stateDir, creating its parent directory if needed. A log that has reached
// logLimitBytes is rotated first. The log names the projects that were
// synced, so it is readable by its owner only, including one an older
// version created world-readable. A state directory that is not absolute
// (the caller could not resolve the default, HOME being unset) is refused,
// so nothing is ever written under the working directory.
func openLog(stateDir string) (*os.File, error) {
	if !filepath.IsAbs(stateDir) {
		return nil, fmt.Errorf("state directory %q is not absolute", stateDir)
	}
	dir := filepath.Join(stateDir, logDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, logFileName)
	rotateLog(path)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	f.Chmod(0o600) // best effort: a log that cannot be narrowed is still written
	return f, nil
}

// rotateLog moves a log that has reached logLimitBytes to path.1, shifting
// the older files up and dropping the one past keptLogs, so the newest lines
// are always in path. Triggers that find a full log at once take turns on an
// exclusive engine/filelock lock of path.lock (made only then, so a log under
// its limit never gets one, and never removed, as filelock requires), and
// each looks at the size again under it, so only the first rotates and the
// fresh log is never rotated again. It is best effort: a lock that stays
// taken past filelock's bound, or a rename that fails, leaves the log to grow
// until the next opening.
func rotateLog(path string) {
	if !full(path) {
		return
	}
	unlock, err := filelock.Acquire(path+".lock", filelock.Options{Perm: 0o600})
	if err != nil {
		return
	}
	defer unlock()
	if !full(path) {
		return
	}
	os.Remove(path + "." + strconv.Itoa(keptLogs))
	for n := keptLogs - 1; n >= 1; n-- {
		os.Rename(path+"."+strconv.Itoa(n), path+"."+strconv.Itoa(n+1))
	}
	os.Rename(path, path+".1")
}

// full reports whether the log at path has reached logLimitBytes.
func full(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Size() >= logLimitBytes
}

// appendLog writes one summary line for a firing of the runner, dated at.
func appendLog(w io.Writer, at time.Time, event, cwd, outcome string, exitCode int, dur time.Duration) {
	fmt.Fprintf(w, "%s event=%s cwd=%s outcome=%s exit=%d duration=%s\n",
		at.UTC().Format(time.RFC3339), logField(event), logField(cwd), outcome, exitCode, dur.Round(time.Millisecond))
}

// logField is a value as one field of a log line: as it is when it is plain, and quoted when it
// holds a space, a control character, a quote or an equals sign, which could end the line or read
// as another field. That is how a directory name can never forge a line of the log.
func logField(s string) string {
	if strings.ContainsFunc(s, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r) || r == '"' || r == '=' || r == utf8.RuneError
	}) {
		return strconv.Quote(s)
	}
	return s
}

// boundedBuffer keeps the first limit bytes written to it and drops the rest, while saying every
// write took all of it, so the writer it is paired with in an io.MultiWriter still gets everything.
type boundedBuffer struct {
	buf   bytes.Buffer
	limit int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if room := b.limit - b.buf.Len(); room > 0 {
		b.buf.Write(p[:min(room, len(p))])
	}
	return len(p), nil
}

func (b *boundedBuffer) String() string { return b.buf.String() }
