// Package claudeprint is the adapter of the Reviewer port of review/app for Claude Code (C25 T4, S3 b): it runs the
// reviewer as one `claude --print` process with no tools, over the instructions of the deployed reviewer agent and the
// brief on its standard input, and answers what it printed on its standard output.
//
// The flags are the ones gentle-ai runs its lens processes with: --tools "" leaves the reviewer no tool, so the brief,
// and the frozen context review/app appends to it, is the whole of what it can inspect; --setting-sources "" loads no
// settings source, the user's or the worktree's; --strict-mcp-config, with no --mcp-config, starts no MCP server, so
// neither the user's servers nor a .mcp.json of the candidate offer it a tool; --no-session-persistence leaves no
// session behind.
// That the installed claude honours them is not something a test of this package can prove: it is proved only by the
// live check of the feature document (phase9-c25-review-hunt T4).
package claudeprint

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/agentgen"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
)

// DefaultTimeout is how long one run of claude may take when the options give none.
const DefaultTimeout = 30 * time.Minute

// maxSaid bounds the part of what claude printed on its standard error that an error message repeats.
const maxSaid = 2000

// maxAgentFile bounds the agent file that is read.
const maxAgentFile = 1 << 20

// systemPrompt replaces the system prompt of Claude Code, so that the instructions on the standard input are the only ones.
const systemPrompt = "You are an isolated code reviewer process. Your only instructions are those delivered in the user message."

// Runner starts a program in a directory with exactly the environment it is given, hands it stdin, and returns what it
// printed on its standard output and its standard error, apart. execrunner.Runner is the process adapter.
type Runner interface {
	OutputStdinIn(ctx context.Context, dir string, env []string, stdin []byte, bin string, args ...string) (stdout, stderr []byte, err error)
}

// Options are what the adapter needs of the process it runs in.
type Options struct {
	// Environ is the environment claude runs under, os.Environ() in production.
	Environ []string
	// Timeout is how long claude may run. Zero, or any duration that is not positive, is DefaultTimeout.
	Timeout time.Duration
	// ClaudeRoot is the Claude configuration directory that holds agents/reviewer.md.
	ClaudeRoot string
}

// Reviewer implements app.Reviewer over a Runner.
type Reviewer struct {
	runner     Runner
	environ    []string
	timeout    time.Duration
	claudeRoot string
}

var _ app.Reviewer = Reviewer{}

// New is the adapter over runner under the environment, the deadline and the Claude root of the options.
func New(runner Runner, o Options) Reviewer {
	timeout := o.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return Reviewer{runner: runner, environ: append([]string{}, o.Environ...), timeout: timeout, claudeRoot: o.ClaudeRoot}
}

// Review implements app.Reviewer: claude runs in worktree with model. Everything that fails after the instructions are
// read wraps app.ErrReviewerDidNotAnswer, names claude and repeats what claude printed on its standard error; a standard
// output with nothing but white space is no answer.
func (r Reviewer) Review(worktree, model string, brief []byte) ([]byte, error) {
	instructions, err := r.instructions()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()
	stdin := []byte(instructions + "\n\n" + string(brief))
	stdout, stderr, err := r.runner.OutputStdinIn(ctx, worktree, r.environ, stdin, "claude",
		"--print", "--output-format", "text", "--tools", "", "--permission-mode", "dontAsk", "--no-session-persistence",
		"--setting-sources", "", "--strict-mcp-config", "--model", model, "--system-prompt", systemPrompt)
	if err != nil {
		if ctx.Err() != nil {
			err = fmt.Errorf("stopped at its deadline of %s: %w", r.timeout, err)
		}
		return nil, fmt.Errorf("%w: claude: %v%s", app.ErrReviewerDidNotAnswer, err, said(stderr))
	}
	if strings.TrimSpace(string(stdout)) == "" {
		return nil, fmt.Errorf("%w: claude printed no reply%s", app.ErrReviewerDidNotAnswer, said(stderr))
	}
	return stdout, nil
}

// instructions are the body of the deployed reviewer agent file of Claude Code.
func (r Reviewer) instructions() (string, error) {
	path := filepath.Join(r.claudeRoot, "agents", "reviewer.md")
	data, err := readLimited(path, maxAgentFile+1)
	if err != nil {
		return "", fmt.Errorf("cannot read the reviewer agent of Claude %s: %v; run: labdrian-overlay apply --target claude", path, err)
	}
	if len(data) > maxAgentFile {
		return "", fmt.Errorf("the reviewer agent of Claude %s is over %d bytes; run: labdrian-overlay apply --target claude", path, maxAgentFile)
	}
	text, err := agentgen.ClaudeInstructions(data)
	if err != nil {
		return "", fmt.Errorf("the reviewer agent of Claude %s is not the one the overlay writes: %v; run: labdrian-overlay apply --target claude", path, err)
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
	return io.ReadAll(io.LimitReader(f, int64(limit)))
}

// said is what claude printed on its standard error as the end of an error message, or nothing.
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
