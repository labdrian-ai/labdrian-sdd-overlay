// Package vault runs the claude-obsidian vault's own scripts confined to
// one resolved vault root. It is the sole longterm-mem package permitted
// to import "os/exec" (R-021): every subprocess call to the vault's
// tooling goes through Runner, never a shell, and never Engram's own CLI.
package vault

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Runner executes vault scripts with the vault root as their working
// directory, refusing any script that resolves outside that root.
type Runner struct {
	Root string
}

// timeoutExitCode is the synthetic exit code Run reports when ctx's
// deadline elapses before the subprocess exits on its own (shell
// timeout(1) convention).
const timeoutExitCode = 124

// waitDelay bounds how long Run drains stdout/stderr after killing a
// timed-out subprocess: a killed shell may have already forked a
// grandchild (e.g. a trailing `sleep`) holding the output pipes open;
// group-killing below normally reaps it too, this is just the backstop.
const waitDelay = 2 * time.Second

// Run executes script — relative to, or already inside, the vault root —
// with args as literal argv elements, never shell-interpreted, cwd = vault
// root (threat matrix: subprocess execution). It refuses to run, and never
// spawns a subprocess for, any script resolving (after symlink evaluation)
// outside the vault root.
//
// Run respects ctx's deadline: a still-running subprocess is killed and
// Run returns timeoutExitCode with whatever output was already captured,
// rather than blocking — timeout is one more exit-code case, not a
// special path.
func (r *Runner) Run(ctx context.Context, script string, args ...string) (stdout, stderr []byte, exitCode int, err error) {
	resolvedRoot, err := filepath.EvalSymlinks(r.Root)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("vault: resolve vault root %s: %w", r.Root, err)
	}

	scriptPath := script
	if !filepath.IsAbs(scriptPath) {
		scriptPath = filepath.Join(r.Root, scriptPath)
	}
	resolvedScript, err := filepath.EvalSymlinks(scriptPath)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("vault: resolve script %s: %w", script, err)
	}

	if !withinRoot(resolvedRoot, resolvedScript) {
		return nil, nil, 0, fmt.Errorf("vault: refusing to run script %s: resolves outside vault root %s", script, r.Root)
	}

	cmd := exec.CommandContext(ctx, resolvedScript, args...)
	cmd.Dir = resolvedRoot
	cmd.Env = restrictedEnv()
	// Own process-group leader so a timeout kills the whole tree, not just
	// the immediate child — a bare Kill() would leave a forked grandchild
	// running and holding the output pipes open.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = waitDelay

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	runErr := cmd.Run()

	if ctx.Err() != nil {
		return stdoutBuf.Bytes(), stderrBuf.Bytes(), timeoutExitCode, nil
	}

	var exitErr *exec.ExitError
	switch {
	case runErr == nil:
		return stdoutBuf.Bytes(), stderrBuf.Bytes(), 0, nil
	case errors.As(runErr, &exitErr):
		return stdoutBuf.Bytes(), stderrBuf.Bytes(), exitErr.ExitCode(), nil
	default:
		return stdoutBuf.Bytes(), stderrBuf.Bytes(), 0, fmt.Errorf("vault: run %s: %w", script, runErr)
	}
}

// withinRoot reports whether target is root itself or lies inside it.
func withinRoot(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// restrictedEnv returns the minimal environment vault subprocesses
// receive: PATH (interpreter lookup), HOME (script caches), LANG (stable
// encoding) — nothing else the current process carries (threat matrix).
func restrictedEnv() []string {
	var env []string
	for _, key := range []string{"PATH", "HOME", "LANG"} {
		if v, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+v)
		}
	}
	return env
}
