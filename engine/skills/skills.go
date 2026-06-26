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
		RenderValidateCore(args, readFile, stdout, stderr, exit)
	case "install":
		RenderInstallCore(args, readFile, os.Getwd, stdout, stderr, exit)
	case "add":
		AddCore(stripVerb(args, "add"), readFile, os.Stat, stdout, stderr, exit)
	case "remove":
		RemoveCore(stripVerb(args, "remove"), readFile, stdout, stderr, exit)
	case "":
		fmt.Fprintln(stderr, "error: skills requires a verb: list, status, validate, install, add, remove")
		exit(1)
	default:
		fmt.Fprintf(stderr, "error: unknown skills verb %q (supported: list, status, validate, install, add, remove)\n", verb)
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
// Parses --registry and --manifest flags, loads both files, and runs Diff.
// Exits 0 when aligned, 1 when any divergence is found (fail-loud per R-031/R-032).
func RenderValidateCore(args []string, readFile readFileFn, stdout, stderr io.Writer, exit func(int)) {
	registryPath := "skills.registry.yaml"
	manifestPath := "overlay.manifest"
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
		}
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
	divs, err := Validate(reg, manifestPath)
	if err != nil {
		for _, d := range divs {
			fmt.Fprintf(stderr, "[%s] %s: %s\n", d.Class, d.Path, d.Detail)
		}
		exit(1)
		return
	}
	fmt.Fprintf(stdout, "registry and manifest aligned (%d skills)\n", len(reg.Skills))
}
