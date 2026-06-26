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
// hooks wired in settings.json, contract readable, registry state). Exit codes:
// 0 = all OK, 1 = a hard check FAILED, 2 = no hard failure but a check is
// DEGRADED (e.g. registry present but its scoped block is missing). The registry
// is fail-loud: an empty or unreadable registry is a FAIL, never a silent OK.
// Intended for manual diagnostics — never called by hooks.
//
// propagate/gate-task accept --embedded-contract <name> to source an
// engine-owned managed contract (e.g. skill-discovery-safety) from the binary
// instead of an external file; propagate then writes that contract's DISTINCT
// marker block. propagate also accepts --require-registry to turn an absent
// registry into a fail-loud error instead of a silent no-op.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/assets"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gadu"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gate"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/prespec"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/propagator"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings"
)

// embeddedContract resolves a named engine-owned managed contract to its content
// and (for propagate) the distinct marker pair + row label that scope its block.
// Returns ok=false for an unknown name so callers can fail loud.
//
// Adding a second managed contract here is the supported extension point: the
// engine ships the canonical text, so the guard propagates on every install with
// no dependency on an external, regenerable skill file.
func embeddedContract(name string) (spec embeddedContractSpec, ok bool) {
	switch name {
	case "skill-discovery-safety":
		return embeddedContractSpec{
			content:     assets.SkillDiscoverySafety,
			beginMarker: propagator.DiscoverySafetyBeginMarker,
			endMarker:   propagator.DiscoverySafetyEndMarker,
			rowLabel:    "skill-discovery-safety",
			// defaultPath is the registry-row Path cell / bare injected line when
			// the caller does not override --contract-path. It is where the
			// overlay deploys the standalone copy of this contract.
			defaultPath: "skills/_shared/skill-discovery-safety.md",
		}, true
	default:
		return embeddedContractSpec{}, false
	}
}

// embeddedContractSpec bundles the resolved attributes of an engine-owned
// managed contract.
type embeddedContractSpec struct {
	content     string
	beginMarker string
	endMarker   string
	rowLabel    string
	defaultPath string
}

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
	case "gadu-generate":
		runGaduGenerate(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "error: unknown subcommand %q\n", os.Args[1])
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  engine propagate --registry <path> (--contract-file <path> [--contract-path <str>] | --embedded-contract <name>) [--require-registry]")
	fmt.Fprintln(os.Stderr, "  engine gate-task (--contract-file <path> | --embedded-contract <name>) [--contract-path <str>]")
	fmt.Fprintln(os.Stderr, "  engine merge-settings --settings <path> --hook-command <binary-path>")
	fmt.Fprintln(os.Stderr, "  engine uninstall-hooks --settings <path> --hook-command <binary-path>")
	fmt.Fprintln(os.Stderr, "  engine status")
	fmt.Fprintln(os.Stderr, "  engine prespec <verb>  (verbs: rank, lint, readiness, brief)")
	fmt.Fprintln(os.Stderr, "  OVERLAY_DIR=<repo-root> gentle-ai-overlay gadu-generate [--check]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Embedded contracts: skill-discovery-safety")
	fmt.Fprintln(os.Stderr, "status exit codes: 0 ok, 1 hard failure, 2 degraded")
}

// overlayRoot resolves the overlay repo root from the OVERLAY_DIR environment
// variable. The installed binary at ~/.claude/bin/gentle-ai-overlay cannot
// reliably locate the repo root via os.Executable() (it resolves to ~, not
// the overlay repo), so OVERLAY_DIR is required. Returns an error when unset.
func overlayRoot() (string, error) {
	if dir := os.Getenv("OVERLAY_DIR"); dir != "" {
		return dir, nil
	}
	return "", fmt.Errorf("OVERLAY_DIR is not set\n" +
		"  Run: OVERLAY_DIR=<overlay-repo-root> gentle-ai-overlay gadu-generate")
}

