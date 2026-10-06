package main

// The command-line adapter of the verbs of `engine skills` that sit behind a use case in
// engine/skills/app (Phase 9 unit H20): each reads its arguments with the one parser
// (skills_flags.go), asks the use case, and tells a person what it answered. The use cases
// parse nothing and print nothing; the words a person reads are this file's.

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills/app"
)

// defaultSkillsRegistry is the registry a verb works on when it is given no --registry: relative,
// so it means the working directory of the process.
const defaultSkillsRegistry = "skills.registry.yaml"

// skillsList is `skills list`: one line for each entry of the registry, sorted by id, with its
// id, source, update strategy and targets, separated by tabs.
func skillsList(deps skills.Deps, args []string, stdout, stderr io.Writer, exit func(int)) {
	parsed, err := skillsRegistryReaderSpec("list").parse(args)
	if err != nil {
		refuseSkillsUsage(err, stderr, exit)
		return
	}
	result, err := app.ListSkills(deps.Registries, app.ListInput{RegistryPath: parsed.value(flagRegistry, defaultSkillsRegistry)})
	if err != nil {
		refuseRegistry(err, false, stderr, exit)
		return
	}
	tellUnread(result.UnreadWarning, stderr)
	for _, s := range result.Skills {
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\n", s.ID, s.SourceType, s.UpdateStrategy, strings.Join(s.Targets, ","))
	}
}

// skillsStatus is `skills status`: how many entries the registry has, and of which source.
func skillsStatus(deps skills.Deps, args []string, stdout, stderr io.Writer, exit func(int)) {
	parsed, err := skillsRegistryReaderSpec("status").parse(args)
	if err != nil {
		refuseSkillsUsage(err, stderr, exit)
		return
	}
	result, err := app.RegistryStatus(deps.Registries, app.StatusInput{RegistryPath: parsed.value(flagRegistry, defaultSkillsRegistry)})
	if err != nil {
		refuseRegistry(err, false, stderr, exit)
		return
	}
	tellUnread(result.UnreadWarning, stderr)
	fmt.Fprintf(stdout, "Total:  %d\n", result.Total)
	fmt.Fprintf(stdout, "Core:   %d\n", result.Core)
	fmt.Fprintf(stdout, "Custom: %d\n", result.Custom)
	fmt.Fprintln(stdout, "Status: OK")
}

// skillsLint is `skills lint`: `lint --rules` prints the rule table and exits 0; `lint <path>`
// prints each warning on stdout and each hard finding on stderr, and exits 1 when there is a
// hard finding, since warnings alone never block. args are the arguments with the verb removed.
func skillsLint(deps skills.Deps, args []string, stdout, stderr io.Writer, exit func(int)) {
	parsed, err := skillsLintSpec.parse(args)
	if err != nil {
		refuseSkillsUsage(err, stderr, exit)
		return
	}
	if parsed.switches["--rules"] {
		fmt.Fprint(stdout, app.LintRuleTable())
		exit(0)
		return
	}
	if len(parsed.words) == 0 || parsed.words[0] == "" {
		fmt.Fprintln(stderr, "error: skills lint requires a path or --rules")
		exit(1)
		return
	}
	result, err := app.LintSkill(deps.ReadFile, app.LintInput{Path: parsed.words[0]})
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		exit(1)
		return
	}
	for _, w := range result.Warnings {
		fmt.Fprintf(stdout, "[lint:%s] %s\n", w.Rule, w.Msg)
	}
	for _, e := range result.Hard {
		fmt.Fprintln(stderr, e.Error())
	}
	if !result.Passed() {
		exit(1)
		return
	}
	exit(0)
}

// refuseSkillsUsage says a command line that was refused and exits 1.
func refuseSkillsUsage(err error, stderr io.Writer, exit func(int)) {
	fmt.Fprintf(stderr, "error: %v\n", err)
	exit(1)
}

// refuseRegistry says why a registry could not be used and exits 1: the words of a store that
// could not be read, or of a registry that is not usable (a verb that names the registry in its
// second refusal, quotePath, puts the path in it).
func refuseRegistry(err error, quotePath bool, stderr io.Writer, exit func(int)) {
	var refusal *app.RegistryError
	switch {
	case errors.As(err, &refusal) && refusal.Unreadable():
		fmt.Fprintf(stderr, "error: reading registry %q: %v\n", refusal.Path, err)
	case errors.As(err, &refusal) && quotePath:
		fmt.Fprintf(stderr, "error: parsing registry %q: %v\n", refusal.Path, err)
	default:
		fmt.Fprintf(stderr, "error: parsing registry: %v\n", err)
	}
	exit(1)
}

// tellUnread says what the reader left out of a registry it read, when it left something out.
func tellUnread(warning string, stderr io.Writer) {
	if warning != "" {
		fmt.Fprintln(stderr, warning)
	}
}
