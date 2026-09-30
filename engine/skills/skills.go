package skills

import (
	"bytes"
	"fmt"
	"io"
	"os"
)

// installCwd is the working directory `skills install` installs into. It is a
// variable only so that a test can point it at a temporary project; nothing else
// assigns it.
var installCwd = os.Getwd

// SkillsCore is the testable CLI core for `engine skills <verb>`. It is
// SkillsCoreAt without a clock and without a locker: the verbs that need neither
// behave identically, approve fails closed because it must not invent an
// approval time, and so does every verb that takes the overlay lock, because it
// must not run unserialized.
func SkillsCore(verb string, args []string, readFile readFileFn, stdout, stderr io.Writer, exit func(int)) {
	SkillsCoreAt(verb, args, readFile, nil, nil, stdout, stderr, exit)
}

// SkillsCoreAt is the testable CLI core for `engine skills <verb>`.
// Dispatches to RenderListCore, RenderStatusCore, RenderValidateCore,
// RenderInstallCore, AddCore, RemoveCore, SyncCore, RenderLintCore,
// RenderApproveCore, RenderProjectRegisterCore, RenderProjectReviseCore,
// RenderProjectStatusCore, or RenderProjectRetireCore.
// Unknown or empty verbs fail loud (exit 1), mirroring the prespec pattern (ADR-2).
// No global state; all I/O is injected. now returns the current time as an
// RFC 3339 UTC timestamp for the verbs that record one (approve); the
// production caller passes the wall clock, and nil is legal for every other verb.
func SkillsCoreAt(verb string, args []string, readFile readFileFn, now func() string, locker Locker, stdout, stderr io.Writer, exit func(int)) {
	release, ok := acquireLocks(verb, args, locker, stderr, exit)
	if !ok {
		return
	}
	defer release()
	switch verb {
	case "list":
		RenderListCore(args, readFile, stdout, stderr, exit)
	case "status":
		RenderStatusCore(args, readFile, stdout, stderr, exit)
	case "validate":
		RenderValidateCore(args, readFile, ScanSkillFiles, stdout, stderr, exit)
	case "install":
		RenderInstallCore(args, readFile, installCwd, stdout, stderr, exit)
	case "add":
		AddCore(stripVerb(args, "add"), readFile, os.Stat, stdout, stderr, exit)
	case "remove":
		RemoveCore(stripVerb(args, "remove"), readFile, stdout, stderr, exit)
	case "sync-manifest":
		SyncCore(stripVerb(args, "sync-manifest"), readFile, stdout, stderr, exit)
	case "lint":
		RenderLintCore(stripVerb(args, "lint"), readFile, stdout, stderr, exit)
	case "approve":
		RenderApproveCore(stripVerb(args, "approve"), readFile, now, stdout, stderr, exit)
	case "project-register":
		RenderProjectRegisterCore(stripVerb(args, "project-register"), readFile, os.Stat, resolvePathKeepingMissing, osProjectFS{}, stdout, stderr, exit)
	case "project-revise":
		RenderProjectReviseCore(stripVerb(args, "project-revise"), readFile, os.ReadDir, os.Stat, resolvePathKeepingMissing, osProjectFS{}, stdout, stderr, exit)
	case "project-status":
		RenderProjectStatusCore(stripVerb(args, "project-status"), readFile, os.ReadDir, resolvePathKeepingMissing, stdout, stderr, exit)
	case "project-retire":
		RenderProjectRetireCore(stripVerb(args, "project-retire"), readFile, os.ReadDir, os.Stat, resolvePathKeepingMissing, osProjectFS{}, stdout, stderr, exit)
	case "":
		fmt.Fprintln(stderr, "error: skills requires a verb: list, status, validate, install, add, remove, sync-manifest, lint, approve, project-register, project-revise, project-status, project-retire")
		exit(1)
	default:
		fmt.Fprintf(stderr, "error: unknown skills verb %q (supported: list, status, validate, install, add, remove, sync-manifest, lint, approve, project-register, project-revise, project-status, project-retire)\n", verb)
		exit(1)
	}
}

// stripVerb removes the first occurrence of verb from args (used so that
// SkillsCore can dispatch to AddCore / RemoveCore without the verb token
// appearing as a spurious positional argument in the downstream flag parser).
func stripVerb(args []string, verb string) []string {
	for i, a := range args {
		if a == verb {
			out := make([]string, 0, len(args)-1)
			out = append(out, args[:i]...)
			out = append(out, args[i+1:]...)
			return out
		}
	}
	return args
}

