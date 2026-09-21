package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/projectid"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/skillstale"
)

// cmdSkillsStale implements the report-only project-tier retirement detector.
// It requires an explicit absolute project root so no invocation can silently
// inspect the command's cwd or a home-directory fallback. The detector and
// Engram store are both read-only; a finding is evidence for a later,
// separately invoked retirement decision, never an instruction to remove.
func cmdSkillsStale(args []string) int {
	fs := flag.NewFlagSet("skills-stale", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	projectRoot := fs.String("project-root", "", "absolute project root to inspect")
	project := fs.String("project", "", projectFlagUsage)
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if *projectRoot == "" || !filepath.IsAbs(*projectRoot) {
		fmt.Fprintln(os.Stderr, "longterm-mem: skills-stale: --project-root must be an absolute path")
		return exitUsage
	}
	info, err := os.Stat(*projectRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "longterm-mem: skills-stale: inspect project root: %v\n", err)
		return exitUsage
	}
	if !info.IsDir() {
		fmt.Fprintf(os.Stderr, "longterm-mem: skills-stale: --project-root is not a directory: %s\n", *projectRoot)
		return exitUsage
	}

	resolvedProject := *project
	if resolvedProject == "" {
		identity, err := projectid.Resolve(*projectRoot)
		if err != nil {
			fmt.Fprintf(os.Stderr, "longterm-mem: skills-stale: --project is required when the project root cannot be resolved: %v\n", err)
			return exitUsage
		}
		resolvedProject = identity.Project
	}

	store, err := engram.Open(os.Getenv(engramDBEnvVar))
	if err != nil {
		fmt.Fprintf(os.Stderr, "longterm-mem: skills-stale: %v\n", err)
		return exitEngramUnavailable
	}
	defer store.Close()

	findings, err := skillstale.Detect(skillstale.Config{
		ProjectRoot: *projectRoot,
		Project:     resolvedProject,
		Store:       store,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "longterm-mem: skills-stale: %v\n", err)
		return exitInternal
	}

	fmt.Printf("longterm-mem: skills-stale: checked %s for project %s\n", *projectRoot, resolvedProject)
	if len(findings) == 0 {
		fmt.Println("  nothing stale was found")
	} else {
		fmt.Print(skillstale.Render(findings))
	}
	fmt.Println("  Nothing was changed. Review this report before making a separate retirement decision.")
	return exitOK
}
