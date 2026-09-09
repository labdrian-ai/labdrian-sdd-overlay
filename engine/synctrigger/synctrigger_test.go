package synctrigger

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// newFakeLongtermMem writes an executable shell script that exits with the
// given code, optionally writing stderrMsg to stderr first. If sleep > 0 it
// sleeps that long instead of exiting immediately, and writes its own pid
// to $SYNCTRIGGER_TEST_PIDFILE first (used by the timeout/orphan test).
func newFakeLongtermMem(t *testing.T, exitCode int, stderrMsg string, sleep time.Duration) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "longterm-mem")
	var script strings.Builder
	script.WriteString("#!/bin/sh\n")
	if sleep > 0 {
		script.WriteString("if [ -n \"$SYNCTRIGGER_TEST_PIDFILE\" ]; then echo $$ > \"$SYNCTRIGGER_TEST_PIDFILE\"; fi\n")
		script.WriteString("sleep " + sleep.String() + "\n")
	}
	if stderrMsg != "" {
		script.WriteString("echo '" + stderrMsg + "' 1>&2\n")
	}
	script.WriteString("exit " + strconv.Itoa(exitCode) + "\n")
	if err := os.WriteFile(path, []byte(script.String()), 0o755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
	return path
}

// lastLogLine returns the last non-empty line of the sync-trigger log under
// stateDir, or "" if the log does not exist / is empty.
func lastLogLine(t *testing.T, stateDir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(stateDir, "logs", "sync-trigger.log"))
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) == 0 {
		return ""
	}
	return lines[len(lines)-1]
}

func TestRunChild_MissingBinary_SkipsWithoutRunning(t *testing.T) {
	stateDir := t.TempDir()
	o := Options{
		Event:    "session-end",
		Cwd:      t.TempDir(),
		StateDir: stateDir,
		Binary:   filepath.Join(stateDir, "bin", "does-not-exist"),
	}

	got := RunChild(o)

	if got.Kind != "skip:no-binary" {
		t.Fatalf("Kind = %q, want %q", got.Kind, "skip:no-binary")
	}
	line := lastLogLine(t, stateDir)
	if !strings.Contains(line, "outcome=skip:no-binary") {
		t.Fatalf("log line = %q, want outcome=skip:no-binary", line)
	}
}

func TestRunChild_Exit2_NotProjectStderr_SkipsNoProject(t *testing.T) {
	stateDir := t.TempDir()
	bin := newFakeLongtermMem(t, 2, "longterm-mem: sync: --project is required: it could not be resolved from the working directory: not a git repo", 0)
	o := Options{Event: "archive", Cwd: t.TempDir(), StateDir: stateDir, Binary: bin}

	got := RunChild(o)

	if got.Kind != "skip:no-project" {
		t.Fatalf("Kind = %q, want %q", got.Kind, "skip:no-project")
	}
	if got.Exit != 2 {
		t.Fatalf("Exit = %d, want 2", got.Exit)
	}
	line := lastLogLine(t, stateDir)
	if !strings.Contains(line, "outcome=skip:no-project") {
		t.Fatalf("log line = %q, want outcome=skip:no-project", line)
	}
}

func TestRunChild_Exit2_OtherStderr_ErrorUsage(t *testing.T) {
	stateDir := t.TempDir()
	bin := newFakeLongtermMem(t, 2, "longterm-mem: sync: unknown flag --bogus", 0)
	o := Options{Event: "session-end", Cwd: t.TempDir(), StateDir: stateDir, Binary: bin}

	got := RunChild(o)

	if got.Kind != "error:usage" {
		t.Fatalf("Kind = %q, want %q", got.Kind, "error:usage")
	}
	line := lastLogLine(t, stateDir)
	if !strings.Contains(line, "outcome=error:usage") {
		t.Fatalf("log line = %q, want outcome=error:usage", line)
	}
}

