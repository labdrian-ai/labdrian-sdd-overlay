package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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
}

// Actions returns the action menu in display order.
//
// Operational actions (target-bound) are listed first under the
// "── Sincronización ──" header; hooks actions (TargetAgnostic) follow under
// the "── Hooks (global ~/.claude) ──" header rendered by viewActions.
func Actions() []Action {
	return []Action{
		{Name: "Estado", Command: "status", Mutating: false, SupportsAll: true,
			Hint: "Resumen del estado actual"},
		{Name: "Verificar sincronización", Command: "sync-check", Mutating: false, SupportsAll: true,
			Hint: "Compara overlay vs upstream"},
		{Name: "Aplicar cambios", Command: "apply", Mutating: true, SupportsAll: true,
			Hint: "Despliega el overlay en los destinos"},
		{Name: "Capturar (actualizar upstream)", Command: "capture", Mutating: true, SupportsAll: false,
			Hint: "Trae cambios de upstream al overlay"},
		// Hooks lifecycle — TargetAgnostic: operate on ~/.claude/settings.json globally.
		{Name: "Estado de hooks", Command: "status-hooks", Mutating: false, TargetAgnostic: true,
			Hint: "Muestra si los hooks están instalados"},
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
		{Name: "Validar skills", Command: "skills", Args: []string{"validate"},
			TargetAgnostic: true, Mutating: false,
			Hint: "Valida el registro de skills"},
		{Name: "Listar skills", Command: "skills", Args: []string{"list"},
			TargetAgnostic: true, Mutating: false,
			Hint: "Lista los skills del overlay"},
		{Name: "Estado skills", Command: "skills", Args: []string{"status"},
			TargetAgnostic: true, Mutating: false,
			Hint: "Estado del registro de skills"},
		{Name: "Actualizar registry SDD Codex", Command: "skill-registry", Args: []string{"refresh", "--target", "codex"},
			TargetAgnostic: true, Mutating: true,
			Hint: "Refresh + executors sdd-* de Codex"},
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
	// SyncBehindOrigin means local HEAD is behind this repo's own origin/main
	// (informational drift; never overridden by, and never masks, the other
	// two verdicts — see classify's precedence and R-006).
	SyncBehindOrigin
)

// TargetVerdict holds the parsed sync-check result for one target.
type TargetVerdict struct {
	Target             string
	UpstreamChanged    int
	OverlayNotDeployed int
	RepoBehindOrigin   int // count; -1 = unavailable (no remote/no cached ref/fetch failed)
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
			// RepoBehindOrigin defaults to -1 (unavailable) until a VERDICT
			// line explicitly sets it — mirrors the backend's NA sentinel and
			// avoids a defensive zero-value collapsing into "0 behind".
			v = &TargetVerdict{Target: name, RepoBehindOrigin: -1}
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
						v.RepoBehindOrigin = -1
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

// runBackend executes the backend action for the selected targets.
//
// When the action supports `all` and every target is selected, it issues a
// single `--target all` invocation. Otherwise it iterates per target (this is
// required for `capture`, which rejects `--target all`). For TargetAgnostic
// actions (hooks lifecycle), no --target is emitted at all.
func runBackend(root string, action Action, selected []Target) commandResult {
	res := commandResult{action: action, targets: selected}
	bin := filepath.Join(root, "bin", "labdrian-overlay")

	allSelected := len(selected) == len(AllTargets())
	argSets := buildArgSets(action, selected, allSelected)

	var sb strings.Builder
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
			res.err = err
			// Extract the exit code so callers can distinguish a hard failure
			// (exit 1) from a degraded/warning result (exit 2).
			if ee, ok := err.(*exec.ExitError); ok {
				res.exitCode = ee.ExitCode()
			} else {
				res.exitCode = -1
			}
		}
	}

	res.output = sb.String()
	if action.Command == "sync-check" {
		res.verdicts = ParseSyncCheck(res.output)
	}
	return res
}
