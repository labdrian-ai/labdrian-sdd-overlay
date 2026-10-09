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

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/assets"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/contract"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/execrunner"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gadu"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gate"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/hookwire"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/pipkg"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/propagator"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewreceipt"
	runtimepkg "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
	runtimecore "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings/settingsfile"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/synctrigger"
)

// The names and default registry paths of the contracts the verbs manage. The minimalism
// contract is the one read from a file by default; anti-generic-design ships in the binary.
const (
	defaultContractPath          = "skills/_shared/minimalism-contract.md"
	embeddedAntiGenericDesign    = "anti-generic-design"
	defaultAntiGenericDesignPath = "skills/_shared/anti-generic-design.md"
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
	case embeddedAntiGenericDesign:
		return embeddedContractSpec{
			content:     assets.AntiGenericDesign,
			beginMarker: propagator.AntiGenericDesignBeginMarker,
			endMarker:   propagator.AntiGenericDesignEndMarker,
			rowLabel:    embeddedAntiGenericDesign,
			// defaultPath is the registry-row Path cell / bare injected line when
			// the caller does not override --contract-path. It is where the
			// overlay deploys the standalone copy of this contract.
			defaultPath: defaultAntiGenericDesignPath,
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

func usage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  engine propagate --registry <path> (--contract-file <path> [--contract-path <str>] | --embedded-contract <name>) [--require-registry]")
	fmt.Fprintln(os.Stderr, "  engine gate-task (--contract-file <path> | --embedded-contract <name>) [--contract-path <str>]")
	fmt.Fprintln(os.Stderr, "  engine merge-settings --settings <path> --hook-command <binary-path>")
	fmt.Fprintln(os.Stderr, "  engine uninstall-hooks --settings <path> --hook-command <binary-path>")
	fmt.Fprintln(os.Stderr, "  engine status")
	fmt.Fprintln(os.Stderr, "  engine prespec <verb>  (verbs: rank, lint, readiness, brief)")
	fmt.Fprintln(os.Stderr, "  engine runtime <action> [--target claude|opencode|codex|pi|all] [--config-root <path>]")
	fmt.Fprintln(os.Stderr, "                          [--component runtime-parity|longterm-mem] [--state-dir <path>]")
	fmt.Fprintln(os.Stderr, "    action: status | install | update | uninstall")
	fmt.Fprintln(os.Stderr, "    --target: opencode (default), claude, codex, pi, or all (--component runtime-parity only)")
	fmt.Fprintln(os.Stderr, "    --component: runtime-parity (default, the --target adapters above), or longterm-mem")
	fmt.Fprintln(os.Stderr, "      (a single component spanning claude+opencode+codex; no update/rollback action)")
	fmt.Fprintln(os.Stderr, "    --state-dir: registration.json directory for --component longterm-mem (default ~/.labdrian-overlay)")
	fmt.Fprintln(os.Stderr, "  engine runtime capabilities [--target claude|codex|pi|opencode|all]")
	fmt.Fprintln(os.Stderr, "    read-only: prints, as JSON, what each runtime adapter declares it supports (default --target all);")
	fmt.Fprintln(os.Stderr, "    every supported or partial claim names the tests that prove it, every partial or unsupported claim")
	fmt.Fprintln(os.Stderr, "    states its limit, and untested appears only for a runtime that cannot be exercised on this machine")
	fmt.Fprintln(os.Stderr, "    reads no configuration, HOME, or file; exit 0 success, 2 unknown --target value, 1 usage error including an unknown flag")
	fmt.Fprintln(os.Stderr, "  engine runtime probe [--target claude|codex|pi|all]")
	fmt.Fprintln(os.Stderr, "    read-only, stat only: prints, as JSON, whether each selected runtime's credentials file is present (default")
	fmt.Fprintln(os.Stderr, "    --target all) and whether the Engram database, the longterm-mem registration record, and the gentle-ai binary")
	fmt.Fprintln(os.Stderr, "    on PATH are present, from the real HOME and PATH; it never opens or reads a file and never runs a program, so")
	fmt.Fprintln(os.Stderr, "    a present credentials file does not prove the runtime is authenticated; exit 0 success, 2 unknown --target value,")
	fmt.Fprintln(os.Stderr, "    1 usage error including an unknown flag")
	fmt.Fprintln(os.Stderr, "  OVERLAY_DIR=<repo-root> gentle-ai-overlay gadu-generate [--check]")
	fmt.Fprintln(os.Stderr, "  engine pipkg build|check --overlay-root <path> --registry <path> --dest-dir <path>")
	fmt.Fprintln(os.Stderr, "    build: writes the labdrian-pi package tree to --dest-dir")
	fmt.Fprintln(os.Stderr, "    check: reports drift between --dest-dir and the current manifest; exit 1 on drift")
	fmt.Fprintln(os.Stderr, "  engine skills <verb>   (verbs: list, status, validate, install, adopt, add, remove, sync-manifest, lint, approve, project-register, project-revise, project-status, project-retire)")
	fmt.Fprintln(os.Stderr, "    list          [--registry <path>]                                                      print sorted registry entries")
	fmt.Fprintln(os.Stderr, "    status        [--registry <path>]                                                      print count summary (total/core/custom)")
	fmt.Fprintln(os.Stderr, "    validate      [--registry <path>] [--manifest <path>] --source-root <path>              cross-check registry vs manifest and skills/ on disk; exit 1 on divergence")
	fmt.Fprintln(os.Stderr, "    install       [--registry <path>] [--source-root <path>] [--project-id <id>]           install project-scoped skills into <cwd>/.claude/skills/ and <cwd>/.agents/skills/, replacing only what it installed and left unmodified")
	fmt.Fprintln(os.Stderr, "                  install records each file it writes (with its SHA-256) in the project lock. It refuses, naming the path, a hand-edited file or a directory it did not install,")
	fmt.Fprintln(os.Stderr, "                  reports every refusal, and then writes nothing for any skill. A second install with no source change prints \"unchanged: <id>\" and writes nothing.")
	fmt.Fprintln(os.Stderr, "    adopt         [--registry <path>] [--source-root <path>] [--project-id <id>]           record skill directories already in the project as installed, only when they are exactly the current source;")
	fmt.Fprintln(os.Stderr, "                  nothing is adopted silently: a directory stays foreign to install until skills adopt records it. adopt writes the project lock and no skill file")
	fmt.Fprintln(os.Stderr, "    add           <id> [--registry <path>] [--manifest <path>] [--source-root <path>] [--repo <url>] [--ref <sha>]  register a skill")
	fmt.Fprintln(os.Stderr, "    remove        <id> [--registry <path>] [--manifest <path>]                             unregister a skill from registry and manifest")
	fmt.Fprintln(os.Stderr, "    sync-manifest [--registry <path>] [--manifest <path>]                                  regenerate */SKILL.md rows from registry")
	fmt.Fprintln(os.Stderr, "    lint          <path> | --rules                                                         lint a SKILL.md file, or print the rule table; exit 1 on any hard error")
	fmt.Fprintln(os.Stderr, "    approve       --id <id> --approver <label> --source-root <path>                        record a human approval of skills/<id>/SKILL.md, bound to the digest of its exact bytes")
	fmt.Fprintln(os.Stderr, "                  a skill that fails the hard lint is refused, except a baseline skill (a grandfathered global skill, which predates the lint budget and will be rewritten in a later feature):")
	fmt.Fprintln(os.Stderr, "                  for those, a size or description finding is printed as a \"warning:\" line on stderr, the approval is still recorded, and the exit is 0;")
	fmt.Fprintln(os.Stderr, "                  any other hard finding, such as a missing front matter, refuses a baseline skill too")
	fmt.Fprintln(os.Stderr, "    project-register --project-root <abs> --candidate <key> [--dry-run] [--registry <path>] <draft-file>")
	fmt.Fprintln(os.Stderr, "                                                                                           register a project-tier procedural skill; --dry-run prints the plan and writes nothing")
	fmt.Fprintln(os.Stderr, "    project-revise   --project-root <abs> --candidate <key> [--dry-run] [--registry <path>] <draft-file>")
	fmt.Fprintln(os.Stderr, "                                                                                           revise an agent-owned project skill; --dry-run prints the plan and writes nothing")
	fmt.Fprintln(os.Stderr, "    project-status   --project-root <abs> [--registry <path>] [<id>]")
	fmt.Fprintln(os.Stderr, "                                                                                           report project-tier ownership and global supersession")
	fmt.Fprintln(os.Stderr, "    project-retire   --project-root <abs> [--dry-run] [--reason <reason>] [--absorbed-into <id>] [--registry <path>] <id>")
	fmt.Fprintln(os.Stderr, "                                                                                           retire an agent-owned project skill; --dry-run prints the removal plan")
	fmt.Fprintln(os.Stderr, "    locking: add, remove, sync-manifest and approve hold an exclusive lock, and validate and install a shared one, on .skills.registry.yaml.lock beside the registry")
	fmt.Fprintln(os.Stderr, "    (created by the first write, never removed, git-ignored; shared holders never create it). A lock taken for 2 seconds means another skills command is in progress:")
	fmt.Fprintln(os.Stderr, "    the verb changes nothing and reports it with exit 2 (retry); exit 1 is unchanged (the verb ran or was refused, or the lock could not be created)")
	fmt.Fprintln(os.Stderr, "    project lock: project-register, project-revise, project-retire, install and adopt also take an exclusive lock on the project root directory itself (--project-root, or")
	fmt.Fprintln(os.Stderr, "    the working directory for install and adopt), so no file is created in the project; a busy one is exit 2 too, and a filesystem that cannot lock a directory refuses the verb (exit 1).")
	fmt.Fprintln(os.Stderr, "    A verb that holds both takes the overlay lock first, then the project lock, so install and adopt can wait up to twice the lock bound (2 seconds each) when both are busy;")
	fmt.Fprintln(os.Stderr, "    project-status takes a shared lock on the project. A writer refuses, with nothing locked, when the registry it would lock does not exist")
	fmt.Fprintln(os.Stderr, "  engine skills guard-hook")
	fmt.Fprintln(os.Stderr, "    internal Claude Code PreToolUse hook command (install-hooks installs it; on an existing install, re-run install-hooks and restart Claude Code to load it):")
	fmt.Fprintln(os.Stderr, "    reads the hook JSON on stdin and denies the agent running 'skills approve' (approval is a human step) or writing a skill's .approval.json record with")
	fmt.Fprintln(os.Stderr, "    Write, Edit, MultiEdit, or NotebookEdit. A denial is {\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"permissionDecision\":\"deny\",")
	fmt.Fprintln(os.Stderr, "    \"permissionDecisionReason\":...}} with exit 0; an allow prints nothing. A speed bump, not a security boundary: it matches the command text and the file name,")
	fmt.Fprintln(os.Stderr, "    so it can be bypassed and can deny text that only spells the invocation. It never blocks on an error (unusable input is an allow): exit 0 always, 1 usage error")
	fmt.Fprintln(os.Stderr, "  engine sync-trigger --event session-end|archive --cwd <path> [--state-dir <path>]")
	fmt.Fprintln(os.Stderr, "    always exits 0 to its caller; detaches a bounded longterm-mem sync and logs its outcome")
	fmt.Fprintln(os.Stderr, "  engine review-receipt capture --cwd <repo> [--change <name>]")
	fmt.Fprintln(os.Stderr, "    persists every surviving approved review receipt to openspec/changes/<change>/review-receipts/")
	fmt.Fprintln(os.Stderr, "    --change resolves ambiguity when more than one active change exists; omit it to auto-detect")
	fmt.Fprintln(os.Stderr, "  engine review-receipt hook --cwd <repo>")
	fmt.Fprintln(os.Stderr, "    fail-closed PreToolUse Bash hook: reads tool_input JSON from stdin, captures before")
	fmt.Fprintln(os.Stderr, "    'gentle-ai review acknowledge-approved', denies (exit 2) on ambiguity or capture failure")
	fmt.Fprintln(os.Stderr, "  engine shaper assess --root <abs> --handoff <rel> --goal <rel> [--view]")
	fmt.Fprintln(os.Stderr, "    read-only readiness assessment; exit 0 ready, 3 draft, 2 invalid, 1 usage or internal error")
	fmt.Fprintln(os.Stderr, "    --view prints exactly the rendered clearance view bytes the Pi dialog displays")
	fmt.Fprintln(os.Stderr, "  engine shaper clearance record --root <abs> --handoff <rel> --goal <rel> --stdin")
	fmt.Fprintln(os.Stderr, "    stores a human clearance decision read only from stdin after re-deriving and checking its binding")
	fmt.Fprintln(os.Stderr, "    not a signature: any process running as the same OS user, including any installed Pi extension, can forge one")
	fmt.Fprintln(os.Stderr, "  engine shaper guard-hook")
	fmt.Fprintln(os.Stderr, "    Claude Code PreToolUse deny guard for clearance recording and the clearance store (exit 2 denies; a speed bump)")
	fmt.Fprintln(os.Stderr, "  engine memory plan --profile <name> [--goal <path>] [--goal-directive <path>] [--handoff-directive <path>]")
	fmt.Fprintln(os.Stderr, "    read-only: resolves a workflow profile's default memory directive, narrows it with an optional")
	fmt.Fprintln(os.Stderr, "    Goal-supplied then handoff-supplied directive, and prints the resulting query plan as JSON")
	fmt.Fprintln(os.Stderr, "    exit 0 plan resolved, 2 refused/invalid input, 1 usage or output error; it never queries or writes memory")
	fmt.Fprintln(os.Stderr, "  engine workflow create --project <id> --workflow <id> --goal <path> --profile <name> [--role-chain <id>]")
	fmt.Fprintln(os.Stderr, "  engine workflow start|pause|resume|status --project <id> --workflow <id>")
	fmt.Fprintln(os.Stderr, "  engine workflow stage  --project <id> --workflow <id> --stage <name>")
	fmt.Fprintln(os.Stderr, "  engine workflow verify --project <id> --workflow <id> --goal <path>")
	fmt.Fprintln(os.Stderr, "  engine workflow close  --project <id> --workflow <id> --outcome completed|abandoned [--reason <text>]")
	fmt.Fprintln(os.Stderr, "    Phase 6 standalone workflow lifecycle: local bookkeeping under $XDG_STATE_HOME/labdrian/workflows/;")
	fmt.Fprintln(os.Stderr, "    no verb requires Gentle AI, gentle-pi, a runtime, memory, or auth (an unavailable dependency is")
	fmt.Fprintln(os.Stderr, "    recorded on the event, and a dependency is recorded available only when its file or binary is seen by stat,")
	fmt.Fprintln(os.Stderr, "    never opened, run, or trusted). --goal is re-read from disk at both create and verify;")
	fmt.Fprintln(os.Stderr, "    no path is ever persisted. exit 0 success, 2 refused/invalid (illegal transition, verify failure,")
	fmt.Fprintln(os.Stderr, "    non-owned state), 1 usage error")
	fmt.Fprintln(os.Stderr, "  engine workflow bind --project <id> --workflow <id>")
	fmt.Fprintln(os.Stderr, "  engine workflow unbind")
	fmt.Fprintln(os.Stderr, "  engine workflow binding")
	fmt.Fprintln(os.Stderr, "    Phase 7 session binding: records which workflow the git repository containing the working directory")
	fmt.Fprintln(os.Stderr, "    follows, in $XDG_STATE_HOME/labdrian/bindings/<repo-key>.json. The key is the SHA-256 of the git common directory,")
	fmt.Fprintln(os.Stderr, "    so every worktree of a repository shares one binding. bind needs an existing, owned, not closed workflow;")
	fmt.Fprintln(os.Stderr, "    it refuses to replace a binding to a workflow that is still active or whose log cannot be read (unbind first)")
	fmt.Fprintln(os.Stderr, "    and replaces a stale one (closed, gone, corrupt, or not ours). unbind is idempotent and prints {\"removed\": true|false}. binding is read-only:")
	fmt.Fprintln(os.Stderr, "    it prints the binding's classification and, when owned, the bound workflow's classification and status.")
	fmt.Fprintln(os.Stderr, "    exit 0 success, 2 refused/invalid (no git repository, a workflow that cannot be bound, a binding file that is not ours")
	fmt.Fprintln(os.Stderr, "    or cannot be used (foreign, malformed, unavailable), another bind or unbind in progress (busy), a binding another")
	fmt.Fprintln(os.Stderr, "    process changed meanwhile), 1 usage error")
	fmt.Fprintln(os.Stderr, "  engine projection hook --event UserPromptSubmit|PreToolUse")
	fmt.Fprintln(os.Stderr, "    internal Claude Code hook command (install-hooks installs it; on an existing install, re-run install-hooks and restart Claude Code to load it): reads the hook JSON on stdin and prints at most one JSON object.")
	fmt.Fprintln(os.Stderr, "    UserPromptSubmit prints {\"hookSpecificOutput\":{\"hookEventName\":\"UserPromptSubmit\",\"additionalContext\":...},\"systemMessage\":...}, which puts the")
	fmt.Fprintln(os.Stderr, "    workflow the repository is bound to (see workflow bind) into the session: id, profile, status, stages, and the memory plan;")
	fmt.Fprintln(os.Stderr, "    the binding to a closed workflow is removed, and the note says whether it was removed, left alone, or the removal failed.")
	fmt.Fprintln(os.Stderr, "    PreToolUse gates a tool call against the bound workflow and never writes: it denies Write, Edit, MultiEdit, and NotebookEdit while the")
	fmt.Fprintln(os.Stderr, "    workflow is paused (never Bash, never a read), and denies a longterm-mem query whose project differs from the memory plan's project")
	fmt.Fprintln(os.Stderr, "    (every query when the plan has no project); it never touches get, promote, Engram tools, or any write. A denial is")
	fmt.Fprintln(os.Stderr, "    {\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"permissionDecision\":\"deny\",\"permissionDecisionReason\":...}} with exit 0; an allow prints")
	fmt.Fprintln(os.Stderr, "    nothing (or only a systemMessage warning) and never permissionDecision allow.")
	fmt.Fprintln(os.Stderr, "    silent (no output at all) when the repository is not bound, the input is not a usable hook input, or the directory is not in a")
	fmt.Fprintln(os.Stderr, "    repository; on UserPromptSubmit a bound workflow that cannot be followed, or a binding store that cannot be read, gives one warning and")
	fmt.Fprintln(os.Stderr, "    nothing is projected (PreToolUse allows quietly). read-only otherwise. exit 0 always (it never blocks a prompt or a tool call), 1 usage error")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Embedded contracts: anti-generic-design")
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

// runPipkg implements the 'pipkg build|check' subcommand.
func runPipkg(args []string) {
	runPipkgCore(newPipkgSource(os.Environ()), args, os.Stdout, os.Stderr, os.Exit)
}

// runPipkgCore is the testable core of the pipkg subcommand: 'build' writes
// the labdrian-pi package tree, 'check' reports drift against it. Requires
// --overlay-root, --registry, and --dest-dir. Fails LOUD on a missing verb
// or missing flag (ADR-4). source is how the builder asks git about the overlay: the git of the
// machine in the program, a fake or pipkg.NoRepository in a test.
func runPipkgCore(source pipkg.SourceRepo, args []string, stdout, stderr io.Writer, exit func(int)) {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "error: pipkg requires a verb: build, check")
		exit(1)
		return
	}
	verb := args[0]
	if verb != "build" && verb != "check" {
		fmt.Fprintf(stderr, "error: unknown pipkg verb %q; expected build or check\n", verb)
		exit(1)
		return
	}

	var overlayRoot, registryPath, destDir string
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--overlay-root":
			i++
			if i >= len(args) {
				fmt.Fprintln(stderr, "error: --overlay-root requires a value")
				exit(1)
				return
			}
			overlayRoot = args[i]
		case "--registry":
			i++
			if i >= len(args) {
				fmt.Fprintln(stderr, "error: --registry requires a value")
				exit(1)
				return
			}
			registryPath = args[i]
		case "--dest-dir":
			i++
			if i >= len(args) {
				fmt.Fprintln(stderr, "error: --dest-dir requires a value")
				exit(1)
				return
			}
			destDir = args[i]
		default:
			fmt.Fprintf(stderr, "error: unknown option %q\n", args[i])
			exit(1)
			return
		}
	}
	if overlayRoot == "" || registryPath == "" || destDir == "" {
		fmt.Fprintln(stderr, "error: pipkg requires --overlay-root, --registry, and --dest-dir")
		exit(1)
		return
	}

	packages := pipkg.Packages{
		Registries: newWarningRegistryRepository(stderr),
		Source:     source,
		Options:    pipkgOptionsFromEnv(os.Getenv),
	}
	if verb == "build" {
		if err := packages.Build(overlayRoot, registryPath, destDir); err != nil {
			fmt.Fprintf(stderr, "pipkg build: %v\n", err)
			exit(1)
			return
		}
		fmt.Fprintf(stdout, "pipkg build: labdrian-pi package written to %s\n", destDir)
		exit(0)
		return
	}

	disclosure, err := packages.Check(overlayRoot, registryPath, destDir)
	if disclosure != "" {
		fmt.Fprintf(stdout, "pipkg check: %s\n", disclosure)
	}
	if err != nil {
		fmt.Fprintf(stderr, "pipkg check: %v\n", err)
		exit(1)
		return
	}
	fmt.Fprintln(stdout, "pipkg check: OK (built package matches the current manifest)")
	exit(0)
}

