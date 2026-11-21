package main

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// fakeCodegraph is the runner of the codegraph-prompt-hook verb: it records what it was asked to start
// and answers what the test gave it, so no test starts the real codegraph.
type fakeCodegraph struct {
	calls  []fakeCodegraphCall
	stdout string
	stderr string
	err    error
}

type fakeCodegraphCall struct {
	env   []string
	stdin string
	bin   string
	args  []string
}

func (f *fakeCodegraph) OutputStdin(_ context.Context, env []string, stdin []byte, bin string, args ...string) ([]byte, []byte, error) {
	f.calls = append(f.calls, fakeCodegraphCall{env: env, stdin: string(stdin), bin: bin, args: args})
	return []byte(f.stdout), []byte(f.stderr), f.err
}

// exitCodeError is what execrunner returns for a program that exits non-zero: an error that knows its code.
type exitCodeError int

func (e exitCodeError) Error() string { return "exit status" }
func (e exitCodeError) ExitCode() int { return int(e) }

func promptFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "codegraph_prompt_hook", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func runCodegraphHook(stdin string, f *fakeCodegraph) *capturedProcess {
	p := newCapturedProcess()
	p.stdin = strings.NewReader(stdin)
	runCodegraphPromptHook(p.process, f, nil)
	return p
}

func TestCodegraphPromptHookSkipsATaskNotification(t *testing.T) {
	for _, name := range []string{"task_notification.json", "system_notification.json"} {
		t.Run(name, func(t *testing.T) {
			f := &fakeCodegraph{stdout: "codegraph output", err: exitCodeError(3)}
			p := runCodegraphHook(promptFixture(t, name), f)
			if len(f.calls) != 0 {
				t.Errorf("codegraph was started %d times, want never", len(f.calls))
			}
			if p.out.String() != "" || p.err.String() != "" || len(p.exits) != 0 {
				t.Errorf("stdout = %q, stderr = %q, exits = %v; a notification prints nothing and exits 0", p.out.String(), p.err.String(), p.exits)
			}
		})
	}
}

func TestCodegraphPromptHookPassesAnOrdinaryPromptThrough(t *testing.T) {
	in := promptFixture(t, "ordinary_prompt.json")
	f := &fakeCodegraph{stdout: "preview\n", stderr: "warn\n"}
	p := runCodegraphHook(in, f)
	if len(f.calls) != 1 {
		t.Fatalf("codegraph was started %d times, want once", len(f.calls))
	}
	c := f.calls[0]
	if c.bin != "codegraph" || strings.Join(c.args, " ") != "prompt-hook" {
		t.Errorf("started %q %q, want codegraph prompt-hook", c.bin, c.args)
	}
	if c.stdin != in {
		t.Errorf("codegraph read %q, want the same bytes as the hook input %q", c.stdin, in)
	}
	if c.env != nil {
		t.Errorf("env = %q, want nil (the environment of the process)", c.env)
	}
	if p.out.String() != "preview\n" || p.err.String() != "warn\n" || len(p.exits) != 0 {
		t.Errorf("stdout = %q, stderr = %q, exits = %v; want codegraph's output and exit 0", p.out.String(), p.err.String(), p.exits)
	}
}

func TestCodegraphPromptHookPassesInputThatIsNotAPromptThrough(t *testing.T) {
	long := `{"prompt":"` + strings.Repeat("x", 2<<20) + `"}`
	for name, in := range map[string]string{
		"not JSON":     "this is not json",
		"empty":        "",
		"no prompt":    `{"hook_event_name":"UserPromptSubmit"}`,
		"non-text":     `{"prompt":{"a":1}}`,
		"over the cap": long,
	} {
		t.Run(name, func(t *testing.T) {
			f := &fakeCodegraph{stdout: "out"}
			p := runCodegraphHook(in, f)
			if len(f.calls) != 1 || f.calls[0].stdin != in {
				t.Fatalf("calls = %d; codegraph must run once on the same bytes", len(f.calls))
			}
			if p.out.String() != "out" || len(p.exits) != 0 {
				t.Errorf("stdout = %q, exits = %v", p.out.String(), p.exits)
			}
		})
	}
}

func TestCodegraphPromptHookRelaysANonZeroExitAndBothStreams(t *testing.T) {
	f := &fakeCodegraph{stdout: "partial", stderr: "boom\n", err: exitCodeError(2)}
	p := runCodegraphHook(promptFixture(t, "ordinary_prompt.json"), f)
	if p.out.String() != "partial" || p.err.String() != "boom\n" {
		t.Errorf("stdout = %q, stderr = %q, want codegraph's", p.out.String(), p.err.String())
	}
	if len(p.exits) != 1 || p.exits[0] != 2 {
		t.Errorf("exits = %v, want [2]", p.exits)
	}
}

