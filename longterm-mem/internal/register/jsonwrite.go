package register

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// WriteMember reads the JSON document at path, splices in containerKey.
// memberKey = newValue (Splice), and atomically replaces the file:
//
//  1. validate the spliced result with json.Valid BEFORE touching the
//     filesystem at all — a splice that would produce malformed JSON is
//     refused with zero side effects, never partially written and never
//     landed on top of a working config;
//  2. only then, back up the original bytes to path+".bak";
//  3. write the new content to a tmp file in the same directory, fsync it,
//     close it, then rename it into place — tmp+rename in the same
//     directory keeps the replacement atomic (a cross-device rename is not
//     atomic, which is the point of "same directory").
func WriteMember(path, containerKey, memberKey string, newValue json.RawMessage) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("register: read %s: %w", path, err)
	}

	spliced, err := Splice(raw, containerKey, memberKey, newValue)
	if err != nil {
		return fmt.Errorf("register: splice %s: %w", path, err)
	}
	if !json.Valid(spliced) {
		return fmt.Errorf("register: splice of %s would produce invalid JSON, not written", path)
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
