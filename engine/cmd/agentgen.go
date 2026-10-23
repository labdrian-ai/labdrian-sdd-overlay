package main

// The 'gadu-generate' subcommand.

import (
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/agentgen"
)

// overlayRoot resolves the overlay repo root from the OVERLAY_DIR environment
// variable. The installed binary at ~/.claude/bin/gentle-ai-overlay cannot
// reliably locate the repo root via os.Executable() (it resolves to ~, not
// the overlay repo), so OVERLAY_DIR is required. Returns an error when unset.
func overlayRoot(getenv func(string) string) (string, error) {
	if dir := getenv("OVERLAY_DIR"); dir != "" {
		return dir, nil
	}
	return "", fmt.Errorf("OVERLAY_DIR is not set\n" +
		"  Run: OVERLAY_DIR=<overlay-repo-root> gentle-ai-overlay gadu-generate")
}

// runAgentgenGenerate implements the 'gadu-generate [--check]' subcommand.
// Without --check: calls agentgen.Generate(repoRoot) to write all artifacts: the Claude, Pi and
// Codex files of every role.
// With    --check: calls agentgen.Check(repoRoot)    to verify they are not stale.
// Exits non-zero on error. OVERLAY_DIR must be set; the installed binary
// cannot resolve the repo root reliably via os.Executable().
//
// The verb keeps the name it had when the generator wrote GADU alone, and so do the words it
// prints around its result: the overlay script and its help text name it, and renaming it is a
// change of the scripts, not of this command (a follow-up of PR 5 of the role design).
func runAgentgenGenerate(p process, d deps, args []string) {
	checkMode := false
	for _, a := range args {
		if a == "--check" {
			checkMode = true
		}
	}

	root, err := overlayRoot(d.env)
	if err != nil {
		fmt.Fprintf(p.stderr, "gadu-generate: %v\n", err)
		p.exit(1)
		return
	}

	if checkMode {
		if err := agentgen.Check(root); err != nil {
			fmt.Fprintf(p.stderr, "gadu-generate --check: %v\n", err)
			p.exit(1)
			return
		}
		fmt.Fprintln(p.stdout, "gadu-generate --check: OK (committed artifacts match generator output)")
		return
	}

	if err := agentgen.Generate(root); err != nil {
		fmt.Fprintf(p.stderr, "gadu-generate: %v\n", err)
		p.exit(1)
		return
	}
	fmt.Fprintln(p.stdout, "gadu-generate: agents/<role>.md, pi/agents/<role>.md, and codex/agents/<role>.toml written")
}
