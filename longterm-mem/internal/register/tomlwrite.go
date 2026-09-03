package register

import (
	"fmt"
	"os"

	"github.com/pelletier/go-toml/v2"
)

// WriteTOMLSection reads the TOML document at path, splices in
// tableKey.memberKey = newSection (TOMLSplice), and durably replaces
// the file:
//
//  1. validate the spliced result BEFORE touching the filesystem at all —
//     it must parse as TOML (go-toml/v2 Unmarshal) AND the resulting
//     tableKey.memberKey.command must equal binary; either failure refuses
//     the write with zero side effects, never partially written and never
//     landed on top of a working config. This is the exact purity seam
//     json.Valid gives WriteMember (jsonwrite.go, 11a-2): a splice bug that
//     silently produced a table with the wrong command, or no table at
//     all, is caught here, not by a human noticing later that codex is
//     talking to the wrong binary;
//  2. only then, back up and replace through replaceConfig — the same
//     backup-then-durable-replace sequence WriteMember uses (D9).
func WriteTOMLSection(path, tableKey, memberKey, binary string, newSection []byte) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("register: read %s: %w", path, err)
	}

	spliced, err := TOMLSplice(raw, tableKey, memberKey, newSection)
	if err != nil {
		return err
	}

	var doc map[string]interface{}
	if err := toml.Unmarshal(spliced, &doc); err != nil {
		return fmt.Errorf("register: splice of %s would produce invalid TOML, not written: %w", path, err)
	}
	command, ok := tomlNestedString(doc, tableKey, memberKey, "command")
	if !ok || command != binary {
		return fmt.Errorf("register: splice of %s would not set %s.%s.command = %q, not written", path, tableKey, memberKey, binary)
	}

	return replaceConfig(path, raw, spliced)
}

// RemoveTOMLSection reads the TOML document at path, removes
// tableKey.memberKey entirely (TOMLRemove), and durably replaces the
// file (R-019), mirroring WriteTOMLSection's own discipline exactly:
//
//  1. validate the result parses as TOML, AND that tableKey.memberKey is
//     genuinely gone afterward (the removal-side counterpart of
//     WriteTOMLSection's command == binary gate — a splice bug that left
//     the table half-removed, or removed the wrong one, is caught here
//     rather than committed), BEFORE touching the filesystem at all;
//  2. only then, back up and replace through replaceConfig.
func RemoveTOMLSection(path, tableKey, memberKey string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("register: read %s: %w", path, err)
	}

	removed, err := TOMLRemove(raw, tableKey, memberKey)
	if err != nil {
		return err
	}

	var doc map[string]interface{}
	if err := toml.Unmarshal(removed, &doc); err != nil {
		return fmt.Errorf("register: removal of %s.%s from %s would produce invalid TOML, not written: %w", tableKey, memberKey, path, err)
	}
	if _, stillPresent := tomlNestedString(doc, tableKey, memberKey, "command"); stillPresent {
		return fmt.Errorf("register: removal of %s.%s from %s did not actually remove it, not written", tableKey, memberKey, path)
	}

	return replaceConfig(path, raw, removed)
}

// tomlNestedString reads doc[tableKey][memberKey][field] as a string,
// tolerating any of the three levels being absent or the wrong shape
// (e.g. a top-level key that is a plain string, not a table) by returning
// ok=false rather than panicking — go-toml/v2 decodes an unconstrained
// document into nested map[string]interface{} values, so every level must
// be re-asserted by hand.
func tomlNestedString(doc map[string]interface{}, tableKey, memberKey, field string) (string, bool) {
	table, ok := doc[tableKey].(map[string]interface{})
	if !ok {
		return "", false
	}
	member, ok := table[memberKey].(map[string]interface{})
	if !ok {
		return "", false
	}
	value, ok := member[field].(string)
	return value, ok
}
