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

const (
	defaultTimeout = 60 * time.Second

	// notProjectMarker is the exact substring longterm-mem's own
	// resolveProjectFlagWith emits (project_resolve.go) when --project is
	// omitted and the working directory does not resolve to a project.
	// Matching on it is how classify tells a genuine usage error (any
	// other flag.Parse failure) from "this cwd just isn't a project" --
	// both share exit 2.
	notProjectMarker = "could not be resolved from the working directory"
)

var validEvents = map[string]bool{"session-end": true, "archive": true}

// Run is the parent entrypoint. It validates its own argv, opens the log,
// locates itself, and detaches a "--child" re-exec of itself under a new
// session (Setsid) so it survives the caller's process-group teardown.
// It always returns 0: every failure in this path -- bad argv, an
// unwritable log, a missing/rebuilding self, a spawn failure -- is
// reported best-effort and swallowed, because the triggering host command
// must never observe a non-zero exit or block on this runner (R-003).
func Run(o Options) int {
	stderr := o.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}

	if !validEvents[o.Event] || !filepath.IsAbs(o.Cwd) {
		fmt.Fprintf(stderr, "sync-trigger: error:usage event=%q cwd=%q\n", o.Event, o.Cwd)
		return 0
	}

	logFile, err := openLog(o.StateDir)
	if err != nil {
		fmt.Fprintf(stderr, "sync-trigger: error:log: %v\n", err)
		return 0
	}
	defer logFile.Close()

	self := o.Self
	if self == "" {
		self, err = os.Executable()
		if err != nil {
			appendLog(logFile, o.Event, o.Cwd, "error:self", 0, 0)
			return 0
		}
	}

	cmd := exec.Command(self, "sync-trigger", "--event", o.Event, "--cwd", o.Cwd, "--state-dir", o.StateDir, "--child")
	if devnull, openErr := os.Open(os.DevNull); openErr == nil {
		cmd.Stdin = devnull
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		appendLog(logFile, o.Event, o.Cwd, "error:spawn", 0, 0)
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

	binary := o.Binary
	if binary == "" {
		binary = filepath.Join(o.StateDir, "bin", "longterm-mem")
	}

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

	if info, err := os.Stat(binary); err != nil || info.IsDir() {
		return logOutcome("skip:no-binary", 0, time.Since(start))
	}

	timeout := o.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, "sync")
	cmd.Dir = o.Cwd
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
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
		return logOutcome("timeout", 0, dur)
	}
	if runErr == nil {
		return logOutcome("ok", 0, dur)
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		exitCode := exitErr.ExitCode()
		return logOutcome(classify(exitCode, stderrBuf.String()), exitCode, dur)
	}
	return logOutcome("error:spawn", 0, dur)
}

// classify maps a longterm-mem sync exit code (and, for the ambiguous
// usage code, its stderr) onto the sync-trigger outcome contract. The
// exit codes it matches are longterm-mem's own published contract
// (longterm-mem/cmd/longterm-mem/exit_codes.go).
func classify(exitCode int, stderr string) string {
	switch exitCode {
	case 0:
		return "ok"
	case 2:
		if strings.Contains(stderr, notProjectMarker) {
			return "skip:no-project"
		}
		return "error:usage"
	case 3:
		return "skip:no-vault"
	case 4:
		return "failure:engram-unavailable"
	case 5:
		return "failure:vault-subprocess"
	default:
		return "failure:exit-" + strconv.Itoa(exitCode)
	}
}

// openLog opens (creating and appending) the sync-trigger log under
// stateDir, creating its parent directory if needed.
func openLog(stateDir string) (*os.File, error) {
	dir := filepath.Join(stateDir, "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return os.OpenFile(filepath.Join(dir, "sync-trigger.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
}

// appendLog writes one summary line for a firing of the runner.
func appendLog(w io.Writer, event, cwd, outcome string, exitCode int, dur time.Duration) {
	fmt.Fprintf(w, "%s event=%s cwd=%s outcome=%s exit=%d duration=%s\n",
		time.Now().UTC().Format(time.RFC3339), event, cwd, outcome, exitCode, dur.Round(time.Millisecond))
}
