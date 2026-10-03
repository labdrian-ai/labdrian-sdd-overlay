package hookwire_test

// Tests for what the guards read of a tool call: the command a shell tool is about to run and
// the paths a file tool is about to write (DecodeToolCall, read by the approve guard and the
// clearance guard), the command alone (DecodeCommand, read by the review-receipt hook), and how
// the two hooks that answer by exit status say it (ExitReply).
//
// The two decoders differ on purpose and the difference is pinned: a field the receipt hook
// does not read, such as a file_path of the wrong type beside the command of an
// acknowledgement, must not stop it from capturing the receipts the acknowledgement burns.

import (
	"errors"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/hookwire"
)

// --- DecodeToolCall ------------------------------------------------------------

func TestToolCallKeepsTheNameTheCommandAndThePaths(t *testing.T) {
	got, err := hookwire.DecodeToolCall([]byte(`{"session_id":"s","cwd":"/r","hook_event_name":"PreToolUse","tool_name":"Bash",` +
		`"tool_input":{"command":"ls -la","description":"d","file_path":"/a","notebook_path":"/b","timeout":5},"tool_use_id":"t"}`))
	if err != nil {
		t.Fatalf("DecodeToolCall() = %v", err)
	}
	want := hookwire.ToolCall{Name: "Bash", Command: "ls -la", FilePath: "/a", NotebookPath: "/b"}
	if got != want {
		t.Errorf("DecodeToolCall() = %+v, want %+v", got, want)
	}
}

func TestToolCallAcceptsWhatItCanRead(t *testing.T) {
	for name, tc := range map[string]struct {
		data string
		want hookwire.ToolCall
	}{
		"an empty object":            {`{}`, hookwire.ToolCall{}},
		"only a name":                {`{"tool_name":"Read"}`, hookwire.ToolCall{Name: "Read"}},
		"a null tool_input":          {`{"tool_name":"Bash","tool_input":null}`, hookwire.ToolCall{Name: "Bash"}},
		"an empty tool_input":        {`{"tool_name":"Bash","tool_input":{}}`, hookwire.ToolCall{Name: "Bash"}},
		"null itself":                {`null`, hookwire.ToolCall{}},
		"padding around the object":  {" \n{\"tool_name\":\"Edit\"}\n ", hookwire.ToolCall{Name: "Edit"}},
		"null fields":                {`{"tool_name":null,"tool_input":{"command":null,"file_path":null}}`, hookwire.ToolCall{}},
		"keys in another case":       {`{"TOOL_NAME":"Bash","TOOL_INPUT":{"COMMAND":"ls","File_Path":"/a"}}`, hookwire.ToolCall{Name: "Bash", Command: "ls", FilePath: "/a"}},
		"the last of a repeated key": {`{"tool_name":"Bash","tool_input":{"command":"ls","command":"pwd"}}`, hookwire.ToolCall{Name: "Bash", Command: "pwd"}},
		"fields it does not read, of any type": {`{"cwd":5,"hook_event_name":7,"session_id":{},"tool_use_id":[],"tool_name":"Bash","tool_input":{"command":"ls","timeout":"soon"}}`,
			hookwire.ToolCall{Name: "Bash", Command: "ls"}},
		"text with escapes": {`{"tool_name":"Bash","tool_input":{"command":"a\nb\t\"c\" \\ é"}}`, hookwire.ToolCall{Name: "Bash", Command: "a\nb\t\"c\" \\ é"}},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := hookwire.DecodeToolCall([]byte(tc.data))
			if err != nil || got != tc.want {
				t.Errorf("DecodeToolCall(%q) = %+v, %v, want %+v", tc.data, got, err, tc.want)
			}
		})
	}
}

