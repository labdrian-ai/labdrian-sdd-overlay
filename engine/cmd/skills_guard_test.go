package main

// Tests for 'skills guard-hook', the Claude Code PreToolUse hook that denies the
// agent running `skills approve`. The decision itself is engine/skills'
// DecideApproveGuard and is tested there; these tests pin the hook's contract
// with Claude Code: what it prints, its exit code, and that nothing it does can
// block a tool call. Everything runs in process on injected streams, so nothing
// here reaches real state.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/hookwire"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

type guardRun struct {
	codes  []int
	stdout string
	stderr string
}

func runGuardHookWith(stdin io.Reader, args ...string) guardRun {
	var out, errBuf bytes.Buffer
	var codes []int
	runSkillsWithStdin(append([]string{"guard-hook"}, args...), stdin, &out, &errBuf, func(c int) { codes = append(codes, c) })
	return guardRun{codes: codes, stdout: out.String(), stderr: errBuf.String()}
}

func runGuardHook(stdin string) guardRun { return runGuardHookWith(strings.NewReader(stdin)) }

// runGuardHookBefore runs the hook with beforeDecision called where it decides, which is where
// a bug in it would panic.
func runGuardHookBefore(stdin string, beforeDecision func()) guardRun {
	var out, errBuf bytes.Buffer
	var codes []int
	runSkillsGuardHook(nil, strings.NewReader(stdin), &out, &errBuf, func(c int) { codes = append(codes, c) }, beforeDecision)
	return guardRun{codes: codes, stdout: out.String(), stderr: errBuf.String()}
}

