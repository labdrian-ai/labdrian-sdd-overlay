// Command registry-propagator ensures the minimalism-contract scoped row
// is present in a target project's .atl/skill-registry.md.
//
// Usage:
//
//	registry-propagator --registry <path> --contract <path>
//
// The tool reads the contract frontmatter to derive the phase scope, then
// inserts or updates the marker-delimited block in the registry file.
// Exits 0 when the registry is already correct (no-op) or was updated.
// Exits 1 on any error.
package main

import (
	"flag"
	"fmt"
	"os"

	propagator "github.com/labdrian-ai/labdrian-sdd-overlay/tools/registry-propagator"
)

func main() {
	registryPath := flag.String("registry", "", "Path to .atl/skill-registry.md (required)")
	contractPath := flag.String("contract", "skills/_shared/minimalism-contract.md",
		"Relative path to minimalism-contract.md as it appears in the registry table")
	contractFile := flag.String("contract-file", "",
		"Filesystem path to the contract file to read frontmatter from (required)")
	flag.Parse()

	if *registryPath == "" {
		fmt.Fprintln(os.Stderr, "error: --registry is required")
		flag.Usage()
		os.Exit(1)
	}
	if *contractFile == "" {
		fmt.Fprintln(os.Stderr, "error: --contract-file is required")
		flag.Usage()
		os.Exit(1)
	}

	contractContent, err := os.ReadFile(*contractFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: reading contract file: %v\n", err)
		os.Exit(1)
	}

	phases, err := propagator.ParseFrontmatter(string(contractContent))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	registryContent, err := os.ReadFile(*registryPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: reading registry file: %v\n", err)
		os.Exit(1)
	}

	cfg := propagator.Config{ContractPath: *contractPath}
	out, changed, err := propagator.Propagate(string(registryContent), cfg, phases)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if !changed {
		fmt.Println("registry: minimalism-contract scope is already correct (no-op)")
		return
	}

	if err := os.WriteFile(*registryPath, []byte(out), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error: writing registry: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("registry: minimalism-contract scoped row inserted/updated")
}
