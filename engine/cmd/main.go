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

// writeFileFn is the type of a function that writes a file (injectable for tests).
type writeFileFn func(string, []byte, os.FileMode) error

// runPropagate implements the 'propagate' subcommand.
// Fails LOUD on any error (exits 1).
func runPropagate(args []string) {
	runPropagateCore(args, os.Stdout, os.Stderr, os.ReadFile, os.WriteFile, os.Exit)
}

// runPropagateCore is the testable core of the propagate subcommand. It accepts
// injectable stdout/stderr, file-reader, file-writer, and exit function so unit
// tests can exercise all branches without real files or OS I/O.
func runPropagateCore(
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	readFile readFileFn,
	writeFile writeFileFn,
	exit func(int),
) {
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
		fmt.Fprintln(stderr, "error: --registry is required")
		exit(1)
		return
	}
	if contractFilePath == "" {
		fmt.Fprintln(stderr, "error: --contract-file is required")
		exit(1)
		return
	}

	contractContent, err := readFile(contractFilePath)
	if err != nil {
		fmt.Fprintf(stderr, "error: reading contract file: %v\n", err)
		exit(1)
		return
	}

	phases, err := propagator.ParseFrontmatter(string(contractContent))
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		exit(1)
		return
	}

	registryContent, err := readFile(registryPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: reading registry file: %v\n", err)
		exit(1)
		return
	}

	cfg := propagator.Config{ContractPath: contractPath}
	out, changed, err := propagator.Propagate(string(registryContent), cfg, phases)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		exit(1)
		return
	}

	if !changed {
		fmt.Fprintln(stdout, "registry: minimalism-contract scope is already correct (no-op)")
		return
	}

	if err := writeFile(registryPath, []byte(out), 0644); err != nil {
		fmt.Fprintf(stderr, "error: writing registry: %v\n", err)
		exit(1)
		return
	}
	fmt.Fprintln(stdout, "registry: minimalism-contract scoped row inserted/updated")
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

	// Item 2: emit a stderr diagnostic when the contract frontmatter is broken so
	// wiring mistakes with a corrupt contract are immediately visible. stdout stays
	// pass-through '{}' and exit 0 (fail-safe contract UNCHANGED).
	if _, err := propagator.ParseFrontmatter(contractContent); err != nil {
		fmt.Fprintf(stderr, "gate-task: warning: contract frontmatter unparseable: %v (passing through)\n", err)
	}

	resp, _ := gate.Process(string(rawInput), cfg)
	fmt.Fprintln(stdout, resp)
}
