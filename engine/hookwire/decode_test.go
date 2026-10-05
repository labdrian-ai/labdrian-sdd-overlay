package hookwire_test

// Tests for what the decoders accept and refuse. Each decoder reads exactly the fields its
// consumer read before the wire format had one home, with the type each one expects: a field
// a decoder does not read can be of any type, one it reads must be of the type it expects,
// and what is refused is refused the way it always was. The golden files of the hooks in
// engine/cmd pin the same rules end to end.

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/hookwire"
)

func TestTheEventNamesAreTheOnesClaudeCodeSends(t *testing.T) {
	if hookwire.EventPreToolUse != "PreToolUse" || hookwire.EventUserPromptSubmit != "UserPromptSubmit" {
		t.Errorf("events = %q and %q, want PreToolUse and UserPromptSubmit", hookwire.EventPreToolUse, hookwire.EventUserPromptSubmit)
	}
	if hookwire.MaxEnvelopeBytes != 1<<20 {
		t.Errorf("MaxEnvelopeBytes = %d, want 1 MiB", hookwire.MaxEnvelopeBytes)
	}
}

// --- the rules both events share --------------------------------------------

// TestBothEventsAgreeOnTheSharedRules: the two decoders read the same envelope (size bound,
// one JSON object, an absolute cleaned cwd), so they accept and refuse the same documents
// in the same way.
func TestBothEventsAgreeOnTheSharedRules(t *testing.T) {
	oversized := `{"cwd":"/r","prompt":"` + strings.Repeat("x", hookwire.MaxEnvelopeBytes) + `"}`
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
		"over the size bound":     {data: oversized, wantErr: true, tooBig: true},
	} {
		t.Run(name, func(t *testing.T) {
			p, pErr := hookwire.DecodeUserPromptSubmit([]byte(tc.data))
			g, gErr := hookwire.DecodePreToolUse([]byte(tc.data))
			if (pErr != nil) != tc.wantErr || (gErr != nil) != tc.wantErr {
				t.Fatalf("errors = %v and %v, want error=%v from both", pErr, gErr, tc.wantErr)
			}
			if tc.wantErr {
				if tc.tooBig && (!errors.Is(pErr, hookwire.ErrTooLarge) || !errors.Is(gErr, hookwire.ErrTooLarge)) {
					t.Errorf("errors %v and %v, want both to wrap ErrTooLarge", pErr, gErr)
				}
				return
			}
			if p.Cwd != tc.wantCwd || g.Cwd != tc.wantCwd {
				t.Errorf("cwd = %q and %q, want %q from both", p.Cwd, g.Cwd, tc.wantCwd)
			}
		})
	}
}

func TestTheSizeBoundIsInclusive(t *testing.T) {
	pad := func(size int) []byte {
		prefix, suffix := `{"pad":"`, `"}`
		return []byte(prefix + strings.Repeat("x", size-len(prefix)-len(suffix)) + suffix)
	}
	for name, decode := range map[string]func([]byte) error{
		"UserPromptSubmit": func(b []byte) error { _, err := hookwire.DecodeUserPromptSubmit(b); return err },
		"PreToolUse":       func(b []byte) error { _, err := hookwire.DecodePreToolUse(b); return err },
	} {
		t.Run(name, func(t *testing.T) {
			if err := decode(pad(hookwire.MaxEnvelopeBytes)); err != nil {
				t.Errorf("an input of exactly the bound = %v, want it accepted", err)
			}
			if err := decode(pad(hookwire.MaxEnvelopeBytes + 1)); !errors.Is(err, hookwire.ErrTooLarge) {
				t.Errorf("an input one byte over the bound = %v, want ErrTooLarge", err)
			}
		})
	}
}

// --- UserPromptSubmit --------------------------------------------------------

func TestUserPromptSubmitKeepsOnlyTheEventAndTheAbsoluteWorkingDirectory(t *testing.T) {
	// The shape Claude Code sends, plus fields it might add: the input is not ours, so
	// unknown fields are ignored rather than refused.
	data := `{
		"session_id": "abc123",
		"transcript_path": "/home/u/.claude/projects/x/abc123.jsonl",
		"cwd": "/home/u/repo/sub",
		"permission_mode": "default",
		"hook_event_name": "UserPromptSubmit",
		"prompt": "do the thing",
		"a_future_field": {"nested": [1, 2, {"deep": null}]}
	}`
	got, err := hookwire.DecodeUserPromptSubmit([]byte(data))
	if err != nil {
		t.Fatalf("DecodeUserPromptSubmit() = %v, want nil", err)
	}
	if want := (hookwire.UserPromptSubmit{Event: "UserPromptSubmit", Cwd: "/home/u/repo/sub"}); got != want {
		t.Fatalf("DecodeUserPromptSubmit() = %+v, want %+v", got, want)
	}
}

