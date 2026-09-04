package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/promote"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultreg"
)

// cmdPromote implements `longterm-mem promote --project P --id <engram_id>
// [--vault DIR]` (R-032): explicitly promote one Engram observation
// through the same page-emission, addressing, and registration path any
// other eligible observation uses (promote.ExplicitPromote ->
// Writer.Promote's explicit=true override, R-007), regardless of its
// automatic eligibility. An invalid or nonexistent id is rejected with
// exit 7 (not_found), never a silent no-op, and a promotion the local-edit
// precedence rule refuses (R-030) exits 6 (registration_conflict) rather
// than reporting a page it deliberately did not write as promoted.
func cmdPromote(args []string) int {
	fs := flag.NewFlagSet("promote", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	project := fs.String("project", "", "project name (required)")
	id := fs.Int64("id", 0, "Engram observation id (required)")
	vaultDir := fs.String("vault", "", "vault path override")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if *project == "" {
		fmt.Fprintln(os.Stderr, "longterm-mem: promote: --project is required")
		return exitUsage
	}
	if *id <= 0 {
		fmt.Fprintln(os.Stderr, "longterm-mem: promote: --id is required")
		return exitUsage
	}

	vaultRoot, err := vaultreg.Resolve(defaultVaultsPath(), *project, *vaultDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "longterm-mem: promote: %v\n", err)
		return vaultExitCode(err)
	}

	store, err := engram.Open(os.Getenv(engramDBEnvVar))
	if err != nil {
		fmt.Fprintf(os.Stderr, "longterm-mem: promote: %v\n", err)
		return exitEngramUnavailable
	}
	defer store.Close()
	declareDegradedEngram(store, "promote")

	result, err := runPromote(store, vaultRoot, *id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "longterm-mem: promote: %v\n", err)
		if errors.Is(err, promote.ErrObservationNotFound) {
			return exitNotFound
		}
		return exitInternal
	}

	// A skip wrote nothing: it is a refusal, not a promotion, and must not
	// wear either the word or the exit code of one. Exit 6
	// (registration_conflict) is this binary's existing code for exactly
	// this shape -- an artifact already occupies the target location,
	// longterm-mem cannot prove it owns it, so it refuses and leaves the
	// file byte-identical (cmd_register.go/cmd_unregister.go's own
	// untagged-entry refusal). The diagnostic goes to stderr, where a
	// caller reading stdout for the promoted address finds nothing to
	// mistake for one.
	if result.Action.Kind == promote.ActionSkippedLocalEdit {
		fmt.Printf("longterm-mem: refused %s (%s)\n", result.Page.Address, result.Action.Kind)
		if result.Action.Diagnostic != nil {
			fmt.Fprintf(os.Stderr, "longterm-mem: promote: %s\n", result.Action.Diagnostic.Detail)
		}
		return exitRegistrationConflict
	}

	fmt.Printf("longterm-mem: promoted %s (%s)\n", result.Page.Address, result.Action.Kind)
	return exitOK
}