func TestRunChild_ExitMapping(t *testing.T) {
	cases := []struct {
		name     string
		exitCode int
		want     string
	}{
		{"vault-not-configured", 3, "skip:no-vault"},
		{"engram-unavailable", 4, "failure:engram-unavailable"},
		{"vault-subprocess-failed", 5, "failure:vault-subprocess"},
		{"generic-internal", 1, "failure:exit-1"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stateDir := t.TempDir()
			bin := newFakeLongtermMem(t, tc.exitCode, "", 0)
			o := Options{Event: "session-end", Cwd: t.TempDir(), StateDir: stateDir, Binary: bin}

			got := RunChild(o)

			if got.Kind != tc.want {
				t.Fatalf("Kind = %q, want %q", got.Kind, tc.want)
			}
			if got.Exit != tc.exitCode {
				t.Fatalf("Exit = %d, want %d", got.Exit, tc.exitCode)
			}
		})
	}
}

func TestRunChild_StderrReachesLogBeforeOutcomeLine(t *testing.T) {
	stateDir := t.TempDir()
	bin := newFakeLongtermMem(t, 3, "vault not configured", 0)
	o := Options{Event: "session-end", Cwd: t.TempDir(), StateDir: stateDir, Binary: bin}

	got := RunChild(o)

	if got.Kind != "skip:no-vault" {
		t.Fatalf("Kind = %q, want %q", got.Kind, "skip:no-vault")
	}
	data, err := os.ReadFile(filepath.Join(stateDir, "logs", "sync-trigger.log"))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	log := string(data)
	stderrIdx := strings.Index(log, "vault not configured")
	if stderrIdx < 0 {
		t.Fatalf("log = %q, want it to contain the child's stderr text", log)
	}
	outcomeIdx := strings.Index(log, "outcome=skip:no-vault")
	if outcomeIdx < 0 {
		t.Fatalf("log = %q, want it to contain the summary line", log)
	}
	if stderrIdx > outcomeIdx {
		t.Fatalf("log = %q, want stderr text before the summary line", log)
	}
}

func TestRunChild_NonExecutableBinary_ErrorBinaryNotExecutable(t *testing.T) {
	stateDir := t.TempDir()
	binDir := t.TempDir()
	bin := filepath.Join(binDir, "longterm-mem")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o644); err != nil {
		t.Fatalf("write non-executable binary: %v", err)
	}
	o := Options{Event: "session-end", Cwd: t.TempDir(), StateDir: stateDir, Binary: bin}

	got := RunChild(o)

	if got.Kind != "error:binary-not-executable" {
		t.Fatalf("Kind = %q, want %q", got.Kind, "error:binary-not-executable")
	}
	line := lastLogLine(t, stateDir)
	if !strings.Contains(line, "outcome=error:binary-not-executable") {
		t.Fatalf("log line = %q, want outcome=error:binary-not-executable", line)
	}
}

func TestRunChild_ExitZero_Ok(t *testing.T) {
	stateDir := t.TempDir()
	bin := newFakeLongtermMem(t, 0, "", 0)
	o := Options{Event: "session-end", Cwd: t.TempDir(), StateDir: stateDir, Binary: bin}

	got := RunChild(o)

	if got.Kind != "ok" {
		t.Fatalf("Kind = %q, want %q", got.Kind, "ok")
	}
	if got.Exit != 0 {
		t.Fatalf("Exit = %d, want 0", got.Exit)
	}
	line := lastLogLine(t, stateDir)
	if !strings.Contains(line, "outcome=ok") {
		t.Fatalf("log line = %q, want outcome=ok", line)
	}
}

func TestRunChild_Timeout_KillsProcessGroup_NoOrphan(t *testing.T) {
	stateDir := t.TempDir()
	pidFile := filepath.Join(t.TempDir(), "pid")
	t.Setenv("SYNCTRIGGER_TEST_PIDFILE", pidFile)
	bin := newFakeLongtermMem(t, 0, "", 5*time.Second)
	o := Options{
		Event:    "session-end",
		Cwd:      t.TempDir(),
		StateDir: stateDir,
		Binary:   bin,
		Timeout:  150 * time.Millisecond,
	}

	start := time.Now()
	got := RunChild(o)
	elapsed := time.Since(start)

	if got.Kind != "timeout" {
		t.Fatalf("Kind = %q, want %q", got.Kind, "timeout")
	}
	if elapsed > 3*time.Second {
		t.Fatalf("RunChild took %s, want well under the 5s sleep (bounded by timeout)", elapsed)
	}
	line := lastLogLine(t, stateDir)
	if !strings.Contains(line, "outcome=timeout") {
		t.Fatalf("log line = %q, want outcome=timeout", line)
	}

	// No orphan: the sleeping process must not still be alive shortly after
	// RunChild returns.
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(pidFile)
		if err == nil && len(strings.TrimSpace(string(data))) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	data, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("read pidfile: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("parse pid %q: %v", data, err)
	}
	proc, _ := os.FindProcess(pid)
	if err := proc.Signal(syscall.Signal(0)); err == nil {
		t.Fatalf("process %d is still alive after RunChild returned (orphan)", pid)
	}
}

