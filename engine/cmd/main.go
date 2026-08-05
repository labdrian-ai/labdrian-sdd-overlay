// Command engine provides subcommands for the deterministic-scoping engine:
//
//	engine propagate --registry <path> --contract-file <path> [--contract-path <str>]
//	engine gate-task --contract-file <path> [--contract-path <str>]
//	engine merge-settings --settings <path> --hook-command <binary-path>
//	engine uninstall-hooks --settings <path> --hook-command <binary-path>
//	engine status
//	engine skills <verb>  (verbs: list, status, validate, install, add, remove, sync-manifest)
//
// propagate: ensures the scoped minimalism-contract BEGIN/END marker block is
// present in a target .atl/skill-registry.md. Fails LOUD on bad input.
// Concurrency-safe: serializes via an exclusive flock on <registry>.lock,
// writes the registry atomically (temp file + rename), and refuses (exit 1)
// to propagate over a registry that exists but is empty/whitespace-only.
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
//
// skills: registry management commands for skills.registry.yaml and overlay.manifest.
// list: print sorted registry entries. status: print count summary.
// validate: cross-check registry vs overlay.manifest, and skills/ on disk vs
// overlay.manifest via the required --source-root flag; exit 1 on divergence.
// install: copy project-scoped skills into <cwd>/.claude/skills.
// add: register a skill (custom or vendored). remove: unregister from registry + manifest.
// sync-manifest: regenerate */SKILL.md rows from skills.registry.yaml.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/assets"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gadu"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gate"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/prespec"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/propagator"
	runtimepkg "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
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
	case "anti-generic-design":
		return embeddedContractSpec{
			content:     assets.AntiGenericDesign,
			beginMarker: propagator.AntiGenericDesignBeginMarker,
			endMarker:   propagator.AntiGenericDesignEndMarker,
			rowLabel:    "anti-generic-design",
			// defaultPath is the registry-row Path cell / bare injected line when
			// the caller does not override --contract-path. It is where the
			// overlay deploys the standalone copy of this contract.
			defaultPath: "skills/_shared/anti-generic-design.md",
		}, true
	case "review-projection-contract":
		return embeddedContractSpec{
			content:     assets.ReviewProjectionContract,
			beginMarker: propagator.ReviewProjectionBeginMarker,
			endMarker:   propagator.ReviewProjectionEndMarker,
			rowLabel:    "review-projection-contract",
			// defaultPath is the registry-row Path cell / bare injected line when
			// the caller does not override --contract-path. It is where the
			// overlay deploys the standalone copy of this contract.
			defaultPath: "skills/_shared/review-projection-contract.md",
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
	case "runtime":
		runRuntime(os.Args[2:])
	case "gadu-generate":
		runGaduGenerate(os.Args[2:])
	case "skills":
		runSkills(os.Args[2:])
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
	fmt.Fprintln(os.Stderr, "  engine runtime <action> [--target claude|opencode|codex|all] [--config-root <path>]")
	fmt.Fprintln(os.Stderr, "    action: status | install | update | uninstall")
	fmt.Fprintln(os.Stderr, "    --target: opencode (default), claude, codex, or all")
	fmt.Fprintln(os.Stderr, "  OVERLAY_DIR=<repo-root> gentle-ai-overlay gadu-generate [--check]")
	fmt.Fprintln(os.Stderr, "  engine skills <verb>   (verbs: list, status, validate, install, add, remove, sync-manifest)")
	fmt.Fprintln(os.Stderr, "    list          [--registry <path>]                                                      print sorted registry entries")
	fmt.Fprintln(os.Stderr, "    status        [--registry <path>]                                                      print count summary (total/core/custom)")
	fmt.Fprintln(os.Stderr, "    validate      [--registry <path>] [--manifest <path>] --source-root <path>              cross-check registry vs manifest and skills/ on disk; exit 1 on divergence")
	fmt.Fprintln(os.Stderr, "    install       [--registry <path>] [--source-root <path>] [--project-id <id>]           copy project-scoped skills into <cwd>/.claude/skills/")
	fmt.Fprintln(os.Stderr, "    add           <id> [--registry <path>] [--manifest <path>] [--source-root <path>] [--repo <url>] [--ref <sha>]  register a skill")
	fmt.Fprintln(os.Stderr, "    remove        <id> [--registry <path>] [--manifest <path>]                             unregister a skill from registry and manifest")
	fmt.Fprintln(os.Stderr, "    sync-manifest [--registry <path>] [--manifest <path>]                                  regenerate */SKILL.md rows from registry")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Embedded contracts: skill-discovery-safety, anti-generic-design")
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
// Without --check: calls gadu.Generate(repoRoot) to write all artifacts.
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
	fmt.Fprintln(os.Stdout, "gadu-generate: agents/GADU.md, opencode/agents/GADU.md, and skills/gadu-operator/SKILL.md written")
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

// ---------------------------------------------------------------------------
// runtime subcommand
// ---------------------------------------------------------------------------

// runRuntime implements the 'runtime <action>' subcommand.
// Supported actions: status, install, update, uninstall.
func runRuntime(args []string) {
	runRuntimeCore(args, os.Stdout, os.Stderr, os.Exit)
}

// runRuntimeCore is the testable core for the 'runtime' subcommand.
func runRuntimeCore(args []string, stdout io.Writer, stderr io.Writer, exit func(int)) {
	action, target, configRoot, err := parseRuntimeArgs(args)
	if err != nil {
		fmt.Fprintln(stderr, err)
		usage()
		exit(1)
		return
	}
	targets := runtimepkg.ExpandTarget(target)

	failed := false
	allTargets := len(targets) > 1

	for _, current := range targets {
		targetRoot := configRoot
		if current == runtimepkg.TargetOpenCode && targetRoot == "" {
			targetRoot = runtimepkg.DefaultOpenCodeConfigRoot()
		}
		adapter := runtimeAdapterForTarget(current, targetRoot)
		result := runtimeLifecycleResult(adapter, action)
		fmt.Fprintln(stdout, result.String())
		actionFailed := false
		switch action {
		case "status":
			switch {
			case result.Status == runtimepkg.CapabilityUnsupported,
				result.Status == runtimepkg.CapabilityRestartRequired:
				actionFailed = true
			case result.Status == runtimepkg.CapabilityPartial && !(allTargets && current == runtimepkg.TargetCodex):
				actionFailed = true
			}
		default:
			if result.Status == runtimepkg.CapabilityUnsupported || result.Status == runtimepkg.CapabilityPartial {
				actionFailed = true
			}
		}
		if actionFailed {
			failed = true
		}
	}

	if failed {
		exit(1)
		return
	}
	exit(0)
}

func runtimeAdapterForTarget(target runtimepkg.Target, configRoot string) runtimepkg.Adapter {
	if target == runtimepkg.TargetOpenCode {
		return runtimepkg.NewOpenCodeAdapter(configRoot)
	}
	if target == runtimepkg.TargetClaude {
		return runtimepkg.NewClaudeAdapter(configRoot)
	}
	if target == runtimepkg.TargetCodex {
		return runtimepkg.NewCodexAdapter(configRoot)
	}
	return runtimepkg.NewFoundationAdapter(target)
}

// parseRuntimeArgs parses minimal runtime subcommand arguments.
func parseRuntimeArgs(args []string) (action string, target runtimepkg.Target, configRoot string, err error) {
	if len(args) == 0 {
		return "", "", "", fmt.Errorf("error: runtime requires an action")
	}
	action = args[0]
	if strings.HasPrefix(action, "-") {
		return "", "", "", fmt.Errorf("error: runtime requires an action: status | install | update | uninstall")
	}

	target = runtimepkg.TargetOpenCode
	for i := 1; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--target":
			i++
			if i >= len(args) {
				return "", "", "", fmt.Errorf("error: --target requires a value")
			}
			target, err = runtimepkg.ParseTarget(args[i])
			if err != nil {
				return "", "", "", err
			}
		case "--config-root":
			i++
			if i >= len(args) {
				return "", "", "", fmt.Errorf("error: --config-root requires a value")
			}
			configRoot = args[i]
		default:
			if strings.HasPrefix(a, "--") {
				return "", "", "", fmt.Errorf("error: unknown flag %q", a)
			}
			return "", "", "", fmt.Errorf("error: unexpected runtime argument %q", a)
		}
	}

	if action != "status" && action != "install" && action != "update" && action != "uninstall" {
		return "", "", "", fmt.Errorf("error: unknown runtime action %q", action)
	}

	return action, target, configRoot, nil
}

