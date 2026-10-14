package main

import (
	"os"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultreg"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultregfs"
)

// The vault registry of this machine: where its file is, what the environment says of the vault, and the one row a
// machine is taken to start with. Everything here is the composition root's to say, because it is the part of the
// program that reads the machine; internal/vaultreg decides, from what it is handed, and internal/vaultregfs
// keeps the file.

const (
	// vaultsFileEnvVar overrides the default vault-registry file path (Anchors: LONGTERM_MEM_VAULTS_FILE).
	vaultsFileEnvVar = "LONGTERM_MEM_VAULTS_FILE"

	// vaultEnvVar is the environment variable that names the vault of any project, over the project's registry
	// row and under the --vault flag (D5).
	vaultEnvVar = "LONGTERM_MEM_VAULT"

	// defaultVaultProject is the project every machine is taken to have a vault for: the registry holds a row for
	// it from the first time it is read. It names only what the registry starts with; resolving a project never
	// falls back to it (R-022), and a row deleted from the registry, this one included, means "not configured".
	defaultVaultProject = "labdrian-sdd-overlay"

	// defaultVaultPath is the vault of defaultVaultProject, unexpanded: a ~ is expanded like any other row's.
	defaultVaultPath = "~/labdrian-brain"
)

// defaultVaultsPath resolves the vault-registry file: LONGTERM_MEM_VAULTS_FILE when set, else
// ~/.labdrian-overlay/vaults.json (D5).
func defaultVaultsPath() string {
	if p := os.Getenv(vaultsFileEnvVar); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".labdrian-overlay", "vaults.json")
	}
	return filepath.Join(home, ".labdrian-overlay", "vaults.json")
}

// defaultVaultRegistry is the registry a machine starts with, as it would be kept at path: one row, for
// defaultVaultProject, marked as the program's own. A command that writes seeds the registry file with it; a
// command that only reads takes a machine that has no file to hold it, and the words of a project with no row name
// path as the place the registry is kept.
func defaultVaultRegistry(path string) vaultreg.Registry {
	return vaultreg.Registry{
		Vaults: map[string]vaultreg.Entry{defaultVaultProject: {Path: defaultVaultPath, Seeded: true}},
		Origin: path,
	}
}

// machineVaults finds the vault of a project on this machine: the registry file it keeps (defaultVaultsPath), the
// row a machine starts with, and the environment as it is when the command asks. It is built for each resolution,
// so a session that outlives a change of the environment reads the new one.
func machineVaults() vaultreg.Vaults {
	store := vaultregfs.Store{Path: defaultVaultsPath()}
	return vaultreg.Vaults{
		Store:       store,
		Defaults:    defaultVaultRegistry(store.Path),
		Environment: vaultreg.Environment{Vault: os.Getenv(vaultEnvVar), Home: os.UserHomeDir},
	}
}
