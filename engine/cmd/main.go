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

// stdinSizeLimit caps how many bytes we read from stdin to prevent a runaway
// producer from exhausting memory. 4 MiB is far beyond any realistic hook input.
const stdinSizeLimit = 4 * 1024 * 1024

// runGateTask implements the 'gate-task' subcommand.
// Fails SAFE on any error (exits 0, emits pass-through response).
func runGateTask(args []string) {
	gateTaskCore(args, os.Stdin, os.Stdout, os.Stderr, os.ReadFile)
}

// readFileFn is the type of a function that reads a file by path (injectable for tests).
type readFileFn func(string) ([]byte, error)

// gateTaskCore is the testable core of the gate-task subcommand. It accepts
// injectable stdin/stdout/stderr and a file-reader so unit tests can exercise
// all branches without real files or OS I/O.
func gateTaskCore(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer, readFile readFileFn) {
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

	// F3: emit a diagnostic when --contract-file is missing so wiring mistakes
	// during PR-B integration are immediately visible. Still fail-safe (exit 0).
	if contractFilePath == "" {
		fmt.Fprintln(stderr, "gate-task: warning: --contract-file not provided; all Task hooks will pass through")
		fmt.Fprintln(stdout, "{}")
		return
	}

	// Fail-safe: if contract file cannot be read, emit diagnostic + pass-through.
	b, err := readFile(contractFilePath)
	if err != nil {
		fmt.Fprintf(stderr, "gate-task: warning: cannot read contract file: %v (passing through)\n", err)
		fmt.Fprintln(stdout, "{}")
		return
	}
	contractContent := string(b)

	// F4: cap stdin reads to stdinSizeLimit so a runaway producer cannot exhaust memory.
	// On truncation the JSON will be malformed → gate.Process absorbs it as pass-through.
	rawInput, err := io.ReadAll(io.LimitReader(stdin, stdinSizeLimit))
	if err != nil {
		// Fail-safe: log to stderr, pass-through on stdout.
		fmt.Fprintf(stderr, "gate-task: warning: cannot read stdin: %v (passing through)\n", err)
		fmt.Fprintln(stdout, "{}")
		return
	}

	cfg := gate.Config{
		ContractPath:    contractPath,
		ContractContent: contractContent,
	}

	resp, _ := gate.Process(string(rawInput), cfg)
	fmt.Fprintln(stdout, resp)
}
