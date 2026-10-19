package hookwire

import (
	"bufio"
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
// one is refused, wrapping ErrTooLarge), in one pass that holds no more of it than a small read
// buffer: the value of tool_response is hashed and counted as it is read, and never kept, and
// every other field is kept in an envelope of at most MaxEnvelopeBytes (a larger one is refused the
// same way), which is then read as the other events' inputs are. Like their decoders it is
// lenient: the fields it does not name are ignored, a relative cwd is read as missing, and a field
// it reads with a value of the wrong type is an error, as is a document that is not exactly one
// JSON object.
func DecodePostToolUse(r io.Reader) (PostToolUse, error) {
	counted := &countingReader{r: io.LimitReader(r, MaxPostToolUseBytes+1)}
	s := &splitter{r: bufio.NewReaderSize(counted, readBufferBytes)}
	out, err := s.split()
	if counted.n > MaxPostToolUseBytes {
		return PostToolUse{}, tooLarge("parse hook input", int(counted.n), MaxPostToolUseBytes)
	}
	if err != nil {
		return PostToolUse{}, fmt.Errorf("parse hook input: %w", err)
	}
	return out, nil
}

const (
	// readBufferBytes is the read buffer of DecodePostToolUse, and sinkBufferBytes what the hash of
	// the response is fed at a time.
	readBufferBytes = 64 << 10
	sinkBufferBytes = 32 << 10
	// maxNestingDepth is the deepest a value may nest, as encoding/json's own bound.
	maxNestingDepth = 10000
)

// splitter reads one JSON object from r, sending the value of its tool_response field to a hash
// and every other byte of it to the envelope.
type splitter struct {
	r *bufio.Reader
}

// split reads the whole input and returns the call it holds.
func (s *splitter) split() (PostToolUse, error) {
	if b, err := s.space(nil); err != nil || b != '{' {
		return PostToolUse{}, errors.New("the input is not a JSON object")
	}
	env := &sink{limit: MaxEnvelopeBytes, what: "the fields other than tool_response"}
	var out PostToolUse
	if err := s.fields(env, &out); err != nil {
		return PostToolUse{}, err
	}
	for {
		b, err := s.r.ReadByte()
		if err == io.EOF {
			break
		}
		if err != nil {
			return PostToolUse{}, err
		}
		if !isSpace(b) {
			return PostToolUse{}, errors.New("the input is more than one JSON object")
		}
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(env.buf, &fields); err != nil {
		return PostToolUse{}, err
	}
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
	return out, nil
}

// fields reads the fields of the top-level object, whose opening brace was read, up to its closing
// one. Each goes to env as it was sent, but for the value of tool_response, which is hashed into
// out and stands in env as null; when the name is sent twice the last value counts, as it does for
// every other field.
func (s *splitter) fields(env *sink, out *PostToolUse) error {
	if err := env.put('{'); err != nil {
		return err
	}
	b, err := s.space(nil)
	if err != nil {
		return err
	}
	if b == '}' {
		return env.put('}')
	}
	for {
		if b != '"' {
			return errors.New("a field name is not text")
		}
		key := &sink{limit: MaxEnvelopeBytes, what: "a field name"}
		if err := s.text(key); err != nil {
			return err
		}
		var name string
		if err := json.Unmarshal(key.buf, &name); err != nil {
			return err
		}
		if err := env.write(key.buf); err != nil {
			return err
		}
		if err := s.expect(':', nil, env); err != nil {
			return err
		}
		if b, err = s.space(nil); err != nil {
			return err
		}
		if name == "tool_response" {
			sum := sha256.New()
			response := &sink{w: sum}
			if err := s.value(b, response, 0); err != nil {
				return err
			}
			if err := response.flush(); err != nil {
				return err
			}
			out.ResponseSHA256, out.ResponseBytes = hex.EncodeToString(sum.Sum(nil)), response.n
			err = env.write([]byte("null"))
		} else {
			err = s.value(b, env, 0)
		}
		if err != nil {
			return err
		}
		if b, err = s.space(nil); err != nil {
			return err
		}
		if err := env.put(b); err != nil {
			return err
		}
		switch b {
		case '}':
			return nil
		case ',':
		default:
			return errors.New("the fields of the object are not separated by commas")
		}
		if b, err = s.space(nil); err != nil {
			return err
		}
	}
}

// value copies to out the JSON value that starts with b, which was read, checking it as it goes.
func (s *splitter) value(b byte, out *sink, depth int) error {
	switch {
	case b == '"':
		return s.text(out)
	case b == '{' || b == '[':
		return s.composite(b, out, depth+1)
	case b == '-' || isDigit(b):
		return s.number(b, out)
	case b == 't':
		return s.literal("true", out)
	case b == 'f':
		return s.literal("false", out)
	case b == 'n':
		return s.literal("null", out)
	}
	return fmt.Errorf("invalid character %q at the start of a value", b)
}

// composite copies an object or an array, whose opening open was read, with what it holds.
func (s *splitter) composite(open byte, out *sink, depth int) error {
	if depth > maxNestingDepth {
		return fmt.Errorf("a value nests deeper than %d", maxNestingDepth)
	}
	closing := byte(']')
	if open == '{' {
		closing = '}'
	}
	if err := out.put(open); err != nil {
		return err
	}
	b, err := s.space(out)
	if err != nil {
		return err
	}
	if b == closing {
		return out.put(b)
	}
	for {
		if open == '{' {
			if b != '"' {
				return errors.New("a field name is not text")
			}
			if err := s.text(out); err != nil {
				return err
			}
			if err := s.expect(':', out, out); err != nil {
				return err
			}
			if b, err = s.space(out); err != nil {
				return err
			}
		}
		if err := s.value(b, out, depth); err != nil {
			return err
		}
		if b, err = s.space(out); err != nil {
			return err
		}
		if err := out.put(b); err != nil {
			return err
		}
		if b == closing {
			return nil
		}
		if b != ',' {
			return fmt.Errorf("invalid character %q after a value", b)
		}
		if b, err = s.space(out); err != nil {
			return err
		}
	}
}

// text copies a string, whose opening quote was read, up to and with its closing quote.
func (s *splitter) text(out *sink) error {
	if err := out.put('"'); err != nil {
		return err
	}
	for {
		b, err := s.next()
		if err != nil {
			return err
		}
		if b < 0x20 {
			return errors.New("a string holds a control character")
		}
		if err := out.put(b); err != nil {
			return err
		}
		switch b {
		case '"':
			return nil
		case '\\':
			if err := s.escape(out); err != nil {
				return err
			}
		}
	}
}

// escape copies what follows a backslash in a string: one of the escape letters, or a u and four
// hex digits.
func (s *splitter) escape(out *sink) error {
	b, err := s.next()
	if err != nil {
		return err
	}
	if err := out.put(b); err != nil {
		return err
	}
	switch b {
	case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
		return nil
	case 'u':
		for i := 0; i < 4; i++ {
			h, err := s.next()
			if err != nil {
				return err
			}
			if !isDigit(h) && (h|0x20 < 'a' || h|0x20 > 'f') {
				return fmt.Errorf("invalid character %q in a string escape", h)
			}
			if err := out.put(h); err != nil {
				return err
			}
		}
		return nil
	}
	return fmt.Errorf("invalid character %q in a string escape", b)
}

// number copies a number, whose first byte first was read.
func (s *splitter) number(first byte, out *sink) error {
	b := first
	if b == '-' {
		if err := out.put(b); err != nil {
			return err
		}
		var err error
		if b, err = s.next(); err != nil {
			return err
		}
	}
	if !isDigit(b) {
		return fmt.Errorf("invalid character %q in a number", b)
	}
	if err := out.put(b); err != nil {
		return err
	}
	if b != '0' {
		if _, err := s.digits(out); err != nil {
			return err
		}
	}
	b, err := s.next()
	if err != nil {
		return err
	}
	if b == '.' {
		if err := out.put(b); err != nil {
			return err
		}
		if n, err := s.digits(out); err != nil || n == 0 {
			return errors.Join(errors.New("a number has no digits after its point"), err)
		}
		if b, err = s.next(); err != nil {
			return err
		}
	}
	if b == 'e' || b == 'E' {
		if err := out.put(b); err != nil {
			return err
		}
		if b, err = s.next(); err != nil {
			return err
		}
		if b == '+' || b == '-' {
			if err := out.put(b); err != nil {
				return err
			}
			if b, err = s.next(); err != nil {
				return err
			}
		}
		if err := s.r.UnreadByte(); err != nil {
			return err
		}
		if n, err := s.digits(out); err != nil || n == 0 {
			return errors.Join(errors.New("a number has no digits in its exponent"), err)
		}
		return nil
	}
	return s.r.UnreadByte()
}

// digits copies the decimal digits that come next, and returns how many there were.
func (s *splitter) digits(out *sink) (int, error) {
	for n := 0; ; n++ {
		b, err := s.next()
		if err != nil {
			return n, err
		}
		if !isDigit(b) {
			return n, s.r.UnreadByte()
		}
		if err := out.put(b); err != nil {
			return n, err
		}
	}
}

// literal copies word, whose first byte was read.
func (s *splitter) literal(word string, out *sink) error {
	if err := out.put(word[0]); err != nil {
		return err
	}
	for i := 1; i < len(word); i++ {
		b, err := s.next()
		if err != nil {
			return err
		}
		if b != word[i] {
			return fmt.Errorf("invalid character %q in literal %s", b, word)
		}
		if err := out.put(b); err != nil {
			return err
		}
	}
	return nil
}

// expect reads past whitespace, copied to space when it is not nil, and the byte want, which it
// copies to out.
func (s *splitter) expect(want byte, space, out *sink) error {
	b, err := s.space(space)
	if err != nil {
		return err
	}
	if b != want {
		return fmt.Errorf("invalid character %q, want %q", b, want)
	}
	return out.put(b)
}

// space reads past whitespace, copying it to out when out is not nil, and returns the first other
// byte.
func (s *splitter) space(out *sink) (byte, error) {
	for {
		b, err := s.next()
		if err != nil || !isSpace(b) {
			return b, err
		}
		if out != nil {
			if err := out.put(b); err != nil {
				return 0, err
			}
		}
	}
}

// next is the next byte of the input; the input ending inside the object is an error.
func (s *splitter) next() (byte, error) {
	b, err := s.r.ReadByte()
	if err == io.EOF {
		return 0, io.ErrUnexpectedEOF
	}
	return b, err
}

func isSpace(b byte) bool { return b == ' ' || b == '\t' || b == '\n' || b == '\r' }

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

// sink is where the bytes of one part of the input go: with a writer, they are handed to it a
// buffer at a time and not kept; without one, they are kept in buf, at most limit of them, and
// one more is refused, wrapping ErrTooLarge. n counts them either way.
type sink struct {
	w     io.Writer
	buf   []byte
	n     int64
	limit int64
	what  string
}

func (k *sink) put(b byte) error {
	if k.w == nil && k.n >= k.limit {
		return tooLarge(k.what, int(k.n)+1, int(k.limit))
	}
	if k.w != nil && k.buf == nil {
		k.buf = make([]byte, 0, sinkBufferBytes)
	}
	k.buf = append(k.buf, b)
	k.n++
	if k.w != nil && len(k.buf) == sinkBufferBytes {
		return k.flush()
	}
	return nil
}

func (k *sink) write(p []byte) error {
	for _, b := range p {
		if err := k.put(b); err != nil {
			return err
		}
	}
	return nil
}

// flush hands what is buffered to the writer.
func (k *sink) flush() error {
	if k.w == nil || len(k.buf) == 0 {
		return nil
	}
	_, err := k.w.Write(k.buf)
	k.buf = k.buf[:0]
	return err
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
