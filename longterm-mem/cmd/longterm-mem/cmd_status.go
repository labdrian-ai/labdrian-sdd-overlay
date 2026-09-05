package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/ops"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vault"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultreg"
)

// cmdStatus implements `longterm-mem status --project P [--vault DIR]
// [--json]` (R-010): report Engram reachability, P's vault provisioning
// state, and the last successful sync completion time. ops.Status never
// fails on an unhealthy field -- a never-provisioned vault or a
// never-synced project is a reported state, not an error -- so this
// command exits 0 once its own inputs (project, vault registry) resolve.
func cmdStatus(args []string) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	project := fs.String("project", "", projectFlagUsage)
	vaultDir := fs.String("vault", "", "vault path override")
	asJSON := fs.Bool("json", false, "print the result as JSON")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	resolvedProject, exit := resolveProjectFlag("status", *project)
	if exit != exitOK {
		return exit
	}

	vaultRoot, err := vaultreg.Resolve(defaultVaultsPath(), resolvedProject, *vaultDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "longterm-mem: status: %v\n", err)
		return vaultExitCode(err)
	}

	deps := ops.StatusDeps{
		VaultRoot:        vaultRoot,
		VaultProvisioned: vault.Provisioned,
		EngramReachable: func(ctx context.Context) (bool, string) {
			store, err := engram.Open(os.Getenv(engramDBEnvVar))
			if err != nil {
				return false, err.Error()
			}
			defer store.Close()
			// Open can succeed via its immutable=1 fallback (see Open's
			// own doc comment) when the primary read-only connection
			// cannot be established -- stale but never unsafe. Without
			// reading Store.Degraded here, a reachable-but-degraded
			// connection was indistinguishable from a healthy one in
			// every status report.
			if degraded, cause := store.Degraded(); degraded {
				return true, fmt.Sprintf("degraded: primary read-only connection unavailable, using stale fallback: %s", cause)
			}
			return true, ""
		},
	}

	report, err := ops.Status(context.Background(), deps, resolvedProject)
	if err != nil {
		fmt.Fprintf(os.Stderr, "longterm-mem: status: %v\n", err)
		return exitInternal
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			fmt.Fprintf(os.Stderr, "longterm-mem: status: encode result: %v\n", err)
			return exitInternal
		}
		return exitOK
	}

	fmt.Printf("longterm-mem: status for %s\n", report.Project)
	fmt.Printf("  engram: reachable=%t%s\n", report.EngramReachable, detailSuffix(report.EngramDetail))
	fmt.Printf("  vault: provisioned=%t\n", report.VaultProvisioned)
	fmt.Printf("  last sync: %s\n", report.LastSyncCompletedAt)
	return exitOK
}

// detailSuffix formats an optional parenthetical detail, shared by
// cmd_status.go and cmd_doctor.go's plain-text output.
func detailSuffix(detail string) string {
	if detail == "" {
		return ""
	}
	return fmt.Sprintf(" (%s)", detail)
}
