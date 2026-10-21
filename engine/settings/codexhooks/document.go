// Package codexhooks owns the SessionStart memory-instructions entry in Codex's shared hooks.json.
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

const matcher = "startup|resume|clear|compact"

// Merge replaces owned handlers once, keeping foreign handlers and groups untouched.
func Merge(data []byte, binary string) ([]byte, bool, error) { return change(data, binary, true) }

// Remove removes only owned handlers and groups/events emptied by their removal.
func Remove(data []byte, binary string) ([]byte, bool, error) { return change(data, binary, false) }

// Present says an owned handler is installed; it does not claim that Codex trusts it.
func Present(data []byte, binary string) (bool, error) {
	_, groups, err := parse(data)
	if err != nil {
		return false, err
	}
	for _, group := range groups {
		_, owned := filter(group, binary)
		if owned {
			return true, nil
		}
	}
	return false, nil
}

func parse(data []byte) ([]byte, []json.RawMessage, error) {
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
	var events map[string]json.RawMessage
	if err := json.Unmarshal(hooks, &events); err != nil || events == nil {
		return nil, nil, fmt.Errorf("hooks must be an object")
	}
	var groups []json.RawMessage
	if event, ok := events["SessionStart"]; ok {
		if err := json.Unmarshal(event, &groups); err != nil || groups == nil {
			return nil, nil, fmt.Errorf("SessionStart must be an array")
		}
		for _, group := range groups {
			var g map[string]json.RawMessage
			if err := json.Unmarshal(group, &g); err != nil || g == nil {
				return nil, nil, fmt.Errorf("SessionStart group must be an object")
			}
			var handlers []json.RawMessage
			if err := json.Unmarshal(g["hooks"], &handlers); err != nil || handlers == nil {
				return nil, nil, fmt.Errorf("group hooks must be an array")
			}
		}
	}
	return hooks, groups, nil
}

// owns checks one command, not tokens split across two handlers in the same group. The settings
// query already recognizes both legacy bare paths and shell-quoted paths (including apostrophes).
func owns(handler json.RawMessage, binary string) bool {
	if binary == "" {
		return false
	}
	var h map[string]any
	if json.Unmarshal(handler, &h) != nil {
		return false
	}
	root := map[string]any{"hooks": map[string]any{"SessionStart": []any{map[string]any{"hooks": []any{h}}}}}
	return settings.HasHookContaining(root, "SessionStart", binary, settings.LabdrianProjectionIdentity)
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

func desired(binary string) json.RawMessage {
	entry := settings.ProjectionHookEntries(binary)["SessionStart"][0].(map[string]any)
	entry["matcher"] = matcher
	data, _ := json.Marshal(entry) // only our freshly built entry is marshaled
	return data
}

func change(data []byte, binary string, install bool) ([]byte, bool, error) {
	if binary == "" {
		return data, false, settings.ErrEmptyHookCommand
	}
	hooks, groups, err := parse(data)
	if err != nil {
		return data, false, err
	}
	wanted := desired(binary)
	kept := make([]json.RawMessage, 0, len(groups)+1)
	installed := false
	changed := false
	for _, group := range groups {
		filtered, owned := filter(group, binary)
		if !owned {
			kept = append(kept, group)
			continue
		}
		// An exact, sole owned entry stays byte-identical. Duplicates and older commands are repaired.
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
	if !changed {
		return data, false, nil
	}
	var event []byte
	if len(kept) > 0 {
		event = array(kept)
	}
	hooks = setMember(hooks, "SessionStart", event)
	if data == nil {
		data = []byte(`{}`)
	}
	return setMember(data, "hooks", hooks), true, nil
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
