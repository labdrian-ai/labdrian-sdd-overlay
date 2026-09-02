package vaultreg

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// writeRegistryFile writes an arbitrary registry JSON body directly to path,
// bypassing Seed, so tests can set up a file that already exists with
// content Seed would never have produced.
func writeRegistryFile(t *testing.T, path string, reg Registry) {
	t.Helper()

	data, err := json.Marshal(reg)
	if err != nil {
		t.Fatalf("marshal fixture registry: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write fixture registry %s: %v", path, err)
	}
}

func TestResolve_ConfiguredOverrideWins(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LONGTERM_MEM_VAULT", "")

	dir := t.TempDir()
	vaultsPath := filepath.Join(dir, "vaults.json")
	wantVault := filepath.Join(dir, "some-other-vault")

	writeRegistryFile(t, vaultsPath, Registry{
		Schema: 1,
		Vaults: map[string]VaultEntry{
			"some-other-project": {Path: wantVault},
		},
	})

	got, err := Resolve(vaultsPath, "some-other-project", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != wantVault {
		t.Fatalf("Resolve = %q, want configured path %q", got, wantVault)
	}
}

func TestResolve_DefaultSeedEntryForOverlayProject(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LONGTERM_MEM_VAULT", "")

	dir := t.TempDir()
	vaultsPath := filepath.Join(dir, "vaults.json")
	// vaultsPath does not exist yet: no fixture row is written for
	// DefaultProject, so a successful resolution can only come from the
	// pre-seeded default row that Resolve itself materializes.

	got, err := Resolve(vaultsPath, DefaultProject, "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	want := filepath.Join(home, "labdrian-brain")
	if got != want {
		t.Fatalf("Resolve = %q, want pre-seeded default %q", got, want)
	}

	// The seed row must now be an ordinary, readable/editable file entry —
	// not something Resolve regenerates from a code constant every call.
	reg, err := Load(vaultsPath)
	if err != nil {
		t.Fatalf("Load after seeding: %v", err)
	}
	entry, ok := reg.Vaults[DefaultProject]
	if !ok {
		t.Fatalf("seeded registry has no row for %q: %+v", DefaultProject, reg.Vaults)
	}
	if entry.Path != DefaultVaultPath {
		t.Fatalf("seeded entry.Path = %q, want %q", entry.Path, DefaultVaultPath)
	}
	if !entry.Seeded {
		t.Fatalf("seeded entry.Seeded = false, want true")
	}
}

func TestResolve_SeedsWhenRegistryDirectoryIsMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LONGTERM_MEM_VAULT", "")

	// Fresh install: neither the registry file nor its parent directory
	// (the dotted state dir under $HOME) exists yet.
	vaultsPath := filepath.Join(t.TempDir(), "state", "vaults.json")

	got, err := Resolve(vaultsPath, DefaultProject, "")
	if err != nil {
		t.Fatalf("Resolve with missing registry directory: %v", err)
	}
	if want := filepath.Join(home, "labdrian-brain"); got != want {
		t.Fatalf("Resolve = %q, want pre-seeded default %q", got, want)
	}
	if _, err := os.Stat(vaultsPath); err != nil {
		t.Fatalf("registry file not materialized after seeding: %v", err)
	}
}

func TestResolve_UnconfiguredNonDefaultProjectRejected(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LONGTERM_MEM_VAULT", "")

	dir := t.TempDir()
	vaultsPath := filepath.Join(dir, "vaults.json")
	// vaultsPath does not exist: Resolve seeds only the DefaultProject row,
	// never one for "some-new-project".

	_, err := Resolve(vaultsPath, "some-new-project", "")
	if !errors.Is(err, ErrVaultNotConfigured) {
		t.Fatalf("Resolve error = %v, want ErrVaultNotConfigured", err)
	}
}

func TestPrecedence_FlagEnvFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := t.TempDir()
	vaultsPath := filepath.Join(dir, "vaults.json")
	rowVault := filepath.Join(dir, "row-vault")
	envVault := filepath.Join(dir, "env-vault")
	flagVault := filepath.Join(dir, "flag-vault")

	writeRegistryFile(t, vaultsPath, Registry{
		Schema: 1,
		Vaults: map[string]VaultEntry{
			"proj": {Path: rowVault},
		},
	})

	t.Run("row is used when neither flag nor env is set", func(t *testing.T) {
		t.Setenv("LONGTERM_MEM_VAULT", "")
		got, err := Resolve(vaultsPath, "proj", "")
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if got != rowVault {
			t.Fatalf("Resolve = %q, want row path %q", got, rowVault)
		}
	})

	t.Run("env wins over row", func(t *testing.T) {
		t.Setenv("LONGTERM_MEM_VAULT", envVault)
		got, err := Resolve(vaultsPath, "proj", "")
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if got != envVault {
			t.Fatalf("Resolve = %q, want env path %q", got, envVault)
		}
	})

	t.Run("flag wins over env and row", func(t *testing.T) {
		t.Setenv("LONGTERM_MEM_VAULT", envVault)
		got, err := Resolve(vaultsPath, "proj", flagVault)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if got != flagVault {
			t.Fatalf("Resolve = %q, want flag path %q", got, flagVault)
		}
	})
}

func TestSeed_OnlyWhenFileAbsent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LONGTERM_MEM_VAULT", "")

	dir := t.TempDir()
	vaultsPath := filepath.Join(dir, "vaults.json")

	// The file already exists but carries no row for DefaultProject — as
	// if a user deleted the pre-seeded row. Seed (invoked lazily by
	// Resolve) must never re-add it: deleting the row means "not
	// configured", not "seed it again".
	writeRegistryFile(t, vaultsPath, Registry{
		Schema: 1,
		Vaults: map[string]VaultEntry{
			"some-other-project": {Path: filepath.Join(dir, "other-vault")},
		},
	})

	_, err := Resolve(vaultsPath, DefaultProject, "")
	if !errors.Is(err, ErrVaultNotConfigured) {
		t.Fatalf("Resolve error = %v, want ErrVaultNotConfigured (seed must not re-add a deleted row)", err)
	}

	reg, err := Load(vaultsPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := reg.Vaults[DefaultProject]; ok {
		t.Fatalf("registry gained a %q row after Resolve; Seed must be a no-op when the file already exists", DefaultProject)
	}
}
