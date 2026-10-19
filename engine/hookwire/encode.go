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

// contextOutput is the JSON object a hook that adds context prints, UserPromptSubmit and
// SessionStart alike. additionalContext must sit inside hookSpecificOutput, next to the event
// name: at the top level Claude Code ignores it without a word.
type contextOutput struct {
	HookSpecificOutput *contextSpecific `json:"hookSpecificOutput,omitempty"`
	SystemMessage      string           `json:"systemMessage,omitempty"`
}

type contextSpecific struct {
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
	out := contextOutput{SystemMessage: r.Warning}
	if r.Context != "" {
		out.HookSpecificOutput = &contextSpecific{HookEventName: EventUserPromptSubmit, AdditionalContext: r.Context}
	}
	return encodeLine(out)
}

// --- SessionStart: context -------------------------------------------------------------------

// SessionStartReply is what a SessionStart hook tells Claude Code: text to add to the session's
// context. It has no warning: the prompt hook already warns about what the session-start hook
// cannot say.
type SessionStartReply struct {
	Context string
}

// Encode renders r as the one JSON object a SessionStart hook writes to stdout, followed by a line
// break: {"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"..."}}, the
// shape of a prompt's context with the event's own name. An empty context yields no output at all.
func (r SessionStartReply) Encode() ([]byte, error) {
	if r.Context == "" {
		return nil, nil
	}
	return encodeLine(contextOutput{HookSpecificOutput: &contextSpecific{HookEventName: EventSessionStart, AdditionalContext: r.Context}})
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
// whole call is echoed, with a model only if the call had one. Like the other replies it keeps
// <, > and & as they are (Claude Code reads the same JSON either way; until Phase 9 batch 12a it
// escaped them), and U+2028 and U+2029 are escaped, as they are in every reply.
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
	return encodeLine(resp)
}
