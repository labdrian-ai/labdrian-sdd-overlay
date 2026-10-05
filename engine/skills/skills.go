package skills

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path/filepath"
)

// SkillsCoreAt is the testable CLI core for `engine skills <verb>`.
// Dispatches to RenderListCore, RenderStatusCore, RenderValidateCore,
// RenderInstallCore, AddCore, RemoveCore, SyncCore, RenderLintCore,
// RenderApproveCore, RenderProjectRegisterCore, RenderProjectReviseCore,
// RenderProjectStatusCore, or RenderProjectRetireCore.
// Unknown or empty verbs fail loud (exit 1), mirroring the prespec pattern (ADR-2).
// No global state; all I/O is injected through deps. deps.Now returns the current time as an
// RFC 3339 UTC timestamp for the verbs that record one (approve); the production caller passes
// the wall clock, and nil is legal for every other verb. A verb that takes the overlay lock, and
// every one does but list, status and lint, refuses to run unserialized when deps.Locker is nil.
//
// deps.Registries is how every verb that works on the registry reads it (and how add and remove
// encode what they write): the composition root builds one, the verbs know no file format.
func SkillsCoreAt(verb string, args []string, deps Deps, stdout, stderr io.Writer, exit func(int)) {
	// install writes into the working directory. It is resolved here, once, before
	// any lock is asked for, and the lock and the verb are both given this answer:
	// a second call could return another directory, and a directory that cannot be
	// named cannot be locked. A verb that cannot name where it writes does not run.
	installRoot := ""
	if verb == "install" || verb == "adopt" {
		cwd, err := workingDirectory(deps)
		if err != nil || !filepath.IsAbs(cwd) {
			reason := fmt.Sprintf("%q is not an absolute path", cwd)
			if err != nil {
				reason = err.Error()
			}
			did := map[string]string{"install": "installed", "adopt": "adopted"}[verb]
			fmt.Fprintf(stderr, "error: skills %s: cannot resolve the project directory it works in (%s); nothing was locked and nothing was %s\n", verb, reason, did)
			exit(1)
			return
		}
		installRoot = cwd
	}
	for attempt := 1; ; attempt++ {
		if done := runLocked(attempt, verb, args, installRoot, deps, stdout, stderr, exit); done {
			return
		}
	}
}

// workingDirectory is the directory the process works in, asked of the port the composition root
// gave. A root that gave none has not wired it, which is an answer a verb can refuse with.
func workingDirectory(deps Deps) (string, error) {
	if deps.Cwd == nil {
		return "", errors.New("no working directory is wired")
	}
	return deps.Cwd()
}

// runLocked is one attempt at a verb under the locks it needs. It reports false
// only when the attempt must be made again: the verb read under a shared lock that
// held nothing, and the lock file appeared while it read (see
// rereadsWhenTheLockFileAppears). That attempt's output is discarded, not printed.
// When the lock file cannot be inspected the attempt is discarded too, and the verb
// refuses with exit 1 instead of trying again.
func runLocked(attempt int, verb string, args []string, installRoot string, deps Deps, stdout, stderr io.Writer, exit func(int)) (done bool) {
	release, provisional, ok := acquireLocks(verb, args, installRoot, deps.Locker, stderr, exit)
	if !ok {
		return true
	}
	if len(provisional) == 0 {
		defer release()
		dispatchVerb(verb, args, installRoot, deps, stdout, stderr, exit)
		return true
	}

	// The read is provisional, so what the verb prints is held back until it is
	// known to stand. exit is recorded, not called: a process exit here would skip
	// the check. The first call is the one that counts, as it would be for a real exit.
	var out, errOut bytes.Buffer
	code, exited := 0, false
	dispatchVerb(verb, args, installRoot, deps, &out, &errOut, func(c int) {
		if !exited {
			code, exited = c, true
		}
	})
	raced, statErr := rereadsWhenTheLockFileAppears(deps.Locker, provisional)
	release()
	if statErr != nil {
		// Whether a writer began cannot be known, so the read cannot be trusted and is
		// not printed. This is a refusal that reading again will not clear, not a busy
		// lock: it is exit 1, with the reason the system gave (which names the path).
		// It wins over raced: a read that may be torn is discarded either way, and
		// reading again would meet the same failure and end in a busy exit.
		fmt.Fprintf(stderr, "error: skills %s: cannot tell whether a writer began while it read: %v; nothing was changed\n", verb, statErr)
		exit(1)
		return true
	}
	if !raced {
		_, _ = out.WriteTo(stdout)
		_, _ = errOut.WriteTo(stderr)
		if exited {
			exit(code)
		}
		return true
	}
	if attempt >= maxRereadAttempts {
		fmt.Fprintf(stderr, "error: skills %s: the registry kept changing while it was being read (%d attempts); nothing was changed, retry in a moment\n", verb, attempt)
		exit(ExitBusy)
		return true
	}
	return false
}