func runtimeLifecycleResult(adapter runtimepkg.Adapter, action string) runtimepkg.LifecycleResult {
	switch action {
	case "status":
		return adapter.Status()
	case "install":
		return adapter.Install()
	case "update":
		return adapter.Update()
	case "uninstall":
		return adapter.Uninstall()
	default:
		return runtimepkg.NewLifecycleResult(adapter.Target(), "status", runtimepkg.CapabilityUnsupported, "unknown runtime action", nil)
	}
}

// runSkills implements the 'skills <verb>' subcommand.
// Requires exactly one verb argument; fails LOUD on missing or unknown verb (ADR-4).
func runSkills(args []string) {
	runSkillsCore(verbFromArgs(args), args, os.Stdout, os.Stderr, os.Exit)
}

// runSkillsCore is the testable core of the skills subcommand.
func runSkillsCore(verb string, args []string, stdout, stderr io.Writer, exit func(int)) {
	if verb == "" {
		fmt.Fprintln(stderr, "error: skills requires a verb: list, status, validate, install, add, remove, sync-manifest")
		exit(1)
		return
	}
	skills.SkillsCore(verb, args, os.ReadFile, stdout, stderr, exit)
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

// atomicWriteFile writes data to a temp file in the same directory as path and
// renames it into place. Unlike os.WriteFile (O_TRUNC), a concurrent reader can
// never observe a partially-written or empty registry: rename(2) is atomic on
// POSIX filesystems, so readers see either the old content or the new content.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	// Best-effort cleanup on any failure path; no-op after a successful rename.
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, perm); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// acquireRegistryLock takes an exclusive advisory flock(2) on lockPath (a
// sidecar file next to the registry), blocking until it is granted. It
// serializes the read-modify-write cycle across concurrent propagate processes
// (both contract hooks fire on every UserPromptSubmit). Returns a release func.
//
// Unix-only by design: this tool runs on Linux and the lock is dependency-free.
// The kernel releases the lock on process exit, so an os.Exit inside the core
// (which skips defers) can never leave the lock held.
func acquireRegistryLock(lockPath string) (release func(), err error) {
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}