// ---------------------------------------------------------------------------
// runtime subcommand
// ---------------------------------------------------------------------------

// runRuntime implements the 'runtime <action>' subcommand.
// Supported actions: status, install, update, uninstall.
func runRuntime(args []string) {
	runRuntimeCore(execrunner.New(), newPipkgSource(os.Environ()), args, os.Stdout, os.Stderr, os.Exit)
}

// componentRuntimeParity and componentLongtermMem are the two values
// --component accepts (D4). componentRuntimeParity is the default and
// preserves every pre-existing --target-based behavior unchanged (10a.7).
const (
	componentRuntimeParity = "runtime-parity"
	componentLongtermMem   = "longterm-mem"
)

// runRuntimeCore is the testable core for the 'runtime' subcommand. commands is how the Pi
// adapter starts the `pi` CLI: the process adapter in the program, a fake in a test, so no test of
// the command can reach a real `pi`. source is how the Pi package builder asks git about the
// overlay: the git of the machine in the program, pipkg.NoRepository in a test.
func runRuntimeCore(commands runtimepkg.CommandRunner, source pipkg.SourceRepo, args []string, stdout io.Writer, stderr io.Writer, exit func(int)) {
	// capabilities is declarative and read-only: it never constructs an
	// adapter, resolves a config root, or reads HOME, so it is dispatched
	// before the lifecycle flags are parsed and shares none of their
	// defaults (its --target defaults to all, not opencode).
	if len(args) > 0 && args[0] == "capabilities" {
		runRuntimeCapabilities(args[1:], stdout, stderr, exit)
		return
	}
	// probe is read-only too: it stats the presence signals under the process's
	// home and PATH and shares none of the lifecycle flags or defaults.
	if len(args) > 0 && args[0] == "probe" {
		home, path := runtimeProbeEnv()
		runRuntimeProbe(args[1:], home, path, stdout, stderr, exit)
		return
	}

	registry, err := newRuntimeRegistry(newWarningRegistryRepository(stderr), settingsfile.Installer{}, commands, source, pipkgOptionsFromEnv(os.Getenv))
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		exit(1)
		return
	}

	action, target, configRoot, component, stateDir, err := parseRuntimeArgs(args, registry)
	if err != nil {
		fmt.Fprintln(stderr, err)
		usage()
		exit(1)
		return
	}

	cfg := runtimeConfigFromEnv(os.Getenv, os.UserHomeDir)

	if component == componentLongtermMem {
		// D4 parse-time refusal: update is rejected here, BEFORE any
		// LongtermMemAdapter is even constructed — never after running one
		// and reporting a failing status. "rollback" needs no separate
		// guard: it is not a recognized action at all (see the action-name
		// validation in parseRuntimeArgs below), so it is already rejected
		// at the exact same point, before any adapter call.
		if action == "update" {
			fmt.Fprintln(stderr, "error: longterm-mem does not support the 'update' action; reinstall instead (--component longterm-mem install)")
			exit(1)
			return
		}
		// The binary path is DERIVED from --state-dir, never resolved
		// independently from HOME: the overlay entrypoint deploys the
		// binary at "$STATE_DIR/bin/longterm-mem" and registers MCP
		// entries naming that exact path, so an adapter that resolved it
		// from HOME under an overridden state dir reported a deployed
		// binary as missing and a genuinely owned entry as unmanaged. An
		// empty stateDir yields an empty binary path here, which
		// NewLongtermMemAdapter fills in with the same default it fills
		// stateDir with — so the un-overridden case is unchanged.
		adapter := runtimepkg.NewLongtermMemAdapter(cfg, stateDir, runtimepkg.LongtermMemBinaryPathForStateDir(stateDir))
		result := runtimeLifecycleResult(adapter, action)
		fmt.Fprintln(stdout, result.String())
		if action == "status" {
			if result.Status != runtimecore.CapabilitySupported {
				exit(1)
				return
			}
			exit(0)
			return
		}
		if result.Status == runtimecore.CapabilityUnsupported || result.Status == runtimecore.CapabilityPartial {
			exit(1)
			return
		}
		exit(0)
		return
	}

	cfg.ConfigRoot = configRoot
	targets := registry.Expand(target)

	// Every adapter is built before the first one acts, so a target the registry cannot build
	// stops the command before anything has been done, never half way through `all`.
	adapters, err := buildRuntimeAdapters(registry, targets, cfg)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		exit(1)
		return
	}

	failed := false
	allTargets := len(adapters) > 1

	for _, adapter := range adapters {
		current := adapter.Target()
		result := runtimeLifecycleResult(adapter, action)
		fmt.Fprintln(stdout, result.String())
		// Pi now has a real Status() implementation (pi-lifecycle, slice
		// 5), so it is reported and aggregated exactly like every other
		// target: an honestly unsupported Pi fails `status --target all`
		// just as an honestly unsupported claude/opencode/codex would
		// (W-03 — there is no more "Pi is exempt" status-only carve-out).
		actionFailed := false
		switch action {
		case "status":
			switch {
			case result.Status == runtimecore.CapabilityRestartRequired:
				actionFailed = true
			case result.Status == runtimecore.CapabilityUnsupported:
				actionFailed = true
			case result.Status == runtimecore.CapabilityPartial && !(allTargets && current == runtimecore.TargetCodex):
				actionFailed = true
			}
		default:
			switch result.Status {
			case runtimecore.CapabilityPartial:
				actionFailed = true
			case runtimecore.CapabilityUnsupported:
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

// parseRuntimeArgs parses minimal runtime subcommand arguments.
func parseRuntimeArgs(args []string, registry *runtimecore.Registry) (action string, target runtimecore.Target, configRoot, component, stateDir string, err error) {
	if len(args) == 0 {
		return "", "", "", "", "", fmt.Errorf("error: runtime requires an action")
	}
	action = args[0]
	if strings.HasPrefix(action, "-") {
		return "", "", "", "", "", fmt.Errorf("error: runtime requires an action: status | install | update | uninstall | capabilities")
	}

	target = runtimecore.TargetOpenCode
	component = componentRuntimeParity
	for i := 1; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--target":
			i++
			if i >= len(args) {
				return "", "", "", "", "", fmt.Errorf("error: --target requires a value")
			}
			target, err = registry.Parse(args[i])
			if err != nil {
				return "", "", "", "", "", err
			}
		case "--config-root":
			i++
			if i >= len(args) {
				return "", "", "", "", "", fmt.Errorf("error: --config-root requires a value")
			}
			configRoot = args[i]
		case "--component":
			i++
			if i >= len(args) {
				return "", "", "", "", "", fmt.Errorf("error: --component requires a value")
			}
			switch args[i] {
			case componentRuntimeParity, componentLongtermMem:
				component = args[i]
			default:
				return "", "", "", "", "", fmt.Errorf("error: unknown --component %q (expected %q or %q)", args[i], componentRuntimeParity, componentLongtermMem)
			}
		case "--state-dir":
			i++
			if i >= len(args) {
				return "", "", "", "", "", fmt.Errorf("error: --state-dir requires a value")
			}
			stateDir = args[i]
		default:
			if strings.HasPrefix(a, "--") {
				return "", "", "", "", "", fmt.Errorf("error: unknown flag %q", a)
			}
			return "", "", "", "", "", fmt.Errorf("error: unexpected runtime argument %q", a)
		}
	}

	if action != "status" && action != "install" && action != "update" && action != "uninstall" {
		return "", "", "", "", "", fmt.Errorf("error: unknown runtime action %q", action)
	}

	return action, target, configRoot, component, stateDir, nil
}

// ---------------------------------------------------------------------------
// sync-trigger subcommand
// ---------------------------------------------------------------------------

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
	synctrigger.Run(o)
	exit(0)
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

// ---------------------------------------------------------------------------
// review-receipt subcommand
// ---------------------------------------------------------------------------

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

func runtimeLifecycleResult(adapter runtimecore.Adapter, action string) runtimecore.LifecycleResult {
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
		return runtimecore.NewLifecycleResult(adapter.Target(), "status", runtimecore.CapabilityUnsupported, "unknown runtime action", nil)
	}
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

	// Check 3b: SessionEnd sync-trigger hook wired. A missing family is
	// WARN/degraded, not FAIL — it means a pre-#291 two-family install that
	// hasn't run the upgrade path yet, not a broken installation.
	checks = append(checks, checkSessionEndHook(settingsRoot, settingsErr, settingsPath))

	// Check 3c: PreToolUse/Bash review-receipt hook wired. Same WARN/degraded
	// tier as SessionEnd — a machine that hasn't run the upgrade path yet is
	// pre-#3a, not broken.
	checks = append(checks, checkReviewReceiptHook(settingsRoot, settingsErr, settingsPath))

	// Check 3d: shaper clearance deny guard (both PreToolUse entries and the
	// permissions.deny backstop). Missing parts are WARN/degraded.
	checks = append(checks, checkShaperClearanceGuard(settingsRoot, settingsErr, settingsPath))

	// Check 3e: projection hook family (UserPromptSubmit context and the two
	// PreToolUse gates). A machine that has not re-run install-hooks since the
	// family landed is WARN/degraded, not broken.
	checks = append(checks, checkProjectionHooks(settingsRoot, settingsErr, settingsPath, binaryPath))

	// Check 3f: skills approve guard (the two PreToolUse entries that deny the
	// agent running skills approve or writing the approval record). A machine
	// that has not re-run install-hooks since the guard landed is WARN/degraded,
	// not broken.
	checks = append(checks, checkApproveGuard(settingsRoot, settingsErr, settingsPath, binaryPath))

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

// remediationNote is the shared WARN note text pointing at the upgrade path
// for any check that goes from a two-family install to the three-family
// state (post-#291): re-run uninstall then install to pick up new entries.
const remediationNote = "run 'labdrian uninstall-hooks' then 'labdrian install-hooks'"

// checkSessionEndHook verifies the SessionEnd sync-trigger entry references
// our binary and the sync-trigger identity token. Unreadable settings is a
// hard FAIL like the other hook checks; a missing entry is WARN/degraded,
// not FAIL — it names the same two remediation commands as the runtime
// status partial message so a pre-upgrade machine (two families, no
// SessionEnd) is actionable rather than treated as a broken install.
func checkSessionEndHook(root map[string]interface{}, settingsErr error, settingsPath string) checkResult {
	label := "hook: SessionEnd (sync-trigger)"
	if settingsErr != nil {
		return checkResult{label: label, ok: false, note: "cannot read " + settingsPath + ": " + settingsErr.Error()}
	}
	if root == nil {
		return checkResult{label: label, ok: true, degraded: true, note: settingsPath + " absent or empty; " + remediationNote}
	}
	hooks, _ := root["hooks"].(map[string]interface{})
	if hooks != nil {
		entries, _ := hooks["SessionEnd"].([]interface{})
		for _, e := range entries {
			if innerHookContainsBinary(e, binaryIdentity) && innerHookContainsBinary(e, settings.LabdrianSyncTriggerIdentity) {
				return checkResult{label: label, ok: true}
			}
		}
	}
	return checkResult{label: label, ok: true, degraded: true, note: "no SessionEnd entry referencing " + binaryIdentity + "; " + remediationNote}
}

// checkReviewReceiptHook verifies the PreToolUse/Bash review-receipt entry
// references our binary and the review-receipt identity token. Unreadable
// settings is a hard FAIL like the other hook checks; a missing entry is
// WARN/degraded, not FAIL — same remediation as checkSessionEndHook.
func checkReviewReceiptHook(root map[string]interface{}, settingsErr error, settingsPath string) checkResult {
	label := `hook: PreToolUse matcher="Bash" (review-receipt)`
	if settingsErr != nil {
		return checkResult{label: label, ok: false, note: "cannot read " + settingsPath + ": " + settingsErr.Error()}
	}
	if root == nil {
		return checkResult{label: label, ok: true, degraded: true, note: settingsPath + " absent or empty; " + remediationNote}
	}
	hooks, _ := root["hooks"].(map[string]interface{})
	if hooks != nil {
		entries, _ := hooks["PreToolUse"].([]interface{})
		for _, e := range entries {
			em, ok := e.(map[string]interface{})
			if !ok || em["matcher"] != "Bash" {
				continue
			}
			if innerHookContainsBinary(e, binaryIdentity) && innerHookContainsBinary(e, settings.LabdrianReviewReceiptIdentity) {
				return checkResult{label: label, ok: true}
			}
		}
	}
	return checkResult{label: label, ok: true, degraded: true, note: "no PreToolUse entry with matcher=\"Bash\" referencing " + binaryIdentity + "; " + remediationNote}
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
	if _, err := contract.Parse(string(data)); err != nil {
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
	hasDesign := containsSubstring(content, propagator.AntiGenericDesignBeginMarker)

	if hasMinimalism && hasDesign {
		return checkResult{label: label, ok: true, note: "scoped block present"}
	}

	var missing []string
	if !hasMinimalism {
		missing = append(missing, "minimalism-contract-scope")
	}
	if !hasDesign {
		missing = append(missing, "anti-generic-design-scope")
	}
	note := fmt.Sprintf(
		"present but scoped block(s) missing: %s (run 'labdrian install-hooks' or propagate)",
		strings.Join(missing, ", "),
	)
	return checkResult{label: label, ok: true, degraded: true, note: note}
}
