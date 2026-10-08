package runtime_test

import (
	"path/filepath"
	"testing"

	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
)

func TestDefaultRootsAreUnderTheHome(t *testing.T) {
	cfg := engineRuntime.Config{Home: "/home/x"}
	for name, c := range map[string]struct{ got, want string }{
		"claude":   {cfg.DefaultClaudeRoot(), "/home/x/.claude"},
		"codex":    {cfg.DefaultCodexRoot(), "/home/x/.codex"},
		"opencode": {cfg.DefaultOpenCodeRoot(), "/home/x/.config/opencode"},
		"state":    {cfg.DefaultStateDir(), "/home/x/.labdrian-overlay"},
		"pi":       {cfg.PiPackageDir(), "/home/x/.labdrian-overlay/pi/labdrian-pi"},
	} {
		if filepath.ToSlash(c.got) != c.want {
			t.Errorf("%s root = %q, want %q", name, c.got, c.want)
		}
	}
}

func TestWithoutAHomeThereIsNoDefaultRoot(t *testing.T) {
	var cfg engineRuntime.Config
	for name, got := range map[string]string{
		"claude":     cfg.DefaultClaudeRoot(),
		"codex":      cfg.DefaultCodexRoot(),
		"opencode":   cfg.DefaultOpenCodeRoot(),
		"state":      cfg.DefaultStateDir(),
		"claude()":   cfg.ClaudeRoot(),
		"codex()":    cfg.CodexRoot(),
		"opencode()": cfg.OpenCodeRoot(),
	} {
		if got != "" {
			t.Errorf("%s root = %q without a home, want none", name, got)
		}
	}
	// The package directory keeps the relative shape it always had without a home; the adapter
	// then refuses to work in it, it is not a place to guess.
	if got := cfg.PiPackageDir(); filepath.ToSlash(got) != "pi/labdrian-pi" {
		t.Errorf("PiPackageDir() = %q without a home, want pi/labdrian-pi", got)
	}
}

func TestCodexHomeWinsOnlyWhenAbsolute(t *testing.T) {
	abs := filepath.Join(t.TempDir(), "codex-home")
	cases := map[string]struct{ codexHome, want string }{
		"absolute":          {abs, abs},
		"absolute, unclean": {abs + "/./sub/..", abs},
		"absolute, padded":  {"  " + abs + "  ", abs},
		"relative":          {"relative/codex", "/home/x/.codex"},
		"blank":             {"   ", "/home/x/.codex"},
		"unset":             {"", "/home/x/.codex"},
	}
	for name, c := range cases {
		got := engineRuntime.Config{Home: "/home/x", CodexHome: c.codexHome}.DefaultCodexRoot()
		if filepath.ToSlash(got) != filepath.ToSlash(c.want) {
			t.Errorf("%s: DefaultCodexRoot() = %q, want %q", name, got, c.want)
		}
	}
}

func TestXDGConfigHomeWinsOnlyWhenAbsolute(t *testing.T) {
	xdg := t.TempDir()
	cases := map[string]struct{ xdg, want string }{
		"absolute": {xdg, filepath.Join(xdg, "opencode")},
		"padded":   {"  " + xdg + " ", filepath.Join(xdg, "opencode")},
		"relative": {"relative/config", "/home/x/.config/opencode"},
		"blank":    {"  ", "/home/x/.config/opencode"},
		"unset":    {"", "/home/x/.config/opencode"},
	}
	for name, c := range cases {
		got := engineRuntime.Config{Home: "/home/x", XDGConfigHome: c.xdg}.DefaultOpenCodeRoot()
		if filepath.ToSlash(got) != filepath.ToSlash(c.want) {
			t.Errorf("%s: DefaultOpenCodeRoot() = %q, want %q", name, got, c.want)
		}
	}
}

func TestTheStateDirOverridesTheDefaultOfThePiPackage(t *testing.T) {
	cfg := engineRuntime.Config{Home: "/home/x", StateDir: "/srv/state"}
	if got := filepath.ToSlash(cfg.PiPackageDir()); got != "/srv/state/pi/labdrian-pi" {
		t.Errorf("PiPackageDir() = %q, want it under the state dir", got)
	}
}

// TestConfigRootNamesTheDirectoryEveryRuntimeWorksIn: --config-root replaces the default root of
// each runtime, and for Pi it is the package directory, as it is for `longterm-mem register`.
func TestConfigRootNamesTheDirectoryEveryRuntimeWorksIn(t *testing.T) {
	cfg := engineRuntime.Config{Home: "/home/x", XDGConfigHome: "/xdg", CodexHome: "/ch", StateDir: "/s", ConfigRoot: "/given"}
	for name, got := range map[string]string{
		"claude": cfg.ClaudeRoot(), "codex": cfg.CodexRoot(), "opencode": cfg.OpenCodeRoot(), "pi": cfg.PiPackageDir(),
	} {
		if got != "/given" {
			t.Errorf("%s root = %q with --config-root /given, want it", name, got)
		}
	}
}