// registryPathFromArgs extracts the cleaned --registry value, empty if absent.
// Used by runPropagate to know which sidecar lock to take before the core runs;
// the core re-parses args itself and stays lock-free (hermetic for unit tests).
func registryPathFromArgs(args []string) string {
	// Keep the LAST occurrence to match runPropagateCore's parser, so the
	// sidecar lock always guards the same file the core reads and writes.
	path := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "--registry" && i+1 < len(args) {
			path = filepath.Clean(args[i+1])
		}
	}
	return path
}

// runPropagate implements the 'propagate' subcommand.
// Fails LOUD on any error (exits 1).
//
// Concurrency safety (two layers, plus the empty-registry guard in the core):
//  1. an exclusive flock on <registry>.lock serializes the read-modify-write
//     against the sibling propagate process spawned by the other contract hook;
//  2. the registry write itself is atomic (temp file + rename), so even a
//     reader outside the lock can never observe a truncated/empty registry.
func runPropagate(args []string) {
	if registryPath := registryPathFromArgs(args); registryPath != "" {
		release, err := acquireRegistryLock(registryPath + ".lock")
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: acquiring registry lock: %v\n", err)
			os.Exit(1)
		}
		defer release()
	}
	runPropagateVerified(args, os.Stdout, os.Stderr, os.ReadFile, atomicWriteFile, os.Exit)
}

// emptyRegistryErrMsg is the exact stderr line runPropagateCore prints when a
// registry exists but is empty/whitespace-only (see the fail-loud guard
// inside runPropagateCore below, which references this same const directly).
// classifyReadRace matches this sentinel via FULL trimmed-equality (core
// prints exactly this one line then exits) to recognize a torn/empty read as
// retryable without changing core's signature. Because core references this
// const directly rather than duplicating the literal, independent wording
// drift between the two sites is structurally impossible; the guard test
// TestEmptyRegistryMsgPinned instead protects against a future edit
// reintroducing an inline literal in either place.
const emptyRegistryErrMsg = "error: registry exists but is empty — refusing to propagate over it"

