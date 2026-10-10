// Command engine provides subcommands for the deterministic-scoping engine:
//
//	engine propagate --registry <path> --contract-file <path> [--contract-path <str>]
//	engine gate-task --contract-file <path> [--contract-path <str>]
//	engine merge-settings --settings <path> --hook-command <binary-path>
//	engine uninstall-hooks --settings <path> --hook-command <binary-path>
//	engine status
//	engine skills <verb>  (verbs: list, status, validate, install, adopt, add, remove, sync-manifest, lint, approve, project-register, project-revise, project-status, project-retire)
//
// propagate: ensures the scoped minimalism-contract BEGIN/END marker block is
// present in a target .atl/skill-registry.md. Fails LOUD on bad input.
// Concurrency-safe: serializes via a bounded exclusive flock on <registry>.lock,
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
// engine-owned managed contract (e.g. anti-generic-design) from the binary
// instead of an external file; propagate then writes that contract's DISTINCT
// marker block. propagate also accepts --require-registry to turn an absent
// registry into a fail-loud error instead of a silent no-op.
//
// skills: registry management commands for skills.registry.yaml and overlay.manifest.
// list: print sorted registry entries. status: print count summary.
// validate: cross-check registry vs overlay.manifest, and skills/ on disk vs
// overlay.manifest via the required --source-root flag, and that every global
// skill has a valid approval record (grandfathered baseline aside); exit 1 on
// divergence.
// install: copy project-scoped skills into <cwd>/.claude/skills.
// add: register a skill (custom or vendored); refused unless a valid approval
// record covers the exact SKILL.md bytes. remove: unregister from registry + manifest.
// sync-manifest: regenerate */SKILL.md rows from skills.registry.yaml.
// lint: lint a SKILL.md file against the authoritative rule table, or print
// that table with --rules; exit 1 on any hard error.
// approve: record a human approval of skills/<id>/SKILL.md, bound to the digest
// of its exact bytes, next to the skill (--id, --approver, --source-root). The
// engine cannot prove a human ran it; the record only proves the bytes match.
// guard-hook: the internal Claude Code PreToolUse hook that denies the agent
// running approve or writing the approval record (see cmd/skills_guard.go); a
// speed bump, not a security boundary, installed by install-hooks.
// project-register/revise/status: manage project-tier procedural skills and
// report ownership from the project lock.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/contract"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gate"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/hookwire"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewreceipt"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings/settingsfile"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/synctrigger"
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
	case "runtime":
		runRuntime(os.Args[2:])
	case "gadu-generate":
		runGaduGenerate(os.Args[2:])
	case "pipkg":
		runPipkg(os.Args[2:])
	case "skills":
		runSkills(os.Args[2:])
	case "sync-trigger":
		runSyncTrigger(os.Args[2:])
	case "review-receipt":
		runReviewReceipt(os.Args[2:])
	case "shaper":
		runShaper(os.Args[2:])
	case "roles":
		runRoles(os.Args[2:])
	case "memory":
		runMemory(os.Args[2:])
	case "workflow":
		runWorkflow(os.Args[2:])
	case "projection":
		runProjection(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "error: unknown subcommand %q\n", os.Args[1])
		usage()
		os.Exit(1)
	}
}

// defaultStateDirName is the overlay state root under $HOME, used when
// --state-dir is not given.
const defaultStateDirName = ".labdrian-overlay"

// runSyncTrigger implements the 'sync-trigger --event <e> --cwd <dir>
// [--state-dir <dir>] [--child]' subcommand.
func runSyncTrigger(args []string) {
	runSyncTriggerCore(args, os.Exit)
}

// runSyncTriggerCore is the testable core of the sync-trigger subcommand.
// It never rejects its own argv: any invalid or missing flag is left to
// synctrigger.Run to classify as error:usage, so this always exits 0
// (R-003) -- the same "core takes an injected exit" shape as
// runRuntimeCore above, but with a fixed exit(0) rather than a computed
// one, because sync-trigger has no failure that is allowed to propagate.
func runSyncTriggerCore(args []string, exit func(int)) {
	o, isChild := parseSyncTriggerArgs(args)
	if o.StateDir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			o.StateDir = filepath.Join(home, defaultStateDirName)
		}
	}

	if isChild {
		synctrigger.RunChild(o)
		exit(0)
		return
	}
	o.ChildArgv = syncTriggerChildArgv
	synctrigger.Run(o)
	exit(0)
}

