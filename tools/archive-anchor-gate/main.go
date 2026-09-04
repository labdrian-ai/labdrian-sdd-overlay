// Command archive-anchor-gate checks that every archive report written under
// the landing-commit anchor convention actually carries a resolvable anchor.
//
// It exists because the previous enforcement was a substring pin over
// skills/inception-pipeline/SKILL.md prose: it verified that the instruction
// was WRITTEN DOWN, never that any record obeyed it. An instruction with no
// behavioural gate is a suggestion that reads like a rule, and the record it
// governs is attested by the same agent that wrote it.
//
// This gate reads the records instead. Either a report's Cycle Timestamps
// section names a landing commit that resolves in this repository — and, when
// it also names an approved tree, `git show -s --format=%T <landing_commit>`
// agrees with it — or the report explicitly records the absent, self-asserted
// or rejected outcome. Silence fails.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

const (
	exitOK      = 0
	exitFinding = 1
	exitUsage   = 2
	exitIO      = 3
)

const usageText = `Usage: archive-anchor-gate --repo PATH [--since YYYY-MM-DD] [--known-gaps PATH]

Checks every openspec/changes/archive/<date>-<change>/archive-report.md dated on
or after the anchor convention against the landing-commit anchor contract.

Options:
  --repo PATH        Repository root to scan (required)
  --since DATE       Convention date; reports archived earlier are out of scope
                     (default ` + ConventionDate + `)
  --known-gaps PATH  Ledger of reports known not to comply yet, one relative
                     path per line; '#' starts a comment. A listed report that
                     FAILS is reported as a known gap and does not fail the run.
                     A listed report that PASSES fails the run as a stale
                     waiver, so the ledger cannot quietly outlive the gap.
  --help             Show this help

Exit codes:
  0  every in-scope report satisfies the anchor contract (known gaps aside)
  1  at least one report does not, or a waiver is stale
  2  invalid command-line usage
  3  the repository or the ledger could not be read
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("archive-anchor-gate", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	repo := flags.String("repo", "", "repository root to scan")
	since := flags.String("since", ConventionDate, "convention date")
	knownGaps := flags.String("known-gaps", "", "ledger of reports known not to comply yet")
	showHelp := flags.Bool("help", false, "show help")

	if err := flags.Parse(args); err != nil {
		fmt.Fprintf(stderr, "error: %v\n\n%s", err, usageText)
		return exitUsage
	}
	if *showHelp {
		fmt.Fprint(stdout, usageText)
		return exitOK
	}
	if *repo == "" {
		fmt.Fprintf(stderr, "error: --repo is required\n\n%s", usageText)
		return exitUsage
	}

	waived, err := readKnownGaps(*knownGaps)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitIO
	}

	reports, findings, err := ScanArchive(*repo, *since)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitIO
	}

	failing := map[string]bool{}
	var open, known []Finding
	for _, finding := range findings {
		failing[finding.Report] = true
		if waived[finding.Report] {
			known = append(known, finding)
			continue
		}
		open = append(open, finding)
	}

	var stale []string
	for report := range waived {
		if !failing[report] {
			stale = append(stale, report)
		}
	}
	sort.Strings(stale)

	for _, report := range reports {
		fmt.Fprintf(stdout, "checked %s (archived %s): anchor %s\n", report.RelPath, report.Date, report.Outcome)
	}
	for _, finding := range known {
		fmt.Fprintf(stdout, "known gap %s\n", finding)
	}
	for _, finding := range open {
		fmt.Fprintf(stderr, "FAIL %s\n", finding)
	}
	for _, report := range stale {
		fmt.Fprintf(stderr, "FAIL stale waiver: %s is listed as a known gap but now satisfies the anchor contract — remove it from the ledger\n", report)
	}

	if len(open) > 0 || len(stale) > 0 {
		return exitFinding
	}
	fmt.Fprintf(stdout, "ok: %d report(s) checked, %d known gap(s)\n", len(reports), len(known))
	return exitOK
}

func readKnownGaps(path string) (map[string]bool, error) {
	waived := map[string]bool{}
	if path == "" {
		return waived, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("reading known-gaps ledger %s: %w", path, err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if idx := strings.Index(line, "#"); idx != -1 {
			line = strings.TrimSpace(line[:idx])
		}
		if line == "" {
			continue
		}
		waived[line] = true
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading known-gaps ledger %s: %w", path, err)
	}
	return waived, nil
}
