package hookwire_test

// The SessionStart event and the working directory of an Agent call (C23 M6,
// docs/architecture/adr-c23-m6-plan-instructions.md): the projection hook answers SessionStart
// with the memory instructions as additionalContext, and gate-task finds the repository whose
// binding gives a sub-agent the same block from the cwd of the call.

import (
	"encoding/json"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/hookwire"
)

func TestTheSessionStartEventIsTheNameClaudeCodeSends(t *testing.T) {
	if hookwire.EventSessionStart != "SessionStart" {
		t.Errorf("EventSessionStart = %q, want SessionStart", hookwire.EventSessionStart)
	}
}

// SessionStart reads the envelope the two other events read: the event and an absolute cwd, the
// source (startup, resume, clear, compact) and the rest ignored.
func TestSessionStartKeepsOnlyTheEventAndTheAbsoluteWorkingDirectory(t *testing.T) {
	for name, tc := range map[string]struct {
		data    string
		want    hookwire.SessionStart
		wantErr bool
	}{
		"the shape Claude Code sends": {data: `{"session_id":"s","transcript_path":"/t.jsonl","cwd":"/home/u/repo/./sub","hook_event_name":"SessionStart","source":"compact","model":7}`,
			want: hookwire.SessionStart{Event: "SessionStart", Cwd: "/home/u/repo/sub"}},
		"a relative cwd is missing":  {data: `{"cwd":"repo"}`, want: hookwire.SessionStart{}},
		"no field":                   {data: `{}`, want: hookwire.SessionStart{}},
		"a cwd of the wrong type":    {data: `{"cwd":7}`, wantErr: true},
		"not an object":              {data: `[]`, wantErr: true},
		"an event of the wrong type": {data: `{"hook_event_name":7}`, wantErr: true},
		"two objects":                {data: `{}{}`, wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := hookwire.DecodeSessionStart([]byte(tc.data))
			if (err != nil) != tc.wantErr || got != tc.want {
				t.Errorf("DecodeSessionStart() = %+v, %v, want %+v, error=%v", got, err, tc.want, tc.wantErr)
			}
		})
	}
}

func TestSessionStartReplyPutsTheContextInsideHookSpecificOutput(t *testing.T) {
	out, err := hookwire.SessionStartReply{Context: "line one\nline <two> & \"three\""}.Encode()
	if err != nil {
		t.Fatal(err)
	}
	want := `{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"line one\nline <two> & \"three\""}}` + "\n"
	if string(out) != want {
		t.Errorf("a context = %q, want %q", out, want)
	}
	if !json.Valid(out) {
		t.Errorf("%q is not JSON", out)
	}
	if out, err := (hookwire.SessionStartReply{}).Encode(); err != nil || len(out) != 0 {
		t.Errorf("an empty reply = %q, %v, want no output", out, err)
	}
}

// The cwd of an Agent call is read leniently: the decoder never refused a cwd of any type, and a
// call it could rewrite before stays one it can rewrite. An absolute path is cleaned; anything
// else is no directory.
func TestAgentCallReadsAnAbsoluteWorkingDirectoryAndRefusesNothingForIt(t *testing.T) {
	for raw, want := range map[string]string{
		`"/home/u/repo/./sub/"`: "/home/u/repo/sub",
		`"repo"`:                "",
		`5`:                     "",
		`{"a":1}`:               "",
		`null`:                  "",
	} {
		got, err := hookwire.DecodeAgentCall([]byte(`{"cwd":` + raw + `,"tool_input":{"subagent_type":"sdd-apply","prompt":"p"}}`))
		if err != nil || got.Cwd != want || got.Prompt != "p" {
			t.Errorf("cwd %s: DecodeAgentCall() = %+v, %v, want cwd %q and the call", raw, got, err, want)
		}
	}
	if got, err := hookwire.DecodeAgentCall([]byte(`{"tool_input":{"prompt":"p"}}`)); err != nil || got.Cwd != "" {
		t.Errorf("no cwd: DecodeAgentCall() = %+v, %v, want no directory", got, err)
	}
}
