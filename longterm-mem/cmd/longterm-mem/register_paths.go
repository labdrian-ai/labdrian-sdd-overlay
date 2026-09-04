package main

import (
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
// convention: claude|opencode|codex select one, all expands to every
// currently-wired target — codex joined "all"'s expansion in 12a.6, once
// its writer (register.RegisterCodex) existed to receive it.
func registerExpandTarget(target string) ([]string, error) {
	switch target {
	case "claude", "opencode", "codex":
		return []string{target}, nil
	case "all":
		return []string{"claude", "opencode", "codex"}, nil
	default:
		return nil, fmt.Errorf("unknown --target %q (want claude|opencode|codex|all)", target)
	}
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
	default:
		return ""
	}
}
