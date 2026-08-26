package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Target is one of the deployable overlay targets.
type Target struct {
	Name string
	Path string
}

// AllTargets is the canonical, ordered list of overlay targets.
// Paths mirror the bash backend's TARGET_PATHS map.
func AllTargets() []Target {
	home, _ := os.UserHomeDir()
	return []Target{
		{Name: "claude", Path: filepath.Join(home, ".claude", "skills")},
		{Name: "opencode", Path: filepath.Join(home, ".config", "opencode", "skills")},
		{Name: "codex", Path: filepath.Join(home, ".codex", "skills")},
	}
}

// Action is a backend subcommand the TUI can invoke.
type Action struct {
	Name           string   // label shown in the menu
	Command        string   // bin/labdrian-overlay subcommand
	Args           []string // additional positional args appended after Command
	Mutating       bool     // requires confirmation
	SupportsAll    bool     // can pass --target all when every target is selected
	TargetAgnostic bool     // when true: invoke WITHOUT --target, skip target selection
	ConfirmMessage string   // per-action confirm copy; empty falls back to generic
	Hint           string   // one-line purpose scent shown in the menu
	Also           []Action // nested sub-actions merged into this menu entry (invisible in the menu)
}

// Actions returns the action menu in display order: Estado, then sync-check,
// then capture, then apply (the natural usage flow), then repo maintenance
// (self-update), then the hooks lifecycle, then skills. Related read-only
// sub-actions (status-hooks; skills validate/list) are folded into their
// primary entry via Also instead of appearing as separate top-level menu
// rows.
func Actions() []Action {
	return []Action{
		{Name: "Estado", Command: "status", Mutating: false, SupportsAll: true,
			Hint: "Resumen del estado actual (targets + hooks)",
			Also: []Action{
				{Command: "status-hooks", TargetAgnostic: true},
			}},
		{Name: "Verificar sincronización", Command: "sync-check", Mutating: false, SupportsAll: true,
			Hint: "Compara overlay vs upstream"},
		{Name: "Capturar (actualizar upstream)", Command: "capture", Mutating: true, SupportsAll: false,
			Hint: "Trae cambios de upstream al overlay"},
		{Name: "Aplicar cambios", Command: "apply", Mutating: true, SupportsAll: true,
			Hint: "Despliega el overlay en los destinos"},
		// Repo maintenance — TargetAgnostic: fast-forwards main only, via the
		// backend's self-update subcommand (D1-D3). Placed right after the
		// core apply flow and before the hooks block (D7).
		{
			Name:           "Actualizar repositorio",
			Command:        "self-update",
			Mutating:       true,
			TargetAgnostic: true,
			ConfirmMessage: "Actualiza SOLO la rama main a origin/main (fast-forward); tu rama actual no se toca y se vuelve a ella al terminar. Rechaza con árbol sucio o main local adelantado.",
			Hint:           "Pone main al día con origin/main (ff-only)",
		},
		// Hooks lifecycle — TargetAgnostic: operate on ~/.claude/settings.json globally.
		{
			Name:           "Instalar hooks",
			Command:        "install-hooks",
			Mutating:       true,
			TargetAgnostic: true,
			ConfirmMessage: "Modifica ~/.claude/settings.json (se crea un respaldo .bak antes de escribir).",
			Hint:           "Compila engine y cablea hooks (paso 1)",
		},
		{
			Name:           "Desinstalar hooks",
			Command:        "uninstall-hooks",
			Mutating:       true,
			TargetAgnostic: true,
			ConfirmMessage: "Elimina los hooks de ~/.claude/settings.json (se crea un respaldo .bak antes de modificar).",
			Hint:           "Quita los hooks de settings.json",
		},
		// Skills registry — read-only, TargetAgnostic. The bash backend injects
		// --registry/--manifest/--source-root defaults, so no flags are needed here.
		{Name: "Skills", Command: "skills", Args: []string{"status"},
			TargetAgnostic: true, Mutating: false,
			Hint: "Estado + valida + lista el registro",
			Also: []Action{
				{Command: "skills", Args: []string{"validate"}, TargetAgnostic: true},
				{Command: "skills", Args: []string{"list"}, TargetAgnostic: true},
			}},
	}
}

// AgentFileEntry records the per-file sync status of a single agent file.
type AgentFileEntry struct {
	Path   string // relative path, e.g. "agents/GADU.md"
	Status string // "IN_SYNC", "OVERLAY_NOT_DEPLOYED", "UPSTREAM_CHANGED"
}

// SyncStatus is a color-coded health classification for a single target.
type SyncStatus int

