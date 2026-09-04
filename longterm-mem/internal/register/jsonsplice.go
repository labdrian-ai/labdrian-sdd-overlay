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
	// createContainer is whether the document has no containerKey at all,
	// so apply must synthesize the container object itself (holding the
	// one new member) rather than insert into an existing one. insertAt,
	// needsComma and indent then describe a position in the ROOT object
	// instead of inside the container.
	createContainer bool
	// insertAt is the offset to insert a new member at (immediately
	// before the container object's closing '}', or before the root
	// object's closing '}' when createContainer is set) when found is
	// false.
	insertAt int
	// needsComma is whether a leading comma must precede the inserted
	// member, i.e. the object being inserted into already has at least
	// one member.
	needsComma bool
	// indent is the whitespace prefix to use for the inserted member's
	// line, borrowed from an existing sibling when one exists.
	indent string
}

// emptyDocumentAsObject maps a JSON config document that holds nothing at
// all onto the empty object it means, so every reader and writer in this
// package treats a zero-byte (or whitespace-only) config exactly as it
// treats `{}`.
//
// This is the JSON half of a guarantee the TOML writer already made and
// the spec already claimed for both: a config file with no content has no
// entries, so `register` inserts into it and `unregister` finds nothing to
// remove. Without it, encoding/json answered "unexpected end of JSON
// input" and the whole install failed on the emptiest, most plausible
// state a fresh machine can present -- while `{}`, one byte-pair away,
// worked. That was never a boundary anyone chose.
//
// It is deliberately whitespace-insensitive rather than a len(raw)==0
// check: "empty" is a property of the CONTENT, and a file holding one
// newline carries exactly as much configuration as one holding nothing.
//
// It is equally deliberately narrow. Only a document with no
// non-whitespace bytes is rewritten; anything else -- including a
// truncated or corrupt document, which is a real failure with a real
// remedy -- is handed to the parser unchanged and still fails loudly.
// Callers pass the ORIGINAL bytes to replaceConfig for the .bak, so the
// backup still records what was actually on disk.
func emptyDocumentAsObject(raw []byte) []byte {
	if len(bytes.TrimSpace(raw)) == 0 {
		return []byte("{}")
	}
	return raw
}

