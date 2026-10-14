package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/repoevidence"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/skillstale"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/skillstalefs"
)

// cmdSkillsStale implements the report-only project-tier retirement detector.
// It requires an explicit absolute project root so no invocation can silently
// inspect the command's cwd or a home-directory fallback. The detector and
// Engram store are both read-only; a finding is evidence for a later,
// separately invoked retirement decision, never an instruction to remove.
func (c commands) cmdSkillsStale(args []string) int {
	fs := flag.NewFlagSet("skills-stale", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	projectRoot := fs.String("project-root", "", "absolute project root to inspect")
	project := fs.String("project", "", projectFlagUsage)
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	a := c.newApp("skills-stale")
	if err := checkProjectRoot(*projectRoot); err != nil {
		return a.refuse(err, exitUsage)
	}

	resolvedProject := *project
	if resolvedProject == "" {
		identity, err := identityRules().Resolve(*projectRoot)
		if err != nil {
			return a.refuse(fmt.Errorf("--project is required when the project root cannot be resolved: %w", err), exitUsage)
		}
		resolvedProject = identity.Project
	}

	store, exit := a.memory()
	if exit != exitOK {
		return exit
	}
	defer store.Close()

	// The detector reads nothing itself: the project's files and repository, the commands on this process's
	// PATH and the instant are handed to it here, where the program meets its surroundings.
	onDisk, err := skillstalefs.OpenProject(*projectRoot, repoevidence.At(*projectRoot))
	if err != nil {
		return a.refuse(err, exitUsage)
	}
	findings, err := skillstale.Detect(skillstale.Config{
		Project:      resolvedProject,
		Source:       onDisk,
		Observations: store,
		Facts:        onDisk,
		Commands:     skillstalefs.PathCommands{PathEnv: os.Getenv("PATH")},
		Now:          c.clock.Now(),
	})
	if err != nil {
		return a.refuse(err, exitInternal)
	}

	reportSkillsStale(*projectRoot, resolvedProject, findings)
	return exitOK
}

// checkProjectRoot refuses a project root that is not an absolute path to a directory, in the words the command
// has always used.
func checkProjectRoot(root string) error {
	if root == "" || !filepath.IsAbs(root) {
		return errors.New("--project-root must be an absolute path")
	}
	info, err := os.Stat(root)
	if err != nil {
		return fmt.Errorf("inspect project root: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("--project-root is not a directory: %s", root)
	}
	return nil
}

// reportSkillsStale prints what the detector found, and that nothing was changed.
func reportSkillsStale(projectRoot, project string, findings []skillstale.Finding) {
	fmt.Printf("longterm-mem: skills-stale: checked %s for project %s\n", projectRoot, project)
	if len(findings) == 0 {
		fmt.Println("  nothing stale was found")
	} else {
		fmt.Print(skillstale.Render(findings))
	}
	fmt.Println("  Nothing was changed. Review this report before making a separate retirement decision.")
}
