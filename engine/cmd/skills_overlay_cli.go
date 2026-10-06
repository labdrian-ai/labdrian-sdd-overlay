package main

// The command-line adapter of the verbs that write an overlay (Phase 9 unit H20): `add`,
// `remove`, `sync-manifest` and `approve`. Each reads its arguments with the one parser
// (skills_flags.go), takes the exclusive lock on the overlay (skills.OverlayLocks), asks its use
// case in engine/skills/app, and tells a person what it answered. The use cases parse nothing and
// print nothing.

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills/app"
)

// defaultSkillsSourceRoot is the skills tree add reads the skill from when it is given no
// --source-root: relative, so it means the working directory of the process.
const defaultSkillsSourceRoot = "skills"

// idWord is the skill a verb that works on one is asked about: the first word that is not empty.
func idWord(words []string) string {
	for _, w := range words {
		if w != "" {
			return w
		}
	}
	return ""
}

// lockOverlay takes the lock verb needs on the overlay of the registry at registryPath. It reports
// false, having told why and exited, when the lock was refused: the verb must not run.
func lockOverlay(verb, registryPath string, deps skills.Deps, stderr io.Writer, exit func(int)) (skills.HeldLocks, bool) {
	held, err := skills.AcquireLocks(verb, deps.Locker, skills.OverlayLocks(verb, registryPath))
	if err != nil {
		refuseLocks(err, stderr, exit)
		return skills.HeldLocks{}, false
	}
	return held, true
}

// overlayWired reports whether the ports a writer of the overlay reads and writes through are
// wired: a composition root that forgot one gets a refusal, not a crash.
func overlayWired(verb string, deps skills.Deps, needsApprovals bool, stderr io.Writer, exit func(int)) bool {
	if needsApprovals && deps.Approvals == nil {
		fmt.Fprintf(stderr, "error: skills %s: no approval record store is wired, so it cannot tell whether a skill is approved\n", verb)
		exit(1)
		return false
	}
	if deps.Project == nil {
		fmt.Fprintf(stderr, "error: skills %s: no project file system is wired, so it cannot read or write files\n", verb)
		exit(1)
		return false
	}
	return true
}

// refuseOverlayWrite says why a writer of the overlay stopped, and exits 1. A finding of the lint
// and a divergence are told as the lines they are; every other refusal is one line.
func refuseOverlayWrite(err error, stderr io.Writer, exit func(int)) {
	var (
		registry   *app.RegistryError
		lint       *app.LintRefusal
		divergence *app.DivergenceError
	)
	switch {
	case errors.As(err, &registry):
		refuseRegistry(err, false, stderr, exit)
		return
	case errors.As(err, &lint):
		for _, finding := range lint.Findings {
			fmt.Fprintln(stderr, finding)
		}
	case errors.As(err, &divergence):
		tellDivergences(divergence.Divergences, stderr)
	default:
		fmt.Fprintf(stderr, "error: %v\n", err)
	}
	exit(1)
}

// skillsAdd is `skills add <id>`: it registers a skill that is in the skills tree and approved.
func skillsAdd(deps skills.Deps, args []string, stdout, stderr io.Writer, exit func(int)) {
	parsed, err := skillsAddSpec.parse(withoutVerb(args, "add"))
	if err != nil {
		refuseSkillsUsage(err, stderr, exit)
		return
	}
	in := app.AddInput{
		RegistryPath: parsed.value(flagRegistry, defaultSkillsRegistry),
		ManifestPath: parsed.value(flagManifest, defaultSkillsManifest),
		SourceRoot:   parsed.value(flagSourceRoot, defaultSkillsSourceRoot),
		ID:           idWord(parsed.words),
		Repo:         parsed.value(flagRepo, ""),
		Ref:          parsed.value(flagRef, ""),
	}
	held, ok := lockOverlay("add", in.RegistryPath, deps, stderr, exit)
	if !ok {
		return
	}
	defer held.Release()
	if !overlayWired("add", deps, true, stderr, exit) {
		return
	}
	result, err := app.AddSkill(app.AddPorts{
		Registries: deps.Registries, Files: deps.ReadFile, Stat: deps.Project.Stat,
		Approvals: deps.Approvals, Staged: deps.Project,
	}, in)
	tellUnread(result.UnreadWarning, stderr)
	if err != nil {
		refuseOverlayWrite(err, stderr, exit)
		return
	}
	fmt.Fprintf(stdout, "added: %s\n", result.ID)
	exit(0)
}

