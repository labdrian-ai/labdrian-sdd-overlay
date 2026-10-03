package hookwire_test

// Tests for the bytes the encoders write. They are the bytes Claude Code reads, so each is
// compared as a whole, not by parts: a key that moves, a space that appears or an escape that
// is written differently is a change of the contract.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/hookwire"
)

func TestPreToolUseReplyOfADenial(t *testing.T) {
	out, err := hookwire.PreToolUseReply{Deny: true, Reason: "workflow wf-1 <of> project proj-1 & more is paused"}.Encode()
	if err != nil {
		t.Fatal(err)
	}
	// The JSON keeps <, > and & as they are, so the transcript stays readable.
	want := `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"workflow wf-1 <of> project proj-1 & more is paused"}}` + "\n"
	if string(out) != want {
		t.Errorf("a denial = %q, want %q", out, want)
	}
}

func TestPreToolUseReplyOfADenialWithAWarning(t *testing.T) {
	out, err := hookwire.PreToolUseReply{Deny: true, Reason: "no", Warning: "w"}.Encode()
	if err != nil {
		t.Fatal(err)
	}
	want := `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"no"},"systemMessage":"w"}` + "\n"
	if string(out) != want {
		t.Errorf("a denial with a warning = %q, want %q", out, want)
	}
}

// An allow is never written as permissionDecision allow, which would bypass the normal
// permission flow of Claude Code: it is nothing, or only a warning.
func TestPreToolUseReplyOfAnAllowIsNothingOrOnlyAWarning(t *testing.T) {
	out, err := hookwire.PreToolUseReply{}.Encode()
	if err != nil || len(out) != 0 {
		t.Errorf("a plain allow = %q, %v, want nothing", out, err)
	}
	out, err = hookwire.PreToolUseReply{Warning: "labdrian: careful"}.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"systemMessage":"labdrian: careful"}` + "\n"; string(out) != want {
		t.Errorf("an allow with a warning = %q, want %q", out, want)
	}
	if out, _ := (hookwire.PreToolUseReply{Reason: "ignored"}).Encode(); len(out) != 0 {
		t.Errorf("a reason without a denial = %q, want nothing", out)
	}
}

// A denial without a reason would leave the model and the user guessing, so it says that it
// is a denial.
func TestPreToolUseReplyOfADenialAlwaysCarriesAReason(t *testing.T) {
	out, err := hookwire.PreToolUseReply{Deny: true}.Encode()
	if err != nil {
		t.Fatal(err)
	}
	want := `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"` + hookwire.DenyFallbackReason + `"}}` + "\n"
	if string(out) != want {
		t.Errorf("a denial with no reason = %q, want %q", out, want)
	}
	if hookwire.DenyFallbackReason == "" {
		t.Error("DenyFallbackReason is empty")
	}
}

func TestPromptReplyPutsTheContextInsideHookSpecificOutput(t *testing.T) {
	ctx := "line one\nline two with <angle brackets> & \"quotes\""
	out, err := hookwire.PromptReply{Context: ctx}.Encode()
	if err != nil {
		t.Fatal(err)
	}
	want := `{"hookSpecificOutput":{"hookEventName":"UserPromptSubmit","additionalContext":"line one\nline two with <angle brackets> & \"quotes\""}}` + "\n"
	if string(out) != want {
		t.Errorf("a context = %q, want %q", out, want)
	}
	// additionalContext at the top level would be ignored by Claude Code without a word.
	var top map[string]json.RawMessage
	if err := json.Unmarshal(out, &top); err != nil || top["additionalContext"] != nil {
		t.Errorf("output %s has additionalContext at the top level", out)
	}
}

func TestPromptReplyCarriesAWarningAsASystemMessage(t *testing.T) {
	out, err := hookwire.PromptReply{Warning: "labdrian: something is wrong"}.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"systemMessage":"labdrian: something is wrong"}` + "\n"; string(out) != want {
		t.Errorf("a warning = %q, want %q", out, want)
	}
	out, err = hookwire.PromptReply{Context: "ctx", Warning: "warn"}.Encode()
	if err != nil {
		t.Fatal(err)
	}
	want := `{"hookSpecificOutput":{"hookEventName":"UserPromptSubmit","additionalContext":"ctx"},"systemMessage":"warn"}` + "\n"
	if string(out) != want {
		t.Errorf("a context and a warning = %q, want %q", out, want)
	}
}

func TestPromptReplyIsEmptyWhenThereIsNothingToSay(t *testing.T) {
	out, err := hookwire.PromptReply{}.Encode()
	if err != nil || len(out) != 0 {
		t.Errorf("an empty reply = %q, %v, want no output, which Claude Code reads as nothing to add", out, err)
	}
}

// --- the Agent tool ------------------------------------------------------------

func TestUpdatedInputEchoesTheWholeToolInputWithTheNewPrompt(t *testing.T) {
	model := "sonnet"
	call := hookwire.AgentCall{Description: "d", Prompt: "old", SubagentType: "sdd-apply", Model: &model}
	out, err := call.UpdatedInput("new\nprompt")
	if err != nil {
		t.Fatal(err)
	}
	// hookEventName and permissionDecision are what make Claude Code take updatedInput at all,
	// and updatedInput must carry every field of the tool input or Claude Code rejects it.
	want := `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow","updatedInput":{"description":"d","prompt":"new\nprompt","subagent_type":"sdd-apply","model":"sonnet"}}}` + "\n"
	if string(out) != want {
		t.Errorf("UpdatedInput() = %q, want %q", out, want)
	}
}

func TestUpdatedInputLeavesOutAModelThatWasNotThere(t *testing.T) {
	out, err := hookwire.AgentCall{Description: "d", SubagentType: "sdd-apply"}.UpdatedInput("p")
	if err != nil {
		t.Fatal(err)
	}
	want := `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow","updatedInput":{"description":"d","prompt":"p","subagent_type":"sdd-apply"}}}` + "\n"
	if string(out) != want {
		t.Errorf("UpdatedInput() = %q, want %q", out, want)
	}
	empty := ""
	out, err = hookwire.AgentCall{Model: &empty}.UpdatedInput("p")
	if err != nil || !strings.Contains(string(out), `"model":""`) {
		t.Errorf("a model that is empty = %q, %v, want it echoed empty", out, err)
	}
}

// updatedInput is written the way json.Marshal writes it, which escapes <, > and & (and
// U+2028 and U+2029): it always was, and Claude Code reads the same JSON either way. The
// other replies keep those characters as they are.
func TestUpdatedInputIsWrittenWithHTMLEscapes(t *testing.T) {
	out, err := hookwire.AgentCall{Description: "d <x> & y", SubagentType: "t"}.UpdatedInput("a<b>&c ")
	if err != nil {
		t.Fatal(err)
	}
	// The escapes are spelled in pieces so no tooling decodes them in this source.
	esc := func(code string) string { return `\` + "u" + code }
	want := `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow","updatedInput":{"description":"d ` +
		esc("003c") + `x` + esc("003e") + ` ` + esc("0026") + ` y","prompt":"a` + esc("003c") + `b` + esc("003e") + esc("0026") + `c` + esc("2028") +
		`","subagent_type":"t"}}}` + "\n"
	if string(out) != want {
		t.Errorf("UpdatedInput() = %q, want %q", out, want)
	}
}

func TestPassThroughIsAnEmptyObject(t *testing.T) {
	if got := string(hookwire.PassThrough()); got != "{}\n" {
		t.Errorf("PassThrough() = %q, want {} and a line break", got)
	}
}