const (
	// SyncUnknown means no VERDICT line was parsed for the target.
	SyncUnknown SyncStatus = iota
	// SyncHealthy means live == main: in sync with gentle-ai.
	SyncHealthy
	// SyncNeedsApply means overlay is not deployed (apply required).
	SyncNeedsApply
	// SyncNeedsCapture means upstream (gentle-ai) changed (capture + apply required).
	SyncNeedsCapture
	// SyncBehindOrigin means local main is behind this repo's own origin/main
	// (informational drift; never overridden by, and never masks, the other
	// two verdicts — see classify's precedence and R-006). Tracks main, not
	// HEAD, because self-update (the source of truth this mirrors) only ever
	// converges main, never the checked-out branch.
	SyncBehindOrigin
)

// RepoBehindOriginNA is the sentinel value for TargetVerdict.RepoBehindOrigin
// when the origin comparison is unavailable (no remote, no cached ref, or a
// requested fetch failed) — distinct from a confirmed 0 commits behind. A
// single named constant (instead of a magic -1 repeated at each use site)
// keeps every producer/consumer of this field agreeing by construction.
const RepoBehindOriginNA = -1

// TargetVerdict holds the parsed sync-check result for one target.
type TargetVerdict struct {
	Target             string
	UpstreamChanged    int
	OverlayNotDeployed int
	RepoBehindOrigin   int // count; RepoBehindOriginNA when unavailable
	Action             string
	Status             SyncStatus
	AgentFiles         []AgentFileEntry // per-file statuses for agents/ paths
}

// classify maps verdict counts to a color status.
//
// Priority (matches the backend ACTION precedence, extended by R-006):
//   - UPSTREAM_CHANGED > 0      -> RED    (gentle-ai synced, needs capture+apply)
//   - OVERLAY_NOT_DEPLOYED > 0  -> YELLOW (needs apply)
//   - REPO_BEHIND_ORIGIN > 0    -> behind-origin (never silently healthy)
//   - otherwise                 -> GREEN (healthy)
func classify(upstreamChanged, overlayNotDeployed, repoBehindOrigin int) SyncStatus {
	switch {
	case upstreamChanged > 0:
		return SyncNeedsCapture
	case overlayNotDeployed > 0:
		return SyncNeedsApply
	case repoBehindOrigin > 0:
		return SyncBehindOrigin
	default:
		return SyncHealthy
	}
}

// ParseSyncCheck extracts per-target verdicts from sync-check stdout.
//
// It reads the machine-friendly lines emitted by the backend:
//
//	VERDICT:<target>:UPSTREAM_CHANGED=N OVERLAY_NOT_DEPLOYED=M REPO_BEHIND_ORIGIN=<n|NA>
//	ACTION:<target>: <text>
//
// It also tracks `=== sync-check: <target> ===` section headers to set the
// active target scope, and captures per-file `agents/...` status lines
// (format: `  <path>: <STATUS>`) into [TargetVerdict.AgentFiles].
//
// Verdicts are returned in first-seen order.
func ParseSyncCheck(output string) []TargetVerdict {
	order := []string{}
	byTarget := map[string]*TargetVerdict{}

	get := func(name string) *TargetVerdict {
		v, ok := byTarget[name]
		if !ok {
			// RepoBehindOrigin defaults to RepoBehindOriginNA until a VERDICT
			// line explicitly sets it — mirrors the backend's NA sentinel and
			// avoids a defensive zero-value collapsing into "0 behind".
			v = &TargetVerdict{Target: name, RepoBehindOrigin: RepoBehindOriginNA}
			byTarget[name] = v
			order = append(order, name)
		}
		return v
	}

	scanner := bufio.NewScanner(strings.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var currentTarget string // tracks the active section for per-file lines
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Section header: "=== sync-check: <target> (<path>) ==="
		if strings.HasPrefix(line, "=== sync-check: ") {
			rest := strings.TrimPrefix(line, "=== sync-check: ")
			currentTarget, _, _ = strings.Cut(rest, " ")
			continue
		}

		if rest, ok := strings.CutPrefix(line, "VERDICT:"); ok {
			// rest = "<target>:UPSTREAM_CHANGED=N OVERLAY_NOT_DEPLOYED=M"
			target, counts, ok := strings.Cut(rest, ":")
			if !ok {
				continue
			}
			v := get(target)
			for _, field := range strings.Fields(counts) {
				key, val, ok := strings.Cut(field, "=")
				if !ok {
					continue
				}
				switch key {
				case "UPSTREAM_CHANGED":
					n, _ := strconv.Atoi(val)
					v.UpstreamChanged = n
				case "OVERLAY_NOT_DEPLOYED":
					n, _ := strconv.Atoi(val)
					v.OverlayNotDeployed = n
				case "REPO_BEHIND_ORIGIN":
					// Dedicated NA pre-check: the field's literal "NA" sentinel
					// MUST be handled before strconv.Atoi. Falling through to a
					// generic `n, _ := strconv.Atoi(val)` (as UPSTREAM_CHANGED/
					// OVERLAY_NOT_DEPLOYED do) would discard the parse error and
					// silently collapse "NA" (or an omitted field) to Go's
					// zero-value 0 — indistinguishable from "confirmed 0 commits
					// behind" and reproducing the exact silent-healthy bug class
					// R-006 exists to eliminate.
					if val == "NA" {
						v.RepoBehindOrigin = RepoBehindOriginNA
					} else if n, err := strconv.Atoi(val); err == nil {
						v.RepoBehindOrigin = n
					}
				}
			}
			v.Status = classify(v.UpstreamChanged, v.OverlayNotDeployed, v.RepoBehindOrigin)
			continue
		}

		if rest, ok := strings.CutPrefix(line, "ACTION:"); ok {
			// rest = "<target>: <text>"
			target, text, ok := strings.Cut(rest, ":")
			if !ok {
				continue
			}
			v := get(target)
			v.Action = strings.TrimSpace(text)
			continue
		}

		// Per-file lines: capture agents/ file statuses within the current section.
		for _, s := range []string{"IN_SYNC", "OVERLAY_NOT_DEPLOYED", "UPSTREAM_CHANGED"} {
			if rest, ok := strings.CutPrefix(line, s+": "); ok {
				if strings.HasPrefix(rest, "agents/") && currentTarget != "" {
					path, _, _ := strings.Cut(rest, " ") // strip "(detail)" suffix
					v := get(currentTarget)
					v.AgentFiles = append(v.AgentFiles, AgentFileEntry{Path: path, Status: s})
				}
				break
			}
		}
	}

	result := make([]TargetVerdict, 0, len(order))
	for _, name := range order {
		result = append(result, *byTarget[name])
	}
	return result
}