// registryAbsentNoopMarker is a stable, path-independent substring of the
// stdout message runPropagateCore emits when the registry is absent and
// --require-registry was not supplied (see runPropagateCore below).
// classifyReadRace matches on this substring to recognize a no-write, no-exit
// outcome as the "registry absent" no-op. Pinned by TestAbsentNoopMarkerPinned.
const registryAbsentNoopMarker = "project does not use the overlay (no-op)"

// maxPropagateWriteAttempts bounds the read-decide-write-verify retry loop in
// runPropagateVerified. It exists to defend against a foreign, uncoordinated
// writer (e.g. "gentle-ai skill-registry refresh", a completely separate
// binary with no knowledge of this project's marker-block convention and no
// participation in <registry>.lock) clobbering the registry between our
// atomic write and our verification read. A handful of attempts is enough to
// win a race against a single competing write; it is intentionally bounded
// (not infinite) so a registry that is being rewritten in a tight loop by
// something else fails LOUD instead of spinning forever. It also bounds the
// read-side race retries (torn/empty read, transient-absent read) classified
// by classifyReadRace below — the same foreign writer that can clobber a
// verified write can just as easily tear or momentarily remove a read.
const maxPropagateWriteAttempts = 3

// readRaceKind classifies one runPropagateCore attempt's outcome as either a
// retryable read-side race or raceNone (a genuine, non-retryable outcome —
// covers hard failures, an already-correct no-op, and a completed write
// awaiting write-side verification, all of which the existing loop branches
// already handle correctly).
type readRaceKind int

const (
	// raceNone is not a read race: forward/handle via the existing branches.
	raceNone readRaceKind = iota
	// raceEmptyRegistry is a torn/empty read (core's fail-loud guard fired).
	raceEmptyRegistry
	// raceAbsentRegistry is a transient absent-registry read (core's no-op
	// branch fired because the registry did not exist on this attempt).
	raceAbsentRegistry
)

// classifyReadRace inspects one runPropagateCore attempt's buffered outcome
// and classifies it per design.md's "Architecture Decisions": a write
// (wrote=true) is NEVER a read race — it belongs to the existing
// write-then-verify branch, so it always classifies as raceNone regardless
// of coreExit/stdout/stderr. Among no-write outcomes: an exit(1) whose
// stderr is a FULL trimmed-equality match against emptyRegistryErrMsg (core
// prints exactly one line then exits) is raceEmptyRegistry; any OTHER
// exit(1) — a genuine hard failure — is raceNone so it forwards immediately
// with no retry. A no-exit (coreExit == -1) outcome whose stdout contains
// registryAbsentNoopMarker is raceAbsentRegistry; otherwise it is the
// existing already-correct no-op, raceNone.
func classifyReadRace(coreExit int, wrote bool, stdout, stderr string) readRaceKind {
	if wrote {
		return raceNone
	}
	if coreExit != -1 {
		// Literal 1, not a named constant: the exact trimmed-equality match
		// against emptyRegistryErrMsg below already fully disambiguates this
		// branch from any other exit(1) path, so a named exit-code constant
		// would only imply an enforced invariant across core's other exit(1)
		// call sites that does not exist (they remain plain literals).
		if coreExit == 1 && strings.TrimSpace(stderr) == emptyRegistryErrMsg {
			return raceEmptyRegistry
		}
		return raceNone
	}
	if strings.Contains(stdout, registryAbsentNoopMarker) {
		return raceAbsentRegistry
	}
	return raceNone
}

// reportInconsistentRegistryState emits the fail-loud diagnostic shared by the
// two mixed-evidence exhaustion outcomes in runPropagateVerified below: a
// mixed read-race sequence exhausting under raceEmptyRegistry (an earlier
// attempt read absent or wrote successfully), and the raceAbsentRegistry
// tie-break where an earlier attempt proved the registry exists. Factored
// into one place so the two call sites can never drift apart on wording.
//
// Deliberately cause-agnostic: earlier wording asserted "a concurrent
// external writer may be tearing reads", but a mixed/inconsistent sequence
// does not always mean a foreign writer race — e.g. a project could
// genuinely stop using the overlay mid-invocation (an uninstall step
// deleting the registry between retries). State the observation, not a
// specific cause.
func reportInconsistentRegistryState(stderr io.Writer, registryPath string, attempts int) {
	fmt.Fprintf(stderr,
		"error: the registry at %s was in an inconsistent state across %d attempts — refusing to propagate\n",
		registryPath, attempts)
}