// TestUserPromptSubmitIgnoresTheSessionAndThePromptEntirely: the projection must not depend
// on either, so neither is read, not even to be validated. A prompt or session id of the
// wrong type is not an error.
func TestUserPromptSubmitIgnoresTheSessionAndThePromptEntirely(t *testing.T) {
	base := `"hook_event_name":"UserPromptSubmit","cwd":"/r"`
	for name, extra := range map[string]string{
		"no session, no prompt":     ``,
		"a session and a prompt":    `,"session_id":"s1","prompt":"hello"`,
		"another session":           `,"session_id":"s2","prompt":"a different prompt"`,
		"a prompt of another type":  `,"prompt":{"unexpected":["structure"]}`,
		"a session of another type": `,"session_id":12345`,
		"a huge prompt":             `,"prompt":"` + strings.Repeat("x", 500_000) + `"`,
	} {
		t.Run(name, func(t *testing.T) {
			got, err := hookwire.DecodeUserPromptSubmit([]byte("{" + base + extra + "}"))
			if err != nil || got != (hookwire.UserPromptSubmit{Event: "UserPromptSubmit", Cwd: "/r"}) {
				t.Fatalf("DecodeUserPromptSubmit() = %+v, %v, want the same event and cwd whatever the session and prompt are", got, err)
			}
		})
	}
}

func TestUserPromptSubmitAcceptsOnlyAnAbsoluteWorkingDirectory(t *testing.T) {
	for _, tt := range []struct{ name, cwd, want string }{
		{"absolute", `"/home/u/repo"`, "/home/u/repo"},
		{"absolute with dot segments is cleaned", `"/home/u/../v/repo"`, "/home/v/repo"},
		{"absolute with a trailing slash and repeated separators is cleaned", `"/home//u/repo/"`, "/home/u/repo"},
		{"relative", `"repo/sub"`, ""},
		{"dot", `"."`, ""},
		{"dot dot", `".."`, ""},
		{"empty", `""`, ""},
		{"null", `null`, ""},
		{"absent", ``, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			data := `{"hook_event_name":"UserPromptSubmit"`
			if tt.cwd != "" {
				data += `,"cwd":` + tt.cwd
			}
			got, err := hookwire.DecodeUserPromptSubmit([]byte(data + "}"))
			if err != nil || got.Cwd != tt.want {
				t.Fatalf("DecodeUserPromptSubmit() = %+v, %v, want cwd %q", got, err, tt.want)
			}
		})
	}
}

func TestUserPromptSubmitAcceptsWhitespaceAndMissingFields(t *testing.T) {
	for _, data := range []string{"{}", " \n {} \n", "\t{\"cwd\":\"/r\"}\n"} {
		if _, err := hookwire.DecodeUserPromptSubmit([]byte(data)); err != nil {
			t.Errorf("DecodeUserPromptSubmit(%q) = %v, want nil", data, err)
		}
	}
	got, err := hookwire.DecodeUserPromptSubmit([]byte(`{"cwd":"/r"}`))
	if err != nil || got.Event != "" || got.Cwd != "/r" {
		t.Errorf("DecodeUserPromptSubmit() = %+v, %v, want no event name and cwd /r", got, err)
	}
}

