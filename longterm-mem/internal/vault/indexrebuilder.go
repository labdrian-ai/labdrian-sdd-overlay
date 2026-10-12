package vault

import "context"

// IndexRebuilder is the adapter behind the IndexRebuilder port that internal/promote owns: it rebuilds the
// retrieval index of the vault at Root by running the vault's own index scripts (Rebuild). promote does not
// import this package; the composition root wires the two. The `index` command, which also asks for a forced
// re-provision, uses Rebuild.
type IndexRebuilder struct {
	// Root is the vault checkout the scripts belong to and run in.
	Root string
}

// RebuildIndex refreshes the index, provisioning the vault first when it was never fully indexed (Rebuild
// without force). Its errors are Rebuild's, in its words.
func (r IndexRebuilder) RebuildIndex(ctx context.Context) error { return r.Rebuild(ctx, false) }

// Rebuild is Rebuild over the vault at Root: with force it provisions the vault again even when it looks
// provisioned, the operator's fix-forward path after an interrupted provision.
func (r IndexRebuilder) Rebuild(ctx context.Context, force bool) error {
	return Rebuild(ctx, &Runner{Root: r.Root}, force)
}
