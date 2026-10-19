package hookwire

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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
// one is refused, wrapping ErrTooLarge), as a stream of tokens: tool_response is hashed and
// dropped, so only the fields the call is read by are kept. Like the other events' decoders it is
// lenient: the fields it does not name are ignored, a relative cwd is read as missing, and a field
// it reads with a value of the wrong type is an error, as is a document that is not exactly one
// JSON object.
func DecodePostToolUse(r io.Reader) (PostToolUse, error) {
	counted := &countingReader{r: io.LimitReader(r, MaxPostToolUseBytes+1)}
	out, err := decodePostToolUse(json.NewDecoder(counted))
	if counted.n > MaxPostToolUseBytes {
		return PostToolUse{}, tooLarge("parse hook input", int(counted.n), MaxPostToolUseBytes)
	}
	if err != nil {
		return PostToolUse{}, fmt.Errorf("parse hook input: %w", err)
	}
	return out, nil
}

func decodePostToolUse(dec *json.Decoder) (PostToolUse, error) {
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return PostToolUse{}, errors.New("the input is not a JSON object")
	}
	var out PostToolUse
	var cwd string
	text := map[string]*string{
		"hook_event_name": &out.Event, "cwd": &cwd, "session_id": &out.SessionID,
		"tool_use_id": &out.CallID, "tool_name": &out.Tool,
	}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return PostToolUse{}, err
		}
		key, _ := tok.(string)
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return PostToolUse{}, err
		}
		switch target, isText := text[key]; {
		case isText:
			if err := json.Unmarshal(raw, target); err != nil {
				return PostToolUse{}, fmt.Errorf("%s: %w", key, err)
			}
		case key == "tool_input":
			out.input = raw
		case key == "tool_response":
			sum := sha256.Sum256(raw)
			out.ResponseSHA256, out.ResponseBytes = hex.EncodeToString(sum[:]), int64(len(raw))
		}
	}
	if _, err := dec.Token(); err != nil {
		return PostToolUse{}, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return PostToolUse{}, errors.New("the input is more than one JSON object")
	}
	out.Cwd = cleanCwd(cwd)
	return out, nil
}

// countingReader counts the bytes read through it.
type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
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
