// Package vaultreg resolves each project's claude-obsidian long-term vault
// path from a per-project registry file, ~/.labdrian-overlay/vaults.json by
// default (D5). Resolution never falls back to a code-level constant for a
// specific project: the registry file is the single source of truth, and
// it is pre-seeded with one default row only when the file does not yet
// exist. Deleting a row — including the pre-seeded one — means "not
// configured" (R-022, R-023), never "guess the old value again".
package vaultreg

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultProject is the project the registry is pre-seeded for the first
// time vaults.json is created. It names only what Seed writes; Resolve
// never special-cases it as a lookup fallback (R-022).
const DefaultProject = "labdrian-sdd-overlay"

// DefaultVaultPath is the vault path Seed writes for DefaultProject,
// unexpanded (Resolve expands the leading ~ like any other row).
const DefaultVaultPath = "~/labdrian-brain"

// schemaVersion is the current vaults.json schema version (D5).
const schemaVersion = 1

// vaultEnvVar is the environment variable that overrides a project's
// registry row, but is itself overridden by the --vault flag (D5).
const vaultEnvVar = "LONGTERM_MEM_VAULT"

// ErrVaultNotConfigured is returned by Resolve when no --vault flag, no
// LONGTERM_MEM_VAULT env var, and no registry row apply to the requested
// project. Callers map it to exit code 3 (R-023).
var ErrVaultNotConfigured = errors.New("vaultreg: vault not configured")

// Registry is the on-disk vaults.json model: one row per project, each an
// ordinary, independently editable/deletable JSON entry (R-022).
type Registry struct {
	Schema int                   `json:"schema"`
	Vaults map[string]VaultEntry `json:"vaults"`
}

// VaultEntry is one project's registry row.
type VaultEntry struct {
	Path   string `json:"path"`
	Seeded bool   `json:"seeded,omitempty"`
}

// Load reads and parses the registry file at path.
func Load(path string) (*Registry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("vaultreg: read %s: %w", path, err)
	}

	var reg Registry
	if err := json.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("vaultreg: parse %s: %w", path, err)
	}
	if reg.Vaults == nil {
		reg.Vaults = map[string]VaultEntry{}
	}
	return &reg, nil
}

// Seed writes the pre-seeded default row (DefaultProject → DefaultVaultPath)
// to path, but only when path does not exist yet. A file that already
// exists is left untouched no matter its contents — even one lacking the
// default row — so deleting the seed row means "not configured", not
// "re-seed it" (D5).
func Seed(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("vaultreg: stat %s: %w", path, err)
	}

	reg := Registry{
		Schema: schemaVersion,
		Vaults: map[string]VaultEntry{
			DefaultProject: {Path: DefaultVaultPath, Seeded: true},
		},
	}
	return writeJSONAtomic(path, reg)
}

// Resolve determines project's vault path using flag > env > registry-row
// precedence (D5). vaultsPath is the registry file to lazily seed (only
// when absent) and read; flagVault is the --vault flag value, empty when
// unset. Every resolved path is ~-expanded and validated absolute before
// being returned. When none of the three sources apply, Resolve returns
// ErrVaultNotConfigured rather than guessing a path (R-023).
func Resolve(vaultsPath, project, flagVault string) (string, error) {
	if flagVault != "" {
		return expandVaultPath(flagVault)
	}
	if envVault := os.Getenv(vaultEnvVar); envVault != "" {
		return expandVaultPath(envVault)
	}

	if err := Seed(vaultsPath); err != nil {
		return "", err
	}
	reg, err := Load(vaultsPath)
	if err != nil {
		return "", err
	}

	entry, ok := reg.Vaults[project]
	if !ok {
		return "", fmt.Errorf("%w: project %q has no vault-registry entry in %s", ErrVaultNotConfigured, project, vaultsPath)
	}
	return expandVaultPath(entry.Path)
}

// expandVaultPath expands a leading ~ to the caller's home directory, then
// validates the result is an absolute path (D5).
func expandVaultPath(raw string) (string, error) {
	expanded := raw

	switch {
	case raw == "~":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("vaultreg: resolve home directory: %w", err)
		}
		expanded = home
	case strings.HasPrefix(raw, "~/"):
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("vaultreg: resolve home directory: %w", err)
		}
		expanded = filepath.Join(home, raw[2:])
	}

	if !filepath.IsAbs(expanded) {
		return "", fmt.Errorf("vaultreg: vault path %q must be absolute after expansion", raw)
	}
	return expanded, nil
}

// writeJSONAtomic marshals v as 2-space-indented JSON and writes it to path
// via tmp-file + fsync + rename, so a reader never observes a partially
// written registry. Extracted for reuse by later sidecar writers (D6).
func writeJSONAtomic(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("vaultreg: marshal %s: %w", path, err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	// A fresh install has no state directory yet; materialize it so the
	// first-run seed cannot fail with ENOENT (review finding
	// R3-seed-missing-parent-dir). 0o700: the registry is per-user config.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("vaultreg: create directory for %s: %w", path, err)
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("vaultreg: create temp file for %s: %w", path, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename below succeeds

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("vaultreg: write temp file for %s: %w", path, err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("vaultreg: fsync temp file for %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("vaultreg: close temp file for %s: %w", path, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("vaultreg: rename temp file into %s: %w", path, err)
	}
	return nil
}
