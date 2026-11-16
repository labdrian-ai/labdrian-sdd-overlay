package register

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// nestedSep joins the two keys of a container that lives one object deeper
// than the document root ("mcp/servers" is root.mcp.servers). opencode v2
// keeps its MCP servers there; every other runtime's container is a single
// root key and never contains this byte, so a plain key still takes the
// original single-level path through Splice/Remove untouched.
const nestedSep = "/"

// displayContainer spells a container key the way opencode's config does
// ("mcp.servers"), for messages.
func displayContainer(containerKey string) string {
	return strings.ReplaceAll(containerKey, nestedSep, ".")
}

// splitContainer reports whether containerKey names a nested container and,
// when it does, its outer and inner keys.
func splitContainer(containerKey string) (outer, inner string, nested bool) {
	outer, inner, nested = strings.Cut(containerKey, nestedSep)
	return outer, inner, nested
}

// spliceNested is Splice for root.outer.inner.memberKey. It never rewrites a
// byte outside the span it changes (D9): when inner already exists, its
// object is spliced as a standalone document wrapped in a one-key object, so
// the single-level locate/apply rules (comma, indentation, replace in place)
// are reused rather than copied; when inner is missing it is inserted into
// outer (or outer is synthesized) holding the one new member.
func spliceNested(raw []byte, outer, inner, memberKey string, newValue json.RawMessage) ([]byte, error) {
	raw = emptyDocumentAsObject(raw)
	loc, err := locate(raw, outer, inner)
	if err != nil {
		return nil, err
	}
	if !loc.found {
		keyLiteral, _ := json.Marshal(memberKey) // plain string; Marshal cannot fail
		wrapped := append(append([]byte("{"), keyLiteral...), ':')
		wrapped = append(wrapped, newValue...)
		wrapped = append(wrapped, '}')
		return loc.apply(raw, outer, inner, wrapped), nil
	}
	valueStart, err := nestedValueStart(raw, loc, inner)
	if err != nil {
		return nil, err
	}
	sub, prefix := wrapObject(raw[valueStart:loc.replaceEnd], inner, indentAt(raw, loc.replaceStart))
	out, err := Splice(sub, inner, memberKey, newValue)
	if err != nil {
		return nil, err
	}
	return replaceSpan(raw, valueStart, loc.replaceEnd, out[len(prefix):len(out)-1]), nil
}

// removeNested is Remove for root.outer.inner.memberKey.
func removeNested(raw []byte, outer, inner, memberKey string) ([]byte, error) {
	raw = emptyDocumentAsObject(raw)
	loc, err := locate(raw, outer, inner)
	if err != nil {
		return nil, err
	}
	if !loc.found {
		return nil, fmt.Errorf("member %q not found in %q, nothing to remove", memberKey, displayContainer(outer+nestedSep+inner))
	}
	valueStart, err := nestedValueStart(raw, loc, inner)
	if err != nil {
		return nil, err
	}
	sub, prefix := wrapObject(raw[valueStart:loc.replaceEnd], inner, indentAt(raw, loc.replaceStart))
	if inLoc, locErr := locate(sub, inner, memberKey); locErr == nil && !inLoc.found {
		return nil, fmt.Errorf("member %q not found in %q, nothing to remove", memberKey, displayContainer(outer+nestedSep+inner))
	}
	out, err := Remove(sub, inner, memberKey)
	if err != nil {
		return nil, err
	}
	return replaceSpan(raw, valueStart, loc.replaceEnd, out[len(prefix):len(out)-1]), nil
}

// nestedValueStart returns the offset of the inner container's value, which
// must be a JSON object, given the located member whose key starts at
// loc.replaceStart.
func nestedValueStart(raw []byte, loc location, inner string) (int, error) {
	keyLiteral, _ := json.Marshal(inner)
	p := loc.replaceStart + len(keyLiteral)
	for p < loc.replaceEnd && raw[p] != ':' {
		p++
	}
	p++
	for p < loc.replaceEnd && isJSONLineSpace(raw[p]) {
		p++
	}
	if p >= loc.replaceEnd || raw[p] != '{' {
		return 0, fmt.Errorf("%q is not a JSON object", inner)
	}
	return p, nil
}

// wrapObject puts value under key in a one-key object whose key sits at indent,
// the depth it really has, so a member inserted into an empty value is indented
// from its real depth.
func wrapObject(value []byte, key, indent string) (wrapped []byte, prefix []byte) {
	keyLiteral, _ := json.Marshal(key)
	prefix = append(append(append([]byte("{\n"), indent...), keyLiteral...), ':')
	wrapped = append(append(append([]byte{}, prefix...), value...), '}')
	return wrapped, prefix
}

func replaceSpan(raw []byte, start, end int, with []byte) []byte {
	var buf bytes.Buffer
	buf.Write(raw[:start])
	buf.Write(with)
	buf.Write(raw[end:])
	return buf.Bytes()
}

// spliceAny and removeAny pick the single-level or nested implementation for
// a container key, so WriteMember/RemoveMember stay one code path.
func spliceAny(raw []byte, containerKey, memberKey string, newValue json.RawMessage) ([]byte, error) {
	if outer, inner, nested := splitContainer(containerKey); nested {
		return spliceNested(raw, outer, inner, memberKey, newValue)
	}
	return Splice(raw, containerKey, memberKey, newValue)
}

func removeAny(raw []byte, containerKey, memberKey string) ([]byte, error) {
	if outer, inner, nested := splitContainer(containerKey); nested {
		return removeNested(raw, outer, inner, memberKey)
	}
	return Remove(raw, containerKey, memberKey)
}

// readNestedMember reads root.outer.inner.memberKey the way readMember reads
// a single-level member: a missing file, outer, inner or member is absence,
// not an error.
func readNestedMember(path, outer, inner, memberKey string) (json.RawMessage, bool, error) {
	val, ok, err := readMember(path, outer, inner)
	if err != nil || !ok {
		return nil, false, err
	}
	var container map[string]json.RawMessage
	if err := json.Unmarshal(val, &container); err != nil {
		return nil, false, fmt.Errorf("parse %s %q: %w", path, displayContainer(outer+nestedSep+inner), err)
	}
	member, ok := container[memberKey]
	return member, ok, nil
}