// TestToolCallDetailsAreTheWordsTheClearanceGuardAlwaysPrinted: the clearance guard fails
// closed, and says why in its denial with the decoder's own words, which the model reads. They
// are kept byte for byte, though they name the type that was once decoded into (it was the
// guard's own, in engine/shaper, before the hook format had one home) and the encoding/json
// words for it; that is a wording an owner may want to change, and deleting legacyDetail in
// hookwire is the whole of the change.
func TestToolCallDetailsAreTheWordsTheClearanceGuardAlwaysPrinted(t *testing.T) {
	const toolInputType = `struct { Command string "json:\"command\""; FilePath string "json:\"file_path\""; NotebookPath string "json:\"notebook_path\"" }`
	for name, tc := range map[string]struct{ data, detail string }{
		"empty":                    {``, `unexpected end of JSON input`},
		"white space":              {" \n\t ", `unexpected end of JSON input`},
		"not JSON":                 {`hello`, `invalid character 'h' looking for beginning of value`},
		"a truncated object":       {`{"tool_name":"Bash","tool_input":{"command":"ls"`, `unexpected end of JSON input`},
		"two objects":              {`{"tool_name":"Bash"}{"tool_name":"Bash"}`, `invalid character '{' after top-level value`},
		"text after the object":    {`{"tool_name":"Bash"} trailing`, `invalid character 't' after top-level value`},
		"an array":                 {`[1]`, `json: cannot unmarshal array into Go value of type shaper.guardHookInput`},
		"a string":                 {`"Bash"`, `json: cannot unmarshal string into Go value of type shaper.guardHookInput`},
		"a number":                 {`7`, `json: cannot unmarshal number into Go value of type shaper.guardHookInput`},
		"true":                     {`true`, `json: cannot unmarshal bool into Go value of type shaper.guardHookInput`},
		"tool_name a number":       {`{"tool_name":7}`, `json: cannot unmarshal number into Go struct field guardHookInput.tool_name of type string`},
		"tool_name an object":      {`{"tool_name":{}}`, `json: cannot unmarshal object into Go struct field guardHookInput.tool_name of type string`},
		"tool_name a boolean":      {`{"tool_name":true}`, `json: cannot unmarshal bool into Go struct field guardHookInput.tool_name of type string`},
		"tool_input a string":      {`{"tool_input":"ls"}`, `json: cannot unmarshal string into Go struct field guardHookInput.tool_input of type ` + toolInputType},
		"a good name, a bad input": {`{"tool_name":"Bash","tool_input":"ls"}`, `json: cannot unmarshal string into Go struct field guardHookInput.tool_input of type ` + toolInputType},
		"tool_input an array":      {`{"tool_input":[1]}`, `json: cannot unmarshal array into Go struct field guardHookInput.tool_input of type ` + toolInputType},
		"tool_input a number":      {`{"tool_input":5}`, `json: cannot unmarshal number into Go struct field guardHookInput.tool_input of type ` + toolInputType},
		"command a number":         {`{"tool_input":{"command":5}}`, `json: cannot unmarshal number into Go struct field .tool_input.command of type string`},
		"command an array":         {`{"tool_input":{"command":["ls"]}}`, `json: cannot unmarshal array into Go struct field .tool_input.command of type string`},
		"file_path an object":      {`{"tool_input":{"file_path":{}}}`, `json: cannot unmarshal object into Go struct field .tool_input.file_path of type string`},
		"notebook_path a boolean":  {`{"tool_input":{"notebook_path":false}}`, `json: cannot unmarshal bool into Go struct field .tool_input.notebook_path of type string`},
		"two fields of wrong type": {`{"tool_name":7,"tool_input":{"command":5}}`, `json: cannot unmarshal number into Go struct field guardHookInput.tool_name of type string`},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := hookwire.DecodeToolCall([]byte(tc.data))
			if err == nil {
				t.Fatalf("DecodeToolCall(%q) = %+v, want an error", tc.data, got)
			}
			if got != (hookwire.ToolCall{}) {
				t.Errorf("DecodeToolCall(%q) returned %+v with its error, want the zero value", tc.data, got)
			}
			if err.Error() != tc.detail {
				t.Errorf("the detail of %q is %q, want %q", tc.data, err.Error(), tc.detail)
			}
		})
	}
}

func TestToolCallDetailKeepsTheErrorItWraps(t *testing.T) {
	_, err := hookwire.DecodeToolCall([]byte(`{"tool_name":7}`))
	var typeErr interface{ Unwrap() error }
	if !errors.As(err, &typeErr) || typeErr.Unwrap() == nil {
		t.Errorf("the error %v does not wrap the one of the JSON decoder", err)
	}
}

// --- DecodeCommand -------------------------------------------------------------

func TestCommandReadsTheCommandOfAnyTool(t *testing.T) {
	for name, tc := range map[string]struct{ data, want string }{
		"a Bash call":                        {`{"tool_name":"Bash","tool_input":{"command":"git status"}}`, "git status"},
		"another tool: the name is not used": {`{"tool_name":"Write","tool_input":{"command":"git status"}}`, "git status"},
		"no tool name":                       {`{"tool_input":{"command":"git status"}}`, "git status"},
		"no command":                         {`{"tool_name":"Bash"}`, ""},
		"a null command":                     {`{"tool_name":"Bash","tool_input":{"command":null}}`, ""},
		"a null tool_input":                  {`{"tool_name":"Bash","tool_input":null}`, ""},
		"null itself":                        {`null`, ""},
		"keys in another case":               {`{"TOOL_NAME":"Bash","TOOL_INPUT":{"COMMAND":"ls"}}`, "ls"},
		"the last of a repeated key":         {`{"tool_input":{"command":"ls","command":"pwd"}}`, "pwd"},
		"padding around the object":          {" \n{\"tool_input\":{\"command\":\"ls\"}}\n ", "ls"},
		"fields of the envelope, odd types":  {`{"cwd":5,"hook_event_name":7,"session_id":{},"tool_name":"Bash","tool_input":{"command":"ls"}}`, "ls"},
		"a file_path of the wrong type":      {`{"tool_name":"Bash","tool_input":{"command":"ls","file_path":5}}`, "ls"},
		"a notebook_path of the wrong type":  {`{"tool_name":"Bash","tool_input":{"command":"ls","notebook_path":[]}}`, "ls"},
		"any other field of the wrong type":  {`{"tool_name":"Bash","tool_input":{"command":"ls","timeout":"soon","description":{}}}`, "ls"},
		"text with escapes":                  {`{"tool_input":{"command":"echo \"a\"\n"}}`, "echo \"a\"\n"},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := hookwire.DecodeCommand([]byte(tc.data))
			if err != nil || got != tc.want {
				t.Errorf("DecodeCommand(%q) = %q, %v, want %q", tc.data, got, err, tc.want)
			}
		})
	}
}

