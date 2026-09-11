package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// This file holds `register`'s pure resolution rules -- which targets a
// --target value names, and where each runtime's configuration, the
// install-state record and the installed binary live when the
// corresponding flag is not given. They are separated from cmd_register.go
// so they can be pinned directly, without a config file, a temp directory
// or an exit code in the way: a default-path rule that silently resolves
// to the wrong place is invisible in an end-to-end test that passes
// --config-root, and an unresolvable one is only observable here as an
// empty string.

// registerExpandTarget expands --target's value into the ordered list of
// concrete targets to register, mirroring the runtime-parity --target
// convention: claude|opencode|codex|pi select one, all expands to every
// currently-wired per-runtime-config target — codex joined "all"'s
// expansion in 12a.6, once its writer (register.RegisterCodex) existed to
// receive it.
//
// pi is deliberately NOT part of "all"'s expansion here: unlike the other
// three, a missing pi package is not "a runtime this machine does not
// run" (register's own all-vs-explicit skip story) but "this machine built
// the package and never ran pi install" — a state piInstalled's own probe
// distinguishes, not a missing config file. cmdRegister appends "pi" to
// the expanded list itself, only when that probe says it looks installed
// (C6).
func registerExpandTarget(target string) ([]string, error) {
	switch target {
	case "claude", "opencode", "codex", "pi":
		return []string{target}, nil
	case "all":
		return []string{"claude", "opencode", "codex"}, nil
	default:
		return nil, fmt.Errorf("unknown --target %q (want claude|opencode|codex|pi|all)", target)
	}
}

// piInstalled probes, read-only, whether labdrian-pi looks genuinely
// installed rather than merely built: packageDir exists AND
// ~/.pi/agent/settings.json lists it under "packages" (C6, design.md A1).
// A built-but-never-`pi install`-ed package fails this probe on purpose —
// mcp.json existing is proof the package was built, not proof Pi will ever
// read it. This never reads or writes ~/.pi/agent/mcp.json itself; that
// file belongs to gentle-pi/pi-engram, not this probe.
func piInstalled(packageDir string) bool {
	if packageDir == "" {
		return false
	}
	if info, err := os.Stat(packageDir); err != nil || !info.IsDir() {
		return false
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return false
	}
	raw, err := os.ReadFile(filepath.Join(home, ".pi", "agent", "settings.json"))
	if err != nil {
		return false
	}
	var settings struct {
		Packages []string `json:"packages"`
	}
	if json.Unmarshal(raw, &settings) != nil {
		return false
	}
	for _, p := range settings.Packages {
		if p == packageDir {
			return true
		}
	}
	return false
}

// defaultRegisterStateDir returns ~/.labdrian-overlay/longterm-mem, the
// directory install-state.json lives in (D9's module-owned state file).
// An empty result means "unresolvable", never "use the current directory"
// -- cmd_register.go refuses on it rather than writing the ownership
// record wherever the process happens to be running.
func defaultRegisterStateDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".labdrian-overlay", "longterm-mem")
}

// defaultRegisterBinaryPath returns the documented persistent install
// path, ~/.labdrian-overlay/bin/longterm-mem. Empty means unresolvable,
// with the same fail-closed contract as defaultRegisterStateDir.
func defaultRegisterBinaryPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".labdrian-overlay", "bin", "longterm-mem")
}

// defaultRegisterConfigRoot resolves target's runtime config root when
// --config-root is not given: $HOME for claude (~/.claude.json is a direct
// child of $HOME, a sibling of ~/.claude/), $XDG_CONFIG_HOME/opencode (or
// ~/.config/opencode) for opencode, $CODEX_HOME (or ~/.codex) for codex.
// A relative $XDG_CONFIG_HOME or $CODEX_HOME is ignored rather than
// resolved against the current directory, since a config root that
// depends on where the command was invoked from is not a config root.
//
// This mirrors engine/runtime's DefaultOpenCodeConfigRoot/
// DefaultCodexConfigRoot resolution exactly, but is a deliberately
// independent re-implementation: longterm-mem and engine are separate Go
// modules (D4's "one writer per file" split), so this command cannot
// import engine's package to share the helper.
func defaultRegisterConfigRoot(target string) string {
	home, homeErr := os.UserHomeDir()
	switch target {
	case "claude":
		if homeErr != nil {
			return ""
		}
		return home
	case "opencode":
		if dir := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); dir != "" && filepath.IsAbs(dir) {
			return filepath.Join(dir, "opencode")
		}
		if homeErr != nil {
			return ""
		}
		return filepath.Join(home, ".config", "opencode")
	case "codex":
		if dir := strings.TrimSpace(os.Getenv("CODEX_HOME")); dir != "" {
			if clean := filepath.Clean(dir); filepath.IsAbs(clean) {
				return clean
			}
		}
		if homeErr != nil {
			return ""
		}
		return filepath.Join(home, ".codex")
	case "pi":
		// Mirrors engine/runtime.DefaultPiPackageDir exactly (D4: a
		// deliberately independent re-implementation, engine and
		// longterm-mem are separate Go modules) — configRoot IS the built
		// package directory for pi, not a user config root (C3).
		stateDir := strings.TrimSpace(os.Getenv("STATE_DIR"))
		if stateDir == "" {
			if homeErr != nil {
				return ""
			}
			stateDir = filepath.Join(home, ".labdrian-overlay")
		}
		return filepath.Join(stateDir, "pi", "labdrian-pi")
	default:
		return ""
	}
}
