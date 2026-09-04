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

// exitDoctorChecksFailed is what doctor returns when it ran every check to
// completion and at least one of them FAILED -- a verdict about the
// vault, not about longterm-mem.
//
// It currently aliases exitInternal because that is the code slice 8a
// specified for this path ("exit 1 if any FAILs"), but exit 1 is named
// "internal" in the published contract, so today a caller cannot tell
// "doctor works and your vault is broken" (a finding to act on) from
// "doctor itself broke" (a bug to report) -- both come back as 1. Naming
// the two paths separately here is as far as this file can honestly go:
// splitting them for real means adding a code to a published contract
// that callers already script against, and choosing that number is the
// maintainer's decision, not this file's. When it is made, only this one
// constant changes.
const exitDoctorChecksFailed = exitInternal

// cmdDoctor implements `longterm-mem doctor --project P [--vault DIR]
// [--json]` (R-011): run the five read-only diagnostic checks and report
// each one individually. ops.Doctor always runs and reports all five
// checks regardless of any single one's own result (slice 7's review
// finding: a per-item failure must never abort a whole run) -- this
// command mirrors that at its own layer: it never returns on the first
// FAIL, it always lets Doctor finish and prints every check's result
// before deciding the exit code.
//
// It prints whatever Doctor returns rather than a list of its own, so the
// count above is documentation, not behaviour -- which is exactly how it
// went on claiming a count one short of the truth once a fifth diagnostic
// was added, through a repair round that reported the contradiction
// resolved after correcting only the spec file. That is why
// TestCmdDoctor_DocumentedCheckCountMatchesOpsDoctor reads the number out
// of a real ops.Doctor run instead of trusting any file's prose.
func cmdDoctor(args []string) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	project := fs.String("project", "", "project name (required)")
	vaultDir := fs.String("vault", "", "vault path override")
	asJSON := fs.Bool("json", false, "print the result as JSON")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if *project == "" {
		fmt.Fprintln(os.Stderr, "longterm-mem: doctor: --project is required")
		return exitUsage
	}

	vaultRoot, err := vaultreg.Resolve(defaultVaultsPath(), *project, *vaultDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "longterm-mem: doctor: %v\n", err)
		return vaultExitCode(err)
	}

	deps := ops.DoctorDeps{
		VaultRoot:           vaultRoot,
		PrerequisitePresent: vault.PrerequisitePresent,
	}

	report, err := ops.Doctor(context.Background(), deps, *project)
	if err != nil {
		fmt.Fprintf(os.Stderr, "longterm-mem: doctor: %v\n", err)
		return exitInternal
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			fmt.Fprintf(os.Stderr, "longterm-mem: doctor: encode result: %v\n", err)
			return exitInternal
		}
	} else {
		fmt.Printf("longterm-mem: doctor for %s\n", report.Project)
		for _, check := range report.Checks {
			fmt.Printf("  [%s] %s%s\n", check.Status, check.Name, detailSuffix(check.Detail))
		}
	}

	for _, check := range report.Checks {
		if check.Status == ops.CheckFailed {
			return exitDoctorChecksFailed
		}
	}
	return exitOK
}