// runPropagateVerified wraps runPropagateCore with a bounded write-then-verify
// retry loop. runPropagateCore trusts a successful writeFile call
// unconditionally: once writeFile returns nil it reports success and exits,
// with no confirmation that the write actually persisted. That trust is safe
// against OTHER "engine propagate" invocations (serialized via the
// <registry>.lock flock acquired in runPropagate below), but NOT against any
// other process that writes the same file outside that lock — such as the
// separate "gentle-ai skill-registry refresh" binary, which regenerates
// .atl/skill-registry.md wholesale and has no reason to know about, respect,
// or even see our lock file.
//
// After each write, runPropagateVerified re-reads the registry and confirms
// the bytes on disk are byte-identical to what was just written. If not — a
// foreign writer won the race — it retries the ENTIRE read-decide-write
// cycle (not just the write) from scratch, because the correct output may
// itself have changed (e.g. the foreign tool may have removed rows this
// propagator needs to coexist with). This directly implements re-verifying
// the actual current on-disk content immediately before trusting any
// decision, rather than trusting a single point-in-time read.
//
// An "already correct" no-op (registry unchanged) never enters the retry
// loop: no write means nothing to verify, so it costs exactly the one read
// runPropagateCore already performs. An absent-registry no-op is different:
// classifyReadRace treats it as a potential mid-refresh race window and does
// retry it (up to maxPropagateWriteAttempts reads) before concluding the
// project genuinely does not use the overlay — see raceAbsentRegistry below.
func runPropagateVerified(
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	readFile readFileFn,
	writeFile writeFileFn,
	exit func(int),
) {
	registryPath := registryPathFromArgs(args)

	// sawEmptyRegistry/sawAbsentRegistry/sawWrote track which evidence kinds
	// were ACTUALLY observed across every attempt in this run (not just the
	// current/last one), so the exhaustion messages below can accurately
	// describe a mixed sequence (e.g. attempt 1 absent, attempts 2-3 empty)
	// instead of always claiming the terminal attempt's kind held throughout.
	//
	// sawWrote covers a case classifyReadRace deliberately makes invisible to
	// read-race classification: any attempt where wrote==true (core
	// successfully wrote, even if a later verification found the bytes
	// clobbered) is STRONGER proof the registry exists than either read-race
	// kind — a write cannot happen against a registry that isn't there. Without
	// tracking it separately, a sequence like "attempt 1 writes valid content
	// but is clobbered before verification, attempts 2-3 read absent" would
	// exhaust with none of the read-race evidence flags set and silently take
	// the exit-0 no-op path, even though attempt 1 proved this project DOES use
	// the overlay.
	var sawEmptyRegistry, sawAbsentRegistry, sawWrote bool

	for attempt := 1; attempt <= maxPropagateWriteAttempts; attempt++ {
		var outBuf, errBuf bytes.Buffer
		var written []byte
		wrote := false
		coreExit := -1

		verifyingWrite := func(path string, data []byte, perm os.FileMode) error {
			if err := writeFile(path, data, perm); err != nil {
				return err
			}
			written = append([]byte(nil), data...)
			wrote = true
			return nil
		}
		coreExitFn := func(code int) { coreExit = code }

		runPropagateCore(args, &outBuf, &errBuf, readFile, verifyingWrite, coreExitFn)

		if wrote {
			// Record this evidence BEFORE classification: classifyReadRace
			// always returns raceNone for wrote==true (a write belongs to the
			// existing write-then-verify branch, never a read race), so this
			// attempt would otherwise be invisible to the exhaustion
			// tie-breaks below even though it is direct proof the registry
			// exists.
			sawWrote = true
		}

		// Classify this attempt's outcome BEFORE the generic hard-failure
		// forward below, so a torn/empty read (which core reports as
		// exit(1), same as a genuine hard failure) is intercepted as
		// retryable instead of being forwarded as a terminal error. Per
		// design.md's Data Flow: order matters — raceEmptyRegistry is
		// checked first, then the unchanged coreExit!=-1 forward, then
		// raceAbsentRegistry (checked before the unchanged !wrote forward).
		switch classifyReadRace(coreExit, wrote, outBuf.String(), errBuf.String()) {
		case raceNone:
			// Not a read race: fall through to the existing hard-failure/
			// no-op/write-verify branches below, unchanged.
		case raceEmptyRegistry:
			sawEmptyRegistry = true
			if attempt < maxPropagateWriteAttempts {
				// Discard this attempt's buffers (they hold core's exit(1)
				// diagnostic, not a real terminal failure) and retry the
				// whole read-decide-write cycle — a torn/empty read is the
				// signature of a concurrent foreign rewrite in progress.
				continue
			}
			if sawAbsentRegistry || sawWrote {
				// A mixed sequence: at least one earlier attempt classified
				// as raceAbsentRegistry, or wrote successfully (proof the
				// registry existed) before it was clobbered — either way
				// "empty on every attempt" would be inaccurate.
				reportInconsistentRegistryState(stderr, registryPath, maxPropagateWriteAttempts)
			} else {
				// Every classified attempt in this run was genuinely
				// raceEmptyRegistry, so the "read empty on every attempt"
				// framing is accurate — but cause-agnostic per
				// reportInconsistentRegistryState's rationale above: a
				// persistently empty/whitespace-only registry does not prove
				// an active concurrent writer race (e.g. a one-time crash or
				// an aborted write left it empty, with no live race at all).
				// State the observation, not a specific cause.
				fmt.Fprintf(stderr,
					"error: the registry at %s read empty on all %d attempts — "+
						"refusing to propagate\n",
					registryPath, maxPropagateWriteAttempts)
			}
			exit(1)
			return
		case raceAbsentRegistry:
			sawAbsentRegistry = true
			if attempt < maxPropagateWriteAttempts {
				// Discard this attempt's buffers and retry — a transient
				// absent read may be a foreign writer's unlink-then-recreate
				// window rather than a genuine "project does not use the
				// overlay" state.
				continue
			}
			if sawEmptyRegistry || sawWrote {
				// An earlier attempt in THIS run observed the registry file
				// existing (torn/empty, or a successful write that was later
				// clobbered) before later attempts classified as absent. That
				// is direct proof this project DOES use the overlay, so
				// exhausting the budget on the absent branch must not
				// silently no-op — prefer the fail-loud outcome over
				// discarding that evidence.
				reportInconsistentRegistryState(stderr, registryPath, maxPropagateWriteAttempts)
				exit(1)
				return
			}
			// Persistently absent after exhausting attempts, with no earlier
			// proof the registry exists: forward the existing exit-0 no-op
			// unchanged — this really is a project that does not use the
			// overlay.
			io.Copy(stdout, &outBuf) //nolint:errcheck
			io.Copy(stderr, &errBuf) //nolint:errcheck
			return
		}

		if coreExit != -1 {
			// Hard failure (bad args, unreadable contract/registry, broken
			// frontmatter, or a real write I/O error) has nothing to do with
			// an external write race — forward it as-is, no retry.
			io.Copy(stdout, &outBuf) //nolint:errcheck
			io.Copy(stderr, &errBuf) //nolint:errcheck
			exit(coreExit)
			return
		}

		if !wrote {
			// No-op (already correct) or registry absent — nothing was
			// written, so there is nothing a foreign writer could have
			// clobbered. Forward the result unchanged.
			io.Copy(stdout, &outBuf) //nolint:errcheck
			io.Copy(stderr, &errBuf) //nolint:errcheck
			return
		}

		// Re-verify: read the registry back and confirm our write actually
		// stuck, byte-for-byte. A foreign writer landing between our atomic
		// rename and this read is exactly the race this loop defends against.
		// A non-nil readErr here is a DIFFERENT failure mode (permission
		// error, transient I/O error) than a byte mismatch, and must not be
		// reported to the operator as if a foreign writer were clobbering
		// the file — the two are distinguished below.
		onDisk, readErr := readFile(registryPath)
		if readErr == nil && bytes.Equal(onDisk, written) {
			io.Copy(stdout, &outBuf) //nolint:errcheck
			io.Copy(stderr, &errBuf) //nolint:errcheck
			return
		}

		if attempt == maxPropagateWriteAttempts {
			if readErr != nil {
				fmt.Fprintf(stderr,
					"error: registry write to %s could not be verified after %d attempts — "+
						"the verification read itself failed: %v\n",
					registryPath, maxPropagateWriteAttempts, readErr)
			} else {
				// Intentionally cause-specific, unlike the read-race
				// exhaustion messages above (reportInconsistentRegistryState
				// and the uniform-empty branch), which are deliberately
				// cause-agnostic because they have no direct evidence of an
				// active writer. Here the certainty level is different: the
				// bytes just written and the bytes on disk provably do not
				// match, which IS strong, direct evidence of a foreign writer
				// clobbering the file between our atomic write and this
				// verification read. Naming that cause is warranted, not an
				// oversight of the wording asymmetry.
				fmt.Fprintf(stderr,
					"error: registry write to %s did not persist after %d attempts — "+
						"a concurrent external writer (unrelated to engine propagate) "+
						"appears to be clobbering it\n",
					registryPath, maxPropagateWriteAttempts)
			}
			exit(1)
			return
		}
		// Otherwise: loop and retry the whole read-decide-write-verify cycle
		// against whatever is on disk now.
	}
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
			//
			// NOTE (design.md "Decision: Transient absent registry"): this
			// --require-registry branch is a genuine hard failure (coreExit
			// != -1, stderr does not match emptyRegistryErrMsg) and therefore
			// gets ZERO of runPropagateVerified's read-race retry protection
			// — classifyReadRace classifies it as raceNone and it forwards
			// immediately on the first attempt, by design (explicit opt-in
			// callers should fail fast, not spend the retry budget on a
			// requirement they asked to enforce strictly). A future caller
			// adopting --require-registry must not assume it is shielded from
			// the exact torn/absent-read race this change closes for the
			// default (non-strict) path.
			if requireRegistry {
				fmt.Fprintf(stderr, "error: registry required but not found at %s (--require-registry)\n", registryPath)
				exit(1)
				return
			}
			fmt.Fprintf(stdout, "propagate: registry not found at %s — %s\n", registryPath, registryAbsentNoopMarker)
			return
		}
		fmt.Fprintf(stderr, "error: reading registry file: %v\n", err)
		exit(1)
		return
	}

	// Fail-loud guard: a registry that EXISTS but is empty/whitespace-only is
	// never a valid input — it is the signature of a torn concurrent read (the
	// incident state) or an otherwise corrupted file. Propagating over it would
	// replace the whole registry with a lone marker block, so refuse instead.
	if strings.TrimSpace(string(registryContent)) == "" {
		fmt.Fprintln(stderr, emptyRegistryErrMsg)
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
	var contractFilePath, contractPath, embeddedName, workContextJSON, workContextFile string
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
	workContext, workContextErr := loadWorkContext(workContextJSON, workContextFile, readFile)
	if workContextErr != nil {
		fmt.Fprintf(stderr, "gate-task: warning: work context ignored: %v (passing through for context-aware contracts)\n", workContextErr)
	} else {
		cfg.WorkContext = workContext
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
	hasDesign := containsSubstring(content, propagator.AntiGenericDesignBeginMarker)
	hasProjection := containsSubstring(content, propagator.ReviewProjectionBeginMarker)

	if hasMinimalism && hasSafety && hasDesign && hasProjection {
		return checkResult{label: label, ok: true, note: "scoped block present"}
	}

	var missing []string
	if !hasMinimalism {
		missing = append(missing, "minimalism-contract-scope")
	}
	if !hasSafety {
		missing = append(missing, "skill-discovery-safety-scope")
	}
	if !hasDesign {
		missing = append(missing, "anti-generic-design-scope")
	}
	if !hasProjection {
		missing = append(missing, "review-projection-contract-scope")
	}
	note := fmt.Sprintf(
		"present but scoped block(s) missing: %s (run 'overlay install-hooks' or propagate)",
		strings.Join(missing, ", "),
	)
	return checkResult{label: label, ok: true, degraded: true, note: note}
}