// RenderValidateCore is the testable CLI core for `engine skills validate`.
// Parses --registry, --manifest, and --source-root flags, loads the registry
// and manifest, and runs both the registry/manifest cross-check (Diff, via
// Validate) and the on-disk cross-check (DiffOnDisk) in the same run.
// Exits 0 only when every check is clean, 1 when any divergence is found
// (fail-loud per R-031/R-032, extended to on-disk divergences by R-005/R-006
// and to the global-skill approval check by CheckApprovals).
//
// --source-root has no default and no cwd-derived fallback (R-002): a caller
// that omits it gets a usage error, never a silent scan of the working
// directory. scanSkills is an injected seam (R-003) so the on-disk check is
// unit-testable without walking a real tree; production callers pass
// ScanSkillFiles.
func RenderValidateCore(args []string, readFile readFileFn, scanSkills func(string) ([]string, error), stdout, stderr io.Writer, exit func(int)) {
	registryPath := "skills.registry.yaml"
	manifestPath := "overlay.manifest"
	sourceRoot := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--registry":
			if i+1 < len(args) {
				registryPath = args[i+1]
				i++
			}
		case "--manifest":
			if i+1 < len(args) {
				manifestPath = args[i+1]
				i++
			}
		case "--source-root":
			if i+1 < len(args) {
				sourceRoot = args[i+1]
				i++
			}
		}
	}

	if sourceRoot == "" {
		fmt.Fprintln(stderr, "error: skills validate requires --source-root <skills-dir>")
		exit(1)
		return
	}

	data, err := readFile(registryPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: reading registry %q: %v\n", registryPath, err)
		exit(1)
		return
	}
	reg, err := ParseRegistry(bytes.NewReader(data))
	if err != nil {
		fmt.Fprintf(stderr, "error: parsing registry: %v\n", err)
		exit(1)
		return
	}

	// Validate loads the manifest via os.Open(manifestPath) and runs Diff.
	regDivs, regErr := Validate(reg, manifestPath)

	// Print registry divergences now, immediately after Validate, and before
	// the three on-disk stages below. Each of those stages can exit(1) on its
	// own fatal error (bad manifest read, bad --source-root, scan failure);
	// printing here first means such a stage failure can never discard
	// already-computed registry divergences (R4-001, R-007).
	if regErr != nil {
		for _, d := range regDivs {
			fmt.Fprintf(stderr, "[%s] %s: %s\n", d.Class, d.Path, d.Detail)
		}
	}

	manifestData, err := readFile(manifestPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: reading manifest %q: %v\n", manifestPath, err)
		exit(1)
		return
	}
	manifestPaths, err := DeployableManifestPaths(bytes.NewReader(manifestData))
	if err != nil {
		fmt.Fprintf(stderr, "error: parsing manifest for on-disk check: %v\n", err)
		exit(1)
		return
	}
	diskPaths, err := scanSkills(sourceRoot)
	if err != nil {
		fmt.Fprintf(stderr, "error: scanning skills directory %q: %v\n", sourceRoot, err)
		exit(1)
		return
	}
	onDiskDivs := DiffOnDisk(diskPaths, manifestPaths)

	// Approval check: every global skill needs a valid human-approval record
	// for its exact SKILL.md bytes, unless it is the grandfathered baseline's.
	approvalDivs, approvals := CheckApprovals(reg, sourceRoot, readFile)

	// Full-scan reporting (R-007): print every divergence from all checks in
	// this one run, never stopping at the first error.
	for _, d := range onDiskDivs {
		fmt.Fprintf(stderr, "[%s] %s: %s\n", d.Class, d.Path, d.Detail)
	}
	for _, d := range approvalDivs {
		fmt.Fprintf(stderr, "[%s] %s: %s\n", d.Class, d.Path, d.Detail)
	}

	if regErr != nil || len(onDiskDivs) > 0 || len(approvalDivs) > 0 {
		exit(1)
		return
	}

	fmt.Fprintf(stdout, "registry and manifest aligned (%d skills)\n", len(reg.Skills))
	fmt.Fprintf(stdout, "skills/ on disk matches overlay.manifest (%d files)\n", len(diskPaths))
	fmt.Fprintf(stdout, "global skill approvals verified (%d skills: %d approved, %d grandfathered)\n",
		approvals.Global, approvals.Approved, approvals.Grandfathered)
}
