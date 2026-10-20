package hookwire

import (
	"encoding/json"
	"fmt"
	"io"
)

// PostToolUse is what is kept of the JSON a PostToolUse hook receives (C23 M7): the event's name,
// the working directory, the session and the call's ids, the tool's name and input, which are
// read as a PreToolUse call's are, and of the tool's response only its SHA-256 and its length.
// The response itself is never kept.
type PostToolUse struct {
	// Event is hook_event_name, or empty when the input has none.
	Event string
	// Cwd is the session's working directory as a cleaned absolute path, or empty when the input
	// has none or it is not an absolute path.
	Cwd string
	// SessionID is session_id and CallID is tool_use_id, each empty when the input has none.
	SessionID string
	CallID    string
	// Tool is tool_name, or empty when the input has none.
	Tool string
	// ResponseSHA256 is the hex SHA-256 of tool_response's JSON value as it was sent, and
	// ResponseBytes its length; "" and 0 when the input has no tool_response.
	ResponseSHA256 string
	ResponseBytes  int64

	// input is tool_input as it was sent, or empty when the input has none.
	input json.RawMessage
}

// Query reads the input of the call as the arguments of a memory query, as PreToolUse.Query.
func (p PostToolUse) Query() QueryArguments { return queryArguments(p.input) }

// Skill is the skill a Skill call names: its "skill" argument when that is text, else "".
func (p PostToolUse) Skill() string { return textArgument(p.input, "skill") }

// FilePath is the file a Read call names: its "file_path" argument when that is text, else "".
func (p PostToolUse) FilePath() string { return textArgument(p.input, "file_path") }

// recordIDKeys are the arguments a read by id names its record by: Engram's mem_get_observation
// ("id") and mem_timeline ("observation_id"), and longterm-mem's get ("engram_id").
var recordIDKeys = []string{"id", "observation_id", "engram_id"}

// RecordID is the record a read by id names: the first of its "id", "observation_id" and
// "engram_id" arguments that is a JSON number or text, as text; "" when there is none.
func (p PostToolUse) RecordID() string {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(p.input, &fields); err != nil {
		return ""
	}
	for _, key := range recordIDKeys {
		var number json.Number
		if err := json.Unmarshal(fields[key], &number); err == nil && number != "" {
			return number.String()
		}
		var text string
		if err := json.Unmarshal(fields[key], &text); err == nil && text != "" {
			return text
		}
	}
	return ""
}

// DecodePostToolUse reads a PostToolUse hook's stdin, at most MaxPostToolUseBytes of it (a longer
// one is refused, wrapping ErrTooLarge), in one pass that holds no more of it than a small read
// buffer (splitField): the value of tool_response is hashed and counted as it is read, and never
// kept, and every other field is kept in an envelope of at most MaxEnvelopeBytes (a larger one is
// refused the same way), which is then read as the other events' inputs are. Like their decoders
// it is lenient: the fields it does not name are ignored, a relative cwd is read as missing, and a
// field it reads with a value of the wrong type is an error, as is a document that is not exactly
// one JSON object.
func DecodePostToolUse(r io.Reader) (PostToolUse, error) {
	counted := &countingReader{r: io.LimitReader(r, MaxPostToolUseBytes+1)}
	split, err := splitField(counted, "tool_response", MaxEnvelopeBytes, "the fields other than tool_response")
	if counted.n > MaxPostToolUseBytes {
		return PostToolUse{}, tooLarge("parse hook input", int(counted.n), MaxPostToolUseBytes)
	}
	if err == nil {
		var out PostToolUse
		out, err = decodePostToolUse(split)
		if err == nil {
			return out, nil
		}
	}
	return PostToolUse{}, fmt.Errorf("parse hook input: %w", err)
}

// decodePostToolUse reads the call from the input with tool_response taken out.
func decodePostToolUse(split fieldSplit) (PostToolUse, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(split.Rest, &fields); err != nil {
		return PostToolUse{}, err
	}
	var out PostToolUse
	var cwd string
	text := map[string]*string{
		"hook_event_name": &out.Event, "cwd": &cwd, "session_id": &out.SessionID,
		"tool_use_id": &out.CallID, "tool_name": &out.Tool,
	}
	for key, target := range text {
		if raw, ok := fields[key]; ok {
			if err := json.Unmarshal(raw, target); err != nil {
				return PostToolUse{}, fmt.Errorf("%s: %w", key, err)
			}
		}
	}
	out.input = fields["tool_input"]
	out.Cwd = cleanCwd(cwd)
	if split.Found {
		out.ResponseSHA256, out.ResponseBytes = split.SHA256, split.Bytes
	}
	return out, nil
}

// --- PostToolUse: context --------------------------------------------------------------------

// PostToolUseReply is what a PostToolUse hook tells Claude Code: one line for the agent's context.
type PostToolUseReply struct {
	Context string
}

// Encode renders r as the one JSON object a PostToolUse hook writes to stdout, followed by a line
// break: {"hookSpecificOutput":{"hookEventName":"PostToolUse","additionalContext":"..."}}. An
// empty context yields no output at all.
func (r PostToolUseReply) Encode() ([]byte, error) {
	if r.Context == "" {
		return nil, nil
	}
	return encodeLine(contextOutput{HookSpecificOutput: &contextSpecific{HookEventName: EventPostToolUse, AdditionalContext: r.Context}})
}
