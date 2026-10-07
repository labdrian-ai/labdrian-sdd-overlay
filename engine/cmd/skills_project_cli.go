package main

// The command-line adapter of the verbs that keep the skills the agent registers in a project
// (Phase 9 unit H20): `project-register`, `project-revise`, `project-status` and `project-retire`.
// Each reads its arguments with the one parser (skills_flags.go), takes the project lock, asks its
// use case in engine/skills/app, and tells a person what it answered. A command line that lacks
// what the verb needs is refused before the lock is asked for, and nothing is read. The use cases
// parse nothing and print nothing.

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills/app"
)

// projectCommand is the command line of a project verb, read and checked: the project is named and
// absolute, and the word the verb needs, when it needs one, is there.
type projectCommand struct {
	parsed skillsArgs
	root   string
	word   string
}

// readProjectCommand reads the command line of a project verb and checks what every one of them
// needs: --project-root, absolute, with no working-directory fallback, so that no read or write can
// be aimed at a location derived from where the process happens to be. It reports false, having
// told why and exited, when the command line is refused.
func readProjectCommand(spec skillsFlagSpec, args []string, stderr io.Writer, exit func(int)) (projectCommand, bool) {
	parsed, err := spec.parseAfterVerb(args)
	if err != nil {
		refuseSkillsUsage(err, stderr, exit)
		return projectCommand{}, false
	}
	root := parsed.value(flagProjectRoot, "")
	switch {
	case root == "":
		fmt.Fprintf(stderr, "error: skills %s requires --project-root <abs> (there is no working-directory fallback)\n", spec.verb)
	case !filepath.IsAbs(root):
		fmt.Fprintf(stderr, "error: skills %s: --project-root %q must be an absolute path\n", spec.verb, root)
	default:
		cmd := projectCommand{parsed: parsed, root: root}
		if len(parsed.words) > 0 {
			cmd.word = parsed.words[0]
		}
		return cmd, true
	}
	exit(1)
	return projectCommand{}, false
}

// requireProjectArgument refuses a command line that lacks the word the verb works on.
func requireProjectArgument(verb, word, what string, stderr io.Writer, exit func(int)) bool {
	if word != "" {
		return true
	}
	fmt.Fprintf(stderr, "error: skills %s requires a %s argument\n", verb, what)
	exit(1)
	return false
}

// lockProject takes the lock verb needs on the project at root, and checks that the ports a project
// verb works through are wired. It reports false, having told why and exited, when the verb must
// not run.
func lockProject(verb, root string, deps skills.Deps, stderr io.Writer, exit func(int)) (skills.HeldLocks, bool) {
	held, err := skills.AcquireLocks(verb, deps.Locker, skills.ProjectLocks(verb, root))
	if err != nil {
		refuseLocks(err, stderr, exit)
		return skills.HeldLocks{}, false
	}
	switch {
	case deps.Project == nil:
		fmt.Fprintf(stderr, "error: skills %s: no project file system is wired, so it cannot read or write files\n", verb)
	case deps.ProjectLocks == nil:
		fmt.Fprintf(stderr, "error: skills %s: no project lock store is wired, so it cannot read the lock of the project\n", verb)
	default:
		return held, true
	}
	held.Release()
	exit(1)
	return skills.HeldLocks{}, false
}

func projectPorts(deps skills.Deps) app.ProjectPorts {
	return app.ProjectPorts{Registries: deps.Registries, Files: deps.ReadFile, Locks: deps.ProjectLocks, Project: deps.Project}
}

// refuseProject says why a project verb stopped, and exits 1. A registry that cannot be used is told
// with the path of the registry in it, and what the executor reported while it put files back is told
// before the error that follows it.
func refuseProject(err error, stderr io.Writer, exit func(int)) {
	var (
		registry *app.RegistryError
		failed   *app.ExecutionError
	)
	switch {
	case errors.As(err, &registry):
		refuseRegistry(err, true, stderr, exit)
		return
	case errors.As(err, &failed):
		fmt.Fprint(stderr, failed.Report)
	}
	fmt.Fprintf(stderr, "error: %v\n", err)
	exit(1)
}

// tellPaths prints one line for each path: the prefix and the path, repo-relative with forward
// slashes, so that each is usable as a git pathspec.
func tellPaths(stdout io.Writer, prefix string, paths []string) {
	for _, p := range paths {
		fmt.Fprintf(stdout, "%s%s\n", prefix, p)
	}
}

// skillsProjectRegister is `skills project-register --project-root <abs> --candidate <key>
// [--dry-run] <draft-file>`: the one entry into the project tier of the registry. Everything it
// prints goes to stdout with paths relative to the project, and a refusal prints nothing there: the
// agent feeds stdout straight to `git add --`, so a path beside a refusal would be a pathspec for a
// file that does not exist. The Pi trust note is told on a write that was done, and never on a
// refusal or a dry run, because it discloses a consequence only a write to .agents/skills creates.
func skillsProjectRegister(deps skills.Deps, args []string, stdout, stderr io.Writer, exit func(int)) {
	cmd, ok := readProjectCommand(skillsProjectRegisterSpec, args, stderr, exit)
	if !ok {
		return
	}
	candidate := cmd.parsed.value(flagCandidate, "")
	if candidate == "" {
		fmt.Fprintln(stderr, "error: skills project-register requires --candidate <key>")
		exit(1)
		return
	}
	if !requireProjectArgument("project-register", cmd.word, "<draft-file>", stderr, exit) {
		return
	}
	held, ok := lockProject("project-register", cmd.root, deps, stderr, exit)
	if !ok {
		return
	}
	defer held.Release()
	result, err := app.ProjectRegister(projectPorts(deps), app.ProjectRegisterInput{
		ProjectRoot: cmd.root, Candidate: candidate, RegistryPath: cmd.parsed.value(flagRegistry, defaultSkillsRegistry),
		DraftPath: cmd.word, DryRun: cmd.parsed.switches[flagDryRun],
	})
	tellUnread(result.UnreadWarning, stderr)
	if err != nil {
		refuseProject(err, stderr, exit)
		return
	}
	if cmd.parsed.switches[flagDryRun] {
		tellPaths(stdout, "plan: ", result.Planned)
		exit(0)
		return
	}
	tellPaths(stdout, "wrote: ", result.Wrote)
	fmt.Fprintf(stdout, "sha256: %s\n", result.SHA256)
	fmt.Fprintf(stdout, "revision: %d\n", result.Revision)
	fmt.Fprintln(stdout, skills.PiTrustNote)
	exit(0)
}

