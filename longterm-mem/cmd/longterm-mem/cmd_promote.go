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
// exit 7 (not_found), never a silent no-op.
func cmdPromote(args []string) int {
	fs := flag.NewFlagSet("promote", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	project := fs.String("project", "", "project name (required)")
	id := fs.Int64("id", 0, "Engram observation id (required)")
	vaultDir := fs.String("vault", "", "vault path override")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *project == "" {
		fmt.Fprintln(os.Stderr, "longterm-mem: promote: --project is required")
		return 2
	}
	if *id <= 0 {
		fmt.Fprintln(os.Stderr, "longterm-mem: promote: --id is required")
		return 2
	}

	vaultRoot, err := vaultreg.Resolve(defaultVaultsPath(), *project, *vaultDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "longterm-mem: promote: %v\n", err)
		return 3
	}

	store, err := engram.Open(os.Getenv(engramDBEnvVar))
	if err != nil {
		fmt.Fprintf(os.Stderr, "longterm-mem: promote: %v\n", err)
		return 4
	}
	defer store.Close()

	result, err := runPromote(store, vaultRoot, *id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "longterm-mem: promote: %v\n", err)
		if errors.Is(err, promote.ErrObservationNotFound) {
			return 7
		}
		return 1
	}

	fmt.Printf("longterm-mem: promoted %s (%s)\n", result.Page.Address, result.Action.Kind)
	return 0
}
