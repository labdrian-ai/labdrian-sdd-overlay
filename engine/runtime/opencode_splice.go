package runtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// A byte splice for opencode.json: a member is inserted into, or cut out of, the owner's bytes and
// every other byte stays where it was (key order, indentation, a final newline). It is the model of the writer in longterm-mem (jsonsplice.go), kept small because the
// adapter only ever touches default_agent and mcp.servers.<name>.

// jsonMember is one member of an object, by byte offsets into the document.
type jsonMember struct {
	name string
	// keyStart is the offset of the key's opening quote; valueStart and valueEnd bound the value.
	keyStart, valueStart, valueEnd int
}

// jsonObject is an object of the document: the offsets of its braces and its members in order.
type jsonObject struct {
	open, close int
	members     []jsonMember
}

func (o jsonObject) member(name string) (jsonMember, bool) {
	for _, m := range o.members {
		if m.name == name {
			return m, true
		}
	}
	return jsonMember{}, false
}

// scanObject reads the object whose opening brace is raw[open]. A key that appears twice is refused:
// which of the two OpenCode honours is not ours to guess, and a splice would touch only one.
func scanObject(raw []byte, open int) (jsonObject, error) {
	dec := json.NewDecoder(bytes.NewReader(raw[open:]))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return jsonObject{}, fmt.Errorf("it is not a JSON object")
	}
	object := jsonObject{open: open}
	seen := map[string]bool{}
	for dec.More() {
		keyStart := open + int(dec.InputOffset())
		keyStart += bytes.IndexByte(raw[keyStart:], '"')
		tok, err := dec.Token()
		if err != nil {
			return jsonObject{}, err
		}
		name, _ := tok.(string)
		if seen[name] {
			return jsonObject{}, fmt.Errorf("it has the key %q twice", name)
		}
		seen[name] = true
		valueStart := open + int(dec.InputOffset())
		for valueStart < len(raw) && (raw[valueStart] == ':' || isJSONSpace(raw[valueStart])) {
			valueStart++
		}
		if err := skipJSONValue(dec); err != nil {
			return jsonObject{}, err
		}
		object.members = append(object.members, jsonMember{name: name, keyStart: keyStart, valueStart: valueStart, valueEnd: open + int(dec.InputOffset())})
	}
	if _, err := dec.Token(); err != nil {
		return jsonObject{}, err
	}
	object.close = open + int(dec.InputOffset()) - 1
	return object, nil
}

