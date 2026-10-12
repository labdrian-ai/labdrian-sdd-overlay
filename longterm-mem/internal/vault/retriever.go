package vault

import (
	"context"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/query"
)

// Retriever is the adapter behind the VaultRetriever port that internal/query owns: it retrieves from the vault
// at Root by running the vault's own retrieval script (Retrieve), and answers in the terms of the port. The
// composition root builds it and hands it to the query.
type Retriever struct {
	// Root is the vault checkout the script belongs to and runs in.
	Root string
}

// RetrieveVault returns up to top candidates of the vault for queryText (the vault's default when top is not
// positive). A vault that was never indexed (the script's not-provisioned exit) is query.ErrVaultNotProvisioned
// and no candidates; any other failure is the error Retrieve reports, in its own words, which the query shows as
// the detail of a degraded vault. project is not sent to the script: a vault is the project's by where it is.
func (r Retriever) RetrieveVault(ctx context.Context, project, queryText string, top int) ([]query.VaultCandidate, error) {
	result, err := Retrieve(ctx, &Runner{Root: r.Root}, project, queryText, top)
	if err != nil {
		return nil, err
	}
	if result.Status == StatusNotProvisioned {
		return nil, query.ErrVaultNotProvisioned
	}
	if result.Candidates == nil {
		return nil, nil
	}
	candidates := make([]query.VaultCandidate, len(result.Candidates))
	for i, c := range result.Candidates {
		candidates[i] = query.VaultCandidate{
			PageAddress: c.PageAddress, AbsolutePath: c.AbsolutePath,
			BM25Score: c.BM25Score, RerankScore: c.RerankScore, Snippet: c.Snippet,
		}
	}
	return candidates, nil
}
