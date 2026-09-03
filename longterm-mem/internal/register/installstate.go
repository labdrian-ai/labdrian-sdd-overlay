package register

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/durable"
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

// Delete removes target's ownership record, if any (12b.4, R-019). A
// target with no record is a no-op, mirroring Get's own tolerant
// "not found" contract -- unregistering a target install-state never owned
// in the first place is exactly Decide's ActionRefuse/unmanaged path
// (writer.go, jsonUninstall/tomlUninstall), which never reaches Delete at
// all.
func (s *InstallState) Delete(target string) {
	if s.Targets == nil {
		return
	}
	delete(s.Targets, target)
}

// Save writes s to path through durable.WriteFile — the same single
// replacement primitive the runtime-config writers use (D6).
//
// Unlike those, install-state.json is longterm-mem's own sidecar in
// longterm-mem's own state directory, so durable.WriteFile's mode- and
// symlink-preservation buy nothing here: nobody else creates or edits this
// file. What it does buy is the reason to route through it anyway. This
// file IS the ownership record, and the whole point of installWithRollback
// is that a config edit with no matching record here makes every later run
// refuse (11b-1); a rename whose directory entry never reached the disk is
// exactly that state after a power loss, and durable.WriteFile fsyncs the
// directory where three hand-rolled copies of this sequence did not. The
// second reason is that there were three hand-rolled copies: a subtle
// sequence duplicated per package is how this module ended up carrying two
// different, and both incomplete, write disciplines at once.
func (s *InstallState) Save(path string) error {
	if s.Schema == 0 {
		s.Schema = installStateSchemaVersion
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("register: marshal %s: %w", path, err)
	}
	data = append(data, '\n')

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("register: create directory for %s: %w", path, err)
	}
	if err := durable.WriteFile(path, data, configCreatePerm); err != nil {
		return fmt.Errorf("register: %w", err)
	}
	return nil
}
