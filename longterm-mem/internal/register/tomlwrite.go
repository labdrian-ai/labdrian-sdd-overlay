package register

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

// WriteTOMLSection reads the TOML document at path, splices in
// tableKey.memberKey = newSection (TOMLSplice), and atomically replaces
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
//  2. only then, back up the original bytes to path+".bak";
//  3. write the new content to a tmp file in the same directory, fsync it,
//     close it, then rename it into place — the same same-directory
//     tmp+rename atomicity WriteMember uses (D9).
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

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("register: create temp file for %s: %w", path, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename below succeeds

	if _, err := tmp.Write(spliced); err != nil {
		tmp.Close()
		return fmt.Errorf("register: write temp file for %s: %w", path, err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("register: fsync temp file for %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("register: close temp file for %s: %w", path, err)
	}

	bakPath := path + ".bak"
	if err := os.WriteFile(bakPath, raw, 0o600); err != nil {
		return fmt.Errorf("register: write backup %s: %w", bakPath, err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("register: rename temp file into %s: %w", path, err)
	}
	return nil
}

// RemoveTOMLSection reads the TOML document at path, removes
// tableKey.memberKey entirely (TOMLRemove), and atomically replaces the
// file (R-019), mirroring WriteTOMLSection's own discipline exactly:
//
//  1. validate the result parses as TOML, AND that tableKey.memberKey is
//     genuinely gone afterward (the removal-side counterpart of
//     WriteTOMLSection's command == binary gate — a splice bug that left
//     the table half-removed, or removed the wrong one, is caught here
//     rather than committed), BEFORE touching the filesystem at all;
//  2. only then, back up the original bytes to path+".bak";
//  3. write the new content to a tmp file in the same directory, fsync it,
//     close it, then rename it into place.
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

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("register: create temp file for %s: %w", path, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename below succeeds

	if _, err := tmp.Write(removed); err != nil {
		tmp.Close()
		return fmt.Errorf("register: write temp file for %s: %w", path, err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("register: fsync temp file for %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("register: close temp file for %s: %w", path, err)
	}

	bakPath := path + ".bak"
	if err := os.WriteFile(bakPath, raw, 0o600); err != nil {
		return fmt.Errorf("register: write backup %s: %w", bakPath, err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("register: rename temp file into %s: %w", path, err)
	}
	return nil
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
