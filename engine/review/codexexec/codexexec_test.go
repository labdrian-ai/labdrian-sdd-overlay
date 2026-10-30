package codexexec_test

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
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/piguard"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/codexexec"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/testhome"
)

func TestMain(m *testing.M) { os.Exit(testhome.Run(m)) }

// fakeRunner is the Runner of the tests: it starts nothing. It records what it was asked to start, and writes a canned
// reply to the path that follows --output-last-message, as codex does.
type fakeRunner struct {
	calls int
	bin   string
	args  []string
	env   []string
	stdin []byte

	reply      *string // the content of the last-message file, or none when nil
	stderr     string
	err        error
	untilDone  bool // wait for the deadline instead of answering
	lastPath   string
	lastExists func() bool
}

func (f *fakeRunner) OutputStdin(ctx context.Context, env []string, stdin []byte, bin string, args ...string) ([]byte, []byte, error) {
	f.calls++
	f.bin, f.args, f.env, f.stdin = bin, append([]string{}, args...), append([]string{}, env...), append([]byte{}, stdin...)
	for i, a := range args {
		if a == "--output-last-message" && i+1 < len(args) {
			f.lastPath = args[i+1]
		}
	}
	if f.untilDone {
		<-ctx.Done()
		return nil, []byte(f.stderr), ctx.Err()
	}
	if f.reply != nil {
		if err := os.WriteFile(f.lastPath, []byte(*f.reply), 0o644); err != nil {
			return nil, nil, err
		}
	}
	return nil, []byte(f.stderr), f.err
}

func str(s string) *string { return &s }

// codexRoot is a Codex root whose agents/reviewer.toml is the file the generator writes for this checkout, and the
// instructions that file holds.
func codexRoot(t *testing.T) (root, instructions string) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(wd, "..", "..", "..")
	artifacts, err := agentgen.Artifacts(repo)
	if err != nil {
		t.Fatal(err)
	}
	root = t.TempDir()
	for _, a := range artifacts {
		if a.Path != "codex/agents/reviewer.toml" {
			continue
		}
		if err := os.MkdirAll(filepath.Join(root, "agents"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "agents", "reviewer.toml"), []byte(a.Content), 0o644); err != nil {
			t.Fatal(err)
		}
		instructions, err = agentgen.CodexInstructions([]byte(a.Content))
		if err != nil {
			t.Fatal(err)
		}
		return root, instructions
	}
	t.Fatal("agentgen.Artifacts has no codex/agents/reviewer.toml")
	return "", ""
}

func reviewer(f *fakeRunner, root string, timeout time.Duration) codexexec.Reviewer {
	return codexexec.New(f, codexexec.Options{Environ: []string{"A=1", "B=2"}, Timeout: timeout, CodexRoot: root})
}

func TestReviewStartsCodexExecReadOnlyWithTheBriefOnStdin(t *testing.T) {
	root, instructions := codexRoot(t)
	f := &fakeRunner{reply: str(`{"findings":[]}`)}
	if _, err := reviewer(f, root, 0).Review("/work/tree", "gpt-x", []byte("the brief\n")); err != nil {
		t.Fatal(err)
	}
	if f.calls != 1 || f.bin != "codex" {
		t.Fatalf("started %d times, bin %q; want once, codex", f.calls, f.bin)
	}
	want := []string{"exec", "--sandbox", "read-only", "--model", "gpt-x", "--cd", "/work/tree", "--skip-git-repo-check",
		"--ephemeral", "--ignore-user-config", "--output-last-message", f.lastPath, "-"}
	if !reflect.DeepEqual(f.args, want) {
		t.Errorf("args = %q, want %q", f.args, want)
	}
	if filepath.Base(f.lastPath) != "last-message.txt" || !strings.HasPrefix(filepath.Base(filepath.Dir(f.lastPath)), "labdrian-review-launch-") {
		t.Errorf("last-message path %q, want <scratch>/last-message.txt in a labdrian-review-launch- directory", f.lastPath)
	}
	if !reflect.DeepEqual(f.env, []string{"A=1", "B=2"}) {
		t.Errorf("env = %q, want the environment of the options", f.env)
	}
	if got, want := string(f.stdin), instructions+"\n\nthe brief\n"; got != want {
		t.Errorf("stdin = %q, want the instructions, a blank line and the brief: %q", got, want)
	}
}

