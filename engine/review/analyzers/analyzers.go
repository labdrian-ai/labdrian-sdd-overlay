// Package analyzers is the adapter of the Analyzers port of review/app: it runs deadcode, staticcheck, shellcheck and
// `go mod tidy -diff`, the programs of the second check of `labdrian review sweep` (own-rdd T6b3, S6 b), on a copy of a tree and returns the lines
// they print in their finding format.
//
// A finding line is `<path>:<line>[:<col>]: <text>`, which is what all three print: deadcode `file:line:col: unreachable
// func: Name`, staticcheck `file:line:col: message (SA1000)` and shellcheck in its gcc format (`-f gcc`). Only a line of
// that shape counts as a finding; the rest of what a program prints is not one.
//
// A program that did not run is told apart from one that found something by what it printed, not by its exit status, whose
// convention differs between the three and is not promised: a program that exits zero ran, one that exits non-zero ran
// when it printed at least one finding line, and one that exits non-zero with none, that is not on the PATH, that cannot be
// started, or that stopped at its deadline did not. The error then wraps app.ErrToolDidNotRun and names the program, and
// what it said on its standard error is repeated (bounded).
//
// The programs run with the environment they are given, in the directory of the module of the copy; the package reads no
// variable of the environment itself.
package analyzers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
)

// DefaultTimeout is how long one start of a program may run when the options give none. deadcode over a module takes
// seconds; a program past ten minutes is stuck.
const DefaultTimeout = 10 * time.Minute

// maxSaid bounds the part of what a program printed on its standard error that an error message repeats.
const maxSaid = 2000

// maxOutput bounds what the programs of the sweep may print on each stream before they are stopped.
const maxOutput = 64 << 20

// finding is the shape of a finding line.
var finding = regexp.MustCompile(`^[^:]+:\d+(?::\d+)?: \S`)

// Runner starts a program in dir with exactly the environment it is given and returns what it printed on its standard
// output and its standard error, apart. ExecRunner is the process adapter.
type Runner interface {
	Output(ctx context.Context, dir string, env []string, bin string, args ...string) (stdout, stderr []byte, err error)
}

// Options are what the adapter needs of the process it runs in.
type Options struct {
	// Environ is the environment the programs run under, os.Environ() in production.
	Environ []string
	// Timeout is how long one start of a program may run. Zero, or any duration that is not positive, is DefaultTimeout.
	Timeout time.Duration
}

// Tools implements app.Analyzers over a Runner.
type Tools struct {
	runner  Runner
	environ []string
	timeout time.Duration
}

var _ app.Analyzers = Tools{}

// New is the adapter over runner under the environment and the deadline of the options.
func New(runner Runner, o Options) Tools {
	timeout := o.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return Tools{runner: runner, environ: append([]string{}, o.Environ...), timeout: timeout}
}

// Deadcode runs `deadcode -test ./...` in the module of the copy. A module the copy has no go.mod for gives none and
// starts nothing: the module is new at the other side of the sweep.
func (t Tools) Deadcode(root, module string) ([]string, error) {
	dir := filepath.Join(root, filepath.FromSlash(module))
	if !isFile(filepath.Join(dir, "go.mod")) {
		return nil, nil
	}
	return t.run("deadcode", dir, "-test", "./...")
}

// Staticcheck runs staticcheck over the packages of the module of the copy that the copy has: a package is a pattern
// "." or "./dir", kept when its directory is there. None left, or no module, gives none and starts nothing.
func (t Tools) Staticcheck(root, module string, packages []string) ([]string, error) {
	dir := filepath.Join(root, filepath.FromSlash(module))
	if !isFile(filepath.Join(dir, "go.mod")) {
		return nil, nil
	}
	var present []string
	for _, pkg := range packages {
		if isDir(filepath.Join(dir, filepath.FromSlash(pkg))) {
			present = append(present, pkg)
		}
	}
	if len(present) == 0 {
		return nil, nil
	}
	return t.run("staticcheck", dir, present...)
}

