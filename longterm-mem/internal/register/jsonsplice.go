// This file holds the JSON-specific byte-splice locate/apply logic (D9);
// see doc.go for the package's own documentation and the D9 semantics
// table every runtime writer shares.
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

// Remove returns a copy of raw with the member at containerKey.memberKey
// removed entirely: its key, its value, AND exactly the one comma its
// removal makes dangling — the trailing comma when a following member
// exists, the leading comma otherwise — so the result never carries a
// stray trailing comma before the container's closing brace, nor two
// commas in a row (D9; R-019 "Selective removal across all three
// runtimes"). Every other byte of raw, including the member's own leading
// indentation/newline, is folded into whichever neighbour now spans across
// that position — removeSpan runs locate/apply's own insertion rule in
// reverse. It errors when memberKey is not present in containerKey;
// callers (jsonUninstall, writer.go) only ever reach this after confirming
// entryPresent themselves, so this is a defensive guard, not the normal
// "nothing to do" path.
func Remove(raw []byte, containerKey, memberKey string) ([]byte, error) {
	loc, err := locate(raw, containerKey, memberKey)
	if err != nil {
		return nil, err
	}
	if !loc.found {
		return nil, fmt.Errorf("register: member %q not found in %q, nothing to remove", memberKey, containerKey)
	}
	return removeSpan(raw, loc.replaceStart, loc.replaceEnd), nil
}

// removeSpan implements Remove's comma/whitespace rule for the byte span
// [start,end): a located member's key-through-value bytes.
//
//   - Forward: skip insignificant whitespace after end; if the next byte is
//     ',', a following member exists (this member was NOT last) — extend
//     end past that comma.
//   - Backward: skip insignificant whitespace before start — this always
//     folds the member's own leading indentation/newline into whichever
//     neighbour ends up adjacent to it, first or middle or last. Only when
//     no trailing comma was found (this member WAS last) and a comma now
//     immediately precedes the trimmed start, also exclude that leading
//     comma — the one every JSON object's last member never has a trailing
//     one for.
func removeSpan(raw []byte, start, end int) []byte {
	fwd := end
	for fwd < len(raw) && isJSONLineSpace(raw[fwd]) {
		fwd++
	}
	hasTrailingComma := fwd < len(raw) && raw[fwd] == ','
	if hasTrailingComma {
		end = fwd + 1
	}

	back := start
	for back > 0 && isJSONLineSpace(raw[back-1]) {
		back--
	}
	if !hasTrailingComma && back > 0 && raw[back-1] == ',' {
		back--
	}
	start = back

	var buf bytes.Buffer
	buf.Write(raw[:start])
	buf.Write(raw[end:])
	return buf.Bytes()
}

// isJSONLineSpace reports whether b is one of the four bytes encoding/json
// itself treats as insignificant whitespace between tokens.
func isJSONLineSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}
