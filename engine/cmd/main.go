// Command engine provides two subcommands for the deterministic-scoping engine:
//
//	engine propagate --registry <path> --contract-file <path> [--contract-path <str>]
//	engine gate-task --contract-file <path> [--contract-path <str>]
//
// propagate: ensures the scoped minimalism-contract BEGIN/END marker block is
// present in a target .atl/skill-registry.md. Fails LOUD on bad input.
//
// gate-task: reads a Claude Code PreToolUse 'Task' tool_input JSON from STDIN,
// inspects subagent_type, and emits the hook response that deterministically
// injects or strips the minimalism-contract path. Fails SAFE on any error.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gate"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/propagator"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "propagate":
		runPropagate(os.Args[2:])
	case "gate-task":
		runGateTask(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "error: unknown subcommand %q\n", os.Args[1])
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  engine propagate --registry <path> --contract-file <path> [--contract-path <str>]")
	fmt.Fprintln(os.Stderr, "  engine gate-task --contract-file <path> [--contract-path <str>]")
}

// runPropagate implements the 'propagate' subcommand.
// Fails LOUD on any error (exits 1).
func runPropagate(args []string) {
	var registryPath, contractFilePath, contractPath string
	contractPath = "skills/_shared/minimalism-contract.md" // default

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--registry":
			i++
			if i < len(args) {
				registryPath = args[i]
			}
		case "--contract-file":
			i++
			if i < len(args) {
				contractFilePath = args[i]
			}
		case "--contract-path":
			i++
			if i < len(args) {
				contractPath = args[i]
			}
		}
	}

	if registryPath == "" {
		fmt.Fprintln(os.Stderr, "error: --registry is required")
		usage()
		os.Exit(1)
	}
	if contractFilePath == "" {
		fmt.Fprintln(os.Stderr, "error: --contract-file is required")
		usage()
		os.Exit(1)
	}

	contractContent, err := os.ReadFile(contractFilePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: reading contract file: %v\n", err)
		os.Exit(1)
	}

	phases, err := propagator.ParseFrontmatter(string(contractContent))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	registryContent, err := os.ReadFile(registryPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: reading registry file: %v\n", err)
		os.Exit(1)
	}

	cfg := propagator.Config{ContractPath: contractPath}
	out, changed, err := propagator.Propagate(string(registryContent), cfg, phases)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if !changed {
		fmt.Println("registry: minimalism-contract scope is already correct (no-op)")
		return
	}

	if err := os.WriteFile(registryPath, []byte(out), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error: writing registry: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("registry: minimalism-contract scoped row inserted/updated")
}

// runGateTask implements the 'gate-task' subcommand.
// Fails SAFE on any error (exits 0, emits pass-through response).
func runGateTask(args []string) {
	var contractFilePath, contractPath string
	contractPath = "skills/_shared/minimalism-contract.md" // default

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--contract-file":
			i++
			if i < len(args) {
				contractFilePath = args[i]
			}
		case "--contract-path":
			i++
			if i < len(args) {
				contractPath = args[i]
			}
		}
	}

	// Fail-safe: if contract file cannot be read, emit pass-through and exit 0.
	var contractContent string
	if contractFilePath != "" {
		b, err := os.ReadFile(contractFilePath)
		if err != nil {
			// Fail-safe: log to stderr, pass-through on stdout, exit 0.
			fmt.Fprintf(os.Stderr, "gate-task: warning: cannot read contract file: %v (passing through)\n", err)
			fmt.Println("{}")
			return
		}
		contractContent = string(b)
	}

	// Read STDIN.
	rawInput, err := io.ReadAll(os.Stdin)
	if err != nil {
		// Fail-safe: log to stderr, pass-through on stdout, exit 0.
		fmt.Fprintf(os.Stderr, "gate-task: warning: cannot read stdin: %v (passing through)\n", err)
		fmt.Println("{}")
		return
	}

	cfg := gate.Config{
		ContractPath:    contractPath,
		ContractContent: contractContent,
	}

	resp, _ := gate.Process(string(rawInput), cfg)
	fmt.Println(resp)
}