// probeBehind extracts the launch-time origin-behind count from cached-only
// sync-check output. It reuses ParseSyncCheck (D4) instead of re-deriving
// REPO_BEHIND_ORIGIN detection, which stays owned by sync-check-verdicts —
// this capability only reads that published value. Any failure to determine
// a concrete count — zero verdicts (e.g. every target dir missing skips the
// VERDICT line), an explicit "NA", or unparseable/garbage output — degrades
// to RepoBehindOriginNA rather than Go's zero value, which would collapse
// into "confirmed 0 behind" (the exact R-006 bug class ParseSyncCheck
// already guards against for its own callers).
func probeBehind(output string) int {
	verdicts := ParseSyncCheck(output)
	if len(verdicts) == 0 {
		return RepoBehindOriginNA
	}
	return verdicts[0].RepoBehindOrigin
}

// probeBehindOriginCmd returns a tea.Cmd that runs a cached-only sync-check
// (no --fetch/--check-origin, per R-001) against root and delivers a
// probeDoneMsg. bubbletea runs the returned func() tea.Msg off the UI
// goroutine, so this never blocks Init()'s first render. CombinedOutput is
// fed to probeBehind even when the process exits non-zero — the probe reads
// a cached ref, it does not require a clean exit.
func probeBehindOriginCmd(root string) tea.Cmd {
	return func() tea.Msg {
		bin := filepath.Join(root, "bin", "labdrian-overlay")
		cmd := exec.Command(bin, "sync-check")
		cmd.Dir = root
		out, _ := cmd.CombinedOutput()
		return probeDoneMsg{behind: probeBehind(string(out))}
	}
}

// RepoRoot resolves the overlay repo root.
//
// Resolution order:
//  1. OVERLAY_DIR env var (set by the bash wrapper when launching the TUI)
//  2. walk up from the executable's directory looking for bin/labdrian-overlay
//  3. walk up from the current working directory looking for bin/labdrian-overlay
func RepoRoot() (string, error) {
	if dir := os.Getenv("OVERLAY_DIR"); dir != "" {
		if hasBackend(dir) {
			return dir, nil
		}
	}

	if exe, err := os.Executable(); err == nil {
		if root, ok := walkUpForBackend(filepath.Dir(exe)); ok {
			return root, nil
		}
	}

	if wd, err := os.Getwd(); err == nil {
		if root, ok := walkUpForBackend(wd); ok {
			return root, nil
		}
	}

	return "", fmt.Errorf("could not locate overlay repo root (bin/labdrian-overlay not found); set OVERLAY_DIR")
}

