package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/promote"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vault"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultreg"
)

// cmdSync implements `longterm-mem sync --project P [--vault DIR]`:
// promote every eligible, unpromoted-or-revised observation (R-009),
// propagate soft-delete/supersession status from Engram (R-033), rebuild
// the vault's index, and record the sync's completion timestamp (R-031) --
// promote.Sync and promote.Propagate wired to a real Engram connection and
// a real vault, the same seam shape cmd_index.go and cmd_query.go already
// use for vault.Rebuild/vault.Retrieve.
func cmdSync(args []string) int {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	project := fs.String("project", "", "project name (required)")
	vaultDir := fs.String("vault", "", "vault path override")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if *project == "" {
		fmt.Fprintln(os.Stderr, "longterm-mem: sync: --project is required")
		return exitUsage
	}

	vaultRoot, err := vaultreg.Resolve(defaultVaultsPath(), *project, *vaultDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "longterm-mem: sync: %v\n", err)
		return vaultExitCode(err)
	}

	store, err := engram.Open(os.Getenv(engramDBEnvVar))
	if err != nil {
		fmt.Fprintf(os.Stderr, "longterm-mem: sync: %v\n", err)
		return exitEngramUnavailable
	}
	defer store.Close()

	precedence, err := promote.LoadPrecedenceStore(vaultRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "longterm-mem: sync: %v\n", err)
		return exitInternal
	}

	runner := &vault.Runner{Root: vaultRoot}
	// rebuildErr is captured from this command's own RebuildIndex closure
	// rather than dug back out of the joined error below. Sync reports the
	// index rebuild's failure and its per-observation failures through one
	// error value, and only the first of the two is a vault subprocess
	// failure: an operator who reads exit 5 goes and looks at the vault's
	// Python tooling, which is the wrong place to look for an observation
	// longterm-mem could not write.
	var rebuildErr error
	deps := promote.Deps{
		Engram: store,
		Writer: &promote.Writer{VaultRoot: vaultRoot, Store: precedence},
		RebuildIndex: func(ctx context.Context) error {
			rebuildErr = vault.Rebuild(ctx, runner, false)
			return rebuildErr
		},
	}

	// Both passes always run. Sync's failures are per-observation by
	// design -- it steps over a broken observation rather than letting it
	// wedge the run -- so returning here on its error would rebuild that
	// wedge one level up: a single unpromotable observation would stop
	// Propagate from ever running, and R-033's soft-delete and
	// supersession statuses would never again land for that project,
	// identically on every retry. The command reports what did land and
	// exits non-zero when anything failed, so a partial run is visible
	// without being fatal to the work that can still be done.
	ctx := context.Background()
	syncReport, syncErr := promote.Sync(ctx, deps, *project)
	propagateReport, propagateErr := promote.Propagate(ctx, deps, *project)

	fmt.Printf("longterm-mem: sync promoted %d observation(s), patched %d page(s)\n", len(syncReport.Promoted), len(propagateReport.Patched))

	if err := errors.Join(syncErr, propagateErr); err != nil {
		fmt.Fprintf(os.Stderr, "longterm-mem: sync: %v\n", err)
		return syncExitCode(store, *project, rebuildErr)
	}
	return exitOK
}

// syncExitCode classifies a failed sync run.
//
// Sync used to answer 5 (vault_subprocess_failed) for everything it could
// not complete, which made the code meaningless: a corrupt Engram database
// and a Python script that crashed and a single unwritable page all read
// as "the vault's tooling failed". Only one of the three is:
//
//   - the index rebuild failed -> exitVaultSubprocessFailed, the one step
//     that actually runs a vault script;
//   - Engram can no longer be listed -> exitEngramUnavailable, and no vault
//     subprocess had to fail for that to happen;
//   - anything else (a per-observation page write, the sync-state record,
//     the precedence store) -> exitInternal.
//
// The Engram question is answered by asking the database rather than by
// matching on error text, so the code stays bound to the condition it
// names instead of to a message string.
func syncExitCode(store *engram.Store, project string, rebuildErr error) int {
	switch {
	case rebuildErr != nil:
		return exitVaultSubprocessFailed
	case !engramReadable(store, project):
		return exitEngramUnavailable
	default:
		return exitInternal
	}
}