// runGaduGenerate implements the 'gadu-generate [--check]' subcommand.
// Without --check: calls gadu.Generate(repoRoot) to write both artifacts.
// With    --check: calls gadu.Check(repoRoot)    to verify they are not stale.
// Exits non-zero on error. OVERLAY_DIR must be set; the installed binary
// cannot resolve the repo root reliably via os.Executable().
func runGaduGenerate(args []string) {
	checkMode := false
	for _, a := range args {
		if a == "--check" {
			checkMode = true
		}
	}

	root, err := overlayRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "gadu-generate: %v\n", err)
		os.Exit(1)
	}

	if checkMode {
		if err := gadu.Check(root); err != nil {
			fmt.Fprintf(os.Stderr, "gadu-generate --check: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stdout, "gadu-generate --check: OK (committed artifacts match generator output)")
		return
	}

	if err := gadu.Generate(root); err != nil {
		fmt.Fprintf(os.Stderr, "gadu-generate: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stdout, "gadu-generate: agents/GADU.md and skills/gadu-operator/SKILL.md written")
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
	var registryPath, contractFilePath, contractPath, embeddedName string
	contractPath = "skills/_shared/minimalism-contract.md" // default
	contractPathExplicit := false
	requireRegistry := false

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
				contractPathExplicit = true
			}
		case "--embedded-contract":
			i++
			if i < len(args) {
				embeddedName = args[i]
			}
		case "--require-registry":
			requireRegistry = true
		}
	}

	if registryPath == "" {
		fmt.Fprintln(stderr, "error: --registry is required")
		exit(1)
		return
	}
	// Clean the registry path to prevent path-traversal via "../" segments.
	registryPath = filepath.Clean(registryPath)

	// Resolve the contract content and its block scope. An embedded contract
	// (engine-owned managed text) takes precedence and overrides marker/label so
	// it writes a DISTINCT block that never collides with minimalism-contract.
	cfg := propagator.Config{ContractPath: contractPath}
	rowLabelForMsg := "minimalism-contract"
	var contractContent string

	if embeddedName != "" {
		spec, ok := embeddedContract(embeddedName)
		if !ok {
			fmt.Fprintf(stderr, "error: unknown embedded contract %q\n", embeddedName)
			exit(1)
			return
		}
		contractContent = spec.content
		cfg.BeginMarker = spec.beginMarker
		cfg.EndMarker = spec.endMarker
		cfg.RowLabel = spec.rowLabel
		rowLabelForMsg = spec.rowLabel
		// Use the embedded contract's own path in the registry row unless the
		// caller explicitly overrode --contract-path.
		if !contractPathExplicit {
			cfg.ContractPath = spec.defaultPath
		}
	} else {
		if contractFilePath == "" {
			fmt.Fprintln(stderr, "error: --contract-file is required")
			exit(1)
			return
		}
		b, err := readFile(contractFilePath)
		if err != nil {
			fmt.Fprintf(stderr, "error: reading contract file: %v\n", err)
			exit(1)
			return
		}
		contractContent = string(b)
	}

	phases, err := propagator.ParseFrontmatter(contractContent)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		exit(1)
		return
	}

	registryContent, err := readFile(registryPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Registry absent. Default: clean no-op (project does not use the
			// overlay). Strict: fail loud so a misconfiguration where the
			// registry was EXPECTED is never an invisible no-op.
			if requireRegistry {
				fmt.Fprintf(stderr, "error: registry required but not found at %s (--require-registry)\n", registryPath)
				exit(1)
				return
			}
			fmt.Fprintf(stdout, "propagate: registry not found at %s — project does not use the overlay (no-op)\n", registryPath)
			return
		}
		fmt.Fprintf(stderr, "error: reading registry file: %v\n", err)
		exit(1)
		return
	}

	out, changed, err := propagator.Propagate(string(registryContent), cfg, phases)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		exit(1)
		return
	}

	if !changed {
		fmt.Fprintf(stdout, "registry: %s scope is already correct (no-op)\n", rowLabelForMsg)
		return
	}

	if err := writeFile(registryPath, []byte(out), 0644); err != nil {
		fmt.Fprintf(stderr, "error: writing registry: %v\n", err)
		exit(1)
		return
	}
	fmt.Fprintf(stdout, "registry: %s scoped row inserted/updated\n", rowLabelForMsg)
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
	var contractFilePath, contractPath, embeddedName string
	contractPath = "skills/_shared/minimalism-contract.md" // default (minimalism)
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
		// contract's own default path (mirrors runPropagateCore behaviour).
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
// It uses real OS deps and chooses the process exit code:
//
//	0 — every check passed (healthy).
//	1 — at least one hard check FAILED.
//	2 — no hard failure, but at least one check is DEGRADED (e.g. the registry
//	    exists but its scoped block is missing). Distinct from 1 so callers can
//	    tell "broken" from "present-but-needs-attention".
func runStatus(_ []string) {
	allOK, degraded := statusCore(os.Stdout, defaultStatusDeps())
	switch {
	case !allOK:
		os.Exit(1)
	case degraded:
		os.Exit(2)
	}
}