func TestUserPromptSubmitRefusesWhatIsNotAJSONObject(t *testing.T) {
	for name, data := range map[string]string{
		"empty":                    "",
		"only whitespace":          " \n\t ",
		"an array":                 `[{"cwd":"/r"}]`,
		"a string":                 `"UserPromptSubmit"`,
		"a number":                 `42`,
		"null":                     `null`,
		"true":                     `true`,
		"plain text":               `not json`,
		"a truncated object":       `{"hook_event_name":"UserPromptSubmit","cwd":"/r"`,
		"data after the object":    `{"cwd":"/r"} {"cwd":"/s"}`,
		"garbage after the object": `{"cwd":"/r"}x`,
		"a cwd of another type":    `{"cwd":42}`,
		"an event of another type": `{"hook_event_name":["UserPromptSubmit"]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := hookwire.DecodeUserPromptSubmit([]byte(data)); err == nil {
				t.Fatalf("DecodeUserPromptSubmit(%q) = %+v, nil, want an error", data, got)
			}
		})
	}
}

// --- PreToolUse --------------------------------------------------------------

func TestPreToolUseKeepsOnlyTheEventTheDirectoryTheToolAndItsArguments(t *testing.T) {
	data := `{
		"session_id": "abc123",
		"transcript_path": "/home/u/.claude/projects/x/abc123.jsonl",
		"cwd": "/home/u/../v/repo/",
		"permission_mode": "default",
		"hook_event_name": "PreToolUse",
		"tool_name": "mcp__longterm-mem__query",
		"tool_input": {"query":"x","project":"proj-1","nested":[1,{"a":null}]},
		"tool_use_id": "toolu_1",
		"a_future_field": {"x": [1, 2]}
	}`
	got, err := hookwire.DecodePreToolUse([]byte(data))
	if err != nil {
		t.Fatalf("DecodePreToolUse() = %v, want nil", err)
	}
	if got.Event != "PreToolUse" || got.Cwd != "/home/v/repo" || got.Tool != "mcp__longterm-mem__query" {
		t.Fatalf("DecodePreToolUse() = %+v, want the event, the cleaned cwd and the tool name", got)
	}
	if q := got.Query(); !q.Named || q.Project != "proj-1" {
		t.Errorf("Query() = %+v, want the project of the call", q)
	}
}

func TestPreToolUseAcceptsAnyToolInputAndMissingFields(t *testing.T) {
	for _, tt := range []struct{ name, data string }{
		{"an object", `{"tool_name":"Edit","tool_input":{"a":1}}`},
		{"a string", `{"tool_name":"Edit","tool_input":"x"}`},
		{"an array", `{"tool_name":"Edit","tool_input":[1]}`},
		{"null", `{"tool_name":"Edit","tool_input":null}`},
		{"absent", `{"tool_name":"Edit"}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := hookwire.DecodePreToolUse([]byte(tt.data))
			if err != nil || got.Tool != "Edit" {
				t.Fatalf("DecodePreToolUse() = %+v, %v, want tool Edit", got, err)
			}
		})
	}
	got, err := hookwire.DecodePreToolUse([]byte(" \n{}\n "))
	if err != nil || got.Event != "" || got.Cwd != "" || got.Tool != "" || got.Query() != (hookwire.QueryArguments{}) {
		t.Errorf("DecodePreToolUse({}) = %+v, %v, want the zero value", got, err)
	}
}

func TestPreToolUseAcceptsOnlyAnAbsoluteWorkingDirectory(t *testing.T) {
	for _, tt := range []struct{ name, cwd, want string }{
		{"absolute", `"/home/u/repo"`, "/home/u/repo"},
		{"absolute with dot segments is cleaned", `"/home/u/../v/repo"`, "/home/v/repo"},
		{"relative", `"repo/sub"`, ""},
		{"empty", `""`, ""},
		{"null", `null`, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := hookwire.DecodePreToolUse([]byte(`{"cwd":` + tt.cwd + `}`))
			if err != nil || got.Cwd != tt.want {
				t.Fatalf("DecodePreToolUse() = %+v, %v, want cwd %q", got, err, tt.want)
			}
		})
	}
}

func TestPreToolUseRefusesWhatIsNotAUsableObject(t *testing.T) {
	for name, data := range map[string]string{
		"empty":                          "",
		"whitespace":                     "  \n",
		"a string":                       `"PreToolUse"`,
		"an array":                       `[]`,
		"a number":                       `1`,
		"null":                           `null`,
		"not JSON":                       `{"tool_name":`,
		"trailing data":                  `{"tool_name":"Edit"} {"x":1}`,
		"a tool name that is a number":   `{"tool_name":7}`,
		"a tool name that is an object":  `{"tool_name":{"x":1}}`,
		"an event name that is a number": `{"hook_event_name":1}`,
		"a cwd that is a number":         `{"cwd":1}`,
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := hookwire.DecodePreToolUse([]byte(data)); err == nil {
				t.Errorf("DecodePreToolUse(%q) = %+v, want an error", data, got)
			}
		})
	}
}

