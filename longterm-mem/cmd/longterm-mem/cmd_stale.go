package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/projectid"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/staleness"
)

// cmdStale implements `longterm-mem stale [--project P]`: report the
// memories this repository disagrees with.
//
// It REPORTS and stops there. Engram is read-only to this module (R-002),
// and no memory is deleted on the word of a heuristic about file paths --
// the operator reads the evidence, which is a commit, and decides. That is
// also why removals and moves are printed apart: a moved file leaves a
// memory out of date about where something lives, never wrong about what it
// is, and folding the two together would invite treating them alike.
func cmdStale(args []string) int {
	fs := flag.NewFlagSet("stale", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	project := fs.String("project", "", projectFlagUsage)
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	resolvedProject, exit := resolveProjectFlag("stale", *project)
	if exit != exitOK {
		return exit
	}

	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "longterm-mem: stale: reading the working directory: %v\n", err)
		return exitUsage
	}
	id, err := projectid.Resolve(wd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "longterm-mem: stale: %v\n", err)
		return exitUsage
	}

	store, err := engram.Open(os.Getenv(engramDBEnvVar))
	if err != nil {
		fmt.Fprintf(os.Stderr, "longterm-mem: stale: %v\n", err)
		return exitEngramUnavailable
	}
	defer store.Close()

	observations, err := store.ListObservations(resolvedProject)
	if err != nil {
		fmt.Fprintf(os.Stderr, "longterm-mem: stale: %v\n", err)
		return exitEngramUnavailable
	}

	findings, err := staleness.Detect(id.WorktreeRoot, observations)
	if err != nil {
		fmt.Fprintf(os.Stderr, "longterm-mem: stale: %v\n", err)
		return exitInternal
	}

	reportStale(resolvedProject, id.WorktreeRoot, len(observations), findings)
	return exitOK
}

func reportStale(project, root string, scanned int, findings []staleness.Finding) {
	fmt.Printf("longterm-mem: stale: %d observations in %s, checked against %s\n", scanned, project, root)
	if len(findings) == 0 {
		fmt.Println("  nothing this repository disagrees with")
		return
	}

	var removed, moved int
	for _, f := range findings {
		fmt.Printf("\n  [%d] %s\n", f.ObservationID, f.Title)
		for _, p := range f.Removed {
			removed++
			fmt.Printf("      REMOVED %s (by %s on %s)\n", p.Path, shortCommit(p.Commit), p.At.Format("2006-01-02"))
		}
		for _, p := range f.Moved {
			moved++
			fmt.Printf("      MOVED   %s -> %s (by %s)\n", p.Path, p.NewPath, shortCommit(p.Commit))
		}
	}

	fmt.Printf("\n  %d memories name something removed after they were written; %d name something that moved.\n", removed, moved)
	fmt.Println("  Nothing was changed. Each REMOVED line carries the commit that removed it: read it before")
	fmt.Println("  deciding, then record the removal so the next agent does not reintroduce what it describes.")
}

func shortCommit(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}
