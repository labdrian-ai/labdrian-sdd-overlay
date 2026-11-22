package hookwire_test

// Tests for DecodeScopedToolCall, the strict decoding of the classifier guard (merge-playbook-into-sdd-overlay
// S5, T3c-a2, L42). H1 pins the inputs a guard could misread; H2 pins that, on a well-formed input, it reads what
// DecodeToolCall reads.

import (
	"errors"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/hookwire"
)

func TestDecodeScopedToolCallRefusesInputAGuardCouldMisread(t *testing.T) {
	refused := []struct{ name, in string }{
		{"repeated command", `{"tool_name":"Bash","tool_input":{"command":"a","command":"b"}}`},
		{"repeated file_path", `{"tool_name":"Write","tool_input":{"file_path":"/a","file_path":"/b"}}`},
		{"repeated tool_name", `{"tool_name":"Read","tool_name":"Bash","tool_input":{}}`},
		{"repeated cwd", `{"cwd":"/a","cwd":"/b","tool_name":"Read","tool_input":{}}`},
		{"repeated tool_input", `{"tool_name":"Read","tool_input":{},"tool_input":{}}`},
		{"repeated unexpected key", `{"tool_name":"Read","x":1,"x":2,"tool_input":{}}`},
		{"Command", `{"tool_name":"Bash","tool_input":{"Command":"a"}}`},
		{"FILE_PATH", `{"tool_name":"Write","tool_input":{"FILE_PATH":"/a"}}`},
		{"Tool_Name", `{"Tool_Name":"Read","tool_input":{}}`},
		{"CWD", `{"CWD":"/a","tool_name":"Read","tool_input":{}}`},
		{"Tool_Input", `{"tool_name":"Read","Tool_Input":{}}`},
		{"Kelvin sign in notebook_path", `{"tool_name":"NotebookEdit","tool_input":{"noteboo` + string(rune(0x212a)) + `_path":"/a"}}`},
		{"literal Kelvin sign in notebook_path", "{\"tool_name\":\"NotebookEdit\",\"tool_input\":{\"notebooK_path\":\"/a\"}}"},
		{"null command", `{"tool_name":"Bash","tool_input":{"command":null}}`},
		{"number file_path", `{"tool_name":"Write","tool_input":{"file_path":1}}`},
		{"null tool_input", `{"tool_name":"Read","tool_input":null}`},
		{"array tool_input", `{"tool_name":"Read","tool_input":[]}`},
		{"top-level null", `null`},
		{"top-level array", `[]`},
		{"trailing object", `{"tool_name":"Read","tool_input":{}}{}`},
		{"lone surrogate in file_path", `{"tool_name":"Write","tool_input":{"file_path":"/a\ud800"}}`},
		{"byte 0xff in command", "{\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"a\xffb\"}}"},
	}
	for _, tc := range refused {
		if _, err := hookwire.DecodeScopedToolCall([]byte(tc.in)); err == nil {
			t.Errorf("%s: DecodeScopedToolCall(%q) = nil error, want an error", tc.name, tc.in)
		}
	}
	over := []byte(strings.Repeat(" ", hookwire.MaxToolCallBytes+1))
	if _, err := hookwire.DecodeScopedToolCall(over); !errors.Is(err, hookwire.ErrTooLarge) {
		t.Errorf("input over the bound: error = %v, want ErrTooLarge", err)
	}
	// An error names the key and never its value.
	_, err := hookwire.DecodeScopedToolCall([]byte(`{"tool_name":"Bash","tool_input":{"command":"a","command":"secret-value"}}`))
	if err == nil || !strings.Contains(err.Error(), "command") || strings.Contains(err.Error(), "secret-value") {
		t.Errorf("repeated key error = %v, want it to name the key and not the value", err)
	}
}

func TestDecodeScopedToolCallReadsWhatDecodeToolCallReads(t *testing.T) {
	cases := []struct {
		name, in string
		want     hookwire.ScopedToolCall
	}{
		{"Read", `{"tool_name":"Read","cwd":"/r","tool_input":{"file_path":"/r/x"}}`,
			hookwire.ScopedToolCall{Name: "Read", Cwd: "/r", FilePath: "/r/x"}},
		{"Write", `{"tool_name":"Write","cwd":"/r","tool_input":{"file_path":"/r/docs/wiki/evidence.md","content":"x"}}`,
			hookwire.ScopedToolCall{Name: "Write", Cwd: "/r", FilePath: "/r/docs/wiki/evidence.md"}},
		{"Edit", `{"tool_name":"Edit","cwd":"/r","tool_input":{"file_path":"/r/a","old_string":"a","new_string":"b","replace_all":false}}`,
			hookwire.ScopedToolCall{Name: "Edit", Cwd: "/r", FilePath: "/r/a"}},
		{"Bash", `{"tool_name":"Bash","cwd":"/r","tool_input":{"command":"ls","description":"d","timeout":1000}}`,
			hookwire.ScopedToolCall{Name: "Bash", Cwd: "/r", Command: "ls"}},
		{"Grep", `{"tool_name":"Grep","cwd":"/r","tool_input":{"pattern":"p","path":"/r/docs","glob":"*.md","-n":true}}`,
			hookwire.ScopedToolCall{Name: "Grep", Cwd: "/r", Path: "/r/docs", Pattern: "p"}},
		{"Glob", `{"tool_name":"Glob","cwd":"/r","tool_input":{"pattern":"**/*.md","path":"/r/docs"}}`,
			hookwire.ScopedToolCall{Name: "Glob", Cwd: "/r", Path: "/r/docs", Pattern: "**/*.md"}},
		{"NotebookEdit", `{"tool_name":"NotebookEdit","tool_input":{"notebook_path":"/r/n.ipynb"}}`,
			hookwire.ScopedToolCall{Name: "NotebookEdit", NotebookPath: "/r/n.ipynb"}},
		{"envelope fields", `{"session_id":"s","transcript_path":"/t","cwd":"/r","permission_mode":"default","hook_event_name":"PreToolUse",` +
			`"agent_id":"a","agent_type":"classifier","tool_name":"Read","tool_input":{"file_path":"/r/x"},"tool_use_id":"u"}`,
			hookwire.ScopedToolCall{Name: "Read", Cwd: "/r", FilePath: "/r/x"}},
	}
	for _, tc := range cases {
		got, err := hookwire.DecodeScopedToolCall([]byte(tc.in))
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s: got %+v, want %+v", tc.name, got, tc.want)
		}
		lenient, err := hookwire.DecodeToolCall([]byte(tc.in))
		if err != nil {
			t.Errorf("%s: DecodeToolCall: %v", tc.name, err)
			continue
		}
		if lenient.Name != got.Name || lenient.Command != got.Command || lenient.FilePath != got.FilePath || lenient.NotebookPath != got.NotebookPath {
			t.Errorf("%s: DecodeToolCall read %+v, DecodeScopedToolCall read %+v", tc.name, lenient, got)
		}
	}
}
