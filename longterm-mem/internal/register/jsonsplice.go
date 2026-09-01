// Package register implements ownership-tagged, byte-preserving edits to
// the MCP configuration files longterm-mem itself does not own — Claude
// Code's and opencode's JSON configs, and (12a) codex's TOML config (D9).
// Every writer here edits only the exact byte span of the member/section it
// owns, rather than decoding and re-encoding the whole document: a
// decode-re-encode round trip would silently reorder keys, reformat
// whitespace, and drop anything the parser doesn't model (e.g. comments),
// rewriting a file a human maintains out from under them.
package register

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// location is the result of locating containerKey.memberKey inside a JSON
// document (see locate): either the exact byte span of an existing member
// to replace, or the byte offset and formatting to insert a new one at.
type location struct {
	// found is whether an existing member named memberKey was located
	// inside the container object.
	found bool
	// replaceStart is the offset of the member key's opening quote
	// (inclusive); replaceEnd is the offset immediately after the
	// member's value (exclusive). Valid only when found is true.
	replaceStart, replaceEnd int
	// insertAt is the offset to insert a new member at (immediately
	// before the container object's closing '}') when found is false.
	insertAt int
	// needsComma is whether a leading comma must precede the inserted
	// member, i.e. the container already has at least one member.
	needsComma bool
	// indent is the whitespace prefix to use for the inserted member's
	// line, borrowed from an existing sibling when one exists.
	indent string
}

// locate walks raw's top-level JSON object to find containerKey (which
// must itself be an object), then walks that container's members looking
// for memberKey, returning byte offsets precise enough for apply to build
// a replacement or insertion without touching any other byte of raw. It
// uses json.Decoder.Token combined with json.Decoder.InputOffset, never
// decoding the document into a generic value and re-encoding it (D9).
//
// Only a containerKey that already exists as a top-level object member is
// supported; a document missing that top-level key returns an error
// (locate: container key %q not found) rather than synthesizing one — every
// real runtime config this package edits already declares its MCP-server
// container object, even when empty, so this keeps locate's traversal
// single-purpose. A future slice can extend this if a real fixture needs
// it.
func locate(raw []byte, containerKey, memberKey string) (location, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))

	root, err := dec.Token()
	if err != nil {
		return location{}, fmt.Errorf("register: parse document: %w", err)
	}
	if d, ok := root.(json.Delim); !ok || d != '{' {
		return location{}, fmt.Errorf("register: document root is not a JSON object")
	}

	for {
		keyStart := dec.InputOffset()
		tok, err := dec.Token()
		if err != nil {
			return location{}, fmt.Errorf("register: parse document: %w", err)
		}
		if d, ok := tok.(json.Delim); ok && d == '}' {
			return location{}, fmt.Errorf("register: container key %q not found", containerKey)
		}
		key, ok := tok.(string)
		if !ok {
			return location{}, fmt.Errorf("register: unexpected non-string key in document")
		}
		if key != containerKey {
			if _, err := skipValue(dec); err != nil {
				return location{}, fmt.Errorf("register: parse document: %w", err)
			}
			continue
		}

		open, err := dec.Token()
		if err != nil {
			return location{}, fmt.Errorf("register: parse document: %w", err)
		}
		if d, ok := open.(json.Delim); !ok || d != '{' {
			return location{}, fmt.Errorf("register: %q is not a JSON object", containerKey)
		}
		containerOpenEnd := dec.InputOffset()

		containerKeyQuote := int(keyStart) + bytes.IndexByte(raw[keyStart:], '"')
		fallbackIndent := indentAt(raw, containerKeyQuote) + "  "
		lastMemberKeyQuote := -1

		for {
			innerStart := dec.InputOffset()
			itok, err := dec.Token()
			if err != nil {
				return location{}, fmt.Errorf("register: parse document: %w", err)
			}
			if d, ok := itok.(json.Delim); ok && d == '}' {
				indent := fallbackIndent
				if lastMemberKeyQuote >= 0 {
					indent = indentAt(raw, lastMemberKeyQuote)
				}
				return location{
					found:      false,
					insertAt:   int(innerStart),
					needsComma: int(innerStart) != int(containerOpenEnd),
					indent:     indent,
				}, nil
			}
			mkey, ok := itok.(string)
			if !ok {
				return location{}, fmt.Errorf("register: unexpected non-string key in %q", containerKey)
			}
			keyQuote := int(innerStart) + bytes.IndexByte(raw[innerStart:], '"')
			lastMemberKeyQuote = keyQuote

			valueEnd, err := skipValue(dec)
			if err != nil {
				return location{}, fmt.Errorf("register: parse document: %w", err)
			}
			if mkey == memberKey {
				return location{
					found:        true,
					replaceStart: keyQuote,
					replaceEnd:   int(valueEnd),
				}, nil
			}
		}
	}
}

// skipValue consumes exactly one JSON value beginning at dec's current
// position — a scalar, or a balanced object/array — and returns the input
// offset immediately after the value's final byte.
func skipValue(dec *json.Decoder) (int64, error) {
	tok, err := dec.Token()
	if err != nil {
		return 0, err
	}
	depth := 0
	if d, ok := tok.(json.Delim); ok && (d == '{' || d == '[') {
		depth = 1
	}
	for depth > 0 {
		tok, err := dec.Token()
		if err != nil {
			return 0, err
		}
		if d, ok := tok.(json.Delim); ok {
			switch d {
			case '{', '[':
				depth++
			case '}', ']':
				depth--
			}
		}
	}
	return dec.InputOffset(), nil
}

// indentAt returns the run of spaces/tabs immediately preceding raw[idx] on
// its line, i.e. that line's indentation up to (not including) idx.
func indentAt(raw []byte, idx int) string {
	start := idx
	for start > 0 && (raw[start-1] == ' ' || raw[start-1] == '\t') {
		start--
	}
	return string(raw[start:idx])
}

// apply builds the spliced document from raw and loc, writing memberKey:
// newValue either over the span loc identifies (replace) or immediately
// before the container's closing brace (insert). It never rewrites any
// byte outside that span.
func (loc location) apply(raw []byte, memberKey string, newValue json.RawMessage) []byte {
	keyLiteral, _ := json.Marshal(memberKey) // memberKey is a plain string; Marshal cannot fail

	var buf bytes.Buffer
	if loc.found {
		buf.Write(raw[:loc.replaceStart])
		buf.Write(keyLiteral)
		buf.WriteString(": ")
		buf.Write(newValue)
		buf.Write(raw[loc.replaceEnd:])
		return buf.Bytes()
	}

	buf.Write(raw[:loc.insertAt])
	if loc.needsComma {
		buf.WriteByte(',')
	}
	buf.WriteByte('\n')
	buf.WriteString(loc.indent)
	buf.Write(keyLiteral)
	buf.WriteString(": ")
	buf.Write(newValue)
	buf.Write(raw[loc.insertAt:])
	return buf.Bytes()
}

// Splice returns a copy of raw with the member at containerKey.memberKey
// either replaced in place (when present) or inserted (when absent), and
// every other byte of raw unchanged (D9; R-016/R-017 "Unrelated entries are
// preserved"). newValue is written verbatim as the member's value — Splice
// performs no validation of newValue's own well-formedness; that is
// WriteMember's job (jsonwrite.go), so its post-write json.Valid gate has
// something real to catch.
func Splice(raw []byte, containerKey, memberKey string, newValue json.RawMessage) ([]byte, error) {
	loc, err := locate(raw, containerKey, memberKey)
	if err != nil {
		return nil, err
	}
	return loc.apply(raw, memberKey, newValue), nil
}
