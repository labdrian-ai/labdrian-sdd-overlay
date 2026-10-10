package main

// The 'gate-task' subcommand: the PreToolUse hook on the Agent tool.

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/contract"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gate"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/hookwire"
)

// stdinSizeLimit caps how many bytes 'gate-task' reads from stdin to prevent a runaway
// producer from exhausting memory. It is the bound of the decoder of the Agent call
// (hookwire.MaxAgentCallBytes), far beyond any realistic hook input: the command reads exactly
// that many bytes, so an input over it reaches the decoder cut short and is let through.
const stdinSizeLimit = hookwire.MaxAgentCallBytes

// runGateTask implements the 'gate-task' subcommand.
// Fails SAFE on any error (exits 0, emits pass-through response).
func runGateTask(p process, args []string) {
	gateTaskCore(args, p.stdin, p.stdout, p.stderr, os.ReadFile)
}

// readFileFn is the type of a function that reads a file by path (injectable for tests).
type readFileFn func(string) ([]byte, error)

// gateTaskCore is the testable core of the gate-task subcommand. It accepts
// injectable stdin/stdout/stderr and a file-reader so unit tests can exercise
// all branches without real files or OS I/O.
func gateTaskCore(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer, readFile readFileFn) {
	var contractFilePath, contractPath, embeddedName, workContextJSON, workContextFile string
	contractPath = defaultContractPath
	contractPathExplicit := false

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
				contractPathExplicit = true
			}
		case "--embedded-contract":
			i++
			if i < len(args) {
				embeddedName = args[i]
			}
		case "--work-context-json":
			i++
			if i < len(args) {
				workContextJSON = args[i]
			}
		case "--work-context-file":
			i++
			if i < len(args) {
				workContextFile = args[i]
			}
		}
	}

	var contractContent string

	if embeddedName != "" {
		// Engine-owned managed contract: content ships in the binary, so the
		// guard injects even when no external contract file exists. Unknown
		// names stay fail-safe (pass-through) per the gate-task contract.
		spec, ok := embeddedContract(embeddedName)
		if !ok {
			fmt.Fprintf(stderr, "gate-task: warning: unknown embedded contract %q (passing through)\n", embeddedName)
			fmt.Fprintln(stdout, "{}")
			return
		}
		contractContent = spec.content
		// When --contract-path was not explicitly provided, use the embedded
		// contract's own default path (mirrors the propagate command).
		if !contractPathExplicit {
			contractPath = spec.defaultPath
		}
	} else {
		// F3: emit a diagnostic when --contract-file is missing so wiring mistakes
		// during PR-B integration are immediately visible. Still fail-safe (exit 0).
		if contractFilePath == "" {
			fmt.Fprintln(stderr, "gate-task: warning: --contract-file not provided; all Agent hooks will pass through")
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
		contractContent = string(b)
	}

	// F4: cap stdin reads to stdinSizeLimit so a runaway producer cannot exhaust memory.
	// On truncation the JSON will be malformed → hookwire's decoder refuses it and the
	// answer is the pass-through.
	rawInput, err := io.ReadAll(io.LimitReader(stdin, stdinSizeLimit))
	if err != nil {
		// Fail-safe: log to stderr, pass-through on stdout.
		fmt.Fprintf(stderr, "gate-task: warning: cannot read stdin: %v (passing through)\n", err)
		fmt.Fprintln(stdout, "{}")
		return
	}

	cfg := gate.Config{Contracts: []gate.ContractConfig{{Path: contractPath, Content: contractContent}}}
	workContext, workContextErr := loadWorkContext(workContextJSON, workContextFile, readFile)
	if workContextErr != nil {
		fmt.Fprintf(stderr, "gate-task: warning: work context ignored: %v (passing through for context-aware contracts)\n", workContextErr)
	} else {
		cfg.WorkContext = workContext
	}

	// Item 2: emit a stderr diagnostic when the contract frontmatter is broken so
	// wiring mistakes with a corrupt contract are immediately visible. stdout stays
	// pass-through '{}' and exit 0 (fail-safe contract UNCHANGED). The gate reads both parses.
	if _, _, err = contract.ParseBoth(contractContent); err != nil {
		fmt.Fprintf(stderr, "gate-task: warning: contract frontmatter unparseable: %v (passing through)\n", err)
	}

	_, _ = stdout.Write(agentGateAnswer(rawInput, cfg))
}

func loadWorkContext(rawJSON, filePath string, readFile readFileFn) (*gate.WorkContext, error) {
	if rawJSON == "" && filePath == "" {
		return nil, nil
	}
	if rawJSON != "" && filePath != "" {
		return nil, fmt.Errorf("use only one of --work-context-json or --work-context-file")
	}
	data := []byte(rawJSON)
	if filePath != "" {
		b, err := readFile(filePath)
		if err != nil {
			return nil, err
		}
		data = b
	}
	var ctx gate.WorkContext
	if err := json.Unmarshal(data, &ctx); err != nil {
		return nil, err
	}
	return &ctx, nil
}
