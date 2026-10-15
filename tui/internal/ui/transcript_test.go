package ui

// The transcript is how the result screen shows what an action did: the screen
// renders it from the invocations the result records. The text is what the
// operator has always read, pinned by the program's goldens end to end; these
// prove it invocation by invocation, over values built by hand.

import (
	"errors"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/tui/internal/domain"
)

func TestTranscript_NamesEachInvocationItsOutputAndItsFailure(t *testing.T) {
	invocations := []domain.Invocation{
		{Args: []string{"status", "--target", "claude"}, Run: domain.Execution{Output: "claude: deployed\n"}},
		{Args: []string{"apply"}, Run: domain.Execution{Output: "boom\n", ExitCode: 1, Err: errors.New("exit status 1")}},
	}

	want := "$ bin/labdrian-overlay status --target claude\n" +
		"claude: deployed\n" +
		"\n" +
		"$ bin/labdrian-overlay apply\n" +
		"boom\n" +
		"\n[exit error: exit status 1]\n"
	if got := transcript(invocations); got != want {
		t.Errorf("transcript =\n%q\nwant\n%q", got, want)
	}
}

// A call that was refused has no output and no exit error: it is shown as
// omitted, with the reason, and the calls around it read as they do.
func TestTranscript_ARefusedInvocationIsOmittedWithItsReason(t *testing.T) {
	invocations := []domain.Invocation{
		{Args: []string{"self-update"}, Run: domain.Execution{Output: "updated\n"}},
		{Args: []string{"apply", "--target", "all"}, Refusal: errors.New("el catálogo cambió")},
		{Args: []string{"version"}, Run: domain.Execution{Output: "v1\n"}},
	}

	want := "$ bin/labdrian-overlay self-update\n" +
		"updated\n" +
		"\n" +
		"$ bin/labdrian-overlay apply --target all\n" +
		"[omitido: el catálogo cambió]\n" +
		"\n" +
		"$ bin/labdrian-overlay version\n" +
		"v1\n"
	if got := transcript(invocations); got != want {
		t.Errorf("transcript =\n%q\nwant\n%q", got, want)
	}
}

// Output is shown as the backend printed it: with no newline at its end the
// failure line follows the last word after one blank line, as it always did,
// and a run that printed nothing shows its "$" line alone.
func TestTranscript_ShowsOutputAsPrintedAndARunThatPrintedNothing(t *testing.T) {
	tests := []struct {
		name        string
		invocations []domain.Invocation
		want        string
	}{
		{"no invocation, no transcript", nil, ""},
		{"a run that printed nothing", []domain.Invocation{{Args: []string{"install-hooks"}}}, "$ bin/labdrian-overlay install-hooks\n"},
		{"output with no final newline, then a failure",
			[]domain.Invocation{{Args: []string{"apply"}, Run: domain.Execution{Output: "half a line", Err: errors.New("signal: killed"), ExitCode: domain.NoExitStatus}}},
			"$ bin/labdrian-overlay apply\nhalf a line\n[exit error: signal: killed]\n"},
		{"a run that failed without a word",
			[]domain.Invocation{{Args: []string{"apply"}, Run: domain.Execution{Err: errors.New("fork/exec: no such file"), ExitCode: domain.NoExitStatus}}},
			"$ bin/labdrian-overlay apply\n\n[exit error: fork/exec: no such file]\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := transcript(tc.invocations); got != tc.want {
				t.Errorf("transcript =\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}

// The output pane scrolls over the transcript, "$" lines and blank separators
// included: that is the text the operator reads, so its length is the length
// of the pane.
func TestMaxScroll_CountsTheLinesOfTheTranscript(t *testing.T) {
	m := newTestModel(t)
	m.width = 80
	m.height = 12 // the pane's viewport is clamped to its 5-line minimum
	m.scr = screenResult
	m.result = domain.CommandResult{
		Action: domain.Action{Name: "Estado", Command: "status"},
		Invocations: []domain.Invocation{
			{Args: []string{"one"}, Run: domain.Execution{Output: "a\nb\nc\n"}},
			{Args: []string{"two"}, Run: domain.Execution{Output: "d\n"}},
		},
	}

	// "$ one", a, b, c, a blank line, "$ two", d: seven lines in a viewport of five.
	if got := m.maxScroll(); got != 2 {
		t.Errorf("maxScroll() = %d, want 2: the pane has the 7 lines of the transcript, not the 4 the backend printed", got)
	}
	if rendered := stripANSI(m.View()); !strings.Contains(rendered, "[lines 1-5 of 7]") {
		t.Errorf("the output pane should count the 7 lines of the transcript, got:\n%s", rendered)
	}
}

// A result with nothing run has nothing to scroll and an empty pane.
func TestMaxScroll_AResultThatRanNothingHasNothingToScroll(t *testing.T) {
	m := newTestModel(t)
	m.height = 12
	m.scr = screenResult
	m.result = domain.CommandResult{Action: domain.Action{Name: "Estado", Command: "status"}}

	if got := m.maxScroll(); got != 0 {
		t.Errorf("maxScroll() = %d, want 0", got)
	}
}