// checkResult holds the result of a single status check.
//
// Three tiers: ok=true & degraded=false → OK; ok=false → FAIL (hard);
// ok=true & degraded=true → WARN (degraded but not a hard failure).
type checkResult struct {
	label    string
	ok       bool
	degraded bool
	note     string
}

// binaryIdentity is the substring used to identify our hook entries in
// settings.json — same logic as Merger.hookCommand substring match.
const binaryIdentity = "gentle-ai-overlay"

// statusCore runs all checks and writes the report to stdout.
// Returns (allOK, degraded): allOK is true only when no check FAILED; degraded
// is true when no check FAILED but at least one is in the WARN/degraded tier.
func statusCore(stdout io.Writer, deps statusDeps) (allOK bool, degraded bool) {
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
	allOK = true
	for _, c := range checks {
		status := "OK  "
		switch {
		case !c.ok:
			status = "FAIL"
			allOK = false
		case c.degraded:
			status = "WARN"
			degraded = true
		}
		if c.note != "" {
			fmt.Fprintf(stdout, "[%s] %s — %s\n", status, c.label, c.note)
		} else {
			fmt.Fprintf(stdout, "[%s] %s\n", status, c.label)
		}
	}
	return allOK, degraded
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

// checkRegistry reports on the .atl/skill-registry.md in cwd, distinguishing
// three outcomes per the fix principles (REGISTRY-AUTHORITATIVE + FAIL-LOUD):
//
//   - ABSENT → OK, quiet. A project that does not use the overlay is not a
//     problem; this is the only branch that stays silently OK.
//   - UNREADABLE (real IO error, not IsNotExist) → FAIL. A genuine OS error must
//     surface, never be downgraded to an OK note.
//   - PRESENT BUT EMPTY / whitespace-only → FAIL. An emptied registry is the
//     incident's misread state: it must be loud, never treated as "zero skills".
//   - PRESENT, scoped block MISSING → WARN (degraded). Actionable, not fatal.
//   - PRESENT, scoped block FOUND → OK.
func checkRegistry(registryPath string, readFile readFileFn) checkResult {
	label := "registry: " + registryPath
	data, err := readFile(registryPath)
	if err != nil {
		if os.IsNotExist(err) {
			return checkResult{label: label, ok: true, note: "not present (project may not use the overlay)"}
		}
		// A real OS error is a genuine failure — fail loud, do not downgrade.
		return checkResult{label: label, ok: false, note: "cannot read: " + err.Error()}
	}
	content := string(data)
	if strings.TrimSpace(content) == "" {
		return checkResult{
			label: label,
			ok:    false,
			note:  "present but EMPTY — run skill-registry refresh; do NOT conclude skills are absent (an empty registry is inconclusive, not zero)",
		}
	}
	hasMinimalism := containsSubstring(content, propagator.BeginMarker)
	hasSafety := containsSubstring(content, propagator.DiscoverySafetyBeginMarker)
	switch {
	case hasMinimalism && hasSafety:
		return checkResult{label: label, ok: true, note: "scoped block present"}
	case !hasMinimalism && !hasSafety:
		return checkResult{label: label, ok: true, degraded: true, note: "present but both scoped blocks missing (run 'overlay install-hooks' or propagate)"}
	case !hasMinimalism:
		return checkResult{label: label, ok: true, degraded: true, note: "present but minimalism-contract-scope block missing (run 'overlay install-hooks' or propagate)"}
	default: // !hasSafety
		return checkResult{label: label, ok: true, degraded: true, note: "present but skill-discovery-safety-scope block missing (run 'overlay install-hooks' or propagate)"}
	}
}
