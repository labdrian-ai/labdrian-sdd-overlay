package runtime

import (
	"path/filepath"
	"strings"
)

// Config is what a runtime adapter is given about the machine it runs on. The composition root
// reads the environment once and fills it; an adapter never reads the environment itself, so the
// same adapter can be pointed at a temporary directory by a test or at another machine's layout
// by a caller.
//
// Every field is a value as the environment gave it, or empty when it gave none. The rules that
// turn them into directories (a relative XDG_CONFIG_HOME is ignored, a blank home is no home) are
// the methods below, so they sit with the adapters that need them and need no file system.
type Config struct {
	// Home is the home directory of the person the process runs as, empty when it cannot be
	// determined. An empty home gives an adapter no default location rather than a guessed one.
	Home string
	// XDGConfigHome is $XDG_CONFIG_HOME as set, used only by the OpenCode root.
	XDGConfigHome string
	// CodexHome is $CODEX_HOME as set, used only by the Codex root.
	CodexHome string
	// OverlayDir is $OVERLAY_DIR as set: the overlay checkout the Pi package is built from.
	OverlayDir string
	// StateDir is $STATE_DIR as set: where the overlay keeps what it deploys.
	StateDir string
	// ConfigRoot is the --config-root the caller gave, empty when it gave none. It names the
	// directory an adapter keeps its files in: the config directory of Claude, Codex and
	// OpenCode, and, for Pi, the directory that holds the package, pi/labdrian-pi, the way
	// StateDir does. Pi's package never sits in the root itself: the same root may be given to
	// every runtime (`--target all`), and building or removing the package replaces its whole
	// directory, which must not be the one the other runtimes keep their settings in.
	ConfigRoot string
}

// DefaultClaudeRoot is ~/.claude, or empty without a home.
func (c Config) DefaultClaudeRoot() string {
	return underHome(c.Home, ".claude")
}

// DefaultCodexRoot is $CODEX_HOME when it is an absolute path, and ~/.codex otherwise.
func (c Config) DefaultCodexRoot() string {
	if dir := strings.TrimSpace(c.CodexHome); dir != "" {
		if clean := filepath.Clean(dir); filepath.IsAbs(clean) {
			return clean
		}
	}
	return underHome(c.Home, ".codex")
}

// DefaultOpenCodeRoot is $XDG_CONFIG_HOME/opencode when XDG_CONFIG_HOME is an absolute path, and
// ~/.config/opencode otherwise.
func (c Config) DefaultOpenCodeRoot() string {
	if dir := strings.TrimSpace(c.XDGConfigHome); dir != "" && filepath.IsAbs(dir) {
		return filepath.Join(dir, "opencode")
	}
	return underHome(c.Home, ".config", "opencode")
}

// DefaultStateDir is ~/.labdrian-overlay, the directory every overlay-owned record lives in, or
// empty without a home.
func (c Config) DefaultStateDir() string {
	return underHome(c.Home, ".labdrian-overlay")
}

// ClaudeRoot is the directory the Claude adapter keeps its settings in.
func (c Config) ClaudeRoot() string { return firstNonEmpty(c.ConfigRoot, c.DefaultClaudeRoot()) }

// CodexRoot is the directory the Codex adapter keeps its manifest in.
func (c Config) CodexRoot() string { return firstNonEmpty(c.ConfigRoot, c.DefaultCodexRoot()) }

// OpenCodeRoot is the directory the OpenCode adapter installs its plugin into.
func (c Config) OpenCodeRoot() string { return firstNonEmpty(c.ConfigRoot, c.DefaultOpenCodeRoot()) }

// PiPackageDir is where the Pi package is built and installed from: <base>/pi/labdrian-pi, the
// base being the --config-root when one was given, else $STATE_DIR, else the default state dir.
func (c Config) PiPackageDir() string {
	base := firstNonEmpty(c.ConfigRoot, c.StateDir, c.DefaultStateDir())
	return filepath.Join(base, "pi", "labdrian-pi")
}

// underHome joins elems under home, and is empty when there is no home.
func underHome(home string, elems ...string) string {
	if home == "" {
		return ""
	}
	return filepath.Join(append([]string{home}, elems...)...)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