// syncTriggerChildArgv is the command line of the detached child the parent starts: this
// command again, with the verb's own flags and --child. It is given to synctrigger.Options, which
// starts the child with it and knows nothing of what it says; parseSyncTriggerArgs is what reads
// it back.
func syncTriggerChildArgv(event, cwd, stateDir string) []string {
	return []string{"sync-trigger", "--event", event, "--cwd", cwd, "--state-dir", stateDir, "--child"}
}

// parseSyncTriggerArgs extracts sync-trigger flags into synctrigger.Options
// and reports whether "--child" was present. It deliberately does not
// validate values (missing/unknown flags simply leave fields empty) --
// synctrigger.Run and RunChild own that validation and both always return
// 0, so a parse error here would only duplicate a check that already
// cannot fail the caller.
func parseSyncTriggerArgs(args []string) (synctrigger.Options, bool) {
	var o synctrigger.Options
	child := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--event":
			i++
			if i < len(args) {
				o.Event = args[i]
			}
		case "--cwd":
			i++
			if i < len(args) {
				o.Cwd = args[i]
			}
		case "--state-dir":
			i++
			if i < len(args) {
				o.StateDir = args[i]
			}
		case "--child":
			child = true
		}
	}
	return o, child
}

// runReviewReceipt implements the 'review-receipt <verb>' subcommand.
// Verbs: capture, hook.
func runReviewReceipt(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "error: review-receipt requires a verb: capture, hook")
		os.Exit(1)
	}
	switch args[0] {
	case "capture":
		runReviewReceiptCapture(args[1:])
	case "hook":
		runReviewReceiptHook(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "error: review-receipt: unknown verb %q (expected capture or hook)\n", args[0])
		os.Exit(1)
	}
}

// parseReviewReceiptArgs extracts --cwd and --change from args.
func parseReviewReceiptArgs(args []string) (cwd, change string) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--cwd":
			i++
			if i < len(args) {
				cwd = args[i]
			}
		case "--change":
			i++
			if i < len(args) {
				change = args[i]
			}
		}
	}
	return
}

// buildReviewReceiptService builds the review receipt capture both verbs run on. When it
// cannot be built it says why on stderr, in the words of the verb (prefix) and as a set-up
// failure, which tells it apart from a failure of the capture itself, and exits with code,
// the one the verb's contract gives a failure: 1 for the capture a person runs, 2 for the
// hook, where exit code 2 denies the acknowledgement. The exit is injected; a caller whose exit
// returns gets no service.
func buildReviewReceiptService(cwd string, stderr io.Writer, exit func(int), prefix string, code int) *reviewreceipt.Service {
	svc, err := newReviewReceiptService(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "%s: set up failed: %v\n", prefix, err)
		exit(code)
		return nil
	}
	return svc
}

// runReviewReceiptCapture implements 'review-receipt capture --cwd <repo>
// [--change <name>]'. With --change, captures directly into that change.
// Without it, auto-detects the single active change: zero active changes is
// a no-op (exit 0); more than one is a loud failure (exit 1) naming
// --change as the remedy.
func runReviewReceiptCapture(args []string) {
	cwd, change := parseReviewReceiptArgs(args)
	if cwd == "" {
		fmt.Fprintln(os.Stderr, "error: --cwd is required")
		os.Exit(1)
	}

	svc := buildReviewReceiptService(cwd, os.Stderr, os.Exit, "error: review-receipt capture", 1)

	if change == "" {
		detected, err := svc.DetectActiveChange()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if detected == "" {
			fmt.Fprintln(os.Stdout, "review-receipt capture: no active change; nothing to capture")
			return
		}
		change = detected
	}

	captured, err := svc.Capture(change)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: review-receipt capture: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stdout, "review-receipt capture: %d receipt(s) captured for %q\n", len(captured), change)
}

