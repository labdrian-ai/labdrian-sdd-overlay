// Package codexexec is the adapter of the Reviewer port of review/app (own-rdd T8b): it runs the reviewer through
// `codex exec --sandbox read-only`, a separate process whose sandbox, not prose, keeps the reviewer from writing.
//
// The reviewer's instructions are the developer_instructions of the Codex agent file the generator deployed
// (<codex root>/agents/reviewer.toml). The brief follows them on the standard input of codex, and the reply is the last
// message codex wrote to a file in a scratch directory that is removed when the run ends.
//
// That the installed Codex honours --sandbox read-only is not something a test of this package can prove: it is proved
// only by the live check of the feature document (own-rdd S9 f).
package codexexec

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/agentgen"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
)

// DefaultTimeout is how long one run of codex may take when the options give none.
const DefaultTimeout = 30 * time.Minute

// maxSaid bounds the part of what codex printed on its standard error that an error message repeats.
const maxSaid = 2000

// maxAgentFile bounds the agent file that is read.
const maxAgentFile = 1 << 20

// Runner starts a program with exactly the environment it is given, hands it stdin, and returns what it printed on its
// standard output and its standard error, apart. execrunner.Runner is the process adapter.
type Runner interface {
	OutputStdin(ctx context.Context, env []string, stdin []byte, bin string, args ...string) (stdout, stderr []byte, err error)
}

// Options are what the adapter needs of the process it runs in.
type Options struct {
	// Environ is the environment codex runs under, os.Environ() in production.
	Environ []string
	// Timeout is how long codex may run. Zero, or any duration that is not positive, is DefaultTimeout.
	Timeout time.Duration
	// CodexRoot is the Codex configuration directory that holds agents/reviewer.toml.
	CodexRoot string
}

// Reviewer implements app.Reviewer over a Runner.
type Reviewer struct {
	runner    Runner
	environ   []string
	timeout   time.Duration
	codexRoot string
}

var _ app.Reviewer = Reviewer{}

// New is the adapter over runner under the environment, the deadline and the Codex root of the options.
func New(runner Runner, o Options) Reviewer {
	timeout := o.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return Reviewer{runner: runner, environ: append([]string{}, o.Environ...), timeout: timeout, codexRoot: o.CodexRoot}
}

// Review implements app.Reviewer. Everything that fails after the instructions are read wraps
// app.ErrReviewerDidNotAnswer, names codex and repeats what codex printed on its standard error.
func (r Reviewer) Review(worktree, model string, brief []byte) ([]byte, error) {
	instructions, err := r.instructions()
	if err != nil {
		return nil, err
	}
	scratch, err := os.MkdirTemp("", "labdrian-review-launch-")
	if err != nil {
		return nil, fmt.Errorf("%w: codex: cannot make its scratch directory: %v", app.ErrReviewerDidNotAnswer, err)
	}
	defer os.RemoveAll(scratch)
	last := filepath.Join(scratch, "last-message.txt")

	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	stdin := []byte(instructions + "\n\n" + string(brief))
	_, stderr, err := r.runner.OutputStdin(ctx, r.environ, stdin, "codex",
		"exec", "--sandbox", "read-only", "--model", model, "--cd", worktree, "--skip-git-repo-check", "--ephemeral",
		"--ignore-user-config", "--output-last-message", last, "-")
	if err != nil {
		if ctx.Err() != nil {
			err = fmt.Errorf("stopped at its deadline of %s: %w", r.timeout, err)
		}
		return nil, fmt.Errorf("%w: codex: %v%s", app.ErrReviewerDidNotAnswer, err, said(stderr))
	}
	reply, err := readLimited(last, review.MaxReplyBytes+1)
	if err != nil {
		return nil, fmt.Errorf("%w: codex left no last message: %v%s", app.ErrReviewerDidNotAnswer, err, said(stderr))
	}
	if len(reply) == 0 {
		return nil, fmt.Errorf("%w: codex left an empty last message%s", app.ErrReviewerDidNotAnswer, said(stderr))
	}
	return reply, nil
}

// instructions are the developer_instructions of the deployed reviewer agent file of Codex.
func (r Reviewer) instructions() (string, error) {
	path := filepath.Join(r.codexRoot, "agents", "reviewer.toml")
	data, err := readLimited(path, maxAgentFile+1)
	if err != nil {
		return "", fmt.Errorf("cannot read the reviewer agent of Codex %s: %v; run: labdrian-overlay apply --target codex", path, err)
	}
	if len(data) > maxAgentFile {
		return "", fmt.Errorf("the reviewer agent of Codex %s is over %d bytes; run: labdrian-overlay apply --target codex", path, maxAgentFile)
	}
	text, err := agentgen.CodexInstructions(data)
	if err != nil {
		return "", fmt.Errorf("the reviewer agent of Codex %s is not the one the overlay writes: %v; run: labdrian-overlay apply --target codex", path, err)
	}
	return text, nil
}

// readLimited reads at most limit bytes of the file.
func readLimited(path string, limit int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, int64(limit)))
	if err != nil {
		return nil, err
	}
	return data, nil
}

// said is what codex printed on its standard error as the end of an error message, or nothing.
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
