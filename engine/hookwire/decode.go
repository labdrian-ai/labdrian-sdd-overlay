package hookwire

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
)

// --- the two events the projection answers -------------------------------------------------

// UserPromptSubmit is what is kept of the JSON a UserPromptSubmit hook receives: the event's
// name and the working directory of the session. Nothing else is read, in particular not the
// session and not the prompt: what a session is told does not depend on either, so every
// session, and every prompt, gets the same view.
type UserPromptSubmit struct {
	// Event is hook_event_name, or empty when the input has none.
	Event string
	// Cwd is the session's working directory as a cleaned absolute path, or empty when the
	// input has none or it is not an absolute path.
	Cwd string
}

// DecodeUserPromptSubmit reads a UserPromptSubmit hook's stdin. The input is Claude Code's, not
// ours, so it is decoded leniently: one JSON object no larger than MaxEnvelopeBytes, with the
// fields it does not name ignored (including ones a later Claude Code adds) and a relative cwd
// read as missing. A field it reads with a value of the wrong type is an error, and so is a
// document that is not exactly one object.
func DecodeUserPromptSubmit(data []byte) (UserPromptSubmit, error) {
	var wire struct {
		HookEventName string `json:"hook_event_name"`
		Cwd           string `json:"cwd"`
	}
	if err := decodeEnvelope(data, &wire); err != nil {
		return UserPromptSubmit{}, err
	}
	return UserPromptSubmit{Event: wire.HookEventName, Cwd: cleanCwd(wire.Cwd)}, nil
}

// PreToolUse is what is kept of the JSON a PreToolUse hook receives: the event's name, the
// working directory of the session, the tool's name, and the tool's input, which is read only
// by Query and only when asked: its shape depends on the tool. Nothing else is read, in
// particular not the session and not the tool call's id.
type PreToolUse struct {
	// Event is hook_event_name, or empty when the input has none.
	Event string
	// Cwd is the session's working directory as a cleaned absolute path, or empty when the
	// input has none or it is not an absolute path.
	Cwd string
	// Tool is tool_name, or empty when the input has none.
	Tool string

	// input is tool_input as it was sent, or empty when the input has none.
	input json.RawMessage
}

// DecodePreToolUse reads a PreToolUse hook's stdin, with the same rules as
// DecodeUserPromptSubmit and tool_name read as text. tool_input may be any JSON value.
func DecodePreToolUse(data []byte) (PreToolUse, error) {
	var wire struct {
		HookEventName string          `json:"hook_event_name"`
		Cwd           string          `json:"cwd"`
		ToolName      string          `json:"tool_name"`
		ToolInput     json.RawMessage `json:"tool_input"`
	}
	if err := decodeEnvelope(data, &wire); err != nil {
		return PreToolUse{}, err
	}
	return PreToolUse{Event: wire.HookEventName, Cwd: cleanCwd(wire.Cwd), Tool: wire.ToolName, input: wire.ToolInput}, nil
}

// QueryArguments is the input of a tool call read as the arguments of a memory query.
type QueryArguments struct {
	// Named is true when the input is a JSON object, whose arguments have names. It is false
	// for every other JSON value and for an input that is absent.
	Named bool
	// Project is the project the query names: the value of its "project" argument, which is
	// empty when there is none, when it is null, when it is not text, or when it is empty.
	Project string
}

// Query reads the input of the call as the arguments of a memory query. A key is matched
// exactly and the last of a repeated key wins. It never fails: what it cannot read it reports
// as not named, or as no project.
func (p PreToolUse) Query() QueryArguments {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(p.input, &fields); err != nil || fields == nil {
		return QueryArguments{}
	}
	var project string
	if err := json.Unmarshal(fields["project"], &project); err != nil {
		project = ""
	}
	return QueryArguments{Named: true, Project: project}
}

// decodeEnvelope is the lenient reading both events share: the input is at most MaxEnvelopeBytes,
// is exactly one JSON object (after surrounding white space), and is decoded into wire, ignoring
// the fields wire does not name. Every event's decoder goes through it, so the events cannot
// disagree on what a usable input is.
func decodeEnvelope(data []byte, wire any) error {
	if len(data) > MaxEnvelopeBytes {
		return fmt.Errorf("parse hook input: %w: %d bytes exceeds the maximum of %d", ErrTooLarge, len(data), MaxEnvelopeBytes)
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return errors.New("parse hook input: the input is not a JSON object")
	}
	if err := json.Unmarshal(trimmed, wire); err != nil {
		return fmt.Errorf("parse hook input: %w", err)
	}
	return nil
}

// cleanCwd returns cwd cleaned when it is absolute and "" otherwise, so dot segments, repeated
// separators and a trailing slash never make one directory look like two, and a relative path
// is read as missing.
func cleanCwd(cwd string) string {
	if filepath.IsAbs(cwd) {
		return filepath.Clean(cwd)
	}
	return ""
}

// --- the Agent tool ----------------------------------------------------------------------------

// AgentCall is a PreToolUse input for the tool that starts a sub-agent. A reply that changes the
// call must echo all of it (UpdatedInput), so the whole call is kept, though a policy decides
// from the type and the prompt alone.
type AgentCall struct {
	Description  string
	Prompt       string
	SubagentType string
	// Model is the model the call asked for, or nil when it asked for none.
	Model *string
}

// DecodeAgentCall reads the Agent tool call of a PreToolUse hook's stdin. It reads tool_name as
// text and does not use it (the hook is installed for the Agent tool), and tool_input as the
// four fields of the tool; the rest of the input, which it does not read, can be of any type.
// It fails for an input that is not one JSON object, for a tool_name or one of the four fields
// of the wrong type, and for a tool_input that is missing or is not an object. A tool_input that
// is null decodes to an empty call, which a policy leaves alone.
//
// There is no bound on the size of the input: the caller reads what it can bear to read, and an
// input cut short is not valid JSON and is refused.
func DecodeAgentCall(data []byte) (AgentCall, error) {
	var envelope struct {
		ToolName  string          `json:"tool_name"`
		ToolInput json.RawMessage `json:"tool_input"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return AgentCall{}, fmt.Errorf("decode agent call: %w", err)
	}
	if len(envelope.ToolInput) == 0 {
		return AgentCall{}, errors.New("decode agent call: the input has no tool_input")
	}
	var wire struct {
		Description  string  `json:"description"`
		Prompt       string  `json:"prompt"`
		SubagentType string  `json:"subagent_type"`
		Model        *string `json:"model,omitempty"`
	}
	if err := json.Unmarshal(envelope.ToolInput, &wire); err != nil {
		return AgentCall{}, fmt.Errorf("decode agent call: tool_input: %w", err)
	}
	return AgentCall{Description: wire.Description, Prompt: wire.Prompt, SubagentType: wire.SubagentType, Model: wire.Model}, nil
}
