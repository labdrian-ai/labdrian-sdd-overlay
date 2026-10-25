// Package codexhooks owns the memory entries of Codex's shared hooks.json: SessionStart (the plan as
// instructions, C23 M12) and PreToolUse and PostToolUse (the memory gate and the read evidence,
// C23 M12b).
// It edits bytes, not files. Foreign values, including unknown fields, number spellings and string
// escapes, are never marshaled again. The command is built by settings' existing projection family,
// so shell quoting and missing-binary safety remain identical to Claude's.
package codexhooks

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/jsonstrict"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings"
)

// memoryMatcher is the matcher of the tool entries: the narrowest one the gate acts on once the
// adapter has translated Codex's names. Codex reads a matcher that holds more than letters, digits,
// "_" and "|" as an unanchored Rust regex, so it is anchored here. It names the four Engram reads
// (with an optional alphanumeric plugin prefix) and longterm-mem query and
// get, in the spelling Codex sends (the server name has its hyphen replaced by an underscore).
const memoryMatcher = `^mcp__(([A-Za-z0-9]+_)*engram__mem_(search|context|get_observation|timeline)|longterm_mem__(query|get))$`

// spec is one entry this package owns: the event, its matcher, and the flags its command carries.
type spec struct {
	event, matcher string
	extra          []string
}

// specs are the owned entries, in the order they are installed. Only the tool entries pass the
// source flag; the SessionStart entry is the one M12 installed and stays as it was.
var specs = []spec{
	{event: "SessionStart", matcher: "startup|resume|clear|compact"},
	{event: "PreToolUse", matcher: memoryMatcher, extra: []string{"--source", "codex"}},
	{event: "PostToolUse", matcher: memoryMatcher, extra: []string{"--source", "codex"}},
}

// Merge replaces owned handlers once, keeping foreign handlers and groups untouched.
func Merge(data []byte, binary string) ([]byte, bool, error) { return change(data, binary, true) }

// Remove removes only owned handlers and groups/events emptied by their removal.
func Remove(data []byte, binary string) ([]byte, bool, error) { return change(data, binary, false) }

// Missing names the events whose exact desired entry is not installed, in install order. A document that
// cannot be read is an error.
func Missing(data []byte, binary string) ([]string, error) {
	_, events, err := parse(data)
	if err != nil {
		return nil, err
	}
	var missing []string
	for _, sp := range specs {
		found := false
		for _, group := range events[sp.event] {
			if equivalent(group, desired(binary, sp)) {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, sp.event)
		}
	}
	return missing, nil
}

// Present says every owned entry is installed; it does not claim that Codex trusts them.
func Present(data []byte, binary string) (bool, error) {
	missing, err := Missing(data, binary)
	return err == nil && len(missing) == 0, err
}