// Tidy runs `go mod tidy -diff` in the module of the copy and gives the requirements its diff removes from go.mod
// (review.TidyFindings). Its finding rule is its own, since a diff is not a finding line: exit 0 is no finding, and a
// non-zero exit after printing a line that starts `--- ` is a run; anything else did not run. A module the copy has no
// go.mod for gives none and starts nothing.
func (t Tools) Tidy(root, module string) ([]string, error) {
	dir := filepath.Join(root, filepath.FromSlash(module))
	if !isFile(filepath.Join(dir, "go.mod")) {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), t.timeout)
	defer cancel()
	stdout, stderr, err := t.runner.Output(ctx, dir, t.environ, "go", "mod", "tidy", "-diff")
	if err == nil {
		return nil, nil
	}
	if ranToAnExit(ctx, err) && (bytes.HasPrefix(stdout, []byte("--- ")) || bytes.Contains(stdout, []byte("\n--- "))) {
		return review.TidyFindings(string(stdout)), nil
	}
	return nil, fmt.Errorf("%w: go mod tidy: %v%s", app.ErrToolDidNotRun, err, said(stderr))
}

// Shellcheck runs `shellcheck -f gcc -- <files>` over the files of the copy, relative to its root, that the copy has;
// the `--` makes a file named like an option a file. None left gives none and starts nothing.
func (t Tools) Shellcheck(root string, files []string) ([]string, error) {
	args := []string{"-f", "gcc", "--"}
	for _, f := range files {
		if isFile(filepath.Join(root, filepath.FromSlash(f))) {
			args = append(args, f)
		}
	}
	if len(args) == 3 {
		return nil, nil
	}
	return t.run("shellcheck", root, args...)
}

// run starts the program in dir and reads its finding lines, or says that it did not run.
func (t Tools) run(tool, dir string, args ...string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), t.timeout)
	defer cancel()
	stdout, stderr, err := t.runner.Output(ctx, dir, t.environ, tool, args...)
	var lines []string
	for _, line := range strings.Split(string(stdout), "\n") {
		if finding.MatchString(line) {
			lines = append(lines, line)
		}
	}
	if err == nil || len(lines) > 0 && ranToAnExit(ctx, err) {
		return lines, nil
	}
	return nil, fmt.Errorf("%w: %s: %v%s", app.ErrToolDidNotRun, tool, err, said(stderr))
}

// ranToAnExit says whether err, the error of a program that ran under ctx, is its exit status: it ran to an exit, and its
// time was not up. A program that was not found, could not be started, was ended by a signal or stopped at its deadline
// has no exit status to speak of.
func ranToAnExit(ctx context.Context, err error) bool {
	var exit *exec.ExitError
	return ctx.Err() == nil && errors.As(err, &exit) && exit.ExitCode() >= 0
}

// said is what a program printed on its standard error as the end of an error message, or nothing.
func said(stderr []byte) string {
	text := strings.TrimSpace(string(stderr))
	if text == "" {
		return ""
	}
	if len(text) > maxSaid {
		text = strings.ToValidUTF8(text[:maxSaid], "") + "..."
	}
	return ": " + text
}

func isFile(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.Mode().IsRegular()
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

// ExecRunner is the process adapter of Runner: it starts the program of the PATH of the process in dir under env, nothing
// of the environment of the process added, and stops it when ctx ends or when it prints more than maxOutput on a stream.
type ExecRunner struct{}

// Output implements Runner.
func (ExecRunner) Output(ctx context.Context, dir string, env []string, bin string, args ...string) (stdout, stderr []byte, err error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	cmd.Env = append([]string{}, env...) // never nil: a nil Env is the environment of the process
	cmd.WaitDelay = 2 * time.Second
	var out, errOut cappedBuffer
	out.max, errOut.max = maxOutput, maxOutput
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err = cmd.Run()
	switch {
	case out.over || errOut.over:
		err = fmt.Errorf("%s printed more than %d bytes", bin, int64(maxOutput))
	case err != nil && ctx.Err() != nil:
		err = fmt.Errorf("%w (%v)", ctx.Err(), err)
	}
	return out.buf.Bytes(), errOut.buf.Bytes(), err
}

// cappedBuffer keeps what a program prints up to max and refuses the rest, which ends the program.
type cappedBuffer struct {
	buf  bytes.Buffer
	max  int
	over bool
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if c.buf.Len()+len(p) > c.max {
		c.over = true
		return 0, errors.New("output is larger than the bound")
	}
	return c.buf.Write(p)
}
