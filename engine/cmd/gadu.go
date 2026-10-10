package main

// The 'gadu-generate' subcommand.

import (
	"fmt"
	"os"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gadu"
)

// overlayRoot resolves the overlay repo root from the OVERLAY_DIR environment
// variable. The installed binary at ~/.claude/bin/gentle-ai-overlay cannot
// reliably locate the repo root via os.Executable() (it resolves to ~, not
// the overlay repo), so OVERLAY_DIR is required. Returns an error when unset.
func overlayRoot() (string, error) {
	if dir := os.Getenv("OVERLAY_DIR"); dir != "" {
		return dir, nil
	}
	return "", fmt.Errorf("OVERLAY_DIR is not set\n" +
		"  Run: OVERLAY_DIR=<overlay-repo-root> gentle-ai-overlay gadu-generate")
}

// runGaduGenerate implements the 'gadu-generate [--check]' subcommand.
// Without --check: calls gadu.Generate(repoRoot) to write all artifacts.
// With    --check: calls gadu.Check(repoRoot)    to verify they are not stale.
// Exits non-zero on error. OVERLAY_DIR must be set; the installed binary
// cannot resolve the repo root reliably via os.Executable().
func runGaduGenerate(args []string) {
	checkMode := false
	for _, a := range args {
		if a == "--check" {
			checkMode = true
		}
	}

	root, err := overlayRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "gadu-generate: %v\n", err)
		os.Exit(1)
	}

	if checkMode {
		if err := gadu.Check(root); err != nil {
			fmt.Fprintf(os.Stderr, "gadu-generate --check: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stdout, "gadu-generate --check: OK (committed artifacts match generator output)")
		return
	}

	if err := gadu.Generate(root); err != nil {
		fmt.Fprintf(os.Stderr, "gadu-generate: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stdout, "gadu-generate: agents/GADU.md, opencode/agents/GADU.md, and skills/gadu-operator/SKILL.md written")
}
