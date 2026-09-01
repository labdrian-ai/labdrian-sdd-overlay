package register

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// installStateSchemaVersion is the current install-state.json schema
// version (D9).
const installStateSchemaVersion = 1

// InstallState is the sidecar ownership record install-state.json holds:
// one TargetRecord per runtime target, keyed by target name (e.g.
// "claude", "opencode", "codex"). This is the ONLY place longterm-mem's
// ownership fingerprint lives — never inside a runtime's own config file,
// so that runtime's schema never carries an unknown key it doesn't
// recognize (R-016, R-017).
type InstallState struct {
	Schema  int                     `json:"schema"`
	Targets map[string]TargetRecord `json:"targets"`
}

// TargetRecord is one runtime target's ownership record: the sha256
// fingerprint (Fingerprint) of the exact MCP entry bytes longterm-mem last
// wrote for that target.
type TargetRecord struct {
	Fingerprint string `json:"fingerprint"`
}

// Fingerprint returns the sha256 hex digest of entry — the exact bytes
// longterm-mem wrote for a runtime's MCP entry. Decide (11a.6) compares
// this against the entry currently on disk to tell an unmodified,
// longterm-mem-owned entry from a stale or hand-edited one.
func Fingerprint(entry []byte) string {
	sum := sha256.Sum256(entry)
	return hex.EncodeToString(sum[:])
}

// LoadInstallState reads install-state.json at path. A file that does not
// exist yet is not an error — a fresh install has no state yet — and
// yields an empty, ready-to-use InstallState instead.
func LoadInstallState(path string) (*InstallState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &InstallState{Schema: installStateSchemaVersion, Targets: map[string]TargetRecord{}}, nil
		}
		return nil, fmt.Errorf("register: read %s: %w", path, err)
	}

	var st InstallState
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("register: parse %s: %w", path, err)
	}
	if st.Targets == nil {
		st.Targets = map[string]TargetRecord{}
	}
	return &st, nil
}

// Get returns target's record and whether one is present.
func (s *InstallState) Get(target string) (TargetRecord, bool) {
	rec, ok := s.Targets[target]
	return rec, ok
}

// Set records rec for target.
func (s *InstallState) Set(target string, rec TargetRecord) {
	if s.Targets == nil {
		s.Targets = map[string]TargetRecord{}
	}
	s.Targets[target] = rec
}

// Save writes s to path atomically: tmp file in the same directory,
// fsync, close, then rename — mirroring vaultreg's writeJSONAtomic
// convention (D6).
func (s *InstallState) Save(path string) error {
	if s.Schema == 0 {
		s.Schema = installStateSchemaVersion
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("register: marshal %s: %w", path, err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("register: create directory for %s: %w", path, err)
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("register: create temp file for %s: %w", path, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename below succeeds

	if _, err := tmp.Write(data); err != nil {
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
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("register: rename temp file into %s: %w", path, err)
	}
	return nil
}
