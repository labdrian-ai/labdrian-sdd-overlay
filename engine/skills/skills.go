package skills

import (
	"bytes"
	"fmt"
	"io"
	"os"
)

// SkillsCore is the testable CLI core for `engine skills <verb>`.
// Dispatches to RenderListCore, RenderStatusCore, or RenderValidateCore.
// Unknown or empty verbs fail loud (exit 1), mirroring the prespec pattern (ADR-2).
// No global state; all I/O is injected.
func SkillsCore(verb string, args []string, readFile readFileFn, stdout, stderr io.Writer, exit func(int)) {
	switch verb {
	case "list":
		RenderListCore(args, readFile, stdout, stderr, exit)
	case "status":
		RenderStatusCore(args, readFile, stdout, stderr, exit)
	case "validate":
		RenderValidateCore(args, readFile, ScanSkillFiles, stdout, stderr, exit)
	case "install":
		RenderInstallCore(args, readFile, os.Getwd, stdout, stderr, exit)
	case "add":
		AddCore(stripVerb(args, "add"), readFile, os.Stat, stdout, stderr, exit)
	case "remove":
		RemoveCore(stripVerb(args, "remove"), readFile, stdout, stderr, exit)
	case "sync-manifest":
		SyncCore(stripVerb(args, "sync-manifest"), readFile, stdout, stderr, exit)
	case "":
		fmt.Fprintln(stderr, "error: skills requires a verb: list, status, validate, install, add, remove, sync-manifest")
		exit(1)
	default:
		fmt.Fprintf(stderr, "error: unknown skills verb %q (supported: list, status, validate, install, add, remove, sync-manifest)\n", verb)
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
// Exits 0 only when both checks are clean, 1 when any divergence is found
// (fail-loud per R-031/R-032, extended to on-disk divergences by R-005/R-006).
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

	// Full-scan reporting (R-007): print every divergence from both checks in
	// this one run, never stopping at the first error.
	if regErr != nil {
		for _, d := range regDivs {
			fmt.Fprintf(stderr, "[%s] %s: %s\n", d.Class, d.Path, d.Detail)
		}
	}
	for _, d := range onDiskDivs {
		fmt.Fprintf(stderr, "[%s] %s: %s\n", d.Class, d.Path, d.Detail)
	}

	if regErr != nil || len(onDiskDivs) > 0 {
		exit(1)
		return
	}

	fmt.Fprintf(stdout, "registry and manifest aligned (%d skills)\n", len(reg.Skills))
	fmt.Fprintf(stdout, "skills/ on disk matches overlay.manifest (%d files)\n", len(diskPaths))
}