func TestReviewAnswersTheLastMessage(t *testing.T) {
	root, _ := codexRoot(t)
	f := &fakeRunner{reply: str("{\"findings\": []}\nwith bytes \xff as they are")}
	got, err := reviewer(f, root, 0).Review("/w", "m", []byte("b"))
	if err != nil || string(got) != *f.reply {
		t.Errorf("Review = %q, %v; want the bytes of the file", got, err)
	}
}

func TestReviewSaysCodexDidNotAnswer(t *testing.T) {
	root, _ := codexRoot(t)
	for _, c := range []struct {
		name string
		f    *fakeRunner
		to   time.Duration
	}{
		{"an exit error and no file", &fakeRunner{err: errors.New("exit status 3"), stderr: "boom said codex"}, 0},
		{"exit 0 and an empty file", &fakeRunner{reply: str(""), stderr: "boom said codex"}, 0},
		{"exit 0 and no file", &fakeRunner{stderr: "boom said codex"}, 0},
		{"the deadline", &fakeRunner{untilDone: true, stderr: "boom said codex"}, 20 * time.Millisecond},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := reviewer(c.f, root, c.to).Review("/w", "m", []byte("b"))
			if got != nil || err == nil {
				t.Fatalf("Review = %q, %v; want an error", got, err)
			}
			if !errors.Is(err, app.ErrReviewerDidNotAnswer) {
				t.Errorf("error %v does not wrap ErrReviewerDidNotAnswer", err)
			}
			for _, want := range []string{"codex", "boom said codex"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
		})
	}
}

func TestReviewNamesTheMissingReviewerAndHowToDeployIt(t *testing.T) {
	root := t.TempDir()
	f := &fakeRunner{reply: str("x")}
	_, err := reviewer(f, root, 0).Review("/w", "m", []byte("b"))
	if err == nil {
		t.Fatal("Review gave no error without a reviewer.toml")
	}
	for _, want := range []string{filepath.Join(root, "agents", "reviewer.toml"), "run: labdrian-overlay apply --target codex"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
	if f.calls != 0 {
		t.Errorf("the runner was called %d times without instructions", f.calls)
	}
}

func TestReviewNamesAReviewerTheGeneratorDidNotWrite(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "agents", "reviewer.toml"), []byte("name = \"reviewer\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := &fakeRunner{reply: str("x")}
	_, err := reviewer(f, root, 0).Review("/w", "m", []byte("b"))
	if err == nil || !strings.Contains(err.Error(), "run: labdrian-overlay apply --target codex") || f.calls != 0 {
		t.Errorf("Review = %v after %d calls; want the deploy hint and no start", err, f.calls)
	}
}

func TestReviewRemovesItsScratch(t *testing.T) {
	root, _ := codexRoot(t)
	for name, f := range map[string]*fakeRunner{
		"a reply":    {reply: str("x")},
		"an error":   {err: errors.New("exit status 1")},
		"no file":    {},
		"an empty":   {reply: str("")},
		"a deadline": {untilDone: true},
	} {
		t.Run(name, func(t *testing.T) {
			to := time.Duration(0)
			if f.untilDone {
				to = 20 * time.Millisecond
			}
			reviewer(f, root, to).Review("/w", "m", []byte("b"))
			if f.lastPath == "" {
				t.Fatal("codex was not started")
			}
			if _, err := os.Stat(filepath.Dir(f.lastPath)); !os.IsNotExist(err) {
				t.Errorf("the scratch directory %s is still there (%v)", filepath.Dir(f.lastPath), err)
			}
		})
	}
}

func TestTheTestsRunInAHomeOfTheirOwn(t *testing.T) { testhome.Check(t) }

func TestNoTestOfThisPackageStartsCodex(t *testing.T) { piguard.CheckTestSources(t, ".") }