// skillsProjectRevise is `skills project-revise --project-root <abs> --candidate <key> [--dry-run]
// <draft-file>`: it replaces the SKILL.md of a skill the agent registered, once the project lock
// proves the agent still owns it.
func skillsProjectRevise(deps skills.Deps, args []string, stdout, stderr io.Writer, exit func(int)) {
	cmd, ok := readProjectCommand(skillsProjectReviseSpec, args, stderr, exit)
	if !ok {
		return
	}
	candidate := cmd.parsed.value(flagCandidate, "")
	if candidate == "" {
		fmt.Fprintln(stderr, "error: skills project-revise requires --candidate <key>")
		exit(1)
		return
	}
	if !requireProjectArgument("project-revise", cmd.word, "<draft-file>", stderr, exit) {
		return
	}
	held, ok := lockProject("project-revise", cmd.root, deps, stderr, exit)
	if !ok {
		return
	}
	defer held.Release()
	result, err := app.ProjectRevise(projectPorts(deps), app.ProjectReviseInput{
		ProjectRoot: cmd.root, Candidate: candidate, DraftPath: cmd.word, DryRun: cmd.parsed.switches[flagDryRun],
	})
	if err != nil {
		refuseProject(err, stderr, exit)
		return
	}
	if cmd.parsed.switches[flagDryRun] {
		tellPaths(stdout, "plan: ", result.Planned)
		exit(0)
		return
	}
	tellPaths(stdout, "wrote: ", result.Wrote)
	fmt.Fprintf(stdout, "sha256: %s\n", result.SHA256)
	fmt.Fprintf(stdout, "revision: %d\n", result.Revision)
	exit(0)
}

// skillsProjectRetire is `skills project-retire --project-root <abs> [--dry-run] <id>`: it removes a
// skill the agent registered from the project, once the lock proves the agent still owns it. It is an
// explicit action: a retirement is never inferred from a detector.
func skillsProjectRetire(deps skills.Deps, args []string, stdout, stderr io.Writer, exit func(int)) {
	cmd, ok := readProjectCommand(skillsProjectRetireSpec, args, stderr, exit)
	if !ok {
		return
	}
	if !requireProjectArgument("project-retire", cmd.word, "<id>", stderr, exit) {
		return
	}
	held, ok := lockProject("project-retire", cmd.root, deps, stderr, exit)
	if !ok {
		return
	}
	defer held.Release()
	result, err := app.ProjectRetire(projectPorts(deps), app.ProjectRetireInput{
		ProjectRoot: cmd.root, RegistryPath: cmd.parsed.value(flagRegistry, defaultSkillsRegistry), ID: cmd.word,
		Reason: cmd.parsed.value(flagReason, ""), AbsorbedInto: cmd.parsed.value(flagAbsorbedInto, ""),
		DryRun: cmd.parsed.switches[flagDryRun],
	})
	tellUnread(result.UnreadWarning, stderr)
	if err != nil {
		refuseProject(err, stderr, exit)
		return
	}
	if cmd.parsed.switches[flagDryRun] {
		tellPaths(stdout, "plan: ", result.Planned)
		exit(0)
		return
	}
	tellPaths(stdout, "removed: ", result.Removed)
	exit(0)
}

// skillsProjectStatus is `skills project-status --project-root <abs> [<id>]`: who owns each skill the
// project lock records, and which global skill supersedes it. It reads and writes nothing.
func skillsProjectStatus(deps skills.Deps, args []string, stdout, stderr io.Writer, exit func(int)) {
	cmd, ok := readProjectCommand(skillsProjectStatusSpec, args, stderr, exit)
	if !ok {
		return
	}
	held, ok := lockProject("project-status", cmd.root, deps, stderr, exit)
	if !ok {
		return
	}
	defer held.Release()
	result, err := app.ProjectStatus(projectPorts(deps), app.ProjectStatusInput{
		ProjectRoot: cmd.root, RegistryPath: cmd.parsed.value(flagRegistry, defaultSkillsRegistry), ID: cmd.word,
	})
	tellUnread(result.UnreadWarning, stderr)
	if err != nil {
		refuseProject(err, stderr, exit)
		return
	}
	for _, s := range result.Skills {
		superseded := s.SupersededBy
		if superseded == "" {
			superseded = "-"
		}
		if s.AgentOwned {
			fmt.Fprintf(stdout, "%s rev:%d owner:agent superseded-by:%s\n", s.ID, s.Revision, superseded)
			continue
		}
		fmt.Fprintf(stdout, "%s rev:%d owner:human (%s) superseded-by:%s\n", s.ID, s.Revision, s.Reason, superseded)
	}
	exit(0)
}
