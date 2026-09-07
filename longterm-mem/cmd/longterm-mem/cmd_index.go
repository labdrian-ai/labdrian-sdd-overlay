package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vault"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultreg"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vecindex"
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
	project := fs.String("project", "", projectFlagUsage)
	vaultDir := fs.String("vault", "", "vault path override")
	force := fs.Bool("rebuild", false, "force re-provisioning of the vault index, even if already provisioned")
	embeddings := fs.Bool("embeddings", false, "build or incrementally update the embedding index instead of the vault index (R-069)")
	embedEndpoint := fs.String("embed-endpoint", "", "embedding backend endpoint (default: loopback ollama)")
	embedModel := fs.String("embed-model", vecindex.DefaultModel, "embedding model name")
	embedDimension := fs.Int("embed-dimension", vecindex.DefaultDimension, "embedding vector dimension")
	embedInputLimit := fs.Int("embed-input-limit", vecindex.DefaultInputLimit, "characters embedded per observation")
	// --allow-remote-embedder is deliberately an `index`-only flag, never a
	// `query` one (design's own open question, resolved index-only): a
	// query should never be the thing that egresses.
	allowRemoteEmbedder := fs.Bool("allow-remote-embedder", false, "allow the embedding backend to be a non-loopback endpoint, for this invocation only (R-071)")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	resolvedProject, exit := resolveProjectFlag("index", *project)
	if exit != exitOK {
		return exit
	}

	if *embeddings {
		return cmdIndexEmbeddings(resolvedProject, embedConfig{
			Endpoint:    *embedEndpoint,
			Model:       *embedModel,
			Dimension:   *embedDimension,
			InputLimit:  *embedInputLimit,
			AllowRemote: *allowRemoteEmbedder,
		})
	}

	vaultRoot, err := vaultreg.Resolve(defaultVaultsPath(), resolvedProject, *vaultDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "longterm-mem: index: %v\n", err)
		return vaultExitCode(err)
	}

	runner := &vault.Runner{Root: vaultRoot}
	if err := vault.Rebuild(context.Background(), runner, *force); err != nil {
		fmt.Fprintf(os.Stderr, "longterm-mem: index: %v\n", err)
		return exitVaultSubprocessFailed
	}

	fmt.Println("longterm-mem: index rebuilt")
	return exitOK
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
