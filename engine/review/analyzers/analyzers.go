// Package analyzers is the adapter of the Analyzers port of review/app: it runs deadcode, staticcheck, errcheck, nilerr,
// shellcheck and `go mod tidy -diff`, the programs of the second check of `labdrian review sweep` (own-rdd T6b3, S6 b;
// phase9-c25-review-hunt T5, S4), on a copy of a tree and returns the lines they print in their finding format.
//
// A finding line is `<path>:<line>[:<col>]: <text>`, which is what deadcode (`file:line:col: unreachable func: Name`),
// staticcheck (`file:line:col: message (SA1000)`), nilerr and shellcheck in its gcc format (`-f gcc`) print; errcheck
// prints a tab where the others print ": ", and its lines are given the shape first. Only a line of that shape counts as
// a finding; the rest of what a program prints is not one.
//
// A program that did not run is told apart from one that found something by what it printed, not by its exit status, whose
// convention differs between them and is not promised (nilerr, whose errors have the finding shape, is the exception:
// see Nilerr): a program that exits zero ran, one that exits non-zero ran
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

// errcheckLine is the position at the start of a line errcheck prints, with the tab that follows it.
var errcheckLine = regexp.MustCompile(`(?m)^([^:\t\n]+:\d+:\d+):\t`)

// errcheckExcludes are the functions errcheck does not report, one per line: the writes of fmt to a writer, which the
// engine makes for every line of its usage and messages and never checks (685 of the 742 lines errcheck v1.30.0 printed
// over engine/cmd on 2026-11-01), so that a changed usage line is no finding of the sweep.
const errcheckExcludes = "fmt.Fprint\nfmt.Fprintf\nfmt.Fprintln\n"

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
// starts nothing: the module is new at the other side of the sweep. A module with no main package and no test, which
// deadcode refuses with exit 1 and only `deadcode: no main packages` on its standard error, gives none: nothing in it
// is reachable from a root, so there is nothing to report.
func (t Tools) Deadcode(root, module string) ([]string, error) {
	dir := filepath.Join(root, filepath.FromSlash(module))
	if !isFile(filepath.Join(dir, "go.mod")) {
		return nil, nil
	}
	lines, err := t.run("deadcode", dir, "-test", "./...")
	if errors.Is(err, app.ErrToolDidNotRun) && strings.HasSuffix(err.Error(), ": deadcode: no main packages") {
		return nil, nil
	}
	return lines, err
}

// Staticcheck runs staticcheck over the packages of the module of the copy that the copy has: a package is a pattern
// "." or "./dir", kept when its directory is there. None left, or no module, gives none and starts nothing.
func (t Tools) Staticcheck(root, module string, packages []string) ([]string, error) {
	dir, present := presentPackages(root, module, packages)
	if len(present) == 0 {
		return nil, nil
	}
	return t.run("staticcheck", dir, present...)
}

// Errcheck runs `errcheck -exclude <file>` over the packages of the module of the copy that the copy has, as Staticcheck
// does, the file naming errcheckExcludes and removed when errcheck ends. errcheck prints `file:line:col:<tab>source`;
// each line is given the finding shape `file:line:col: unchecked error: source`.
func (t Tools) Errcheck(root, module string, packages []string) ([]string, error) {
	dir, present := presentPackages(root, module, packages)
	if len(present) == 0 {
		return nil, nil
	}
	exclude, err := excludeFile()
	if err != nil {
		return nil, fmt.Errorf("%w: errcheck: write its exclude file: %v", app.ErrToolDidNotRun, err)
	}
	defer func() { _ = os.Remove(exclude) }() // best effort: a file left in the temporary directory is harmless
	return t.read("errcheck", dir, append([]string{"-exclude", exclude}, present...), func(stdout, _ []byte) []byte {
		return errcheckLine.ReplaceAll(stdout, []byte("$1: unchecked error: "))
	})
}

// excludeFile writes errcheckExcludes to a new file of the temporary directory and gives its path.
func excludeFile() (string, error) {
	f, err := os.CreateTemp("", "errcheck-exclude-*")
	if err != nil {
		return "", err
	}
	_, err = f.WriteString(errcheckExcludes)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// Nilerr runs nilerr over the packages of the module of the copy that the copy has, as Staticcheck does. nilerr is an
// analysis driver, so its rule is its own: it prints its findings on its standard error, and exits 3 when it reports
// and 1 when a package does not load, printing the errors of the compiler, which have the finding shape. Exit 0 is no
// finding, exit 3 with a finding line is a run, and anything else did not run.
func (t Tools) Nilerr(root, module string, packages []string) ([]string, error) {
	dir, present := presentPackages(root, module, packages)
	if len(present) == 0 {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), t.timeout)
	defer cancel()
	_, stderr, err := t.runner.Output(ctx, dir, t.environ, "nilerr", present...)
	if err == nil {
		return nil, nil
	}
	var exit *exec.ExitError
	if lines := findingLines(stderr); len(lines) > 0 && ranToAnExit(ctx, err) && errors.As(err, &exit) && exit.ExitCode() == 3 {
		return lines, nil
	}
	return nil, fmt.Errorf("%w: nilerr: %v%s", app.ErrToolDidNotRun, err, said(stderr))
}

// presentPackages is the directory of the module of the copy and the packages, patterns "." or "./dir", whose directory
// the copy has. A module the copy has no go.mod for has none: the module is new at the other side of the sweep.
func presentPackages(root, module string, packages []string) (string, []string) {
	dir := filepath.Join(root, filepath.FromSlash(module))
	if !isFile(filepath.Join(dir, "go.mod")) {
		return dir, nil
	}
	var present []string
	for _, pkg := range packages {
		if isDir(filepath.Join(dir, filepath.FromSlash(pkg))) {
			present = append(present, pkg)
		}
	}
	return dir, present
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

// run starts the program in dir and reads its finding lines from its standard output, or says that it did not run.
func (t Tools) run(tool, dir string, args ...string) ([]string, error) {
	return t.read(tool, dir, args, func(stdout, _ []byte) []byte { return stdout })
}

// read starts the program in dir and reads its finding lines from the text that text makes of what it printed, or says
// that it did not run.
func (t Tools) read(tool, dir string, args []string, text func(stdout, stderr []byte) []byte) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), t.timeout)
	defer cancel()
	stdout, stderr, err := t.runner.Output(ctx, dir, t.environ, tool, args...)
	lines := findingLines(text(stdout, stderr))
	if err == nil || len(lines) > 0 && ranToAnExit(ctx, err) {
		return lines, nil
	}
	return nil, fmt.Errorf("%w: %s: %v%s", app.ErrToolDidNotRun, tool, err, said(stderr))
}

// findingLines are the lines of text that have the finding shape, in order.
func findingLines(text []byte) []string {
	var lines []string
	for _, line := range strings.Split(string(text), "\n") {
		if finding.MatchString(line) {
			lines = append(lines, line)
		}
	}
	return lines
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