func TestCodegraphPromptHookExitsLikeAShellWhenCodegraphCannotBeStarted(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code int
	}{
		{"not found", errors.New(`exec: "codegraph": executable file not found in $PATH`), 127},
		{"not executable", &fs.PathError{Op: "fork/exec", Path: "codegraph", Err: fs.ErrPermission}, 126},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := runCodegraphHook(promptFixture(t, "ordinary_prompt.json"), &fakeCodegraph{err: c.err})
			if p.out.String() != "" {
				t.Errorf("stdout = %q, want nothing", p.out.String())
			}
			if n := strings.Count(p.err.String(), "\n"); n != 1 || !strings.HasPrefix(p.err.String(), "codegraph: ") {
				t.Errorf("stderr = %q, want one line starting with \"codegraph: \"", p.err.String())
			}
			if len(p.exits) != 1 || p.exits[0] != c.code {
				t.Errorf("exits = %v, want [%d]", p.exits, c.code)
			}
		})
	}
}

func TestCodegraphPromptHookSaysWhenCodegraphPrintsTooMuch(t *testing.T) {
	f := &fakeCodegraph{stdout: "cut", err: errCodegraphTooMuch}
	p := runCodegraphHook(promptFixture(t, "ordinary_prompt.json"), f)
	if p.out.String() != "cut" || !strings.HasPrefix(p.err.String(), "codegraph: ") || len(p.exits) != 1 || p.exits[0] != 1 {
		t.Errorf("stdout = %q, stderr = %q, exits = %v; want the held output, one codegraph line, exit 1", p.out.String(), p.err.String(), p.exits)
	}
}

// signalError is what execrunner returns for a program a signal ended: no exit code of its own, and the
// wait status that names the signal.
type signalError struct{ sig syscall.Signal }

func (signalError) Error() string { return "signal: terminated" }
func (signalError) ExitCode() int { return -1 }
func (e signalError) Sys() any    { return syscall.WaitStatus(e.sig) }

func TestCodegraphPromptHookExitsLikeAShellForASignal(t *testing.T) {
	for sig, want := range map[syscall.Signal]int{syscall.SIGTERM: 143, syscall.SIGKILL: 137} {
		p := runCodegraphHook(promptFixture(t, "ordinary_prompt.json"), &fakeCodegraph{err: signalError{sig}})
		if len(p.exits) != 1 || p.exits[0] != want {
			t.Errorf("signal %d: exits = %v, want [%d]", sig, p.exits, want)
		}
	}
}

func TestCodegraphPromptHookReportsTooMuchOutputBeforeAnExitCode(t *testing.T) {
	f := &fakeCodegraph{stdout: "cut", err: errors.Join(exitCodeError(2), errCodegraphTooMuch)}
	p := runCodegraphHook(promptFixture(t, "ordinary_prompt.json"), f)
	if !strings.HasPrefix(p.err.String(), "codegraph: ") || len(p.exits) != 1 || p.exits[0] != 1 {
		t.Errorf("stderr = %q, exits = %v; want one codegraph line naming the output bound and exit 1", p.err.String(), p.exits)
	}
}

func TestCodegraphPromptHookPassesAPromptThatQuotesTheNotificationMarkThrough(t *testing.T) {
	in := `{"hook_event_name":"UserPromptSubmit","prompt":"why does [SYSTEM NOTIFICATION - NOT USER INPUT] show up in my logs?"}`
	f := &fakeCodegraph{stdout: "out"}
	p := runCodegraphHook(in, f)
	if len(f.calls) != 1 || p.out.String() != "out" {
		t.Errorf("calls = %d, stdout = %q; a user prompt that quotes the mark must reach codegraph", len(f.calls), p.out.String())
	}
}

func TestCodegraphPromptHookIsDispatchedAndDocumented(t *testing.T) {
	p := newCapturedProcess()
	p.stdin = strings.NewReader(promptFixture(t, "task_notification.json"))
	run(p.process, testDeps(), []string{"codegraph-prompt-hook"})
	if p.out.String() != "" || p.err.String() != "" || len(p.exits) != 0 {
		t.Errorf("through run: stdout = %q, stderr = %q, exits = %v; a notification prints nothing", p.out.String(), p.err.String(), p.exits)
	}
	if !strings.Contains(captureUsage(t), "engine codegraph-prompt-hook") {
		t.Error("usage does not list engine codegraph-prompt-hook")
	}
}
