// Package evidencesh is the adapter of the Classified port of review/app (S6, T4w): it asks the deployed evidence store,
// `sh <script> reviewed-each <root>/docs/wiki <review>...`, whether the evidence page of a working tree holds an Evidence
// row for each of the stored results, through a Runner (execrunner), so the program that starts it stays the one place that
// starts programs of the machine. It runs one `sh` for each call and splits no call: past the limits of one exec (the
// header of evidence.sh names them) the call is an error. Nothing it is told is remembered. Its bound, the timeout, holds
// with every process the run started when the Runner kills the run's process group at the deadline, as the Runner of the
// stop hook does (execrunner.WithProcessGroup, on Unix); over a Runner that kills only sh, a subshell that holds the pipes can
// keep the call up to the Runner's own grace.
//
// What each run means, the first row that applies:
//   - no reviews: nothing runs, the answer is empty;
//   - the deadline passed: an error wrapping context.DeadlineExceeded;
//   - the exec failed, whatever the cause (no sh, a vector past ARG_MAX, an output past the runner's bound): an error
//     wrapping it;
//   - sh ended by a signal, or exited with a status other than 0 (the script's 2 for a refused identifier, 3 for no awk, 4
//     for a page it cannot answer about, sh's own 126 and 127, any other): an error naming the status and the first line of
//     stderr, never an answer;
//   - exit 0: exactly one line `yes` or `no` for each review, each ended by one line feed, and nothing else; any other
//     output is an error that does not quote it.
package evidencesh

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
)

// DefaultTimeout is how long one run may take when Options.Timeout says no limit. It is an assumption of L55 that the
// timed stop of T4w measured: one run is `sh`, one `awk` and one read of one page. The run is stopped at it with every
// process it started when the Runner kills the process group, and with sh alone otherwise (the package doc).
const DefaultTimeout = 2 * time.Second

// Runner starts a program with exactly the environment it is given and returns what it printed on its standard output and
// on its standard error, apart. The process adapter is execrunner.Runner.
type Runner interface {
	Output(ctx context.Context, env []string, bin string, args ...string) (stdout, stderr []byte, err error)
}

// Options are what the adapter needs of the process it runs in.
type Options struct {
	// Script is the absolute path of the deployed evidence.sh.
	Script string
	// Environ is the whole environment of the run, as execrunner takes it: sh finds awk and dirname on its PATH. A nil one
	// is the environment of the process.
	Environ []string
	// Timeout is how long one run may take. Zero, or any duration that is not positive, is DefaultTimeout.
	Timeout time.Duration
}

// Script implements app.Classified over a Runner. The zero value has no Runner and cannot be used; New makes one.
type Script struct {
	runner  Runner
	script  string
	environ []string
	timeout time.Duration
}

var _ app.Classified = Script{}

// New is the adapter over runner, told of the process by o. The environment is copied.
func New(runner Runner, o Options) Script {
	timeout := o.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return Script{runner: runner, script: o.Script, environ: slices.Clone(o.Environ), timeout: timeout}
}

// Classified says, for each of reviews, whether the evidence page at <root>/docs/wiki holds an Evidence row with that
// Review (app.Classified). The reviews reach the script in order as arguments of one run. The package doc says what each
// outcome is; the exit table of the script is that of `reviewed-each`: 0 with one line for each review, 2 for a usage
// error, 3 for no awk, 4 for a page it cannot answer about, and never an answer for any of them.
func (s Script) Classified(root string, reviews []string) ([]bool, error) {
	if len(reviews) == 0 {
		return []bool{}, nil
	}
	if !filepath.IsAbs(s.script) {
		return nil, fmt.Errorf("the evidence script %q is not an absolute path", s.script)
	}
	run := "sh " + s.script + " reviewed-each"
	args := append([]string{s.script, "reviewed-each", filepath.Join(root, "docs", "wiki")}, reviews...)
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()
	stdout, stderr, err := s.runner.Output(ctx, s.environ, "sh", args...)
	if err != nil {
		return nil, s.refusal(run, stderr, err)
	}
	return answers(run, stdout, len(reviews))
}

// refusal is the error of a run that failed, as the package doc words it. said is the first line of stderr whole, its
// bytes before the first line feed, nothing trimmed; the hook folds and escapes it.
func (s Script) refusal(run string, stderr []byte, err error) error {
	said := ""
	if len(stderr) > 0 {
		first, _, _ := strings.Cut(string(stderr), "\n")
		said = ": " + first
	}
	var exit *exec.ExitError
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("%s gave no answer within %v: %w", run, s.timeout, err)
	case !errors.As(err, &exit):
		return fmt.Errorf("%s could not run: %w", run, err)
	case exit.ExitCode() == -1:
		return fmt.Errorf("%s was stopped by a signal (%v)%s", run, exit, said)
	}
	return fmt.Errorf("%s exited %d%s", run, exit.ExitCode(), said)
}

// answers reads the stdout of a run that exited 0: n lines, each yes or no and each ended by one line feed.
func answers(run string, stdout []byte, n int) ([]bool, error) {
	refused := fmt.Errorf("%s exited 0 without one line yes or no for each of the %d results", run, n)
	text, ended := strings.CutSuffix(string(stdout), "\n")
	if !ended {
		return nil, refused
	}
	lines := strings.Split(text, "\n")
	if len(lines) != n {
		return nil, refused
	}
	out := make([]bool, n)
	for i, line := range lines {
		switch line {
		case "yes":
			out[i] = true
		case "no":
		default:
			return nil, refused
		}
	}
	return out, nil
}
