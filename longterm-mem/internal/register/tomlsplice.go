package register

import (
	"bytes"
	"fmt"
	"regexp"

	"github.com/pelletier/go-toml/v2"
)

// tomlLocation is locateTOMLSection's result: either the exact byte span
// of an existing table to replace, or nothing (found is false), in which
// case the caller appends the new table at EOF instead (there is no
// closing delimiter to insert before, unlike a JSON object).
type tomlLocation struct {
	// found is whether a table header matching tableKey.memberKey was
	// located.
	found bool
	// start is the byte offset of the header line's first byte
	// (inclusive); end is the offset of the next top-level-or-nested
	// table header line, or len(raw) when the section runs to EOF
	// (exclusive). Valid only when found is true.
	start, end int
}

// tomlHeaderPattern builds the header regex for tableKey.memberKey, e.g.
// `^\s*\[mcp_servers\.(?:"longterm-mem"|longterm-mem)\]\s*$` for
// tableKey="mcp_servers", memberKey="longterm-mem" (12a.1) — codex's own
// config.toml may quote the dotted key's last segment or not; both are
// valid TOML for a key containing no character requiring quoting, and a
// human editing the file by hand may have used either form.
func tomlHeaderPattern(tableKey, memberKey string) *regexp.Regexp {
	table := regexp.QuoteMeta(tableKey)
	member := regexp.QuoteMeta(memberKey)
	return regexp.MustCompile(`^\s*\[` + table + `\.(?:"` + member + `"|` + member + `)\]\s*$`)
}

// tomlAnyHeaderPattern matches any TOML table header line — used to find
// where an existing tableKey.memberKey section ends, mirroring
// engine/runtime's read-only codexHeaderPattern scan (D9's fingerprint
// side; this package owns the write side).
var tomlAnyHeaderPattern = regexp.MustCompile(`^\s*\[`)

// locateTOMLSection walks raw line by line looking for a header matching
// tableKey.memberKey (see tomlHeaderPattern). When found, the section's
// span runs from that header line up to (but not including) any blank
// line(s) trailing its last content line, ending at the next line
// matching tomlAnyHeaderPattern (any other table header, nested or not)
// or EOF — task 12a.1's stated rule, refined to leave a blank-line
// separator before the following section (if any) outside the span, so a
// replace never eats the formatting that visually separates the located
// table from what comes after it. This is a line-oriented scan, not a
// full TOML parse: it never decodes or re-encodes raw (D9), so nothing
// outside the located span is ever at risk of being reformatted.
func locateTOMLSection(raw []byte, tableKey, memberKey string) tomlLocation {
	header := tomlHeaderPattern(tableKey, memberKey)

	headerStart := -1
	contentEnd := -1
	offset := 0
	for offset < len(raw) {
		lineStart := offset
		line, nextOffset := nextTOMLLine(raw, offset)
		offset = nextOffset

		if headerStart == -1 {
			if header.Match(line) {
				headerStart = lineStart
				contentEnd = nextOffset
			}
			continue
		}
		if tomlAnyHeaderPattern.Match(line) {
			return tomlLocation{found: true, start: headerStart, end: contentEnd}
		}
		if len(bytes.TrimSpace(line)) > 0 {
			contentEnd = nextOffset
		}
	}

	if headerStart == -1 {
		return tomlLocation{found: false}
	}
	return tomlLocation{found: true, start: headerStart, end: contentEnd}
}

// nextTOMLLine returns the line starting at offset (without its trailing
// '\n', if any) and the offset of the byte immediately after that '\n'
// (or len(raw) when offset's line is the last one and has no trailing
// newline).
func nextTOMLLine(raw []byte, offset int) (line []byte, next int) {
	if idx := bytes.IndexByte(raw[offset:], '\n'); idx != -1 {
		return raw[offset : offset+idx], offset + idx + 1
	}
	return raw[offset:], len(raw)
}

// apply builds the spliced document from raw and loc: newSection either
// overwrites the span loc identifies (replace) or is appended at EOF,
// preceded by a blank line separator when raw is non-empty (insert). It
// never rewrites any byte outside that span — the same D9 byte-
// preservation contract Splice (jsonsplice.go) holds for JSON. newSection
// is written verbatim and is expected to already end in a trailing '\n'.
func (loc tomlLocation) apply(raw, newSection []byte) []byte {
	if loc.found {
		var buf bytes.Buffer
		buf.Write(raw[:loc.start])
		buf.Write(newSection)
		buf.Write(raw[loc.end:])
		return buf.Bytes()
	}

	var buf bytes.Buffer
	buf.Write(raw)
	if len(raw) > 0 {
		if raw[len(raw)-1] != '\n' {
			buf.WriteByte('\n')
		}
		buf.WriteByte('\n')
	}
	buf.Write(newSection)
	return buf.Bytes()
}

