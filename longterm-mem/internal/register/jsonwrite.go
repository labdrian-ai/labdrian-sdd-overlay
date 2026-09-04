package register

import (
	"encoding/json"
	"fmt"
	"os"
)

// WriteMember reads the JSON document at path, splices in containerKey.
// memberKey = newValue (Splice), and durably replaces the file:
//
//  1. validate the spliced result with json.Valid BEFORE touching the
//     filesystem at all — a splice that would produce malformed JSON is
//     refused with zero side effects, never partially written and never
//     landed on top of a working config;
//  2. only then, hand both halves to replaceConfig, which backs the
//     original bytes up to path+".bak" and replaces path through
//     durable.WriteFile — atomic, and preserving the mode and any symlink
//     the user's own config carries.
func WriteMember(path, containerKey, memberKey string, newValue json.RawMessage) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	spliced, err := Splice(raw, containerKey, memberKey, newValue)
	if err != nil {
		return fmt.Errorf("splice %s: %w", path, err)
	}
	if !json.Valid(spliced) {
		return fmt.Errorf("splice of %s would produce invalid JSON, not written", path)
	}

	return replaceConfig(path, raw, spliced)
}

// RemoveMember reads the JSON document at path, removes containerKey.
// memberKey entirely (Remove), and durably replaces the file (R-019),
// mirroring WriteMember's own discipline exactly:
//
//  1. validate the result with json.Valid, AND that memberKey is genuinely
//     gone from containerKey afterward, BEFORE touching the filesystem at
//     all — a removal that would produce malformed JSON, or that somehow
//     left the member behind, is refused with zero side effects;
//  2. only then, back up and replace through replaceConfig.
func RemoveMember(path, containerKey, memberKey string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	removed, err := Remove(raw, containerKey, memberKey)
	if err != nil {
		return fmt.Errorf("remove from %s: %w", path, err)
	}
	if !json.Valid(removed) {
		return fmt.Errorf("removing %s.%s from %s would produce invalid JSON, not written", containerKey, memberKey, path)
	}
	if stillPresent := jsonMemberPresent(removed, containerKey, memberKey); stillPresent {
		return fmt.Errorf("removal of %s.%s from %s did not actually remove it, not written", containerKey, memberKey, path)
	}

	return replaceConfig(path, raw, removed)
}

// jsonMemberPresent reports whether containerKey.memberKey is still present
// in raw — RemoveMember's own post-removal purity gate. A decode failure is
// treated as "not present" here rather than surfaced: json.Valid already
// gated the caller before this runs, so raw is known-valid JSON by this
// point.
func jsonMemberPresent(raw []byte, containerKey, memberKey string) bool {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		return false
	}
	containerRaw, ok := doc[containerKey]
	if !ok {
		return false
	}
	var container map[string]json.RawMessage
	if err := json.Unmarshal(containerRaw, &container); err != nil {
		return false
	}
	_, present := container[memberKey]
	return present
}