// locate walks raw's top-level JSON object to find containerKey (which
// must itself be an object), then walks that container's members looking
// for memberKey, returning byte offsets precise enough for apply to build
// a replacement or insertion without touching any other byte of raw. It
// uses json.Decoder.Token combined with json.Decoder.InputOffset, never
// decoding the document into a generic value and re-encoding it (D9).
//
// A document whose root has no containerKey at all is not an error: locate
// returns a createContainer location and apply synthesizes the container
// object holding the one member, mirroring what the TOML writer has always
// done with a missing table (tomlsplice.go's apply appends it at EOF).
//
// This used to be an error, on the reasoning that "every real runtime
// config this package edits already declares its MCP-server container
// object, even when empty". That was simply untrue, and untrue in the
// worst direction: a fresh opencode install writes an opencode.json with
// no "mcp" key, and Claude Code's ~/.claude.json has no "mcpServers" key
// until something adds one — so the config on a brand-new machine, the
// commonest one there is, was the single case the JSON writer hard-failed
// on while the TOML writer succeeded. Two writers behind one `register
// --target all` disagreeing about whether a missing container is fatal is
// not a scope boundary; it is a defect with a rationale attached.
//
// Synthesis stays inside D9's byte-identity contract: the container is
// inserted as one span before the root object's closing brace, reusing the
// same indentAt/needsComma rules as a member insertion, and no other byte
// of the document is rewritten. The .bak replaceConfig writes before any
// edit lands (configwrite.go) is the safety net if a runtime ever wanted a
// container shape other than a plain object.
func locate(raw []byte, containerKey, memberKey string) (location, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))

	root, err := dec.Token()
	if err != nil {
		return location{}, fmt.Errorf("parse document: %w", err)
	}
	if d, ok := root.(json.Delim); !ok || d != '{' {
		return location{}, fmt.Errorf("document root is not a JSON object")
	}
	rootOpenEnd := dec.InputOffset()
	rootBrace := bytes.IndexByte(raw, '{')
	lastRootKeyQuote := -1

	for {
		keyStart := dec.InputOffset()
		tok, err := dec.Token()
		if err != nil {
			return location{}, fmt.Errorf("parse document: %w", err)
		}
		if d, ok := tok.(json.Delim); ok && d == '}' {
			indent := indentAt(raw, rootBrace) + "  "
			if lastRootKeyQuote >= 0 {
				indent = indentAt(raw, lastRootKeyQuote)
			}
			return location{
				createContainer: true,
				insertAt:        int(keyStart),
				needsComma:      int(keyStart) != int(rootOpenEnd),
				indent:          indent,
			}, nil
		}
		key, ok := tok.(string)
		if !ok {
			return location{}, fmt.Errorf("unexpected non-string key in document")
		}
		if key != containerKey {
			lastRootKeyQuote = int(keyStart) + bytes.IndexByte(raw[keyStart:], '"')
			if _, err := skipValue(dec); err != nil {
				return location{}, fmt.Errorf("parse document: %w", err)
			}
			continue
		}

		open, err := dec.Token()
		if err != nil {
			return location{}, fmt.Errorf("parse document: %w", err)
		}
		if d, ok := open.(json.Delim); !ok || d != '{' {
			return location{}, fmt.Errorf("%q is not a JSON object", containerKey)
		}
		containerOpenEnd := dec.InputOffset()

		containerKeyQuote := int(keyStart) + bytes.IndexByte(raw[keyStart:], '"')
		fallbackIndent := indentAt(raw, containerKeyQuote) + "  "
		lastMemberKeyQuote := -1

		for {
			innerStart := dec.InputOffset()
			itok, err := dec.Token()
			if err != nil {
				return location{}, fmt.Errorf("parse document: %w", err)
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
				return location{}, fmt.Errorf("unexpected non-string key in %q", containerKey)
			}
			keyQuote := int(innerStart) + bytes.IndexByte(raw[innerStart:], '"')
			lastMemberKeyQuote = keyQuote

			valueEnd, err := skipValue(dec)
			if err != nil {
				return location{}, fmt.Errorf("parse document: %w", err)
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
// newValue over the span loc identifies (replace), immediately before the
// container's closing brace (insert), or inside a freshly synthesized
// containerKey object before the ROOT object's closing brace
// (createContainer). It never rewrites any byte outside that span.
func (loc location) apply(raw []byte, containerKey, memberKey string, newValue json.RawMessage) []byte {
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
	if loc.createContainer {
		containerLiteral, _ := json.Marshal(containerKey) // likewise a plain string
		buf.Write(containerLiteral)
		buf.WriteString(": {\n")
		buf.WriteString(loc.indent)
		buf.WriteString("  ")
	}
	buf.Write(keyLiteral)
	buf.WriteString(": ")
	buf.Write(newValue)
	if loc.createContainer {
		buf.WriteByte('\n')
		buf.WriteString(loc.indent)
		buf.WriteByte('}')
	}
	buf.Write(raw[loc.insertAt:])
	return buf.Bytes()
}

// Splice returns a copy of raw with the member at containerKey.memberKey
// replaced in place (when present), inserted into the existing container
// (when the container exists but the member does not), or inserted into a
// containerKey object synthesized at the document root (when the container
// does not exist either) — and every other byte of raw unchanged (D9;
// R-016/R-017 "Unrelated entries are preserved"). newValue is written
// verbatim as the member's value — Splice performs no validation of
// newValue's own well-formedness; that is WriteMember's job (jsonwrite.go),
// so its post-write json.Valid gate has something real to catch.
func Splice(raw []byte, containerKey, memberKey string, newValue json.RawMessage) ([]byte, error) {
	raw = emptyDocumentAsObject(raw)
	loc, err := locate(raw, containerKey, memberKey)
	if err != nil {
		return nil, err
	}
	return loc.apply(raw, containerKey, memberKey, newValue), nil
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
	raw = emptyDocumentAsObject(raw)
	loc, err := locate(raw, containerKey, memberKey)
	if err != nil {
		return nil, err
	}
	if !loc.found {
		return nil, fmt.Errorf("member %q not found in %q, nothing to remove", memberKey, containerKey)
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
