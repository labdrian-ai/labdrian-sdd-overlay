package hookwire

// A one-pass JSON splitter: it reads exactly one JSON object and takes one of its top-level fields
// out, hashing that field's value as it is read instead of keeping it, and keeping the rest of the
// object, bounded, for encoding/json to read.
//
// It is written by hand because encoding/json cannot do this: Decoder.Decode holds a whole value,
// and Decoder.Token holds a whole string, so a response of tens of MiB would be held in memory to
// be hashed (C23 M7 review A, R4-buffered-response). The splitter is a strict RFC 8259 recognizer
// of the values it copies, so the bytes it hashes are a JSON value, and it nests at most
// maxNestingDepth deep, as encoding/json does.

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const (
	// readBufferBytes is the read buffer of a split, and sinkBufferBytes what the hash of the field
	// is fed at a time.
	readBufferBytes = 64 << 10
	sinkBufferBytes = 32 << 10
	// maxNestingDepth is the deepest a value may nest, as encoding/json's own bound.
	maxNestingDepth = 10000
)

// fieldSplit is one JSON object with one of its top-level fields taken out.
type fieldSplit struct {
	// Rest is the object as it was sent, with the field's value replaced by null.
	Rest []byte
	// Found reports whether the object had the field; SHA256 is the hex SHA-256 of its value as
	// it was sent and Bytes its length. When the name is sent twice the last value counts, as it
	// does for encoding/json.
	Found  bool
	SHA256 string
	Bytes  int64
}

// splitField reads exactly one JSON object from r, with nothing after it but whitespace, and takes
// its top-level field name out. The rest of the object is kept, at most restLimit bytes of it (one
// more is refused, wrapping ErrTooLarge, as restWhat); the field's value is hashed and never kept.
func splitField(r io.Reader, name string, restLimit int64, restWhat string) (fieldSplit, error) {
	s := &splitter{r: bufio.NewReaderSize(r, readBufferBytes)}
	if b, err := s.space(nil); err != nil || b != '{' {
		return fieldSplit{}, errors.New("the input is not a JSON object")
	}
	rest := &sink{limit: restLimit, what: restWhat}
	var out fieldSplit
	if err := s.fields(name, rest, &out); err != nil {
		return fieldSplit{}, err
	}
	for {
		b, err := s.r.ReadByte()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fieldSplit{}, err
		}
		if !isSpace(b) {
			return fieldSplit{}, errors.New("the input is more than one JSON object")
		}
	}
	out.Rest = rest.buf
	return out, nil
}

// splitter reads JSON from r, copying each value it reads to a sink.
type splitter struct {
	r *bufio.Reader
}

// fields reads the fields of the top-level object, whose opening brace was read, up to its closing
// one. Each goes to rest as it was sent, but for the value of the field name, which is hashed into
// out and stands in rest as null.
func (s *splitter) fields(name string, rest *sink, out *fieldSplit) error {
	if err := rest.put('{'); err != nil {
		return err
	}
	b, err := s.space(nil)
	if err != nil {
		return err
	}
	if b == '}' {
		return rest.put('}')
	}
	for {
		if b != '"' {
			return errors.New("a field name is not text")
		}
		// No name can be longer than the rest it goes into.
		key := &sink{limit: rest.limit, what: "a field name"}
		if err := s.text(key); err != nil {
			return err
		}
		var field string
		if err := json.Unmarshal(key.buf, &field); err != nil {
			return err
		}
		if err := rest.write(key.buf); err != nil {
			return err
		}
		if err := s.expect(':', nil, rest); err != nil {
			return err
		}
		if b, err = s.space(nil); err != nil {
			return err
		}
		if field == name {
			err = s.hash(b, out)
			if err == nil {
				err = rest.write([]byte("null"))
			}
		} else {
			err = s.value(b, rest, 0)
		}
		if err != nil {
			return err
		}
		if b, err = s.space(nil); err != nil {
			return err
		}
		if err := rest.put(b); err != nil {
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

// hash reads the value that starts with b, which was read, into a SHA-256 and records it in out.
func (s *splitter) hash(b byte, out *fieldSplit) error {
	sum := sha256.New()
	value := &sink{w: sum}
	if err := s.value(b, value, 0); err != nil {
		return err
	}
	if err := value.flush(); err != nil {
		return err
	}
	out.Found, out.SHA256, out.Bytes = true, hex.EncodeToString(sum.Sum(nil)), value.n
	return nil
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

// text copies a string, whose opening quote was read, up to and with its closing quote. The bytes
// that need no check of their own are copied in runs, straight from the read buffer; only quotes,
// backslashes and control characters are read one at a time.
func (s *splitter) text(out *sink) error {
	if err := out.put('"'); err != nil {
		return err
	}
	for {
		if err := s.plainRun(out); err != nil {
			return err
		}
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

// plainRun copies the bytes of a string that come next and are neither a quote, a backslash nor a
// control character, as many as the read buffer holds at a time. The end of the input is left for
// next to report.
func (s *splitter) plainRun(out *sink) error {
	for {
		if _, err := s.r.Peek(1); err != nil {
			return nil
		}
		buf, _ := s.r.Peek(s.r.Buffered())
		n := 0
		for n < len(buf) && buf[n] != '"' && buf[n] != '\\' && buf[n] >= 0x20 {
			n++
		}
		if n == 0 {
			return nil
		}
		if err := out.write(buf[:n]); err != nil {
			return err
		}
		if _, err := s.r.Discard(n); err != nil {
			return err
		}
		if n < len(buf) {
			return nil
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

// put copies one byte, as write does.
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

// write copies p whole: kept when the sink has no writer, else buffered, and handed to the writer
// directly when it is a buffer or more.
func (k *sink) write(p []byte) error {
	if k.w == nil {
		if k.n+int64(len(p)) > k.limit {
			return tooLarge(k.what, int(k.n)+len(p), int(k.limit))
		}
		k.buf = append(k.buf, p...)
		k.n += int64(len(p))
		return nil
	}
	k.n += int64(len(p))
	if len(k.buf)+len(p) > sinkBufferBytes {
		if err := k.flush(); err != nil {
			return err
		}
		if len(p) >= sinkBufferBytes {
			_, err := k.w.Write(p)
			return err
		}
	}
	if k.buf == nil {
		k.buf = make([]byte, 0, sinkBufferBytes)
	}
	k.buf = append(k.buf, p...)
	if len(k.buf) == sinkBufferBytes {
		return k.flush()
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
