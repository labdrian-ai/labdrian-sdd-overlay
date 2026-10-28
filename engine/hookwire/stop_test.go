package hookwire_test

// The Stop event (own-rdd T6a, S6): Claude Code runs a Stop hook when the agent is about to stop. The hook reads the
// session, the working directory and whether a stop hook already made the agent continue, and blocks the stop with a
// top-level decision and a reason, the shape Claude Code documents; an allow is no output at all.

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/hookwire"
)

func TestTheStopEventIsTheNameClaudeCodeSends(t *testing.T) {
	if hookwire.EventStop != "Stop" {
		t.Errorf("EventStop = %q, want Stop", hookwire.EventStop)
	}
}

// Stop reads the envelope the other events read, and three fields more or less: the event, an absolute cwd, the
// session, and stop_hook_active, which is true while the agent is continuing because a stop hook asked it to.
func TestStopKeepsTheSessionTheDirectoryAndWhetherAStopHookIsAlreadyContinuing(t *testing.T) {
	for name, tc := range map[string]struct {
		data    string
		want    hookwire.Stop
		wantErr bool
	}{
		"the shape Claude Code sends": {
			data: `{"session_id":"s-1","transcript_path":"/t.jsonl","cwd":"/home/u/repo/./sub","permission_mode":"default","hook_event_name":"Stop","stop_hook_active":true,"extra":[1]}`,
			want: hookwire.Stop{Event: "Stop", Cwd: "/home/u/repo/sub", SessionID: "s-1", Continuing: true},
		},
		"a stop hook that is not continuing":    {data: `{"session_id":"s-2","cwd":"/r","hook_event_name":"Stop","stop_hook_active":false}`, want: hookwire.Stop{Event: "Stop", Cwd: "/r", SessionID: "s-2"}},
		"no stop_hook_active is not continuing": {data: `{"session_id":"s","cwd":"/r"}`, want: hookwire.Stop{Cwd: "/r", SessionID: "s"}},
		"a relative cwd is missing":             {data: `{"cwd":"repo","session_id":"s"}`, want: hookwire.Stop{SessionID: "s"}},
		"no field":                              {data: `{}`, want: hookwire.Stop{}},
		"stop_hook_active of the wrong type":    {data: `{"stop_hook_active":"yes"}`, wantErr: true},
		"a session of the wrong type":           {data: `{"session_id":7}`, wantErr: true},
		"not an object":                         {data: `[]`, wantErr: true},
		"two objects":                           {data: `{}{}`, wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := hookwire.DecodeStop([]byte(tc.data))
			if (err != nil) != tc.wantErr || got != tc.want {
				t.Errorf("DecodeStop() = %+v, %v, want %+v, error=%v", got, err, tc.want, tc.wantErr)
			}
		})
	}
}

func TestStopRefusesAnInputOverTheBound(t *testing.T) {
	data := `{"cwd":"/r","padding":"` + strings.Repeat("x", hookwire.MaxEnvelopeBytes) + `"}`

	if _, err := hookwire.DecodeStop([]byte(data)); !errors.Is(err, hookwire.ErrTooLarge) {
		t.Errorf("DecodeStop() of an input over the bound = %v, want ErrTooLarge", err)
	}
}

func TestStopReplyBlocksWithADecisionAndAReasonAtTheTopLevel(t *testing.T) {
	out, err := hookwire.StopReply{Block: true, Reason: "review it:\nlabdrian review bind --base <a> --head \"b\" & more"}.Encode()
	if err != nil {
		t.Fatal(err)
	}

	want := `{"decision":"block","reason":"review it:\nlabdrian review bind --base <a> --head \"b\" & more"}` + "\n"
	if string(out) != want {
		t.Errorf("a block = %q, want %q", out, want)
	}
	var decoded map[string]string
	if !json.Valid(out) || json.Unmarshal(out, &decoded) != nil || decoded["decision"] != "block" {
		t.Errorf("%q is not the JSON object of a block", out)
	}
}

// A block never reaches the agent without a word, and an allow says nothing at all.
func TestStopReplyHasAReasonWhenItIsGivenNoneAndSaysNothingWhenItDoesNotBlock(t *testing.T) {
	out, err := hookwire.StopReply{Block: true}.Encode()
	if err != nil || !strings.Contains(string(out), `"decision":"block"`) || !strings.Contains(string(out), hookwire.StopFallbackReason) {
		t.Errorf("a block with no reason = %q, %v, want a block that says %q", out, err, hookwire.StopFallbackReason)
	}
	if out, err := (hookwire.StopReply{Reason: "ignored without a block"}).Encode(); err != nil || len(out) != 0 {
		t.Errorf("a reply that does not block = %q, %v, want no output", out, err)
	}
}
