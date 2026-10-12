package query

import (
	"context"
	"errors"
)

// DefaultTopN is the number of results each source is asked for when the caller gives no bound (D8: the
// vault's retrieval and the memory's search share one bound). The vault adapter's own default is this
// constant (vault.DefaultTopN), so the number is written once, here.
const DefaultTopN = 5

// ErrVaultNotProvisioned is what a VaultRetriever answers for a vault that was never indexed: a typed outcome
// of the retrieval, not a failure of it (R-024, R-026). The query is then answered from the memory alone and
// says so in Result.VaultStatus. A retriever returns it as it is or wrapped, and the query recognises it with
// errors.Is; the words of an error are never read to tell the two apart.
var ErrVaultNotProvisioned = errors.New("query: the vault is not provisioned")

// ErrNoVault rejects a call that names the vault as a source when Deps carries no VaultRetriever, or one that
// holds a nil pointer: the call fails in the query's own words instead of panicking in a retriever that is not
// there.
var ErrNoVault = errors.New("query: no vault retriever is configured for a source that reads it")

// VaultCandidate is one page the vault's retrieval offered for a query, in the terms this package merges: the
// address of the page, where it is, its two native scores and the snippet the vault cut. The scores are the
// vault's own and are never fused with another source's (D8); the snippet was cut by the vault, so this module
// holds no full body to measure it against.
type VaultCandidate struct {
	PageAddress  string
	AbsolutePath string
	BM25Score    float64
	RerankScore  float64
	Snippet      string
}

// VaultRetriever is the port through which the vault source reads: up to top candidates of project's vault for
// query, in the vault's own order. The package owns it and the vault adapter (internal/vault) satisfies it by
// running the vault's retrieval script. A vault that was never indexed answers ErrVaultNotProvisioned and no
// candidates; any other error is a retrieval that failed, and the query reports it as a degraded vault.
type VaultRetriever interface {
	RetrieveVault(ctx context.Context, project, query string, top int) ([]VaultCandidate, error)
}