// TOMLSplice returns a copy of raw with the tableKey.memberKey table
// either replaced in place (when a header matching it is found) or
// appended at EOF (when absent), and every other byte of raw unchanged
// (D9; R-018 "Unrelated sections and ordering are preserved"). newSection
// is written verbatim as the table's full text (header line included) —
// TOMLSplice performs no validation of newSection's own well-formedness;
// that is WriteTOMLSection's job (tomlwrite.go), so its post-write
// go-toml/v2 validation gate has something real to catch.
//
// It returns an error when it cannot prove where the located table ends,
// and then returns no bytes at all: see assertSpanIsWholeTable. A caller
// must never receive a partially-overwritten document as a success.
func TOMLSplice(raw []byte, tableKey, memberKey string, newSection []byte) ([]byte, error) {
	loc := locateTOMLSection(raw, tableKey, memberKey)
	if loc.found {
		if err := assertSpanIsWholeTable(raw[loc.start:loc.end], tableKey, memberKey); err != nil {
			return nil, err
		}
	}
	return loc.apply(raw, newSection), nil
}

// TOMLRemove returns a copy of raw with the tableKey.memberKey table
// removed entirely — header line, body, AND exactly the one blank-line
// separator its removal makes dangling — and every other byte of raw
// unchanged (D9; R-019 "Selective removal across all three runtimes").
// This is TOMLSplice's insert-path blank-line rule (apply, above) run in
// reverse: the separator AFTER the table is trimmed when a following
// section exists (this table was not last), otherwise the separator
// BEFORE it is trimmed (this table WAS last) — the same forward/backward
// duality Remove (jsonsplice.go) uses for JSON's comma. It shares
// assertSpanIsWholeTable's guard: a span the line-oriented scan cannot
// prove is a whole table is refused rather than removed. It errors when no
// header matching tableKey.memberKey is found — callers only ever reach
// this after confirming the table is present.
func TOMLRemove(raw []byte, tableKey, memberKey string) ([]byte, error) {
	loc := locateTOMLSection(raw, tableKey, memberKey)
	if !loc.found {
		return nil, fmt.Errorf("table %s.%s not found, nothing to remove", tableKey, memberKey)
	}
	if err := assertSpanIsWholeTable(raw[loc.start:loc.end], tableKey, memberKey); err != nil {
		return nil, err
	}

	start, end := loc.start, loc.end
	if nextHeaderStart, ok := tomlFollowingHeaderStart(raw, end); ok {
		end = nextHeaderStart
	} else {
		start = tomlTrimBackwardBlankLines(raw, start)
	}

	var buf bytes.Buffer
	buf.Write(raw[:start])
	buf.Write(raw[end:])
	return buf.Bytes(), nil
}

// tomlFollowingHeaderStart scans forward from offset, skipping only blank
// lines, and reports the byte offset of the next table header line when
// one immediately follows — i.e. whether the table TOMLRemove just located
// has a following section at all, mirroring locateTOMLSection's own "any
// header ends the span" scan discipline.
func tomlFollowingHeaderStart(raw []byte, offset int) (int, bool) {
	for offset < len(raw) {
		lineStart := offset
		line, next := nextTOMLLine(raw, offset)
		if len(bytes.TrimSpace(line)) == 0 {
			offset = next
			continue
		}
		return lineStart, tomlAnyHeaderPattern.Match(line)
	}
	return 0, false
}

// tomlTrimBackwardBlankLines walks backward from start one whole line at a
// time, consuming every blank line immediately preceding it, and returns
// the offset right after the nearest non-blank line — the mirror of
// tomlFollowingHeaderStart's forward scan, used only when the table
// TOMLRemove located has no following section.
func tomlTrimBackwardBlankLines(raw []byte, start int) int {
	pos := start
	for pos > 0 {
		lineEnd := pos - 1 // raw[lineEnd] is the previous line's trailing '\n'
		lineStart := bytes.LastIndexByte(raw[:lineEnd], '\n') + 1
		if len(bytes.TrimSpace(raw[lineStart:lineEnd])) != 0 {
			return pos
		}
		pos = lineStart
	}
	return pos
}

// assertSpanIsWholeTable checks that the located span is a whole table and
// not a truncated prefix of one.
//
// The scan that produced it is line-oriented by design — it must never
// decode and re-encode the document (D9) — so a line whose first
// non-space byte is '[' is indistinguishable from the next table header
// without a full TOML lexer. An array element written on its own line,
// and a header-looking line inside a multi-line string, both look exactly
// like the next section. Ending the span there and writing over it
// strands the rest of the value after the replacement, producing a
// structurally corrupt document.
//
// Rather than emulate a lexer badly, the span is checked for the one
// property a correctly-located table span always has and a truncated one
// never does: it parses as a TOML document on its own. This is a read of
// bytes already located, not a decode-re-encode of the document, so the
// byte-preservation contract is untouched. A span that fails is refused
// with an actionable message instead of silently half-written.
func assertSpanIsWholeTable(span []byte, tableKey, memberKey string) error {
	var probe map[string]interface{}
	if err := toml.Unmarshal(span, &probe); err != nil {
		return fmt.Errorf("cannot determine where the %s.%s table ends (a line beginning with %q inside it is indistinguishable from the next table header); edit it by hand or put its values on one line each: %w", tableKey, memberKey, "[", err)
	}
	return nil
}
