package projection_test

// Tests for the pieces the two hook events share: the lenient hook-input parser,
// the event-aware panic warning, and the list of tools the gate cares about.

import (
	"errors"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
)

// TestPanicWarningNamesTheEventItHappenedIn: the recovered-panic warning used to
// say "the prompt was not affected" also when the panic happened in the
// PreToolUse gate, where no prompt is involved. Each event now says what it
// actually did to its own subject, and never mentions the other's.
func TestPanicWarningNamesTheEventItHappenedIn(t *testing.T) {
	prompt := projection.PanicWarning(projection.HookEventUserPromptSubmit, "boom")
	if !strings.Contains(prompt, "the prompt was not affected") || strings.Contains(prompt, "tool call") {
		t.Errorf("prompt warning %q, want it to speak of the prompt only", prompt)
	}
	tool := projection.PanicWarning(projection.HookEventPreToolUse, "boom")
	if !strings.Contains(tool, "tool call") || strings.Contains(tool, "prompt") {
		t.Errorf("tool warning %q, want it to speak of the tool call only", tool)
	}
	for _, w := range []string{prompt, tool} {
		if !strings.Contains(w, "boom") || !strings.Contains(w, "internal error") || !strings.HasPrefix(w, "labdrian:") {
			t.Errorf("warning %q lost the labdrian prefix, the error, or its cause", w)
		}
	}
	// An unrecognized event gets the neutral wording, not either subject.
	other := projection.PanicWarning("SomethingElse", "boom")
	if strings.Contains(other, "prompt") || strings.Contains(other, "tool call") || !strings.Contains(other, "boom") {
		t.Errorf("warning for an unknown event %q, want neutral wording that keeps the cause", other)
	}
}

// TestGateRelevantSelectsOnlyTheToolsTheGateChecks: the gate needs the binding and
// the workflow only for a file-edit tool or a longterm-mem query; every other
// tool is decided from its name alone, so the hook never touches the stores for
// it.
func TestGateRelevantSelectsOnlyTheToolsTheGateChecks(t *testing.T) {
	for _, tool := range []string{"Write", "Edit", "MultiEdit", "NotebookEdit", "mcp__longterm-mem__query", "mcp__plugin_x_longterm-mem__query"} {
		if !projection.GateRelevant(tool) {
			t.Errorf("GateRelevant(%q) = false, want true", tool)
		}
	}
	for _, tool := range []string{
		"", "Bash", "Read", "Grep", "Glob", "Task", "WebFetch", "TodoWrite", "write", "Writer",
		"mcp__longterm-mem__get", "mcp__longterm-mem__promote", "mcp__longterm-memx__query", "mcp__engram__mem_save",
	} {
		if projection.GateRelevant(tool) {
			t.Errorf("GateRelevant(%q) = true, want false", tool)
		}
	}
}

// TestEditToolsIsTheGatedListInAStableOrder: EditTools is what README, the help,
// and the capability declaration are checked against, and it must be the very
// list the gate denies while paused. Editing the result changes nothing.
func TestEditToolsIsTheGatedListInAStableOrder(t *testing.T) {
	got := projection.EditTools()
	want := []string{"Write", "Edit", "MultiEdit", "NotebookEdit"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("EditTools() = %v, want %v", got, want)
	}
	got[0] = "Tampered"
	if again := projection.EditTools(); again[0] != "Write" {
		t.Errorf("editing the result changed the list: %v", again)
	}
	for _, tool := range projection.EditTools() {
		if !projection.GateRelevant(tool) {
			t.Errorf("edit tool %q is not gate-relevant", tool)
		}
	}
}

// TestBothHookInputParsersAgreeOnTheSharedRules: ParseHookInput and
// ParsePreToolUseInput read the same envelope (size cap, one JSON object, an
// absolute cleaned cwd), so they take one parser and must accept and refuse the
// same documents in the same way.
func TestBothHookInputParsersAgreeOnTheSharedRules(t *testing.T) {
	oversized := `{"cwd":"/r","prompt":"` + strings.Repeat("x", projection.MaxHookInputBytes) + `"}`
	for name, tc := range map[string]struct {
		data    string
		wantErr bool
		tooBig  bool
		wantCwd string
	}{
		"clean object":            {data: `{"hook_event_name":"E","cwd":"/a/b"}`, wantCwd: "/a/b"},
		"unclean cwd is cleaned":  {data: `{"cwd":"/a//b/./c/../"}`, wantCwd: "/a/b"},
		"relative cwd is missing": {data: `{"cwd":"a/b"}`, wantCwd: ""},
		"no cwd":                  {data: `{}`, wantCwd: ""},
		"padding is trimmed":      {data: " \n{\"cwd\":\"/r\"}\n ", wantCwd: "/r"},
		"an array":                {data: `[{"cwd":"/r"}]`, wantErr: true},
		"null":                    {data: `null`, wantErr: true},
		"empty":                   {data: ``, wantErr: true},
		"a cwd of the wrong type": {data: `{"cwd":7}`, wantErr: true},
		"two objects":             {data: `{"cwd":"/r"}{"cwd":"/r"}`, wantErr: true},
		"over the size cap":       {data: oversized, wantErr: true, tooBig: true},
	} {
		t.Run(name, func(t *testing.T) {
			p, pErr := projection.ParseHookInput([]byte(tc.data))
			g, gErr := projection.ParsePreToolUseInput([]byte(tc.data))
			if (pErr != nil) != tc.wantErr || (gErr != nil) != tc.wantErr {
				t.Fatalf("errors = %v and %v, want error=%v from both", pErr, gErr, tc.wantErr)
			}
			if tc.wantErr {
				if tc.tooBig && (!errors.Is(pErr, projection.ErrHookInputTooLarge) || !errors.Is(gErr, projection.ErrHookInputTooLarge)) {
					t.Errorf("errors %v and %v, want both to wrap ErrHookInputTooLarge", pErr, gErr)
				}
				return
			}
			if p.Cwd != tc.wantCwd || g.Cwd != tc.wantCwd {
				t.Errorf("cwd = %q and %q, want %q from both", p.Cwd, g.Cwd, tc.wantCwd)
			}
		})
	}
}