func TestRun_BadEvent_ErrorUsage(t *testing.T) {
	stateDir := t.TempDir()
	var stderr strings.Builder
	o := Options{Event: "bogus-event", Cwd: t.TempDir(), StateDir: stateDir, Stderr: &stderr}

	got := Run(o)

	if got != 0 {
		t.Fatalf("Run() = %d, want 0", got)
	}
	if !strings.Contains(stderr.String(), "error:usage") {
		t.Fatalf("stderr = %q, want it to mention error:usage", stderr.String())
	}
}

func TestRun_UnwritableLogDir_ErrorLog(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	stateDir := t.TempDir()
	logsDir := filepath.Join(stateDir, "logs")
	if err := os.MkdirAll(logsDir, 0o500); err != nil {
		t.Fatalf("mkdir logs: %v", err)
	}
	t.Cleanup(func() { os.Chmod(logsDir, 0o700) })

	var stderr strings.Builder
	o := Options{Event: "session-end", Cwd: t.TempDir(), StateDir: stateDir, Stderr: &stderr}

	got := Run(o)

	if got != 0 {
		t.Fatalf("Run() = %d, want 0", got)
	}
	if !strings.Contains(stderr.String(), "error:log") {
		t.Fatalf("stderr = %q, want it to mention error:log", stderr.String())
	}
}

func TestRun_NonExecutableSelf_ErrorSpawn(t *testing.T) {
	stateDir := t.TempDir()
	self := filepath.Join(t.TempDir(), "not-executable")
	if err := os.WriteFile(self, []byte("#!/bin/sh\nexit 0\n"), 0o644); err != nil {
		t.Fatalf("write self: %v", err)
	}

	var stderr strings.Builder
	o := Options{Event: "session-end", Cwd: t.TempDir(), StateDir: stateDir, Self: self, Stderr: &stderr}

	got := Run(o)

	if got != 0 {
		t.Fatalf("Run() = %d, want 0", got)
	}
	line := lastLogLine(t, stateDir)
	if !strings.Contains(line, "outcome=error:spawn") {
		t.Fatalf("log line = %q, want outcome=error:spawn", line)
	}
	if !strings.Contains(stderr.String(), "error:spawn") {
		t.Fatalf("stderr = %q, want it to mention error:spawn", stderr.String())
	}
}

func TestRun_HappyPath_DetachesAndLogsWithin2s(t *testing.T) {
	stateDir := t.TempDir()
	self := filepath.Join(t.TempDir(), "fake-self")
	// Simulates the detached "--child" re-exec: it writes its own summary
	// line to the log the real parent already opened (stdout/stderr are
	// redirected to that same file, but this script writes directly to
	// prove the file Run opened is genuinely shared with the child). Run
	// invokes self as: sync-trigger --event <event> --cwd <cwd> --state-dir
	// <dir> --child, so within this script $1=sync-trigger, $2=--event,
	// $3=<event>, $4=--cwd, $5=<cwd>.
	script := "#!/bin/sh\necho \"$(date -u +%Y-%m-%dT%H:%M:%SZ) event=$3 cwd=$5 outcome=ok exit=0 duration=1ms\" >> \"" + stateDir + "/logs/sync-trigger.log\"\n"
	if err := os.WriteFile(self, []byte(script), 0o755); err != nil {
		t.Fatalf("write self: %v", err)
	}

	o := Options{Event: "session-end", Cwd: t.TempDir(), StateDir: stateDir, Self: self}

	got := Run(o)

	if got != 0 {
		t.Fatalf("Run() = %d, want 0", got)
	}

	deadline := time.Now().Add(2 * time.Second)
	var line string
	for time.Now().Before(deadline) {
		line = lastLogLine(t, stateDir)
		if strings.Contains(line, "outcome=ok") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(line, "outcome=ok") {
		t.Fatalf("log line after 2s = %q, want it to contain outcome=ok", line)
	}
}