// skillsRemove is `skills remove <id>`: it takes a skill out of the registry and the manifest, and
// deletes no file of the skill.
func skillsRemove(deps skills.Deps, args []string, stdout, stderr io.Writer, exit func(int)) {
	parsed, err := skillsRemoveSpec.parse(withoutVerb(args, "remove"))
	if err != nil {
		refuseSkillsUsage(err, stderr, exit)
		return
	}
	in := app.RemoveInput{
		RegistryPath: parsed.value(flagRegistry, defaultSkillsRegistry),
		ManifestPath: parsed.value(flagManifest, defaultSkillsManifest),
		ID:           idWord(parsed.words),
	}
	held, ok := lockOverlay("remove", in.RegistryPath, deps, stderr, exit)
	if !ok {
		return
	}
	defer held.Release()
	if !overlayWired("remove", deps, false, stderr, exit) {
		return
	}
	result, err := app.RemoveSkill(app.RemovePorts{Registries: deps.Registries, Files: deps.ReadFile, Staged: deps.Project}, in)
	tellUnread(result.UnreadWarning, stderr)
	if err != nil {
		refuseOverlayWrite(err, stderr, exit)
		return
	}
	fmt.Fprintf(stdout, "removed: %s\n", result.ID)
	exit(0)
}

// skillsSync is `skills sync-manifest`: it brings the manifest into line with the registry.
func skillsSync(deps skills.Deps, args []string, stdout, stderr io.Writer, exit func(int)) {
	parsed, err := skillsSyncSpec.parse(withoutVerb(args, "sync-manifest"))
	if err != nil {
		refuseSkillsUsage(err, stderr, exit)
		return
	}
	in := app.SyncInput{
		RegistryPath: parsed.value(flagRegistry, defaultSkillsRegistry),
		ManifestPath: parsed.value(flagManifest, defaultSkillsManifest),
	}
	held, ok := lockOverlay("sync-manifest", in.RegistryPath, deps, stderr, exit)
	if !ok {
		return
	}
	defer held.Release()
	if !overlayWired("sync-manifest", deps, false, stderr, exit) {
		return
	}
	result, err := app.SyncManifest(app.SyncPorts{Registries: deps.Registries, Files: deps.ReadFile, Staged: deps.Project}, in)
	tellUnread(result.UnreadWarning, stderr)
	if err != nil {
		refuseOverlayWrite(err, stderr, exit)
		return
	}
	if result.InSync {
		fmt.Fprintln(stdout, "overlay.manifest already in sync")
		exit(0)
		return
	}
	fmt.Fprintf(stdout, "sync-manifest: %d added, %d dropped, %d retagged\n",
		len(result.Changes.Added), len(result.Changes.Dropped), len(result.Changes.Retagged))
	exit(0)
}

// skillsApprove is `skills approve --id <id> --approver <label> --source-root <dir>`: it records
// a human approval of the exact bytes of a skill. Output is on stdout, with forward slashes in the
// path; every refusal exits 1 with the reason on stderr and nothing on stdout.
//
// The lock is the one of the registry the wrapper names, which approve does not read: a path
// given explicitly is only the name of the lock, and the default is not (skills.OverlayLocks).
func skillsApprove(deps skills.Deps, args []string, stdout, stderr io.Writer, exit func(int)) {
	parsed, err := skillsApproveSpec.parse(withoutVerb(args, "approve"))
	if err != nil {
		refuseSkillsUsage(err, stderr, exit)
		return
	}
	approver, approverGiven := parsed.values[flagApprover]
	in := app.ApproveInput{
		ID:            parsed.value(flagID, ""),
		Approver:      approver,
		ApproverGiven: approverGiven,
		SourceRoot:    parsed.value(flagSourceRoot, ""),
	}
	held, ok := lockOverlay("approve", parsed.value(flagRegistry, defaultSkillsRegistry), deps, stderr, exit)
	if !ok {
		return
	}
	defer held.Release()
	if !overlayWired("approve", deps, true, stderr, exit) {
		return
	}
	result, err := app.ApproveSkill(app.ApprovePorts{Files: deps.ReadFile, Approvals: deps.Approvals, Staged: deps.Project, Now: deps.Now}, in)
	if err != nil {
		refuseOverlayWrite(err, stderr, exit)
		return
	}
	// A warning says these bytes are approved, so none is told for an approval that was refused
	// or failed.
	for _, warning := range result.Warnings {
		fmt.Fprintln(stderr, warning)
	}
	fmt.Fprintf(stdout, "%s: %s\n", result.Verdict, result.ID)
	fmt.Fprintf(stdout, "sha256: %s\n", result.Digest)
	fmt.Fprintf(stdout, "record: %s\n", filepath.ToSlash(result.RecordPath))
	exit(0)
}