func parse(data []byte) ([]byte, map[string][]json.RawMessage, error) {
	// nil means no file; a non-nil empty slice is an existing malformed document.
	if data == nil {
		data = []byte(`{}`)
	}
	if err := jsonstrict.CheckUTF8(data); err != nil {
		return nil, nil, err
	}
	if err := jsonstrict.CheckNoDuplicateKeys(data); err != nil {
		return nil, nil, err
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, nil, err
	}
	if root == nil {
		return nil, nil, fmt.Errorf("hooks document must be an object")
	}
	hooks := root["hooks"]
	if hooks == nil {
		hooks = []byte(`{}`)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(hooks, &raw); err != nil || raw == nil {
		return nil, nil, fmt.Errorf("hooks must be an object")
	}
	events := map[string][]json.RawMessage{}
	for _, sp := range specs {
		event, ok := raw[sp.event]
		if !ok {
			continue
		}
		var groups []json.RawMessage
		if err := json.Unmarshal(event, &groups); err != nil || groups == nil {
			return nil, nil, fmt.Errorf("%s must be an array", sp.event)
		}
		for _, group := range groups {
			var g map[string]json.RawMessage
			if err := json.Unmarshal(group, &g); err != nil || g == nil {
				return nil, nil, fmt.Errorf("%s group must be an object", sp.event)
			}
			var handlers []json.RawMessage
			if err := json.Unmarshal(g["hooks"], &handlers); err != nil || handlers == nil {
				if sp.event == "SessionStart" {
					return nil, nil, fmt.Errorf("group hooks must be an array")
				}
				return nil, nil, fmt.Errorf("%s group hooks must be an array", sp.event)
			}
		}
		events[sp.event] = groups
	}
	return hooks, events, nil
}

// owns checks one command, not tokens split across two handlers in the same group, with the
// recognizer the Claude families use, so legacy bare paths and shell-quoted paths (including
// apostrophes) are recognized alike.
func owns(handler json.RawMessage, binary string) bool {
	if binary == "" {
		return false
	}
	var h map[string]any
	if json.Unmarshal(handler, &h) != nil {
		return false
	}
	command, _ := h["command"].(string)
	return settings.CommandOwnedBy(command, binary, settings.LabdrianProjectionIdentity)
}

func filter(group json.RawMessage, binary string) (json.RawMessage, bool) {
	var g map[string]json.RawMessage
	_ = json.Unmarshal(group, &g)
	var handlers []json.RawMessage
	_ = json.Unmarshal(g["hooks"], &handlers)
	kept := make([]json.RawMessage, 0, len(handlers))
	changed := false
	for _, h := range handlers {
		if owns(h, binary) {
			changed = true
		} else {
			kept = append(kept, h)
		}
	}
	if !changed {
		return group, false
	}
	if len(kept) == 0 {
		return nil, true
	}
	return setMember(group, "hooks", array(kept)), true
}

// desired is the entry of one spec, built by the same projection command builder the Claude entries
// use, so the shell quoting and the missing-binary guard are identical. Its keys are marshaled in
// sorted order, which is how the SessionStart entry of M12 was written, so it stays byte-identical.
func desired(binary string, sp spec) json.RawMessage {
	entry := map[string]any{
		"hooks": []any{map[string]any{
			"type":    "command",
			"command": settings.ProjectionCommand(binary, sp.event, sp.extra...),
		}},
		"matcher": sp.matcher,
	}
	data, _ := json.Marshal(entry) // only our freshly built entry is marshaled
	return data
}

func change(data []byte, binary string, install bool) ([]byte, bool, error) {
	if binary == "" {
		return data, false, settings.ErrEmptyHookCommand
	}
	hooks, events, err := parse(data)
	if err != nil {
		return data, false, err
	}
	changed := false
	for _, sp := range specs {
		kept, eventChanged := editEvent(events[sp.event], binary, desired(binary, sp), install)
		if !eventChanged {
			continue
		}
		changed = true
		var event []byte
		if len(kept) > 0 {
			event = array(kept)
		}
		hooks = setMember(hooks, sp.event, event)
	}
	if !changed {
		return data, false, nil
	}
	if data == nil {
		data = []byte(`{}`)
	}
	return setMember(data, "hooks", hooks), true, nil
}

// editEvent installs or removes the owned handlers of one event's groups. An exact, sole owned
// entry stays byte-identical; duplicates and older commands of ours are repaired into one.
func editEvent(groups []json.RawMessage, binary string, wanted json.RawMessage, install bool) ([]json.RawMessage, bool) {
	kept := make([]json.RawMessage, 0, len(groups)+1)
	installed := false
	changed := false
	for _, group := range groups {
		filtered, owned := filter(group, binary)
		if !owned {
			kept = append(kept, group)
			continue
		}
		if install && !installed && equivalent(group, wanted) {
			kept = append(kept, group)
			installed = true
			continue
		}
		changed = true
		if filtered != nil {
			kept = append(kept, filtered)
		}
		if install && !installed {
			kept = append(kept, wanted)
			installed = true
		}
	}
	if install && !installed {
		kept = append(kept, wanted)
		changed = true
	}
	return kept, changed
}

func equivalent(a, b []byte) bool {
	var x, y any
	_ = json.Unmarshal(a, &x)
	_ = json.Unmarshal(b, &y)
	ax, _ := json.Marshal(x)
	by, _ := json.Marshal(y)
	return bytes.Equal(ax, by)
}

func array(values []json.RawMessage) []byte {
	var out bytes.Buffer
	out.WriteByte('[')
	for i, v := range values {
		if i > 0 {
			out.WriteByte(',')
		}
		out.Write(v)
	}
	out.WriteByte(']')
	return out.Bytes()
}

// member spans come from encoding/json, not a second JSON scanner. Only the selected value (or
// the selected member plus one separator when removing) is spliced. All other keys/values keep
// their original bytes, including indentation inside foreign groups and arbitrary unknown data.
type member struct {
	key               string
	start, value, end int
}

func members(data []byte) []member {
	dec := json.NewDecoder(bytes.NewReader(data))
	_, _ = dec.Token()
	var out []member
	for dec.More() {
		start := int(dec.InputOffset())
		for data[start] == ' ' || data[start] == '\n' || data[start] == '\r' || data[start] == '\t' || data[start] == ',' {
			start++
		}
		key, _ := dec.Token()
		var raw json.RawMessage
		_ = dec.Decode(&raw)
		end := int(dec.InputOffset())
		out = append(out, member{key: key.(string), start: start, value: end - len(raw), end: end})
	}
	return out
}
func setMember(data []byte, key string, value []byte) []byte {
	fields := members(data)
	splice := func(start, end int, replacement []byte) []byte {
		out := append([]byte(nil), data[:start]...)
		out = append(out, replacement...)
		return append(out, data[end:]...)
	}
	for i, m := range fields {
		if m.key == key {
			if value != nil {
				return splice(m.value, m.end, value)
			}
			if i+1 < len(fields) {
				return splice(m.start, fields[i+1].start, nil)
			}
			if i > 0 {
				return splice(fields[i-1].end, m.end, nil)
			}
			return splice(m.start, m.end, nil)
		}
	}
	if value == nil {
		return data
	}
	end := bytes.LastIndexByte(data, '}')
	encoded, _ := json.Marshal(key)
	addition := append(encoded, ':')
	addition = append(addition, value...)
	if len(fields) > 0 {
		addition = append([]byte{','}, addition...)
	}
	return splice(end, end, addition)
}
