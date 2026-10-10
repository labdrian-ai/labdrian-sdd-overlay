package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

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
	project := fs.String("project", "", projectFlagUsage)
	vaultDir := fs.String("vault", "", "vault path override")
	dryRun := fs.Bool("dry-run", false, "report what sync would promote and write nothing")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	// A dry run must not record this repository's derived names either: an
	// operator asking "what would this do" has not asked it to do anything.
	resolve := resolveProjectFlag
	if *dryRun {
		resolve = resolveProjectFlagReadOnly
	}
	resolvedProject, exit := resolve("sync", *project)
	if exit != exitOK {
		return exit
	}

	vaultRoot, err := vaultreg.Resolve(defaultVaultsPath(), resolvedProject, *vaultDir)
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
	declareDegradedEngram(store, "sync")

	writer, err := newPromoteWriter(vaultRoot)
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
		Memory: store,
		Writer: writer,
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

	if *dryRun {
		plan, err := promote.Plan(ctx, deps, resolvedProject)
		if err != nil {
			fmt.Fprintf(os.Stderr, "longterm-mem: sync --dry-run: %v\n", err)
			return exitInternal
		}
		// Both counts, in the same shape the real run reports them
		// (`promoted N, patched M`). A dry run that named only the
		// promotions would tell an operator nothing is rewritten
		// immediately before a re-sync rewrites existing pages.
		fmt.Printf("longterm-mem: sync --dry-run: would promote %d observation(s), patch %d page(s), skip %d, into %s\n", plan.WouldPromote, plan.WouldPatch, plan.Skipped, vaultRoot)
		for _, title := range plan.Titles {
			fmt.Printf("  would promote: %s\n", title)
		}
		for _, addr := range plan.PatchAddresses {
			fmt.Printf("  would patch: %s\n", addr)
		}
		for _, f := range plan.Failed {
			fmt.Fprintf(os.Stderr, "  cannot read promoted state for observation %d: %v\n", f.ObservationID, f.Err)
		}
		warnSkippedPages(os.Stderr, "longterm-mem: sync --dry-run:", plan.Unreadable)
		warnLeftAlone(os.Stderr, "longterm-mem: sync --dry-run:", "would be left alone, nothing would be written for them", plan.Withheld)
		fmt.Println("longterm-mem: sync --dry-run: nothing was written")
		switch {
		case len(plan.Failed) > 0:
			return exitInternal
		case len(plan.Withheld) > 0:
			return exitRegistrationConflict
		}
		return exitOK
	}

	syncReport, syncErr := promote.Sync(ctx, deps, resolvedProject)
	propagateReport, propagateErr := promote.Propagate(ctx, deps, resolvedProject)

	fmt.Printf("longterm-mem: sync promoted %d observation(s), patched %d page(s)\n", len(syncReport.Promoted), len(propagateReport.Patched))

	// A page that cannot be read is skipped and named, as the doctor names it, and the observations it keeps
	// from being decided are left alone: the run did everything else.
	warnSkippedPages(os.Stderr, "longterm-mem: sync:", syncReport.Unreadable)
	warnLeftAlone(os.Stderr, "longterm-mem: sync:", "left alone, nothing was written for them", syncReport.Withheld)
	warnLeftAlone(os.Stderr, "longterm-mem: sync:", "left alone, nothing was patched for them", propagateReport.Withheld)

	if err := errors.Join(syncErr, propagateErr); err != nil {
		fmt.Fprintf(os.Stderr, "longterm-mem: sync: %v\n", err)
		return syncExitCode(store, resolvedProject, rebuildErr)
	}
	if len(syncReport.Withheld)+len(propagateReport.Withheld) > 0 {
		// Nothing failed, but an observation was refused on the grounds that its page could not be told from
		// an unreadable one, and left byte-identical: the code `promote --id` answers for the same refusal.
		return exitRegistrationConflict
	}
	return exitOK
}

// warnSkippedPages names, one line each and on w, the pages a run could not read and skipped.
func warnSkippedPages(w io.Writer, prefix string, unreadable []promote.UnreadablePage) {
	for _, page := range unreadable {
		fmt.Fprintf(w, "%s warning: %s; it was skipped\n", prefix, page)
	}
}

// warnLeftAlone says, on w, how many observations a run left alone because no readable page is theirs while a
// page could not be read, and which. It says nothing when there are none.
func warnLeftAlone(w io.Writer, prefix, outcome string, withheld []int64) {
	if len(withheld) == 0 {
		return
	}
	ids := make([]string, len(withheld))
	for i, id := range withheld {
		ids[i] = strconv.FormatInt(id, 10)
	}
	noun := "observations"
	if len(withheld) == 1 {
		noun = "observation"
	}
	fmt.Fprintf(w, "%s warning: %d observation(s) %s: no readable page belongs to them, and a page that could not be read may (%s %s)\n",
		prefix, len(withheld), outcome, noun, strings.Join(ids, ", "))
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
