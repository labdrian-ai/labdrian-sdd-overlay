package claudeprint_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/agentgen"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/claudeprint"
)

// fakeRunner is the Runner of the tests: it records the one start and answers with canned streams.
type fakeRunner struct {
	calls    int
	dir, bin string
	env      []string
	args     []string
	stdin    string
	deadline bool

	stdout, stderr string
	err            error
}

func (f *fakeRunner) OutputStdinIn(ctx context.Context, dir string, env []string, stdin []byte, bin string, args ...string) ([]byte, []byte, error) {
	f.calls++
	_, f.deadline = ctx.Deadline()
	f.dir, f.env, f.stdin, f.bin, f.args = dir, env, string(stdin), bin, append([]string{}, args...)
	return []byte(f.stdout), []byte(f.stderr), f.err
}

// claudeRoot is a Claude configuration directory that holds the reviewer agent file the generator writes, and the body
// of that file.
func claudeRoot(t *testing.T) (string, string) {
	t.Helper()
	artifacts, err := agentgen.Artifacts(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range artifacts {
		if a.Path != "agents/reviewer.md" {
			continue
		}
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "agents"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "agents", "reviewer.md"), []byte(a.Content), 0o644); err != nil {
			t.Fatal(err)
		}
		body, err := agentgen.ClaudeInstructions([]byte(a.Content))
		if err != nil {
			t.Fatal(err)
		}
		return root, body
	}
	t.Fatal("agentgen.Artifacts has no agents/reviewer.md")
	return "", ""
}

func TestReviewRunsClaudePrintWithNoToolsInTheWorktreeAndAnswersItsStdout(t *testing.T) {
	root, body := claudeRoot(t)
	f := &fakeRunner{stdout: `{"findings":[]}`}
	r := claudeprint.New(f, claudeprint.Options{Environ: []string{"A=1"}, ClaudeRoot: root})

	reply, err := r.Review("/work/review-abc", "claude-sonnet-5-5", []byte("the brief\n"))

	if err != nil || string(reply) != `{"findings":[]}` {
		t.Fatalf("Review = %q, %v; want the stdout of claude", reply, err)
	}
	wantArgs := []string{"--print", "--output-format", "text", "--tools", "", "--permission-mode", "dontAsk",
		"--no-session-persistence", "--setting-sources", "", "--strict-mcp-config", "--model", "claude-sonnet-5-5",
		"--system-prompt", "You are an isolated code reviewer process. Your only instructions are those delivered in the user message."}
	if f.calls != 1 || f.bin != "claude" || f.dir != "/work/review-abc" || !reflect.DeepEqual(f.args, wantArgs) {
		t.Errorf("started %d times %q in %q with %q; want once claude in the worktree with %q", f.calls, f.bin, f.dir, f.args, wantArgs)
	}
	if want := body + "\n\nthe brief\n"; f.stdin != want {
		t.Errorf("stdin = %q; want the instructions of the reviewer agent, a blank line and the brief", f.stdin)
	}
	if !reflect.DeepEqual(f.env, []string{"A=1"}) || !f.deadline {
		t.Errorf("environment %q, deadline %v; want the one of the options and a deadline", f.env, f.deadline)
	}
}

func TestReviewWithoutATimeoutRunsUnderTheDefaultOne(t *testing.T) {
	if claudeprint.DefaultTimeout != 30*time.Minute {
		t.Errorf("DefaultTimeout = %s; want 30m, as codexexec", claudeprint.DefaultTimeout)
	}
}

func TestReviewWhenClaudeDoesNotAnswer(t *testing.T) {
	for _, c := range []struct {
		name           string
		stdout, stderr string
		err            error
		want           []string
	}{
		{"an exit with an error", "partial", "boom said claude", errors.New("exit status 1"), []string{"claude", "exit status 1", "boom said claude"}},
		{"its deadline", "", "", context.DeadlineExceeded, []string{"claude", "deadline"}},
		{"an empty stdout", "", "nothing to say", nil, []string{"claude", "printed no reply", "nothing to say"}},
		{"only white space", " \n\t\n", "", nil, []string{"claude", "printed no reply"}},
		{"stderr repeated bounded", "", strings.Repeat("x", 5000), errors.New("exit status 2"), []string{strings.Repeat("x", 2000) + "..."}},
	} {
		t.Run(c.name, func(t *testing.T) {
			root, _ := claudeRoot(t)
			f := &fakeRunner{stdout: c.stdout, stderr: c.stderr, err: c.err}

			reply, err := claudeprint.New(f, claudeprint.Options{ClaudeRoot: root}).Review("/w", "m", []byte("b"))

			if reply != nil || !errors.Is(err, app.ErrReviewerDidNotAnswer) {
				t.Fatalf("Review = %q, %v; want no reply and an error wrapping ErrReviewerDidNotAnswer", reply, err)
			}
			for _, part := range c.want {
				if !strings.Contains(err.Error(), part) {
					t.Errorf("error %q; want it to name %q", err, part)
				}
			}
			if strings.Contains(err.Error(), strings.Repeat("x", 2001)) {
				t.Errorf("error repeats more than 2000 bytes of stderr")
			}
		})
	}
}

func TestReviewRefusesAMissingOrForeignAgentFileBeforeClaudeStarts(t *testing.T) {
	for _, c := range []struct {
		name    string
		content *string
		want    string
	}{
		{"missing", nil, "cannot read the reviewer agent of Claude"},
		{"foreign", ptr("---\nname: reviewer\n---\n\nbody\n"), "is not the one the overlay writes"},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			if c.content != nil {
				if err := os.MkdirAll(filepath.Join(root, "agents"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, "agents", "reviewer.md"), []byte(*c.content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			f := &fakeRunner{stdout: "reply"}

			_, err := claudeprint.New(f, claudeprint.Options{ClaudeRoot: root}).Review("/w", "m", []byte("b"))

			if err == nil || !strings.Contains(err.Error(), c.want) || !strings.Contains(err.Error(), "run: labdrian-overlay apply --target claude") || f.calls != 0 {
				t.Errorf("Review error = %v, claude calls = %d; want %q and the apply command, never started", err, f.calls, c.want)
			}
		})
	}
}

func ptr(s string) *string { return &s }
