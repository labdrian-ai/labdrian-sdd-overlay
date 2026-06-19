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
	Name      string // label shown in the menu
	Command   string // bin/overlay subcommand
	Mutating  bool   // requires confirmation
	SupportsAll bool // can pass --target all when every target is selected
}

// Actions returns the action menu in display order.
func Actions() []Action {
	return []Action{
		{Name: "Estado", Command: "status", Mutating: false, SupportsAll: true},
		{Name: "Verificar sincronización", Command: "sync-check", Mutating: false, SupportsAll: true},
		{Name: "Aplicar cambios", Command: "apply", Mutating: true, SupportsAll: true},
		{Name: "Capturar (actualizar upstream)", Command: "capture", Mutating: true, SupportsAll: false},
	}
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
)

// TargetVerdict holds the parsed sync-check result for one target.
type TargetVerdict struct {
	Target            string
	UpstreamChanged   int
	OverlayNotDeployed int
	Action            string
	Status            SyncStatus
}

// classify maps verdict counts to a color status.
//
// Priority (matches the backend ACTION precedence):
//   - UPSTREAM_CHANGED > 0  -> RED   (gentle-ai synced, needs capture+apply)
//   - OVERLAY_NOT_DEPLOYED > 0 -> YELLOW (needs apply)
//   - otherwise -> GREEN (healthy)
func classify(upstreamChanged, overlayNotDeployed int) SyncStatus {
	switch {
	case upstreamChanged > 0:
		return SyncNeedsCapture
	case overlayNotDeployed > 0:
		return SyncNeedsApply
	default:
		return SyncHealthy
	}
}

// ParseSyncCheck extracts per-target verdicts from sync-check stdout.
//
// It reads the machine-friendly lines emitted by the backend:
//
//	VERDICT:<target>:UPSTREAM_CHANGED=N OVERLAY_NOT_DEPLOYED=M
//	ACTION:<target>: <text>
//
// Verdicts are returned in first-seen order.
func ParseSyncCheck(output string) []TargetVerdict {
	order := []string{}
	byTarget := map[string]*TargetVerdict{}

	get := func(name string) *TargetVerdict {
		v, ok := byTarget[name]
		if !ok {
			v = &TargetVerdict{Target: name}
			byTarget[name] = v
			order = append(order, name)
		}
		return v
	}

	scanner := bufio.NewScanner(strings.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

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
				n, _ := strconv.Atoi(val)
				switch key {
				case "UPSTREAM_CHANGED":
					v.UpstreamChanged = n
				case "OVERLAY_NOT_DEPLOYED":
					v.OverlayNotDeployed = n
				}
			}
			v.Status = classify(v.UpstreamChanged, v.OverlayNotDeployed)
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
//  2. walk up from the executable's directory looking for bin/overlay
//  3. walk up from the current working directory looking for bin/overlay
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

	return "", fmt.Errorf("could not locate overlay repo root (bin/overlay not found); set OVERLAY_DIR")
}

func hasBackend(root string) bool {
	info, err := os.Stat(filepath.Join(root, "bin", "overlay"))
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
	action    Action
	targets   []Target
	output    string // combined stdout+stderr across invocations
	verdicts  []TargetVerdict
	err       error
}

// runBackend executes the backend action for the selected targets.
//
// When the action supports `all` and every target is selected, it issues a
// single `--target all` invocation. Otherwise it iterates per target (this is
// required for `capture`, which rejects `--target all`).
func runBackend(root string, action Action, selected []Target) commandResult {
	res := commandResult{action: action, targets: selected}
	bin := filepath.Join(root, "bin", "overlay")

	allSelected := len(selected) == len(AllTargets())

	var argSets [][]string
	if action.SupportsAll && allSelected {
		argSets = [][]string{{action.Command, "--target", "all"}}
	} else {
		for _, t := range selected {
			argSets = append(argSets, []string{action.Command, "--target", t.Name})
		}
	}

	var sb strings.Builder
	for i, args := range argSets {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(fmt.Sprintf("$ bin/overlay %s\n", strings.Join(args, " ")))

		cmd := exec.Command(bin, args...)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		sb.Write(out)
		if err != nil {
			sb.WriteString(fmt.Sprintf("\n[exit error: %v]\n", err))
			res.err = err
		}
	}

	res.output = sb.String()
	if action.Command == "sync-check" {
		res.verdicts = ParseSyncCheck(res.output)
	}
	return res
}