func TestCommandRefusesWhatItCannotRead(t *testing.T) {
	for name, data := range map[string]string{
		"empty":                 ``,
		"white space":           " \n ",
		"not JSON":              `not json`,
		"a truncated object":    `{"tool_name":"Bash","tool_input":{"command":"ls"`,
		"an array":              `[{"tool_name":"Bash","tool_input":{"command":"ls"}}]`,
		"a string":              `"x"`,
		"two objects":           `{"tool_input":{"command":"ls"}}{}`,
		"text after the object": `{"tool_input":{"command":"ls"}} trailing`,
		"tool_name a number":    `{"tool_name":7,"tool_input":{"command":"ls"}}`,
		"tool_name an object":   `{"tool_name":{},"tool_input":{"command":"ls"}}`,
		"tool_input a string":   `{"tool_name":"Bash","tool_input":"ls"}`,
		"tool_input an array":   `{"tool_name":"Bash","tool_input":["ls"]}`,
		"tool_input a number":   `{"tool_name":"Bash","tool_input":5}`,
		"command a number":      `{"tool_name":"Bash","tool_input":{"command":42}}`,
		"command an array":      `{"tool_name":"Bash","tool_input":{"command":["ls"]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := hookwire.DecodeCommand([]byte(data)); err == nil {
				t.Errorf("DecodeCommand(%q) = %q, want an error", data, got)
			}
		})
	}
}

// --- ExitReply -------------------------------------------------------------------

func TestExitReplySaysAllowAndBlockByTheExitStatus(t *testing.T) {
	if hookwire.ExitAllow != 0 || hookwire.ExitBlock != 2 {
		t.Errorf("the exit statuses are %d and %d, want 0 (allow) and 2 (block, the one Claude Code reads as a denial)", hookwire.ExitAllow, hookwire.ExitBlock)
	}
	if got := (hookwire.ExitReply{}).Code(); got != hookwire.ExitAllow {
		t.Errorf("an allow exits %d, want %d", got, hookwire.ExitAllow)
	}
	if got := (hookwire.ExitReply{Block: true, Message: "no"}).Code(); got != hookwire.ExitBlock {
		t.Errorf("a block exits %d, want %d", got, hookwire.ExitBlock)
	}
}

// What a hook of this kind says goes to stderr, which Claude Code feeds to the model on a block:
// the message and a line break, and nothing for no message.
func TestExitReplyWritesItsMessageAsALine(t *testing.T) {
	if got := string((hookwire.ExitReply{Block: true, Message: "denied: why"}).Stderr()); got != "denied: why\n" {
		t.Errorf("Stderr() = %q, want the message and a line break", got)
	}
	if got := (hookwire.ExitReply{}).Stderr(); len(got) != 0 {
		t.Errorf("an allow wrote %q, want nothing", got)
	}
	if got := string((hookwire.ExitReply{Block: true, Message: "two\nlines\n"}).Stderr()); got != "two\nlines\n\n" {
		t.Errorf("Stderr() = %q, want the message as it is and one line break more", got)
	}
	// A block with no message is still a block, and says nothing.
	r := hookwire.ExitReply{Block: true}
	if r.Code() != hookwire.ExitBlock || len(r.Stderr()) != 0 {
		t.Errorf("a block with no message = exit %d, %q, want exit 2 and nothing", r.Code(), r.Stderr())
	}
	// A message on an allow is still said: the caller decides what it says.
	if got := string((hookwire.ExitReply{Message: "note"}).Stderr()); got != "note\n" {
		t.Errorf("a message on an allow = %q, want it said", got)
	}
	if strings.Contains(string((hookwire.ExitReply{Block: true, Message: "x"}).Stderr()), "\r") {
		t.Error("Stderr() writes a carriage return")
	}
}