// needsProject lists the verbs that read or write the files of a project or of an overlay through
// the Deps' Project.
var needsProject = map[string]bool{
	"add": true, "install": true, "adopt": true,
	"project-register": true, "project-revise": true, "project-status": true, "project-retire": true,
}

// dispatchVerb runs the verb. The locks, if it needs any, are already held, and
// installRoot is the directory install was resolved to and locked.
func dispatchVerb(verb string, args []string, installRoot string, deps Deps, stdout, stderr io.Writer, exit func(int)) {
	readFile, registries := deps.ReadFile, deps.Registries
	// A composition root that forgot a port: a refusal, not a crash.
	if deps.Tree == nil && (verb == "validate" || verb == "install" || verb == "adopt") {
		fmt.Fprintf(stderr, "error: skills %s: no skill tree is wired, so it cannot read the skills of the overlay\n", verb)
		exit(1)
		return
	}
	if deps.Project == nil && needsProject[verb] {
		fmt.Fprintf(stderr, "error: skills %s: no project file system is wired, so it cannot read or write files\n", verb)
		exit(1)
		return
	}
	switch verb {
	case "list":
		RenderListCore(args, registries, stdout, stderr, exit)
	case "status":
		RenderStatusCore(args, registries, stdout, stderr, exit)
	case "validate":
		RenderValidateCore(args, readFile, registries, deps.Tree.ScanSkillFiles, stdout, stderr, exit)
	case "install":
		env := installEnvOf(deps, func() (string, error) { return installRoot, nil })
		env.readProject = readFile
		renderInstall(env, args, stdout, stderr, exit)
	case "adopt":
		env := installEnvOf(deps, func() (string, error) { return installRoot, nil })
		env.readProject = readFile
		renderAdopt(env, args, stdout, stderr, exit)
	case "add":
		AddCore(stripVerb(args, "add"), readFile, registries, deps.Project.Stat, stdout, stderr, exit)
	case "remove":
		RemoveCore(stripVerb(args, "remove"), readFile, registries, stdout, stderr, exit)
	case "sync-manifest":
		SyncCore(stripVerb(args, "sync-manifest"), readFile, registries, stdout, stderr, exit)
	case "lint":
		RenderLintCore(stripVerb(args, "lint"), readFile, stdout, stderr, exit)
	case "approve":
		RenderApproveCore(stripVerb(args, "approve"), readFile, deps.Now, stdout, stderr, exit)
	case "project-register":
		RenderProjectRegisterCore(stripVerb(args, "project-register"), readFile, registries, deps.Project.Stat, deps.Project.ResolvePath, deps.Project, stdout, stderr, exit)
	case "project-revise":
		RenderProjectReviseCore(stripVerb(args, "project-revise"), readFile, deps.Project.ReadDir, deps.Project.Stat, deps.Project.ResolvePath, deps.Project, stdout, stderr, exit)
	case "project-status":
		RenderProjectStatusCore(stripVerb(args, "project-status"), readFile, registries, deps.Project.ReadDir, deps.Project.ResolvePath, stdout, stderr, exit)
	case "project-retire":
		RenderProjectRetireCore(stripVerb(args, "project-retire"), readFile, registries, deps.Project.ReadDir, deps.Project.Stat, deps.Project.ResolvePath, deps.Project, stdout, stderr, exit)
	case "":
		fmt.Fprintln(stderr, "error: skills requires a verb: list, status, validate, install, adopt, add, remove, sync-manifest, lint, approve, project-register, project-revise, project-status, project-retire")
		exit(1)
	default:
		fmt.Fprintf(stderr, "error: unknown skills verb %q (supported: list, status, validate, install, adopt, add, remove, sync-manifest, lint, approve, project-register, project-revise, project-status, project-retire)\n", verb)
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
func RenderValidateCore(args []string, readFile readFileFn, registries RegistryRepository, scanSkills func(string) ([]string, error), stdout, stderr io.Writer, exit func(int)) {
	registryPath := defaultRegistryPath
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

	reg, ok := readRegistryForVerb(registries, registryPath, false, stderr, exit)
	if !ok {
		return
	}

	// The manifest is read once, and the registry is compared with what was read. A manifest that
	// cannot be read has no divergence to tell: the refusal below says why.
	manifestData, readErr := readFile(manifestPath)
	var regDivs []Divergence
	var regErr error
	if readErr == nil {
		regDivs, regErr = ValidateAgainstManifest(reg, manifestData)
	}

	// Print registry divergences now, immediately after the comparison, and before
	// the three on-disk stages below. Each of those stages can exit(1) on its
	// own fatal error (bad manifest read, bad --source-root, scan failure);
	// printing here first means such a stage failure can never discard
	// already-computed registry divergences (R4-001, R-007).
	if regErr != nil {
		for _, d := range regDivs {
			fmt.Fprintf(stderr, "[%s] %s: %s\n", d.Class, d.Path, d.Detail)
		}
	}

	if readErr != nil {
		fmt.Fprintf(stderr, "error: reading manifest %q: %v\n", manifestPath, readErr)
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
