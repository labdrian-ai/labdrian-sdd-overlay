package hookwire_test

import (
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/hookwire"
)

func TestPromptText(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		want   string
		wantOK bool
	}{
		{"an object with a text prompt", `{"prompt":"hello","cwd":"/r"}`, "hello", true},
		{"white space around the object", " \n{\"prompt\":\"hi\"}\n", "hi", true},
		{"an empty prompt is a prompt", `{"prompt":""}`, "", true},
		{"escapes are decoded", `{"prompt":"<task-notification>\n"}`, "<task-notification>\n", true},
		{"no prompt field", `{"cwd":"/r"}`, "", false},
		{"a null prompt", `{"prompt":null}`, "", false},
		{"a prompt that is not text", `{"prompt":7}`, "", false},
		{"an array", `[{"prompt":"x"}]`, "", false},
		{"not JSON", `hello`, "", false},
		{"empty input", ``, "", false},
		{"two objects", `{"prompt":"a"}{"prompt":"b"}`, "", false},
		{"an input over the envelope bound", `{"prompt":"` + strings.Repeat("x", hookwire.MaxEnvelopeBytes) + `"}`, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := hookwire.PromptText([]byte(c.in))
			if got != c.want || ok != c.wantOK {
				t.Errorf("PromptText = (%q, %v), want (%q, %v)", got, ok, c.want, c.wantOK)
			}
		})
	}
}

func TestIsTaskNotification(t *testing.T) {
	cases := []struct {
		name   string
		prompt string
		want   bool
	}{
		{"starts with the tag", "<task-notification>\n<task-id>a</task-id>", true},
		{"the system notice wrapped in a reminder", "<system-reminder>\n[SYSTEM NOTIFICATION - NOT USER INPUT]\nx", true},
		{"blank lines before the notice line", "<system-reminder>\n\n  [SYSTEM NOTIFICATION - NOT USER INPUT]\nx", true},
		{"a prompt that quotes the notice mid-text", "intro [SYSTEM NOTIFICATION - NOT USER INPUT] tail", false},
		{"a quoted notice on a line of its own", "why does\n[SYSTEM NOTIFICATION - NOT USER INPUT]\nappear?", false},
		{"a reminder whose first line is something else", "<system-reminder>\nnote\n[SYSTEM NOTIFICATION - NOT USER INPUT]\n", false},
		{"the tag in the middle of a prompt", "please explain <task-notification> to me", false},
		{"the tag after white space is not the start", "\n<task-notification>", false},
		{"an ordinary prompt", "how does the merge work?", false},
		{"an empty prompt", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := hookwire.IsTaskNotification(c.prompt); got != c.want {
				t.Errorf("IsTaskNotification(%q) = %v, want %v", c.prompt, got, c.want)
			}
		})
	}
}
