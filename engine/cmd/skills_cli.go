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

// skillsVerb is a verb of the CLI adapter: it is given the arguments the program was, the verb
// among them where the person put it.
type skillsVerb func(deps skills.Deps, args []string, stdout, stderr io.Writer, exit func(int))

// skillsCLIVerbs are the verbs that run as use cases behind this adapter; every other verb is
// still run by skills.SkillsCoreAt. A verb that has moved takes no lock here only because it
// needs none: the ones that do are moved with the locks they take.
var skillsCLIVerbs = map[string]skillsVerb{
	"list":   skillsList,
	"status": skillsStatus,
	"lint": func(deps skills.Deps, args []string, stdout, stderr io.Writer, exit func(int)) {
		skillsLint(deps, withoutVerb(args, "lint"), stdout, stderr, exit)
	},
}

// withoutVerb removes the first word that is the verb, so that it is not read as the word that
// follows it: the verbs that take a word (lint takes a path) would otherwise take the verb.
func withoutVerb(args []string, verb string) []string {
	for i, a := range args {
		if a == verb {
			out := make([]string, 0, len(args)-1)
			out = append(out, args[:i]...)
			return append(out, args[i+1:]...)
		}
	}
	return args
}

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

// defaultSkillsManifest is the manifest validate compares the registry with when it is given no
// --manifest: relative, so it means the working directory of the process.
const defaultSkillsManifest = "overlay.manifest"

// validateOutcome is what one read of validate answered: the result, and the error that stopped it.
type validateOutcome struct {
	result app.ValidateResult
	err    error
}

// skillsValidate is `skills validate`: it takes the shared lock on the overlay, which it holds for
// the whole read and takes again when a writer began during it, and tells every divergence of the
// registry, the manifest, the skills tree and the approvals in one run. It exits 1 on any, and
// prints the registry and manifest aligned only when every check is clean.
func skillsValidate(deps skills.Deps, args []string, stdout, stderr io.Writer, exit func(int)) {
	parsed, err := skillsValidateSpec.parse(args)
	if err != nil {
		refuseSkillsUsage(err, stderr, exit)
		return
	}
	if deps.Tree == nil {
		fmt.Fprintln(stderr, "error: skills validate: no skill tree is wired, so it cannot read the skills of the overlay")
		exit(1)
		return
	}
	if deps.Approvals == nil {
		fmt.Fprintln(stderr, "error: skills validate: no approval record store is wired, so it cannot tell whether a skill is approved")
		exit(1)
		return
	}
	in := app.ValidateInput{
		RegistryPath: parsed.value(flagRegistry, defaultSkillsRegistry),
		ManifestPath: parsed.value(flagManifest, defaultSkillsManifest),
		SourceRoot:   parsed.value(flagSourceRoot, ""),
	}
	ports := app.ValidatePorts{Registries: deps.Registries, Manifest: deps.ReadFile, Tree: deps.Tree, Approvals: deps.Approvals}
	outcome, err := skills.ReadConsistently("validate", deps.Locker, skills.OverlayLocks("validate", in.RegistryPath), func() validateOutcome {
		result, err := app.ValidateOverlay(ports, in)
		return validateOutcome{result: result, err: err}
	})
	if err != nil {
		refuseLocks(err, stderr, exit)
		return
	}
	tellValidate(outcome, stdout, stderr, exit)
}

// tellValidate says what validate found: every divergence on stderr in the order the checks ran,
// the reason it stopped when it did, and on a clean run the three lines that say it passed.
func tellValidate(outcome validateOutcome, stdout, stderr io.Writer, exit func(int)) {
	res, err := outcome.result, outcome.err
	var registry *app.RegistryError
	switch {
	case errors.Is(err, app.ErrSourceRootRequired):
		fmt.Fprintf(stderr, "error: %v\n", err)
		exit(1)
		return
	case errors.As(err, &registry):
		refuseRegistry(err, false, stderr, exit)
		return
	}
	tellUnread(res.UnreadWarning, stderr)
	for _, note := range res.SharedPathNotes {
		fmt.Fprintln(stderr, note)
	}
	// What the registry and the manifest disagree on is told before any later check can stop the
	// run, so that a failure of a later stage never discards a divergence already known.
	if res.RegistryDiverges {
		tellDivergences(res.RegistryDivergences, stderr)
	}
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		exit(1)
		return
	}
	tellDivergences(res.OnDisk, stderr)
	tellDivergences(res.Unapproved, stderr)
	if res.NotVerifiable != nil {
		fmt.Fprintf(stderr, "error: %v\n", res.NotVerifiable)
	}
	if !res.Passed() {
		exit(1)
		return
	}
	fmt.Fprintf(stdout, "registry and manifest aligned (%d skills)\n", res.SkillCount)
	fmt.Fprintf(stdout, "skills/ on disk matches overlay.manifest (%d files)\n", res.DiskFiles)
	fmt.Fprintf(stdout, "global skill approvals verified (%d skills: %d approved, %d grandfathered)\n",
		res.Approvals.Global, res.Approvals.Approved, res.Approvals.Grandfathered)
}

func tellDivergences(divs []skills.Divergence, stderr io.Writer) {
	for _, d := range divs {
		fmt.Fprintf(stderr, "[%s] %s: %s\n", d.Class, d.Path, d.Detail)
	}
}

// refuseLocks says why the locks of a verb were refused, or its read could not be trusted, and
// exits with the code that answers it: 2 for a lock that stayed taken, 1 for the rest.
func refuseLocks(err error, stderr io.Writer, exit func(int)) {
	fmt.Fprintf(stderr, "error: %v\n", err)
	code := 1
	var coded interface{ ExitCode() int }
	if errors.As(err, &coded) {
		code = coded.ExitCode()
	}
	exit(code)
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
