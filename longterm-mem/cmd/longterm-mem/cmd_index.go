package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vault"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vecindex"
)

// cmdIndex implements `longterm-mem index --project P [--vault DIR]
// [--rebuild]`: resolve P's vault (vaultreg.Vaults), then rebuild its
// index (vault.Rebuild), provisioning it first when never (fully) indexed
// (R-005). --rebuild forces re-provisioning even on an already-provisioned
// vault — an operator's fix-forward path when a prior provision step was
// interrupted partway through. A failing rebuild step is reported as a
// failure, never a false success (R-025), via exit code 5
// (vault_subprocess_failed).
func (c commands) cmdIndex(args []string) int {
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
	a := c.newApp("index")
	resolvedProject, exit := a.project(*project, false)
	if exit != exitOK {
		return exit
	}

	if *embeddings {
		return c.cmdIndexEmbeddings(resolvedProject, embedConfig{
			Endpoint:    *embedEndpoint,
			Model:       *embedModel,
			Dimension:   *embedDimension,
			InputLimit:  *embedInputLimit,
			AllowRemote: *allowRemoteEmbedder,
		})
	}

	vaultRoot, exit := a.vaultRoot(resolvedProject, *vaultDir, false)
	if exit != exitOK {
		return exit
	}

	if err := (vault.IndexRebuilder{Root: vaultRoot}).Rebuild(context.Background(), *force); err != nil {
		return a.refuse(err, exitVaultSubprocessFailed)
	}

	fmt.Println("longterm-mem: index rebuilt")
	return exitOK
}