// TestQueryArgumentsAreReadTheWayTheMemoryGateReadThem: Named says the arguments are a JSON
// object; Project is what it names as its project, which is empty when the argument is
// missing, null, not text, or empty. A key is matched exactly and the last of two wins.
func TestQueryArgumentsAreReadTheWayTheMemoryGateReadThem(t *testing.T) {
	for _, tt := range []struct {
		name      string
		toolInput string
		want      hookwire.QueryArguments
	}{
		{"a project", `{"query":"q","project":"proj-1"}`, hookwire.QueryArguments{Named: true, Project: "proj-1"}},
		{"no project", `{"query":"q"}`, hookwire.QueryArguments{Named: true}},
		{"an empty project", `{"project":""}`, hookwire.QueryArguments{Named: true}},
		{"a null project", `{"project":null}`, hookwire.QueryArguments{Named: true}},
		{"a project that is a number", `{"project":12}`, hookwire.QueryArguments{Named: true}},
		{"a project that is a list", `{"project":["proj-1"]}`, hookwire.QueryArguments{Named: true}},
		{"a key in another case is another key", `{"PROJECT":"proj-1"}`, hookwire.QueryArguments{Named: true}},
		{"the last of two keys wins", `{"project":"a","project":"proj-1"}`, hookwire.QueryArguments{Named: true, Project: "proj-1"}},
		{"an empty object", `{}`, hookwire.QueryArguments{Named: true}},
		{"a string", `"proj-1"`, hookwire.QueryArguments{}},
		{"an array", `[1]`, hookwire.QueryArguments{}},
		{"a number", `5`, hookwire.QueryArguments{}},
		{"null", `null`, hookwire.QueryArguments{}},
		{"absent", ``, hookwire.QueryArguments{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			data := `{"tool_name":"mcp__longterm-mem__query"`
			if tt.toolInput != "" {
				data += `,"tool_input":` + tt.toolInput
			}
			got, err := hookwire.DecodePreToolUse([]byte(data + `}`))
			if err != nil {
				t.Fatalf("DecodePreToolUse() = %v", err)
			}
			if q := got.Query(); q != tt.want {
				t.Errorf("Query() = %+v, want %+v", q, tt.want)
			}
		})
	}
}

// --- the Agent tool ------------------------------------------------------------

func TestAgentCallKeepsTheFourFieldsOfTheAgentTool(t *testing.T) {
	got, err := hookwire.DecodeAgentCall([]byte(`{"session_id":"s","cwd":5,"hook_event_name":7,"tool_name":"Agent","tool_input":` +
		`{"description":"d","prompt":"p","subagent_type":"sdd-apply","model":"opus","run_in_background":true}}`))
	if err != nil {
		t.Fatalf("DecodeAgentCall() = %v, want nil: the fields it does not read can be of any type", err)
	}
	model := "opus"
	if got.Description != "d" || got.Prompt != "p" || got.SubagentType != "sdd-apply" || got.Model == nil || *got.Model != model {
		t.Errorf("DecodeAgentCall() = %+v, want description d, prompt p, subagent_type sdd-apply and model opus", got)
	}
}

// The decoder bounds its own input, as the decoders of the two events do: the caller reads at
// most MaxAgentCallBytes, and a caller that did not would still be refused here. An input cut
// short at the bound is not valid JSON and is refused for that, which is what 'gate-task'
// relies on (it reads exactly this many bytes).
func TestTheSizeBoundOfAnAgentCallIsInclusive(t *testing.T) {
	pad := func(size int) []byte {
		prefix, suffix := `{"tool_name":"Agent","tool_input":{"subagent_type":"sdd-apply","prompt":"`, `"}}`
		return []byte(prefix + strings.Repeat("x", size-len(prefix)-len(suffix)) + suffix)
	}
	got, err := hookwire.DecodeAgentCall(pad(hookwire.MaxAgentCallBytes))
	if err != nil || got.SubagentType != "sdd-apply" {
		t.Errorf("an input of exactly the bound = %+v, %v, want it read", got, err)
	}
	got, err = hookwire.DecodeAgentCall(pad(hookwire.MaxAgentCallBytes + 1))
	if !errors.Is(err, hookwire.ErrTooLarge) || got.SubagentType != "" || got.Prompt != "" {
		t.Errorf("an input one byte over the bound = %+v, %v, want the zero value and ErrTooLarge", got, err)
	}
	if err != nil && !strings.Contains(err.Error(), strconv.Itoa(hookwire.MaxAgentCallBytes)) {
		t.Errorf("the refusal %q does not say what the bound is", err)
	}
}