func hasBackend(root string) bool {
	info, err := os.Stat(filepath.Join(root, "bin", "labdrian-overlay"))
	return err == nil && !info.IsDir()
}

func walkUpForBackend(start string) (string, bool) {
	dir := start
	for {
		if hasBackend(dir) {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// commandResult is the outcome of running the backend for one or more targets.
type commandResult struct {
	action   Action
	targets  []Target
	output   string // combined stdout+stderr across invocations
	verdicts []TargetVerdict
	err      error
	// exitCode captures the process exit code when err is non-nil. It
	// distinguishes a hard failure (code 1) from a degraded/warning result
	// (code 2, e.g. 'engine status' DEGRADED). Zero when err is nil.
	exitCode int
}

// buildArgSets constructs the argument sets to pass to the backend binary.
//
// Routing priority:
//  1. TargetAgnostic: single invocation with NO --target. The three hooks
//     subcommands (status-hooks, install-hooks, uninstall-hooks) operate on
//     ~/.claude/settings.json globally and do not accept --target. Verified
//     against bin/labdrian-overlay: cmd_status_hooks, cmd_install_hooks, and
//     cmd_uninstall_hooks receive "$@" from the dispatcher but parse no
//     arguments; extra flags are silently ignored. We omit --target anyway
//     because it is semantically incorrect and future-proofs against stricter
//     argument parsing.
//  2. SupportsAll + allSelected: single `--target all` invocation.
//  3. Default: one invocation per selected target.
func buildArgSets(action Action, selected []Target, allSelected bool) [][]string {
	switch {
	case action.TargetAgnostic:
		return [][]string{append([]string{action.Command}, action.Args...)}
	case action.SupportsAll && allSelected:
		return [][]string{{action.Command, "--target", "all"}}
	default:
		var sets [][]string
		for _, t := range selected {
			sets = append(sets, []string{action.Command, "--target", t.Name})
		}
		return sets
	}
}

// allArgSets flattens the primary action's arg sets with those of every
// nested Also action, each routed through buildArgSets independently.
func allArgSets(action Action, selected []Target, allSelected bool) [][]string {
	sets := buildArgSets(action, selected, allSelected)
	for _, sub := range action.Also {
		sets = append(sets, buildArgSets(sub, selected, allSelected)...)
	}
	return sets
}

// invocationSeverity ranks a single invocation's outcome so runBackend can
// aggregate multiple invocations (primary action plus any merged Also
// actions, which may be structurally different backend subcommands with
// different exit-code contracts) by worst outcome rather than by which one
// ran last. Exit code 2 ("degraded") ranks below any other non-nil error
// ("hard failure"); a nil err ranks lowest of all.
func invocationSeverity(err error, exitCode int) int {
	switch {
	case err == nil:
		return 0
	case exitCode == 2:
		return 1 // degraded
	default:
		return 2 // hard failure
	}
}

// runBackend executes the backend action for the selected targets.
//
// When the action supports `all` and every target is selected, it issues a
// single `--target all` invocation. Otherwise it iterates per target (this is
// required for `capture`, which rejects `--target all`). For TargetAgnostic
// actions (hooks lifecycle), no --target is emitted at all. Nested Also
// actions contribute their own invocations, routed and appended after the
// primary action's.
func runBackend(root string, action Action, selected []Target) commandResult {
	res := commandResult{action: action, targets: selected}
	bin := filepath.Join(root, "bin", "labdrian-overlay")

	allSelected := len(selected) == len(AllTargets())
	argSets := allArgSets(action, selected, allSelected)

	var sb strings.Builder
	severity := 0
	for i, args := range argSets {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(fmt.Sprintf("$ bin/labdrian-overlay %s\n", strings.Join(args, " ")))

		cmd := exec.Command(bin, args...)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		sb.Write(out)
		if err != nil {
			sb.WriteString(fmt.Sprintf("\n[exit error: %v]\n", err))
			// Extract the exit code so callers can distinguish a hard failure
			// (exit 1) from a degraded/warning result (exit 2).
			exitCode := -1
			if ee, ok := err.(*exec.ExitError); ok {
				exitCode = ee.ExitCode()
			}
			// Aggregate by worst severity seen across all invocations in this
			// commandResult, not "last failing invocation wins": a later
			// degraded (exit 2) invocation must never mask an earlier hard
			// failure (or vice versa) once the Also merge concatenates
			// invocations from different backend subcommands into this loop.
			if sev := invocationSeverity(err, exitCode); sev > severity {
				severity = sev
				res.err = err
				res.exitCode = exitCode
			}
		}
	}

	res.output = sb.String()
	if action.Command == "sync-check" {
		res.verdicts = ParseSyncCheck(res.output)
	}
	return res
}