// skipJSONValue consumes one value, scalar or balanced object or array.
func skipJSONValue(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	depth := 0
	if d, ok := tok.(json.Delim); ok && (d == '{' || d == '[') {
		depth = 1
	}
	for depth > 0 {
		tok, err := dec.Token()
		if err != nil {
			return err
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
	return nil
}

func isJSONSpace(b byte) bool { return b == ' ' || b == '\t' || b == '\n' || b == '\r' }

// rootObject scans the document's root object, and refuses bytes after it.
func rootObject(raw []byte) (jsonObject, error) {
	open := bytes.IndexFunc(raw, func(r rune) bool { return r != ' ' && r != '\t' && r != '\n' && r != '\r' })
	if open < 0 {
		return jsonObject{}, fmt.Errorf("it is empty")
	}
	root, err := scanObject(raw, open)
	if err != nil {
		return jsonObject{}, err
	}
	if len(bytes.TrimSpace(raw[root.close+1:])) > 0 {
		return jsonObject{}, fmt.Errorf("it has content after the JSON object")
	}
	return root, nil
}

// objectAt walks path from the root and returns the object it names, or ok false when a step is absent.
func objectAt(raw []byte, path []string) (object jsonObject, ok bool, err error) {
	object, err = rootObject(raw)
	if err != nil {
		return jsonObject{}, false, err
	}
	for _, step := range path {
		member, present := object.member(step)
		if !present {
			return jsonObject{}, false, nil
		}
		if member.valueStart >= len(raw) || raw[member.valueStart] != '{' {
			return jsonObject{}, false, fmt.Errorf("%q is not a JSON object", step)
		}
		if object, err = scanObject(raw, member.valueStart); err != nil {
			return jsonObject{}, false, err
		}
	}
	return object, true, nil
}

// spliceOpenCodeMember inserts name: value into the object at path, creating the objects of path the
// document lacks. The member must not be there yet.
func spliceOpenCodeMember(raw []byte, path []string, name string, value any) ([]byte, error) {
	for depth := len(path); depth >= 0; depth-- {
		object, ok, err := objectAt(raw, path[:depth])
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		wrapped := value
		memberName := name
		if depth < len(path) {
			memberName = path[depth]
			wrapped = nestedValue(path[depth+1:], name, value)
		}
		return insertMember(raw, object, memberName, wrapped)
	}
	return nil, fmt.Errorf("it is not a JSON object")
}

func nestedValue(path []string, name string, value any) any {
	wrapped := map[string]any{name: value}
	for i := len(path) - 1; i >= 0; i-- {
		wrapped = map[string]any{path[i]: wrapped}
	}
	return wrapped
}

// insertMember writes the member after the last one of object, in the style of that one: on its own
// line at its indentation, or on the same line when the object is written inline.
func insertMember(raw []byte, object jsonObject, name string, value any) ([]byte, error) {
	if len(object.members) == 0 {
		closeIndent := lineIndent(raw, object.open)
		child := closeIndent + "  "
		text, err := memberText(name, value, child, true)
		if err != nil {
			return nil, err
		}
		return replaceBytes(raw, object.open+1, object.close, "\n"+child+text+"\n"+closeIndent), nil
	}
	last := object.members[len(object.members)-1]
	indent, ownLine := leadOf(raw, last.keyStart)
	text, err := memberText(name, value, indent, ownLine)
	if err != nil {
		return nil, err
	}
	if ownLine {
		return replaceBytes(raw, last.valueEnd, last.valueEnd, ",\n"+indent+text), nil
	}
	return replaceBytes(raw, last.valueEnd, last.valueEnd, ", "+text), nil
}

// memberText is `"name": value`, the value indented from indent when it spans lines.
func memberText(name string, value any, indent string, multiline bool) (string, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if multiline {
		encoder.SetIndent(indent, "  ")
	}
	if err := encoder.Encode(value); err != nil {
		return "", err
	}
	key, err := json.Marshal(name)
	if err != nil {
		return "", err
	}
	return string(key) + ": " + strings.TrimRight(buf.String(), "\n"), nil
}

func replaceBytes(raw []byte, start, end int, with string) []byte {
	out := make([]byte, 0, len(raw)+len(with))
	out = append(out, raw[:start]...)
	out = append(out, with...)
	return append(out, raw[end:]...)
}

// lineIndent is the whitespace that starts the line holding raw[index].
func lineIndent(raw []byte, index int) string {
	start := bytes.LastIndexByte(raw[:index], '\n') + 1
	end := start
	for end < index && (raw[end] == ' ' || raw[end] == '\t') {
		end++
	}
	return string(raw[start:end])
}

// leadOf is the whitespace before raw[index] on its line, and whether nothing but it precedes index there.
func leadOf(raw []byte, index int) (indent string, ownLine bool) {
	start := index
	for start > 0 && (raw[start-1] == ' ' || raw[start-1] == '\t') {
		start--
	}
	return string(raw[start:index]), start == 0 || raw[start-1] == '\n'
}

// removeOpenCodeMember cuts name out of the object at path, with the one comma its removal leaves
// dangling. It reports false when the member is not there.
func removeOpenCodeMember(raw []byte, path []string, name string) ([]byte, bool, error) {
	object, ok, err := objectAt(raw, path)
	if err != nil || !ok {
		return raw, false, err
	}
	member, present := object.member(name)
	if !present {
		return raw, false, nil
	}
	start, end := member.keyStart, member.valueEnd
	forward := end
	for forward < len(raw) && isJSONSpace(raw[forward]) {
		forward++
	}
	trailingComma := forward < len(raw) && raw[forward] == ','
	if trailingComma {
		end = forward + 1
	}
	back := start
	for back > 0 && isJSONSpace(raw[back-1]) {
		back--
	}
	if !trailingComma && back > 0 && raw[back-1] == ',' {
		back--
	}
	return replaceBytes(raw, back, end, ""), true, nil
}

// pruneEmptyObject removes the object at path from its parent when it holds no member.
func pruneEmptyObject(raw []byte, path []string) ([]byte, error) {
	object, ok, err := objectAt(raw, path)
	if err != nil || !ok || len(object.members) > 0 {
		return raw, err
	}
	out, _, err := removeOpenCodeMember(raw, path[:len(path)-1], path[len(path)-1])
	return out, err
}
