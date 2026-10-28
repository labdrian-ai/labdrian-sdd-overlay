package main

// Tests for 'roles builder-guard' (own-rdd S14 d-f), the Claude Code PreToolUse hook that denies launching
// the builder on a task the shaper did not design. The decision is engine/oddtask's and is tested there;
// these tests pin the hook's contract with Claude Code through the public verb: what it prints, its exit
// code, which agents it leaves alone, and that a builder launch it cannot check is denied. Everything runs in
// process on injected streams, in temporary repositories.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/hookwire"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/oddtask"
)

const bgDoc = "## Tasks\n\n- [ ] T1 — S14 — delegated — designed — shaped: L5 — commit: -\n- [ ] T2 — S14 — delegated — not designed — commit: -\n\n" +
	"## Log\n\n- **L5** (2026-10-28, shaper): design\n"

// bgRepo is a repository whose odd/tasks/f.md is bgDoc; it returns its root.
func bgRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "odd", "tasks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "odd", "tasks", "f.md"), []byte(bgDoc), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// bgCall is the hook input of an Agent call.
func bgCall(t *testing.T, cwd, agent, prompt string) string {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"session_id":      "s-1",
		"cwd":             cwd,
		"hook_event_name": "PreToolUse",
		"tool_name":       "Agent",
		"tool_input":      map[string]any{"description": "d", "subagent_type": agent, "prompt": prompt},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func bgRun(stdin string, args ...string) guardRun {
	var out, errBuf bytes.Buffer
	var codes []int
	runRolesCore(append([]string{"builder-guard"}, args...), strings.NewReader(stdin), &out, &errBuf, func(c int) { codes = append(codes, c) }, nil)
	return guardRun{codes: codes, stdout: out.String(), stderr: errBuf.String()}
}

func TestBuilderGuardAllowsABuilderLaunchOnATaskTheShaperDesigned(t *testing.T) {
	root := bgRepo(t)
	for name, prompt := range map[string]string{
		"a relative path":  "Do T1.\nSpec: odd/tasks/f.md, T1, S14\n",
		"an absolute path": "Spec: " + filepath.Join(root, "odd", "tasks", "f.md") + ", T1",
	} {
		assertGuardSilent(t, name, bgRun(bgCall(t, root, "builder", prompt)))
	}
}

func TestBuilderGuardDeniesABuilderLaunchWithTheExactJSONAndExit0(t *testing.T) {
	root := bgRepo(t)
	r := bgRun(bgCall(t, root, "builder", "Spec: odd/tasks/f.md, T2"))
	reason := decodeGuardDenial(t, r)
	path := filepath.Join(root, "odd", "tasks", "f.md")
	want := "labdrian builder guard: task T2 of " + path + ` is not marked as designed by the shaper (its line has no "shaped: L<n>"). ` +
		`The builder never designs: launch the shaper first to design the task and mark its line "shaped: L<n>", then launch the builder with the line "Spec: odd/tasks/<feature>.md, T<n>" in its prompt. ` +
		`This guard reads text only, so it is a speed bump, not a security boundary.`
	if reason != want {
		t.Fatalf("reason = %q, want %q", reason, want)
	}
	var out struct {
		HookSpecificOutput struct {
			HookEventName, PermissionDecision, PermissionDecisionReason string
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &out); err != nil {
		t.Fatal(err)
	}
	if got := out.HookSpecificOutput; got.HookEventName != "PreToolUse" || got.PermissionDecision != "deny" {
		t.Fatalf("hookSpecificOutput = %+v, want a PreToolUse deny", got)
	}
}

func TestBuilderGuardDeniesABuilderLaunchThatCitesNothing(t *testing.T) {
	root := bgRepo(t)
	for name, c := range map[string]struct {
		prompt string
		reason oddtask.Reason
	}{
		"no citation":  {"just build it", oddtask.ReasonNoCitation},
		"two citation": {"Spec: odd/tasks/f.md, T1\nSpec: odd/tasks/f.md, T1", oddtask.ReasonManyCitations},
		"a bad one":    {"Spec: odd/tasks/f.md", oddtask.ReasonBadCitation},
	} {
		if got := decodeGuardDenial(t, bgRun(bgCall(t, root, "builder", c.prompt))); got != oddtask.Message(c.reason) {
			t.Errorf("%s: reason = %q, want %q", name, got, oddtask.Message(c.reason))
		}
	}
}

func TestBuilderGuardDeniesWhenTheDocumentCannotBeRead(t *testing.T) {
	root := bgRepo(t)
	gone := filepath.Join(root, "odd", "tasks", "gone.md")
	got := decodeGuardDenial(t, bgRun(bgCall(t, root, "builder", "Spec: odd/tasks/gone.md, T1")))
	if want := oddtask.Message(oddtask.UnreadableReason(gone, oddtask.CauseNotFound)); got != want {
		t.Fatalf("reason = %q, want %q", got, want)
	}
	// A relative path with a session that has no cwd cannot be resolved.
	got = decodeGuardDenial(t, bgRun(bgCall(t, "", "builder", "Spec: odd/tasks/f.md, T1")))
	if want := oddtask.Message(oddtask.UnreadableReason("odd/tasks/f.md", oddtask.CauseNoCwd)); got != want {
		t.Fatalf("reason = %q, want %q", got, want)
	}
}

func TestBuilderGuardLeavesEveryOtherAgentAloneAndReadsNothing(t *testing.T) {
	root := bgRepo(t)
	for _, agent := range []string{"verifier", "reviewer", "shaper", "general-purpose", "Builder", "builder ", "builders", ""} {
		for _, prompt := range []string{"no citation at all", "Spec: odd/tasks/gone.md, T9", "Spec: odd/tasks/f.md, T2"} {
			assertGuardSilent(t, agent+" "+prompt, bgRun(bgCall(t, root, agent, prompt)))
		}
	}
}

func TestBuilderGuardDeniesABuilderLaunchWhenItPanics(t *testing.T) {
	var out, errBuf bytes.Buffer
	var codes []int
	runRolesBuilderGuard(nil, strings.NewReader(bgCall(t, "/repo", "builder", "Spec: odd/tasks/f.md, T1")), &out, &errBuf,
		func(c int) { codes = append(codes, c) }, panickingDocs{})
	r := guardRun{codes: codes, stdout: out.String(), stderr: errBuf.String()}
	if !strings.Contains(r.stderr, "internal error") {
		t.Errorf("stderr = %q, want the panic reported", r.stderr)
	}
	r.stderr = ""
	if got := decodeGuardDenial(t, r); got != oddtask.Message(oddtask.ReasonInternal) {
		t.Fatalf("reason = %q, want %q", got, oddtask.Message(oddtask.ReasonInternal))
	}
}

type panickingDocs struct{}

func (panickingDocs) Read(string, string) (oddtask.File, error) { panic("boom") }

func TestBuilderGuardLetsThroughWhatItCannotTellIsABuilder(t *testing.T) {
	for name, in := range map[string]string{
		"not json":             "not json",
		"empty":                "",
		"no tool_input":        `{"tool_name":"Agent"}`,
		"a prompt of a number": `{"tool_name":"Agent","tool_input":{"subagent_type":"builder","prompt":3}}`,
		"a type of a number":   `{"tool_name":"Agent","tool_input":{"subagent_type":3,"prompt":"x"}}`,
		"input cut short":      `{"tool_name":"Agent","tool_input":{"subagent_type":"builder","prompt":"Spec`,
	} {
		r := bgRun(in)
		if r.stdout != "" || !reflect.DeepEqual(r.codes, []int{0}) {
			t.Errorf("%s: exits %v, stdout %q, want an allow with exit 0", name, r.codes, r.stdout)
		}
		if strings.Count(r.stderr, "\n") != 1 || !strings.HasPrefix(r.stderr, "roles builder-guard:") {
			t.Errorf("%s: stderr = %q, want one line that says the call was not checked", name, r.stderr)
		}
	}
}

func TestBuilderGuardLetsThroughAStdinThatFails(t *testing.T) {
	var out, errBuf bytes.Buffer
	var codes []int
	runRolesCore([]string{"builder-guard"}, failingReader{}, &out, &errBuf, func(c int) { codes = append(codes, c) }, nil)
	if out.String() != "" || !reflect.DeepEqual(codes, []int{0}) || strings.Count(errBuf.String(), "\n") != 1 {
		t.Fatalf("exits %v, stdout %q, stderr %q, want an allow with exit 0 and one line on stderr", codes, out.String(), errBuf.String())
	}
}

func TestBuilderGuardDeniesAnAgentCallOverTheBoundWhateverTheAgent(t *testing.T) {
	root := bgRepo(t)
	want := oddtask.Message(oddtask.TooLargeReason(hookwire.MaxAgentCallBytes))
	big := strings.Repeat("x", hookwire.MaxAgentCallBytes)
	for _, agent := range []string{"builder", "verifier"} {
		if got := decodeGuardDenial(t, bgRun(bgCall(t, root, agent, "Spec: odd/tasks/f.md, T1\n"+big))); got != want {
			t.Errorf("%s: reason = %q, want %q", agent, got, want)
		}
	}
	// An endless input is cut at the bound, not read to its end.
	if got := decodeGuardDenial(t, bgRun(strings.Repeat(" ", hookwire.MaxAgentCallBytes+10))); got != want {
		t.Errorf("padding: reason = %q, want %q", got, want)
	}
}

// bgEndlessReader never ends and counts what is read from it.
type bgEndlessReader struct{ n int }

func (e *bgEndlessReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = ' '
	}
	e.n += len(p)
	return len(p), nil
}

func TestBuilderGuardReadsOneByteMoreThanTheBoundOfAnEndlessInput(t *testing.T) {
	in := &bgEndlessReader{}
	var out, errBuf bytes.Buffer
	var codes []int
	runRolesCore([]string{"builder-guard"}, in, &out, &errBuf, func(c int) { codes = append(codes, c) }, nil)
	if in.n != hookwire.MaxAgentCallBytes+1 {
		t.Fatalf("read %d bytes of an endless input, want %d", in.n, hookwire.MaxAgentCallBytes+1)
	}
	if !strings.Contains(out.String(), `"permissionDecision":"deny"`) || !reflect.DeepEqual(codes, []int{0}) {
		t.Fatalf("exits %v, stdout %q, want a denial with exit 0", codes, out.String())
	}
}

func TestBuilderGuardTakesNoArgument(t *testing.T) {
	r := bgRun(bgCall(t, "/repo", "builder", "x"), "--now")
	if !reflect.DeepEqual(r.codes, []int{1}) || r.stdout != "" || !strings.Contains(r.stderr, `unexpected argument "--now"`) {
		t.Fatalf("exits %v, stdout %q, stderr %q, want exit 1 and the argument named", r.codes, r.stdout, r.stderr)
	}
}

// The verbs that existed before keep their answers when no verb or an unknown one is given: builder-guard is a
// hook command, not one a person runs, so the usage lines do not list it.
func TestRolesPreviousVerbsStillAnswerAsBefore(t *testing.T) {
	cases := map[string]struct {
		args []string
		want string
	}{
		"no verb":      {nil, "error: roles requires a verb: validate, next, resume, append, match-shaper\n"},
		"unknown verb": {[]string{"bogus"}, "error: roles: unknown verb \"bogus\" (expected validate, next, resume, append, or match-shaper)\n"},
		"validate":     {[]string{"validate"}, "error: roles validate: --file is required\n"},
		"match-shaper": {[]string{"match-shaper"}, "error: roles match-shaper: --root is required\n"},
	}
	for name, c := range cases {
		var out, errBuf bytes.Buffer
		var codes []int
		runRolesCore(c.args, strings.NewReader(""), &out, &errBuf, func(n int) { codes = append(codes, n) }, nil)
		if errBuf.String() != c.want || out.String() != "" || !reflect.DeepEqual(codes, []int{1}) {
			t.Errorf("%s: exits %v, stdout %q, stderr %q, want exit 1 and %q", name, codes, out.String(), errBuf.String(), c.want)
		}
	}
}