// runReviewReceiptHook implements 'review-receipt hook --cwd <repo>': the
// fail-closed PreToolUse Bash hook entry point. Reads the raw hook input
// JSON from stdin, has engine/hookwire read the command out of it, asks
// reviewreceipt.Service.CheckCommand, and exits with the status of its
// verdict, printing its reason (if any) to stderr.
func runReviewReceiptHook(args []string) {
	cwd, _ := parseReviewReceiptArgs(args)
	if cwd == "" {
		fmt.Fprintln(os.Stderr, "error: --cwd is required")
		os.Exit(hookwire.ExitBlock)
	}

	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		// Fail closed: an unreadable hook input is treated the same as any
		// other capture-resolution failure -- deny rather than silently
		// allow an acknowledgement this hook could not even inspect.
		fmt.Fprintf(os.Stderr, "review-receipt hook: read stdin: %v\n", err)
		os.Exit(hookwire.ExitBlock)
	}

	// Fail closed, as for an unreadable input: a hook that cannot be set up cannot guard
	// the acknowledgement it was started for, so its set-up failure exits 2 like a denial.
	svc := buildReviewReceiptService(cwd, os.Stderr, os.Exit, "review-receipt hook", hookwire.ExitBlock)

	// Malformed or empty input is treated the same as a command that is not an
	// acknowledgement -- pass through -- because a hook that cannot even see a command is
	// not looking at an acknowledge-approved invocation in the first place.
	command, err := hookwire.DecodeCommand(raw)
	if err != nil {
		os.Exit(hookwire.ExitAllow)
	}
	verdict := svc.CheckCommand(command)
	reply := hookwire.ExitReply{Block: verdict.Deny, Message: verdict.Reason}
	_, _ = os.Stderr.Write(reply.MessageLine())
	os.Exit(reply.Code())
}

// runSkills implements the 'skills <verb>' subcommand.
// Requires exactly one verb argument; fails LOUD on missing or unknown verb (ADR-4).
func runSkills(args []string) {
	runSkillsWithStdin(args, os.Stdin, os.Stdout, os.Stderr, os.Exit)
}

// runSkillsCore is the testable core of the skills subcommand: the entry with the ports of the
// program.
func runSkillsCore(verb string, args []string, stdout, stderr io.Writer, exit func(int)) {
	runSkillsCoreWith(newSkillsDeps(), verb, args, stdout, stderr, exit)
}

// runSkillsCoreWith is the entry of the skills subcommand over the ports it is given: it names the
// verb the person typed, finds it in the table and runs it.
func runSkillsCoreWith(deps skills.Deps, verb string, args []string, stdout, stderr io.Writer, exit func(int)) {
	if verb == "" {
		fmt.Fprintf(stderr, "error: skills requires a verb: %s\n", skillsVerbList)
		exit(1)
		return
	}
	run, ok := skillsCLIVerbs[verb]
	if !ok {
		fmt.Fprintf(stderr, "error: unknown skills verb %q (supported: %s)\n", verb, skillsVerbList)
		exit(1)
		return
	}
	run(deps, args, stdout, stderr, exit)
}

// wallClockUTC is the production clock handed to the skills core: the current
// time as an RFC 3339 UTC timestamp with whole seconds, the only shape an
// approval record accepts. engine/skills cannot read the time itself because
// its import allowlist excludes "time", so the wall clock is decided here.
func wallClockUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
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

// stdinSizeLimit caps how many bytes 'gate-task' reads from stdin to prevent a runaway
// producer from exhausting memory. It is the bound of the decoder of the Agent call
// (hookwire.MaxAgentCallBytes), far beyond any realistic hook input: the command reads exactly
// that many bytes, so an input over it reaches the decoder cut short and is let through.
const stdinSizeLimit = hookwire.MaxAgentCallBytes

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

	if err := (settingsfile.Installer{}).Install(settingsPath, hookCommand); err != nil {
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

	if err := (settingsfile.Installer{}).Uninstall(settingsPath, hookCommand); err != nil {
		fmt.Fprintf(os.Stderr, "error: uninstall-hooks: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stdout, "uninstall-hooks: hooks removed successfully")
}
