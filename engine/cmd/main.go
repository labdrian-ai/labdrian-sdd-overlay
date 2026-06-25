// Command engine provides subcommands for the deterministic-scoping engine:
//
//	engine propagate --registry <path> --contract-file <path> [--contract-path <str>]
//	engine gate-task --contract-file <path> [--contract-path <str>]
//	engine merge-settings --settings <path> --hook-command <binary-path>
//	engine uninstall-hooks --settings <path> --hook-command <binary-path>
//	engine status
//
// propagate: ensures the scoped minimalism-contract BEGIN/END marker block is
// present in a target .atl/skill-registry.md. Fails LOUD on bad input.
//
// gate-task: reads a Claude Code PreToolUse 'Agent' tool_input JSON from STDIN,
// inspects subagent_type, and emits the hook response that deterministically
// injects or strips the minimalism-contract path. Fails SAFE on any error.
//
// merge-settings: safely merges two hook entries (UserPromptSubmit + PreToolUse)
// into a Claude Code settings.json. Preserves all existing keys, is idempotent,
// atomic (write+rename), creates a .bak backup, and refuses to write if the
// existing file contains invalid JSON.
//
// uninstall-hooks: removes exactly our two hook entries from settings.json,
// leaving all other keys and hooks intact. Idempotent; no-op if file absent.
//
// status: checks and reports the health of the overlay installation (binary,
// hooks wired in settings.json, contract readable). Exits 0 if all OK, 1 if
// any check fails. Intended for manual diagnostics — never called by hooks.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gate"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/prespec"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/propagator"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings"
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
	case "merge-settings":
		runMergeSettings(os.Args[2:])
	case "uninstall-hooks":
		runUninstallHooks(os.Args[2:])
	case "status":
		runStatus(os.Args[2:])
	case "prespec":
		runPrespec(os.Args[2:])
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
	fmt.Fprintln(os.Stderr, "  engine merge-settings --settings <path> --hook-command <binary-path>")
	fmt.Fprintln(os.Stderr, "  engine uninstall-hooks --settings <path> --hook-command <binary-path>")
	fmt.Fprintln(os.Stderr, "  engine status")
	fmt.Fprintln(os.Stderr, "  engine prespec <verb>  (verbs: rank, lint, readiness, brief)")
}

// runPrespec implements the 'prespec <verb>' subcommand.
// Requires exactly one verb argument; fails LOUD on missing or unknown verb (ADR-4).
func runPrespec(args []string) {
	runPrespecCore(verbFromArgs(args), os.Stdin, os.Stdout, os.Stderr, os.Exit)
}

// runPrespecCore is the testable core of the prespec subcommand.
func runPrespecCore(verb string, stdin io.Reader, stdout io.Writer, stderr io.Writer, exit func(int)) {
	if verb == "" {
		fmt.Fprintln(stderr, "error: prespec requires a verb: rank, lint, readiness, brief")
		exit(1)
		return
	}
	prespec.PrespecCore(verb, stdin, stdout, stderr, exit)
}

// verbFromArgs extracts the first positional argument as the verb, empty if absent.
func verbFromArgs(args []string) string {
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			return a
		}
	}
	return ""
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
	// Clean the registry path to prevent path-traversal via "../" segments.
	registryPath = filepath.Clean(registryPath)
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
		if os.IsNotExist(err) {
			// Registry absent: this project does not use the overlay. Clean no-op.
			fmt.Fprintf(stdout, "propagate: registry not found at %s — project does not use the overlay (no-op)\n", registryPath)
			return
		}
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

// parseMergeSettingsArgs extracts --settings and --hook-command from args.
func parseMergeSettingsArgs(args []string) (settingsPath, hookCommand string) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--settings":
			i++
			if i < len(args) {
				settingsPath = args[i]
			}
		case "--hook-command":
			i++
			if i < len(args) {
				hookCommand = args[i]
			}
		}
	}
	return
}