func guardToolInput(t *testing.T, tool string, toolInput map[string]any) string {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"session_id":      "s-1",
		"cwd":             "/repo",
		"hook_event_name": "PreToolUse",
		"tool_name":       tool,
		"tool_input":      toolInput,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// assertGuardSilent requires an allow: no stdout, no stderr, one exit 0.
func assertGuardSilent(t *testing.T, label string, r guardRun) {
	t.Helper()
	if r.stdout != "" || r.stderr != "" || !reflect.DeepEqual(r.codes, []int{0}) {
		t.Errorf("%s: exits %v, stdout %q, stderr %q, want a silent exit 0", label, r.codes, r.stdout, r.stderr)
	}
}

// decodeGuardDenial requires stdout to be exactly the one JSON object Claude
// Code reads as a PreToolUse denial, with exit 0 and nothing on stderr, and
// returns the reason.
func decodeGuardDenial(t *testing.T, r guardRun) string {
	t.Helper()
	if !reflect.DeepEqual(r.codes, []int{0}) || r.stderr != "" {
		t.Fatalf("exits %v, stderr %q, want exactly one exit 0 and no stderr (a denial is JSON, never exit 2)", r.codes, r.stderr)
	}
	if !strings.HasPrefix(r.stdout, "{") || !strings.HasSuffix(r.stdout, "}\n") || strings.Count(r.stdout, "\n") != 1 || !json.Valid([]byte(r.stdout)) {
		t.Fatalf("stdout %q is not exactly one JSON object on one line", r.stdout)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(r.stdout), &top); err != nil {
		t.Fatal(err)
	}
	if len(top) != 1 || top["hookSpecificOutput"] == nil {
		t.Fatalf("stdout carries %v, want only hookSpecificOutput", top)
	}
	var specific map[string]string
	if err := json.Unmarshal(top["hookSpecificOutput"], &specific); err != nil {
		t.Fatal(err)
	}
	if len(specific) != 3 || specific["hookEventName"] != "PreToolUse" || specific["permissionDecision"] != "deny" || specific["permissionDecisionReason"] == "" {
		t.Fatalf("hookSpecificOutput = %v, want hookEventName PreToolUse, permissionDecision deny, and a reason", specific)
	}
	return specific["permissionDecisionReason"]
}

func TestSkillsGuardHook_DeniesAnApproveInvocationWithTheDocumentedJSON(t *testing.T) {
	r := runGuardHook(guardToolInput(t, "Bash", map[string]any{"command": "cd repo && labdrian skills approve --id my-skill --approver alice"}))
	reason := decodeGuardDenial(t, r)
	if want := skills.DecideApproveGuard(skills.ApproveGuardCall{Tool: "Bash", Command: "labdrian skills approve"}).Reason; reason != want {
		t.Errorf("reason = %q, want the decision's reason %q", reason, want)
	}
	// The reason is read by people: <id> and <name> stay as they are.
	if !strings.Contains(r.stdout, "--id <id> --approver <name>") {
		t.Errorf("stdout %q escaped the placeholders in the command the human is told to run", r.stdout)
	}
}

func TestSkillsGuardHook_DeniesWritingTheApprovalRecord(t *testing.T) {
	for _, tc := range []struct{ tool, field string }{
		{"Write", "file_path"}, {"Edit", "file_path"}, {"MultiEdit", "file_path"}, {"NotebookEdit", "notebook_path"},
	} {
		t.Run(tc.tool, func(t *testing.T) {
			r := runGuardHook(guardToolInput(t, tc.tool, map[string]any{tc.field: "/repo/skills/my-skill/.approval.json"}))
			if reason := decodeGuardDenial(t, r); !strings.Contains(reason, skills.ApprovalRecordName) {
				t.Errorf("reason %q does not name the record file", reason)
			}
		})
	}
}

func TestSkillsGuardHook_AnAllowPrintsNothingAtAll(t *testing.T) {
	// Never an explicit allow: that would bypass Claude Code's own permission flow.
	for name, input := range map[string]string{
		"a harmless Bash call":          guardToolInput(t, "Bash", map[string]any{"command": "ls -la"}),
		"another skills verb":           guardToolInput(t, "Bash", map[string]any{"command": "labdrian skills validate --source-root skills"}),
		"a Write of documentation":      guardToolInput(t, "Write", map[string]any{"file_path": "/repo/README.md", "content": "run labdrian skills approve"}),
		"a Read of the record":          guardToolInput(t, "Read", map[string]any{"file_path": "/repo/skills/x/.approval.json"}),
		"a Write to another file":       guardToolInput(t, "Write", map[string]any{"file_path": "/repo/skills/x/SKILL.md"}),
		"a payload with unknown fields": `{"tool_name":"Bash","tool_input":{"command":"true"},"new_field":[1,2]}`,
	} {
		t.Run(name, func(t *testing.T) { assertGuardSilent(t, name, runGuardHook(input)) })
	}
}

// The hook runs on every Bash and file-edit call, so whatever goes wrong the
// call goes through: unusable input, an unreadable stdin, a failed write of the
// answer, and a panic are all an exit 0, and never the exit 2 that blocks.
func TestSkillsGuardHook_NothingItDoesCanBlockACall(t *testing.T) {
	t.Run("unusable input", func(t *testing.T) {
		for name, input := range map[string]string{
			"empty":         "",
			"not JSON":      "labdrian skills approve",
			"truncated":     `{"tool_name":"Bash","tool_input":{"command":"labdrian skills app`,
			"binary":        "\x00\x01\xff\xfe",
			"an array":      `[1,2,3]`,
			"a null":        `null`,
			"a wrong shape": `{"tool_name":"Bash","tool_input":"labdrian skills approve"}`,
		} {
			assertGuardSilent(t, name, runGuardHook(input))
		}
	})

	t.Run("stdin that cannot be read", func(t *testing.T) {
		assertGuardSilent(t, "read error", runGuardHookWith(errReader{}))
	})

	t.Run("an answer that cannot be written", func(t *testing.T) {
		var codes []int
		runSkillsWithStdin([]string{"guard-hook"},
			strings.NewReader(guardToolInput(t, "Bash", map[string]any{"command": "labdrian skills approve"})),
			failingWriter{}, io.Discard, func(c int) { codes = append(codes, c) })
		if !reflect.DeepEqual(codes, []int{0}) {
			t.Errorf("exits %v, want a single exit 0 when stdout cannot be written", codes)
		}
	})

	t.Run("a panic in the decision", func(t *testing.T) {
		r := runGuardHookBefore(guardToolInput(t, "Bash", map[string]any{"command": "labdrian skills approve"}), func() { panic("boom") })
		if !reflect.DeepEqual(r.codes, []int{0}) {
			t.Errorf("exits %v, want a single exit 0 (a Go panic exits 2, which blocks the call)", r.codes)
		}
		if strings.Contains(r.stdout, "permissionDecision") {
			t.Errorf("stdout %q carries a permission decision; a panic must never decide", r.stdout)
		}
		if !strings.Contains(r.stderr, "boom") {
			t.Errorf("stderr %q does not report the internal error", r.stderr)
		}
	})
}

// TestSkillsGuardHook_ADegradedGuardSaysSo: a recovered panic used to be visible
// only on stderr, which Claude Code shows only in verbose mode, so a guard that
// had stopped guarding looked like a guard that allowed. It now also shows the
// user one short sanitized systemMessage, the way the projection hook does for a
// PreToolUse panic, and still never decides: exit 0, no permission decision.
func TestSkillsGuardHook_ADegradedGuardSaysSo(t *testing.T) {
	r := runGuardHookBefore(guardToolInput(t, "Bash", map[string]any{"command": "labdrian skills approve --id x"}),
		func() { panic("boom\nsecond line \x1b[31mred " + strings.Repeat("x", 5000)) })
	if !reflect.DeepEqual(r.codes, []int{0}) || !strings.Contains(r.stderr, "internal error: boom") {
		t.Fatalf("exits %v, stderr %q, want exit 0 and the panic reported on stderr", r.codes, r.stderr)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(r.stdout), &top); err != nil || len(top) != 1 || top["systemMessage"] == nil {
		t.Fatalf("stdout %q (%v), want only a systemMessage: the call is allowed", r.stdout, err)
	}
	var warning string
	if err := json.Unmarshal(top["systemMessage"], &warning); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(warning, "boom second line") || strings.ContainsAny(warning, "\n\x1b") || len(warning) > 700 {
		t.Errorf("warning %q is not one short clean line naming the error", warning)
	}
	for _, want := range []string{"approve guard", "tool call", "not denied"} {
		if !strings.Contains(warning, want) {
			t.Errorf("warning %q does not say %q: it must name the guard and what happened to the call", warning, want)
		}
	}
	if strings.Contains(warning, "workflow") || strings.Contains(warning, "projection") {
		t.Errorf("warning %q speaks of the projection hook; it is the approve guard that failed", warning)
	}
}

// The read is bounded: an endless stdin is never read to the end, and is denied as too large.
func TestSkillsGuardHook_ReadsAtMostTheBound(t *testing.T) {
	src := &endlessReader{}
	r := runGuardHookWith(src)
	if reason := decodeGuardDenial(t, r); !strings.Contains(reason, "too large") {
		t.Errorf("endless input: reason %q, want the denial for the size", reason)
	}
	// endlessReader (projection_hook_test.go) counts the bytes it was asked for.
	// One byte past the bound is enough to know the input is over it.
	if limit := hookwire.MaxToolCallBytes + 1; src.read != limit {
		t.Errorf("read %d bytes, want exactly %d: up to one byte past the bound", src.read, limit)
	}
}

// The bound is the cause of the denial, not a coincidence of the content: the same call, a
// command that would be denied, is denied within the bound and denied one byte over it, and so
// is a command that would be allowed, because the agent controls the length of its own command
// and a guard that let padding switch it off would guard nothing. The bound is inclusive, and
// the two denials differ: the one over the bound says the call was too large and names the
// bound, and the golden file of the eight mebibytes pins its bytes.
func TestSkillsGuardHook_ACallOverTheBoundIsDeniedBecauseOfTheBound(t *testing.T) {
	const suffix = `"}}`
	padded := func(prefix string, size int) string {
		return prefix + strings.Repeat("a", size-len(prefix)-len(suffix)) + suffix
	}
	const matching = `{"tool_name":"Bash","tool_input":{"command":"labdrian skills approve --id x","padding":"`
	const unrelated = `{"tool_name":"Bash","tool_input":{"command":"ls","padding":"`
	for name, size := range map[string]int{
		"well within the bound": hookwire.MaxToolCallBytes - 1024,
		"exactly the bound":     hookwire.MaxToolCallBytes,
	} {
		reason := decodeGuardDenial(t, runGuardHook(padded(matching, size)))
		if reason == "" || strings.Contains(reason, "too large") {
			t.Errorf("%s: a matching call has reason %q, want the denial of the command", name, reason)
		}
		assertGuardSilent(t, name+": an unrelated call", runGuardHook(padded(unrelated, size)))
	}
	for name, prefix := range map[string]string{"a matching call": matching, "an unrelated call": unrelated} {
		reason := decodeGuardDenial(t, runGuardHook(padded(prefix, hookwire.MaxToolCallBytes+1)))
		for _, want := range []string{"skills approve guard", "too large", fmt.Sprint(hookwire.MaxToolCallBytes), "skills approve"} {
			if !strings.Contains(reason, want) {
				t.Errorf("%s one byte over the bound: reason %q does not say %q", name, reason, want)
			}
		}
	}
}

// What the guard says when its decision panics is made of the recovered value, which is
// anything: an error, a number, a struct, and not only the strings the golden file pins. Each is
// one short clean line that names the guard and says the call was not denied.
func TestGuardPanicWarningNamesWhatWasRecoveredWhateverItIs(t *testing.T) {
	for name, tc := range map[string]struct {
		recovered any
		want      string
	}{
		"an error":                 {errors.New("disk on fire"), "disk on fire"},
		"a wrapped error":          {fmt.Errorf("outer: %w", errors.New("inner")), "outer: inner"},
		"a number":                 {42, "42"},
		"a struct":                 {struct{ Code int }{7}, "7"},
		"a runtime error":          {runtimeErrorOf(t), "index out of range"},
		"a string with a new line": {"first\nsecond\x1b[31m", "first second"},
	} {
		t.Run(name, func(t *testing.T) {
			got := guardPanicWarning(tc.recovered)
			if !strings.Contains(got, tc.want) {
				t.Errorf("the warning %q does not name the recovered value (%q)", got, tc.want)
			}
			if strings.ContainsAny(got, "\n\x1b") || len(got) > 700 {
				t.Errorf("the warning %q is not one short clean line", got)
			}
			for _, want := range []string{"approve guard", "not denied"} {
				if !strings.Contains(got, want) {
					t.Errorf("the warning %q does not say %q", got, want)
				}
			}
		})
	}
}

// runtimeErrorOf is the value recover returns for a runtime panic: an error that is not ours.
func runtimeErrorOf(t *testing.T) (recovered any) {
	t.Helper()
	defer func() { recovered = recover() }()
	var empty []int
	_ = empty[len(empty)]
	return nil
}

func TestSkillsGuardHook_RefusesAnArgumentItDoesNotUnderstand(t *testing.T) {
	for _, args := range [][]string{{"--event", "PreToolUse"}, {"extra"}, {"--"}} {
		r := runGuardHookWith(strings.NewReader(guardToolInput(t, "Bash", map[string]any{"command": "labdrian skills approve"})), args...)
		if !reflect.DeepEqual(r.codes, []int{1}) || r.stdout != "" || !strings.Contains(r.stderr, "guard-hook") {
			t.Errorf("args %v: exits %v, stdout %q, stderr %q, want exit 1, no stdout, and a message naming guard-hook", args, r.codes, r.stdout, r.stderr)
		}
	}
}

// The verb sits beside the real skills verbs without disturbing them.
func TestSkillsWithStdin_OtherVerbsStillReachTheSkillsCore(t *testing.T) {
	var out, errBuf bytes.Buffer
	var codes []int
	runSkillsWithStdin([]string{"nuke"}, strings.NewReader(""), &out, &errBuf, func(c int) { codes = append(codes, c) })
	if len(codes) == 0 || codes[0] != 1 || !strings.Contains(errBuf.String(), `unknown skills verb "nuke"`) {
		t.Errorf("exits %v, stderr %q, want the skills core's unknown-verb refusal", codes, errBuf.String())
	}
	out.Reset()
	errBuf.Reset()
	codes = nil
	runSkillsWithStdin(nil, strings.NewReader(""), &out, &errBuf, func(c int) { codes = append(codes, c) })
	if len(codes) == 0 || codes[0] != 1 || !strings.Contains(errBuf.String(), "skills requires a verb") {
		t.Errorf("exits %v, stderr %q, want the missing-verb refusal", codes, errBuf.String())
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("stdin closed") }

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("stdout closed") }
