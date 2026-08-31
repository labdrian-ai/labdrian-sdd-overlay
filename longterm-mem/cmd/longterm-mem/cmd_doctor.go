package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/ops"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vault"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultreg"
)

// cmdDoctor implements `longterm-mem doctor --project P [--vault DIR]
// [--json]` (R-011): run the four read-only diagnostic checks and report
// each one individually. ops.Doctor always runs and reports all four
// checks regardless of any single one's own result (slice 7's review
// finding: a per-item failure must never abort a whole run) -- this
// command mirrors that at its own layer: it never returns on the first
// FAIL, it always lets Doctor finish and prints every check's result
// before deciding the exit code.
func cmdDoctor(args []string) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	project := fs.String("project", "", "project name (required)")
	vaultDir := fs.String("vault", "", "vault path override")
	asJSON := fs.Bool("json", false, "print the result as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *project == "" {
		fmt.Fprintln(os.Stderr, "longterm-mem: doctor: --project is required")
		return 2
	}

	vaultRoot, err := vaultreg.Resolve(defaultVaultsPath(), *project, *vaultDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "longterm-mem: doctor: %v\n", err)
		return 3
	}

	deps := ops.DoctorDeps{
		VaultRoot:           vaultRoot,
		PrerequisitePresent: vault.PrerequisitePresent,
	}

	report, err := ops.Doctor(context.Background(), deps, *project)
	if err != nil {
		fmt.Fprintf(os.Stderr, "longterm-mem: doctor: %v\n", err)
		return 1
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			fmt.Fprintf(os.Stderr, "longterm-mem: doctor: encode result: %v\n", err)
			return 1
		}
	} else {
		fmt.Printf("longterm-mem: doctor for %s\n", report.Project)
		for _, check := range report.Checks {
			fmt.Printf("  [%s] %s%s\n", check.Status, check.Name, detailSuffix(check.Detail))
		}
	}

	for _, check := range report.Checks {
		if check.Status == ops.CheckFailed {
			return 1
		}
	}
	return 0
}
