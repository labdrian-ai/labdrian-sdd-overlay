package promote

import "context"

// IndexRebuilder is the port through which a sync makes the pages it wrote findable: it rebuilds the vault's
// retrieval index after the promotion completes (R-031). The package owns it and the vault adapter
// (internal/vault) satisfies it by running the vault's own index scripts; the composition root hands it to
// Sync. A failure is the rebuild's own, and Sync reports it without recording the completion of the sync.
type IndexRebuilder interface {
	RebuildIndex(ctx context.Context) error
}

// IndexRebuilderFunc is an IndexRebuilder written as a function, for a caller that wants to see the call.
type IndexRebuilderFunc func(ctx context.Context) error

// RebuildIndex calls f.
func (f IndexRebuilderFunc) RebuildIndex(ctx context.Context) error { return f(ctx) }