func TestAgentCallDoesNotConsultTheToolName(t *testing.T) {
	for _, name := range []string{`"tool_name":"Bash",`, ``} {
		got, err := hookwire.DecodeAgentCall([]byte(`{` + name + `"tool_input":{"subagent_type":"sdd-apply","prompt":"p"}}`))
		if err != nil || got.SubagentType != "sdd-apply" {
			t.Errorf("DecodeAgentCall(%q) = %+v, %v, want the call whatever the tool is named", name, got, err)
		}
	}
}

func TestAgentCallMatchesKeysWithoutRegardToCaseAndTheLastKeyWins(t *testing.T) {
	got, err := hookwire.DecodeAgentCall([]byte(`{"TOOL_INPUT":{"Description":"d","SUBAGENT_TYPE":"sdd-explore","subagent_type":"sdd-apply","Prompt":"p","MODEL":"opus"}}`))
	if err != nil || got.SubagentType != "sdd-apply" || got.Prompt != "p" || got.Description != "d" || got.Model == nil || *got.Model != "opus" {
		t.Errorf("DecodeAgentCall() = %+v, %v, want the fields found in any case, the last of a repeated key winning", got, err)
	}
}

func TestAgentCallRefusesWhatItCannotUse(t *testing.T) {
	const good = `"description":"d","subagent_type":"sdd-apply","prompt":"p"`
	for name, data := range map[string]string{
		"empty":                           ``,
		"white space":                     " \n\t ",
		"not JSON":                        `hello`,
		"an array":                        `[{"tool_input":{` + good + `}}]`,
		"a string":                        `"x"`,
		"two objects":                     `{"tool_input":{` + good + `}}{}`,
		"a truncated object":              `{"tool_input":{` + good,
		"tool_name of the wrong type":     `{"tool_name":7,"tool_input":{` + good + `}}`,
		"tool_name an object":             `{"tool_name":{},"tool_input":{` + good + `}}`,
		"tool_input missing":              `{"tool_name":"Agent"}`,
		"tool_input a string":             `{"tool_name":"Agent","tool_input":"x"}`,
		"tool_input a number":             `{"tool_name":"Agent","tool_input":5}`,
		"tool_input an array":             `{"tool_name":"Agent","tool_input":[1]}`,
		"description of the wrong type":   `{"tool_input":{"description":5,"subagent_type":"sdd-apply","prompt":"p"}}`,
		"prompt of the wrong type":        `{"tool_input":{"description":"d","subagent_type":"sdd-apply","prompt":5}}`,
		"subagent_type of the wrong type": `{"tool_input":{"description":"d","subagent_type":5,"prompt":"p"}}`,
		"model of the wrong type":         `{"tool_input":{"description":"d","subagent_type":"sdd-apply","prompt":"p","model":5}}`,
		"model an object":                 `{"tool_input":{"description":"d","subagent_type":"sdd-apply","prompt":"p","model":{}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := hookwire.DecodeAgentCall([]byte(data)); err == nil {
				t.Errorf("DecodeAgentCall(%q) = %+v, want an error", data, got)
			}
		})
	}
}

// What DecodeAgentCall accepts although there is nothing to act on: the policy decides, from
// an empty sub-agent type or an empty prompt, to leave the call alone.
func TestAgentCallAcceptsWhatThePolicyLeavesAlone(t *testing.T) {
	for name, data := range map[string]string{
		"tool_input null":      `{"tool_name":"Agent","tool_input":null}`,
		"an empty object":      `{"tool_input":{}}`,
		"an empty prompt":      `{"tool_input":{"subagent_type":"sdd-apply","prompt":""}}`,
		"a null prompt":        `{"tool_input":{"subagent_type":"sdd-apply","prompt":null}}`,
		"a null model":         `{"tool_input":{"subagent_type":"sdd-apply","prompt":"p","model":null}}`,
		"fields not read, odd": `{"cwd":5,"tool_input":{"subagent_type":"sdd-apply","prompt":"p","command":5,"run_in_background":"yes"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := hookwire.DecodeAgentCall([]byte(data)); err != nil {
				t.Errorf("DecodeAgentCall(%q) = %v, want nil", data, err)
			}
		})
	}
	got, err := hookwire.DecodeAgentCall([]byte(`{"tool_input":{"subagent_type":"sdd-apply","prompt":"p","model":null}}`))
	if err != nil || got.Model != nil {
		t.Errorf("a null model = %+v, %v, want no model", got, err)
	}
}