// runMergeSettings implements the 'merge-settings' subcommand.
// Fails LOUD on any error (exits 1).
func runMergeSettings(args []string) {
	settingsPath, hookCommand := parseMergeSettingsArgs(args)

	if settingsPath == "" {
		fmt.Fprintln(os.Stderr, "error: --settings is required")
		os.Exit(1)
	}
	if hookCommand == "" {
		fmt.Fprintln(os.Stderr, "error: --hook-command is required")
		os.Exit(1)
	}

	m := settings.NewMerger(settingsPath, hookCommand)
	if err := m.Install(); err != nil {
		fmt.Fprintf(os.Stderr, "error: merge-settings: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stdout, "merge-settings: hooks installed successfully")
}

// runUninstallHooks implements the 'uninstall-hooks' subcommand.
// Fails LOUD on any error (exits 1).
func runUninstallHooks(args []string) {
	settingsPath, hookCommand := parseMergeSettingsArgs(args)

	if settingsPath == "" {
		fmt.Fprintln(os.Stderr, "error: --settings is required")
		os.Exit(1)
	}
	if hookCommand == "" {
		fmt.Fprintln(os.Stderr, "error: --hook-command is required")
		os.Exit(1)
	}

	m := settings.NewMerger(settingsPath, hookCommand)
	if err := m.Uninstall(); err != nil {
		fmt.Fprintf(os.Stderr, "error: uninstall-hooks: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stdout, "uninstall-hooks: hooks removed successfully")
}

// ---------------------------------------------------------------------------
// status subcommand
// ---------------------------------------------------------------------------

// statusDeps bundles the injectable dependencies for statusCore so unit tests
// can exercise all branches without touching the real filesystem or home dir.
type statusDeps struct {
	// stat reports whether a path exists and is accessible (like os.Stat).
	stat func(string) (os.FileInfo, error)
	// readFile reads a file by path (like os.ReadFile).
	readFile readFileFn
	// loadSettings reads and JSON-decodes settings.json. Returns nil map on
	// file-not-found (not an error — hooks simply absent).
	loadSettings func(string) (map[string]interface{}, error)
	// home returns the current user's home directory ($HOME).
	home func() string
	// cwd returns the current working directory (for registry check).
	cwd func() string
}

// defaultStatusDeps returns the real OS dependencies used in production.
func defaultStatusDeps() statusDeps {
	return statusDeps{
		stat:     os.Stat,
		readFile: os.ReadFile,
		loadSettings: func(path string) (map[string]interface{}, error) {
			data, err := os.ReadFile(path)
			if os.IsNotExist(err) {
				return nil, nil // file absent → no hooks
			}
			if err != nil {
				return nil, err
			}
			var root map[string]interface{}
			if err := json.Unmarshal(data, &root); err != nil {
				return nil, fmt.Errorf("invalid JSON: %w", err)
			}
			return root, nil
		},
		home: func() string { return os.Getenv("HOME") },
		cwd: func() string {
			d, _ := os.Getwd()
			return d
		},
	}
}

// runStatus is the public entry point for the 'status' subcommand.
// It uses real OS deps and exits with the result code from statusCore.
func runStatus(_ []string) {
	allOK := statusCore(os.Stdout, defaultStatusDeps())
	if !allOK {
		os.Exit(1)
	}
}

// checkResult holds the result of a single status check.
type checkResult struct {
	label string
	ok    bool
	note  string
}

// binaryIdentity is the substring used to identify our hook entries in
// settings.json — same logic as Merger.hookCommand substring match.
const binaryIdentity = "gentle-ai-overlay"

// statusCore runs all checks and writes the report to stdout.
// Returns true if every check passed (caller may exit 0), false otherwise.
func statusCore(stdout io.Writer, deps statusDeps) bool {
	home := deps.home()
	binaryPath := filepath.Join(home, ".claude", "bin", "gentle-ai-overlay")
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	contractPath := filepath.Join(home, ".claude", "skills", "_shared", "minimalism-contract.md")

	var checks []checkResult

	// Check 1: binary present + executable.
	checks = append(checks, checkBinary(binaryPath, deps.stat))

	// Check 2: UserPromptSubmit hook wired.
	// Check 3: PreToolUse/Agent hook wired.
	settingsRoot, settingsErr := deps.loadSettings(settingsPath)
	checks = append(checks, checkUserPromptSubmitHook(settingsRoot, settingsErr, settingsPath))
	checks = append(checks, checkPreToolUseHook(settingsRoot, settingsErr, settingsPath))

	// Check 4: contract readable + frontmatter parses.
	checks = append(checks, checkContract(contractPath, deps.readFile))

	// Check 5 (best-effort): registry block present in CWD.
	if cwd := deps.cwd(); cwd != "" {
		registryPath := filepath.Join(cwd, ".atl", "skill-registry.md")
		checks = append(checks, checkRegistry(registryPath, deps.readFile))
	}

	// Emit report.
	allOK := true
	for _, c := range checks {
		status := "OK  "
		if !c.ok {
			status = "FAIL"
			allOK = false
		}
		if c.note != "" {
			fmt.Fprintf(stdout, "[%s] %s — %s\n", status, c.label, c.note)
		} else {
			fmt.Fprintf(stdout, "[%s] %s\n", status, c.label)
		}
	}
	return allOK
}

// checkBinary verifies the engine binary is present and executable.
func checkBinary(binaryPath string, stat func(string) (os.FileInfo, error)) checkResult {
	label := "binary: " + binaryPath
	fi, err := stat(binaryPath)
	if err != nil {
		if os.IsNotExist(err) {
			return checkResult{label: label, ok: false, note: "not found"}
		}
		return checkResult{label: label, ok: false, note: err.Error()}
	}
	// Check executable bit (owner execute).
	if fi.Mode()&0o111 == 0 {
		return checkResult{label: label, ok: false, note: "exists but not executable"}
	}
	return checkResult{label: label, ok: true}
}

// checkUserPromptSubmitHook verifies the UserPromptSubmit entry references our binary.
func checkUserPromptSubmitHook(root map[string]interface{}, settingsErr error, settingsPath string) checkResult {
	label := "hook: UserPromptSubmit (propagate)"
	if settingsErr != nil {
		return checkResult{label: label, ok: false, note: "cannot read " + settingsPath + ": " + settingsErr.Error()}
	}
	if root == nil {
		return checkResult{label: label, ok: false, note: settingsPath + " absent or empty"}
	}
	hooks, _ := root["hooks"].(map[string]interface{})
	if hooks == nil {
		return checkResult{label: label, ok: false, note: "hooks key missing in settings.json"}
	}
	entries, _ := hooks["UserPromptSubmit"].([]interface{})
	for _, e := range entries {
		if innerHookContainsBinary(e, binaryIdentity) {
			return checkResult{label: label, ok: true}
		}
	}
	return checkResult{label: label, ok: false, note: "no UserPromptSubmit entry referencing " + binaryIdentity}
}

// checkPreToolUseHook verifies the PreToolUse/Agent entry references our binary.
func checkPreToolUseHook(root map[string]interface{}, settingsErr error, settingsPath string) checkResult {
	label := `hook: PreToolUse matcher="Agent" (gate-task)`
	if settingsErr != nil {
		return checkResult{label: label, ok: false, note: "cannot read " + settingsPath + ": " + settingsErr.Error()}
	}
	if root == nil {
		return checkResult{label: label, ok: false, note: settingsPath + " absent or empty"}
	}
	hooks, _ := root["hooks"].(map[string]interface{})
	if hooks == nil {
		return checkResult{label: label, ok: false, note: "hooks key missing in settings.json"}
	}
	entries, _ := hooks["PreToolUse"].([]interface{})
	for _, e := range entries {
		em, ok := e.(map[string]interface{})
		if !ok {
			continue
		}
		// Must have matcher == "Agent" AND reference our binary.
		if em["matcher"] != "Agent" {
			continue
		}
		if innerHookContainsBinary(e, binaryIdentity) {
			return checkResult{label: label, ok: true}
		}
	}
	return checkResult{label: label, ok: false, note: `no PreToolUse entry with matcher="Agent" referencing ` + binaryIdentity}
}

// innerHookContainsBinary returns true if the hook entry (outer object) contains
// binarySubstring in any of its inner hooks[].command strings.
// This mirrors the identity logic used by settings.Merger.
func innerHookContainsBinary(e interface{}, binarySubstring string) bool {
	em, ok := e.(map[string]interface{})
	if !ok {
		return false
	}
	innerHooks, _ := em["hooks"].([]interface{})
	for _, ih := range innerHooks {
		ihm, ok := ih.(map[string]interface{})
		if !ok {
			continue
		}
		if cmd, ok := ihm["command"].(string); ok {
			if containsSubstring(cmd, binarySubstring) {
				return true
			}
		}
	}
	return false
}

// containsSubstring is a thin wrapper around strings.Contains.
func containsSubstring(s, sub string) bool {
	return strings.Contains(s, sub)
}

// checkContract verifies the minimalism-contract file is readable with valid frontmatter.
func checkContract(contractPath string, readFile readFileFn) checkResult {
	label := "contract: " + contractPath
	data, err := readFile(contractPath)
	if err != nil {
		if os.IsNotExist(err) {
			return checkResult{label: label, ok: false, note: "not found"}
		}
		return checkResult{label: label, ok: false, note: err.Error()}
	}
	if _, err := propagator.ParseFrontmatter(string(data)); err != nil {
		return checkResult{label: label, ok: false, note: "frontmatter error: " + err.Error()}
	}
	return checkResult{label: label, ok: true}
}

// checkRegistry is a best-effort check: reports whether a .atl/skill-registry.md
// exists in cwd and contains the scoped minimalism-contract block.
// Never returns ok=false — it's best-effort (always reports but never fails the suite).
func checkRegistry(registryPath string, readFile readFileFn) checkResult {
	label := "registry (best-effort): " + registryPath
	data, err := readFile(registryPath)
	if err != nil {
		if os.IsNotExist(err) {
			return checkResult{label: label, ok: true, note: "not present (project may not use the overlay)"}
		}
		return checkResult{label: label, ok: true, note: "cannot read: " + err.Error()}
	}
	content := string(data)
	if containsSubstring(content, propagator.BeginMarker) {
		return checkResult{label: label, ok: true, note: "scoped block present"}
	}
	return checkResult{label: label, ok: true, note: "present but scoped block missing (run 'overlay install-hooks' or propagate)"}
}
