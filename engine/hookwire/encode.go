package hookwire

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// DenyFallbackReason is the reason of a denial that has none, so a denial never reaches the model
// and the user without a word. A policy that has a reason of its own for that state words it
// itself; this is only the last resort.
const DenyFallbackReason = "this tool call was denied by a hook."

// --- PreToolUse: deny or warn --------------------------------------------------------------

// PreToolUseReply is what a PreToolUse hook tells Claude Code about a call it will not rewrite:
// to deny it, with the reason, and/or to show the user a warning. Nothing here ever says
// "allow": that would bypass the normal permission flow of Claude Code, so an allowed call is
// answered with nothing, or only a warning.
type PreToolUseReply struct {
	// Deny is true to deny the call. Reason then says why and how to proceed.
	Deny   bool
	Reason string
	// Warning is one line for the user.
	Warning string
}

// preToolUseOutput is the JSON object a PreToolUse hook prints to deny a call: the decision and
// its reason sit inside hookSpecificOutput, next to the event name, and the hook exits 0 (the
// JSON decides).
type preToolUseOutput struct {
	HookSpecificOutput *preToolUseSpecific `json:"hookSpecificOutput,omitempty"`
	SystemMessage      string              `json:"systemMessage,omitempty"`
}

type preToolUseSpecific struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision"`
	PermissionDecisionReason string `json:"permissionDecisionReason"`
}

// Encode renders r as the one JSON object a PreToolUse hook writes to stdout, followed by a line
// break. A denial is {"hookSpecificOutput":{"hookEventName":"PreToolUse",
// "permissionDecision":"deny","permissionDecisionReason":"..."}}; a warning is a systemMessage.
// An allow with no warning yields no output at all, which Claude Code reads as "nothing to say".
// The JSON keeps <, > and & as they are, so the transcript stays readable.
func (r PreToolUseReply) Encode() ([]byte, error) {
	if !r.Deny && r.Warning == "" {
		return nil, nil
	}
	out := preToolUseOutput{SystemMessage: r.Warning}
	if r.Deny {
		reason := r.Reason
		if reason == "" {
			reason = DenyFallbackReason
		}
		out.HookSpecificOutput = &preToolUseSpecific{
			HookEventName:            EventPreToolUse,
			PermissionDecision:       "deny",
			PermissionDecisionReason: reason,
		}
	}
	return encodeLine(out)
}

// --- UserPromptSubmit: context and warning -------------------------------------------------

// PromptReply is what a UserPromptSubmit hook tells Claude Code: text to add to the session's
// context and/or a warning for the user.
type PromptReply struct {
	Context string
	Warning string
}

// userPromptSubmitOutput is the JSON object a UserPromptSubmit hook prints. additionalContext
// must sit inside hookSpecificOutput, next to the event name: at the top level Claude Code
// ignores it without a word.
type userPromptSubmitOutput struct {
	HookSpecificOutput *userPromptSubmitSpecific `json:"hookSpecificOutput,omitempty"`
	SystemMessage      string                    `json:"systemMessage,omitempty"`
}

type userPromptSubmitSpecific struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext"`
}

// Encode renders r as the one JSON object a UserPromptSubmit hook writes to stdout, followed by
// a line break: the context as additionalContext inside hookSpecificOutput, and the warning as
// systemMessage. A part that is empty is left out, and a reply with neither yields no output at
// all, which Claude Code reads as "nothing to add". The JSON keeps <, > and & as they are.
func (r PromptReply) Encode() ([]byte, error) {
	if r.Context == "" && r.Warning == "" {
		return nil, nil
	}
	out := userPromptSubmitOutput{SystemMessage: r.Warning}
	if r.Context != "" {
		out.HookSpecificOutput = &userPromptSubmitSpecific{HookEventName: EventUserPromptSubmit, AdditionalContext: r.Context}
	}
	return encodeLine(out)
}

// encodeLine writes v as JSON followed by a line break, without escaping <, > and &.
func encodeLine(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("hookwire: encode hook output: %w", err)
	}
	return buf.Bytes(), nil
}

// --- PreToolUse: let a call through, rewritten or not ---------------------------------------

// PassThrough is the answer that lets a call go ahead unchanged: an empty JSON object, which
// Claude Code reads as "no change".
func PassThrough() []byte { return []byte("{}\n") }

// hookResponse is the PreToolUse reply that carries a rewritten tool input. hookEventName and
// permissionDecision are required by Claude Code: without them, updatedInput is silently
// ignored.
type hookResponse struct {
	HookSpecificOutput hookSpecificOutput `json:"hookSpecificOutput"`
}

type hookSpecificOutput struct {
	HookEventName      string       `json:"hookEventName"`
	PermissionDecision string       `json:"permissionDecision"`
	UpdatedInput       updatedInput `json:"updatedInput"`
}

// updatedInput is the full echo of the tool input with only the prompt changed. Every field of
// the original must be present: Claude Code rejects the reply if one the tool requires, such as
// description, is missing.
type updatedInput struct {
	Description  string  `json:"description"`
	Prompt       string  `json:"prompt"`
	SubagentType string  `json:"subagent_type"`
	Model        *string `json:"model,omitempty"`
}

// UpdatedInput is the reply that lets the call go ahead with prompt in place of its own: the
// whole call is echoed, with a model only if the call had one. Unlike the other replies it is
// written the way json.Marshal writes it, which escapes <, > and & (and U+2028 and U+2029); it
// always was, and Claude Code reads the same JSON either way.
func (a AgentCall) UpdatedInput(prompt string) ([]byte, error) {
	resp := hookResponse{HookSpecificOutput: hookSpecificOutput{
		HookEventName:      EventPreToolUse,
		PermissionDecision: "allow",
		UpdatedInput: updatedInput{
			Description:  a.Description,
			Prompt:       prompt,
			SubagentType: a.SubagentType,
			Model:        a.Model,
		},
	}}
	b, err := json.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("hookwire: encode updated input: %w", err)
	}
	return append(b, '\n'), nil
}
