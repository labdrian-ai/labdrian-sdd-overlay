// Package vaultreg decides where the long-term vault of a project is: the --vault flag, else the environment's
// LONGTERM_MEM_VAULT, else the project's row in a per-project registry, ~/.labdrian-overlay/vaults.json by default
// (D5). Resolution never falls back to a code-level constant for a specific project: the registry is the single
// source of truth, and a row deleted from it, the one a machine starts with included, means "not configured"
// (R-022, R-023), never "guess the old value again".
//
// The package owns the rule and the ports it reads through, and touches nothing of the machine. Resolve is a
// function of what it is handed: the flag, the environment's view (Environment), the registry and the project.
// Where the registry is kept, and how it is read and written, is the Store port, which internal/vaultregfs
// answers on a file system and the composition root wires; what a machine that has no registry is taken to hold,
// and the row a command that writes seeds it with, are the Defaults the composition root hands to Vaults.
package vaultreg

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// ErrVaultNotConfigured is returned by Resolve when no --vault flag, no LONGTERM_MEM_VAULT value, and no registry
// row apply to the requested project. Callers map it to exit code 3 (R-023).
var ErrVaultNotConfigured = errors.New("vaultreg: vault not configured")

// Registry is the registry of vaults: one row per project, each an ordinary, independently editable and deletable
// entry (R-022).
type Registry struct {
	// Vaults are the rows, by project.
	Vaults map[string]Entry
	// Origin is where the registry is kept, which the words of a project with no row name.
	Origin string
}

// Entry is one project's row.
type Entry struct {
	// Path is the vault of the project, with a leading ~ not yet expanded.
	Path string
	// Seeded marks a row the program wrote, as opposed to one a person added.
	Seeded bool
}

// Environment is what Resolve is told about the machine it runs on. It is built by whoever reads the machine, so
// that Resolve reads none of it.
type Environment struct {
	// Vault is the value of the environment's LONGTERM_MEM_VAULT, the empty string when it is unset or empty.
	Vault string
	// Home answers the home directory of the user. It is asked only when a path begins with ~, so a machine whose
	// home cannot be told is no worse off for a vault named by an absolute path.
	Home func() (string, error)
}

// Overridden says whether the flag or the environment names the vault, in which case the registry is not
// consulted at all and Resolve needs none.
func Overridden(flag string, env Environment) bool {
	_, overridden := override(flag, env)
	return overridden
}

// override is the vault the flag or, failing it, the environment names, as it was written.
func override(flag string, env Environment) (path string, overridden bool) {
	switch {
	case flag != "":
		return flag, true
	case env.Vault != "":
		return env.Vault, true
	default:
		return "", false
	}
}

// Resolve determines project's vault path using flag > environment > registry-row precedence (D5). flag is the
// --vault flag value, empty when unset. Every resolved path is ~-expanded and validated absolute before being
// returned. When none of the three sources apply, Resolve returns ErrVaultNotConfigured rather than guessing a
// path (R-023).
func Resolve(flag string, env Environment, reg Registry, project string) (string, error) {
	if path, overridden := override(flag, env); overridden {
		return expandVaultPath(path, env.Home)
	}
	entry, ok := reg.Vaults[project]
	if !ok {
		return "", fmt.Errorf("%w: project %q has no vault-registry entry in %s", ErrVaultNotConfigured, project, reg.Origin)
	}
	return expandVaultPath(entry.Path, env.Home)
}

// expandVaultPath expands a leading ~ to the home directory home answers, then validates the result is an
// absolute path (D5).
func expandVaultPath(raw string, home func() (string, error)) (string, error) {
	expanded := raw

	switch {
	case raw == "~":
		dir, err := homeDirectory(home)
		if err != nil {
			return "", err
		}
		expanded = dir
	case strings.HasPrefix(raw, "~/"):
		dir, err := homeDirectory(home)
		if err != nil {
			return "", err
		}
		expanded = filepath.Join(dir, raw[2:])
	}

	if !filepath.IsAbs(expanded) {
		return "", fmt.Errorf("vaultreg: vault path %q must be absolute after expansion", raw)
	}
	return expanded, nil
}

// errNoHome is what a path that begins with ~ meets when the environment was built without a way to tell the home
// directory: the caller's fault, said in the words of the failure to tell it.
var errNoHome = errors.New("the environment has no source of the home directory")

// homeDirectory asks home for the home directory, or says it cannot be told.
func homeDirectory(home func() (string, error)) (string, error) {
	if home == nil {
		return "", fmt.Errorf("vaultreg: resolve home directory: %w", errNoHome)
	}
	dir, err := home()
	if err != nil {
		return "", fmt.Errorf("vaultreg: resolve home directory: %w", err)
	}
	return dir, nil
}
