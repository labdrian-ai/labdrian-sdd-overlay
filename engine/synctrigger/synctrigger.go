// Package synctrigger implements the non-blocking sync-trigger runner: a
// parent that detaches a child of itself and returns immediately, and a
// child that runs `longterm-mem sync` under a bounded timeout, classifies
// the outcome, and appends one summary line to an operator-discoverable
// log. Both the parent's pre-spawn failures and the child's sync outcome
// are contract-mapped so the triggering host command (a Claude Code hook,
// or the archive closure-feedback step) never sees a non-zero exit or a
// blocking call.
package synctrigger

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Options configures a sync-trigger run. Run uses it to validate its own
// argv and spawn the detached child; RunChild uses it to locate and invoke
// the longterm-mem binary. Callers (engine/cmd) are responsible for
// resolving defaults that depend on the environment (e.g. the state
// directory) before constructing Options -- this package accepts what it
// is given and does not consult HOME itself, which keeps it testable
// without environment coupling.
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
	// means os.Executable().
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
// Options.ChildArgv gives (the child that runs RunChild), under a new
// session (Setsid) so it survives the caller's process-group teardown.
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

	self := o.Self
	if self == "" {
		self, err = os.Executable()
		if err != nil {
			reportFailure(stderr, kindSelf, err)
			appendLog(logFile, o.Event, o.Cwd, kindSelf, 0, 0)
			return 0
		}
	}

	if o.ChildArgv == nil {
		reportFailure(stderr, kindSpawn, "no command line was given for the child")
		appendLog(logFile, o.Event, o.Cwd, kindSpawn, 0, 0)
		return 0
	}
	cmd := exec.Command(self, o.ChildArgv(o.Event, o.Cwd, o.StateDir)...)
	if devnull, openErr := os.Open(os.DevNull); openErr == nil {
		cmd.Stdin = devnull
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		reportFailure(stderr, kindSpawn, err)
		appendLog(logFile, o.Event, o.Cwd, kindSpawn, 0, 0)
		return 0
	}

	return 0
}

// RunChild runs longterm-mem sync to completion (or until it times out),
// classifies the outcome, appends one summary log line, and returns that
// Outcome. It is invoked by the detached "--child" re-exec that Run
// starts -- never by Run itself -- so it always runs in its own process,
// already free of the caller's process group.
func RunChild(o Options) Outcome {
	start := time.Now()

	logFile, logErr := openLog(o.StateDir)
	if logErr == nil {
		defer logFile.Close()
	}

	logOutcome := func(kind string, exitCode int, dur time.Duration) Outcome {
		outcome := Outcome{Kind: kind, Exit: exitCode, Dur: dur}
		if logFile != nil {
			appendLog(logFile, o.Event, o.Cwd, outcome.Kind, outcome.Exit, outcome.Dur)
		}
		return outcome
	}

	if !validArgs(o.Event, o.Cwd) {
		reportUsage(o.stderr(), o.Event, o.Cwd)
		return logOutcome(kindUsage, 0, time.Since(start))
	}

	binary := o.Binary
	if binary == "" {
		binary = filepath.Join(o.StateDir, binDirName, defaultBinaryName)
	}

	info, statErr := os.Stat(binary)
	if statErr != nil || info.IsDir() {
		return logOutcome(kindNoBinary, 0, time.Since(start))
	}
	if info.Mode()&0o111 == 0 {
		return logOutcome(kindNotExecutable, 0, time.Since(start))
	}

	timeout := o.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, syncCommand)
	cmd.Dir = o.Cwd
	var stderrBuf bytes.Buffer
	if logFile != nil {
		cmd.Stderr = io.MultiWriter(&stderrBuf, logFile)
	} else {
		cmd.Stderr = &stderrBuf
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 5 * time.Second

	runErr := cmd.Run()
	dur := time.Since(start)

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return logOutcome(kindTimeout, 0, dur)
	}
	if runErr == nil {
		return logOutcome(kindOK, 0, dur)
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		exitCode := exitErr.ExitCode()
		return logOutcome(classify(exitCode, stderrBuf.String()), exitCode, dur)
	}
	return logOutcome(kindSpawn, 0, dur)
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
// stateDir, creating its parent directory if needed.
func openLog(stateDir string) (*os.File, error) {
	dir := filepath.Join(stateDir, logDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return os.OpenFile(filepath.Join(dir, logFileName), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
}

// appendLog writes one summary line for a firing of the runner.
func appendLog(w io.Writer, event, cwd, outcome string, exitCode int, dur time.Duration) {
	fmt.Fprintf(w, "%s event=%s cwd=%s outcome=%s exit=%d duration=%s\n",
		time.Now().UTC().Format(time.RFC3339), event, cwd, outcome, exitCode, dur.Round(time.Millisecond))
}
