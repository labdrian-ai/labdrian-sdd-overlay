package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/query"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultreg"
)

// engramDBEnvVar overrides the default Engram database path (D8 CLI).
const engramDBEnvVar = "LONGTERM_MEM_ENGRAM_DB"

// maxTopN bounds --top (2b-2 review advisory).
const maxTopN = 50

// unsetTopN distinguishes "not provided" from an explicit, rejectable 0.
const unsetTopN = -1

// cmdQuery implements `longterm-mem query --project P "<text>" [--top N]
// [--vault DIR] [--json]` (R-006, R-026). A query text beginning with "-"
// needs no dedicated guard: flag.Parse already rejects a leading-dash
// positional as an unrecognized flag (exit 2) unless "--" precedes it, at
// which point it is stdlib's own literal positional arg (2b-2 advisory).
func cmdQuery(args []string) int {
	fs := flag.NewFlagSet("query", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	project := fs.String("project", "", projectFlagUsage)
	vaultDir := fs.String("vault", "", "vault path override")
	top := fs.Int("top", unsetTopN, "results per source, 1-50 (default 5)")
	asJSON := fs.Bool("json", false, "print the result as JSON")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	resolvedProject, exit := resolveProjectFlag("query", *project)
	if exit != exitOK {
		return exit
	}
	if *top != unsetTopN && (*top <= 0 || *top > maxTopN) {
		fmt.Fprintf(os.Stderr, "longterm-mem: query: --top must be between 1 and %d\n", maxTopN)
		return exitUsage
	}
	rest := fs.Args()
	if len(rest) != 1 || strings.TrimSpace(rest[0]) == "" {
		fmt.Fprintln(os.Stderr, "longterm-mem: query: a single query text argument is required")
		return exitUsage
	}

	vaultRoot, err := vaultreg.Resolve(defaultVaultsPath(), resolvedProject, *vaultDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "longterm-mem: query: %v\n", err)
		return vaultExitCode(err)
	}

	store, err := engram.Open(os.Getenv(engramDBEnvVar))
	if err != nil {
		fmt.Fprintf(os.Stderr, "longterm-mem: query: %v\n", err)
		return exitEngramUnavailable
	}
	defer store.Close()

	requestedTop := 0
	if *top != unsetTopN {
		requestedTop = *top
	}

	result, err := runQuery(context.Background(), store, vaultRoot, query.Request{Project: resolvedProject, Query: rest[0], Top: requestedTop})
	if err != nil {
		// query.Run degrades a failing vault to a diagnostic and only
		// ever errors on the Engram side (a missing project is rejected
		// before this call), so its failure is engram_unavailable.
		fmt.Fprintf(os.Stderr, "longterm-mem: query: %v\n", err)
		return exitEngramUnavailable
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(result); err != nil {
			fmt.Fprintf(os.Stderr, "longterm-mem: query: encode result: %v\n", err)
			return exitInternal
		}
		return exitOK
	}

	fmt.Printf("longterm-mem: query %q (vault_status=%s)\n", result.Query, result.VaultStatus)
	for _, row := range result.Results {
		label := row.PageAddress
		if label == "" {
			label = fmt.Sprintf("engram:%d", row.EngramID)
		}
		fmt.Printf("  [%d] %-8s %s %s\n", row.Rank, row.Source, label, row.Title)
	}
	for _, d := range result.Diagnostics {
		fmt.Fprintf(os.Stderr, "WARN %s: %s\n", d.Code, d.Detail)
	}
	return exitOK
}
