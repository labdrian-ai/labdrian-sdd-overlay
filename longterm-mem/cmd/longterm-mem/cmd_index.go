package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vault"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultreg"
)

// vaultsFileEnvVar overrides the default vault-registry file path (Anchors:
// LONGTERM_MEM_VAULTS_FILE).
const vaultsFileEnvVar = "LONGTERM_MEM_VAULTS_FILE"

// cmdIndex implements `longterm-mem index --project P [--vault DIR]
// [--rebuild]`: resolve P's vault (vaultreg.Resolve), then rebuild its
// index (vault.Rebuild), provisioning it first when never (fully) indexed
// (R-005). --rebuild forces re-provisioning even on an already-provisioned
// vault — an operator's fix-forward path when a prior provision step was
// interrupted partway through. A failing rebuild step is reported as a
// failure, never a false success (R-025), via exit code 5
// (vault_subprocess_failed).
func cmdIndex(args []string) int {
	fs := flag.NewFlagSet("index", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	project := fs.String("project", "", "project name (required)")
	vaultDir := fs.String("vault", "", "vault path override")
	force := fs.Bool("rebuild", false, "force re-provisioning of the vault index, even if already provisioned")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *project == "" {
		fmt.Fprintln(os.Stderr, "longterm-mem: index: --project is required")
		return 2
	}

	vaultRoot, err := vaultreg.Resolve(defaultVaultsPath(), *project, *vaultDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "longterm-mem: index: %v\n", err)
		return 3
	}

	runner := &vault.Runner{Root: vaultRoot}
	if err := vault.Rebuild(context.Background(), runner, *force); err != nil {
		fmt.Fprintf(os.Stderr, "longterm-mem: index: %v\n", err)
		return 5
	}

	fmt.Println("longterm-mem: index rebuilt")
	return 0
}

// defaultVaultsPath resolves the vault-registry file: LONGTERM_MEM_VAULTS_FILE
// when set, else ~/.labdrian-overlay/vaults.json (D5).
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
