package main

// The documentation of the skills approve guard: README, the engine usage text,
// and the wrapper's install-hooks and uninstall-hooks summaries. Like the
// projection hook's documentation test, they pin what a user must be told (what
// the guard does, that it is a speed bump, and that install-hooks must be re-run
// and Claude Code restarted) and keep a hook-family count from going stale.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// normalizeDocText collapses white space and drops backticks, so a wrapped or
// code-formatted sentence still matches.
func normalizeDocText(raw string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(raw, "`", "")), " ")
}

// The guard is installed by install-hooks, so the documentation must say what it
// does, that it is a speed bump and not a security boundary, and what a user
// must do to get it: re-run install-hooks on an existing install and restart
// Claude Code.
func TestDocsDescribeTheApproveGuard(t *testing.T) {
	readme, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	// Each document's own description of the hook, not the rest of the text: the
	// projection hook's paragraph says the same about restarting Claude Code.
	sections := map[string]string{
		"README":  docSection(t, string(readme), "gentle-ai-overlay skills guard-hook\n", "overlay --help\n"),
		"usage()": docSection(t, captureUsage(t), "  engine skills guard-hook\n", "  engine sync-trigger"),
	}
	for name, text := range sections {
		for _, want := range []string{
			"skills guard-hook",
			"denies the agent",
			"speed bump",
			"not a security boundary",
			"re-run install-hooks",
			"restart Claude Code",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("%s does not say %q", name, want)
			}
		}
	}
}

// docSection returns the normalized text from the first occurrence of start up
// to the next occurrence of end after it.
func docSection(t *testing.T, whole, start, end string) string {
	t.Helper()
	i := strings.Index(whole, start)
	if i < 0 {
		t.Fatalf("marker %q not found", start)
	}
	j := strings.Index(whole[i+len(start):], end)
	if j < 0 {
		t.Fatalf("marker %q not found after %q", end, start)
	}
	return normalizeDocText(whole[i : i+len(start)+j])
}

// The install-hooks and uninstall-hooks summaries name every family the verbs
// manage, and give no count that goes stale each time a family is added.
func TestInstallHooksSummariesNameEveryFamilyWithoutCounting(t *testing.T) {
	readme, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	overlay, err := os.ReadFile(filepath.Join("..", "..", "bin", "labdrian-overlay"))
	if err != nil {
		t.Fatalf("read bin/labdrian-overlay: %v", err)
	}
	sections := map[string]string{
		"README install-hooks":    docSection(t, string(readme), "overlay install-hooks\n", "overlay uninstall-hooks\n"),
		"README uninstall-hooks":  docSection(t, string(readme), "overlay uninstall-hooks\n", "overlay status-hooks\n"),
		"install-hooks summary":   docSection(t, string(overlay), `echo "install-hooks complete."`, "Run 'labdrian doctor'"),
		"uninstall-hooks summary": docSection(t, string(overlay), `echo "uninstall-hooks complete."`, "Run 'labdrian doctor'"),
	}
	for name, text := range sections {
		for _, stale := range []string{"three hook families", "five entries", "five hook entries", "two pairs + SessionEnd"} {
			if strings.Contains(text, stale) {
				t.Errorf("%s still says %q", name, stale)
			}
		}
		for _, want := range []string{"projection", "approve guard"} {
			if !strings.Contains(text, want) {
				t.Errorf("%s does not mention the %s family", name, want)
			}
		}
	}
	for _, name := range []string{"README install-hooks", "install-hooks summary"} {
		if text := sections[name]; !strings.Contains(text, "restart Claude Code") {
			t.Errorf("%s does not say Claude Code must be restarted to load hooks", name)
		}
	}
	// The wrapper's own --help text says the same of install-hooks.
	help := docSection(t, string(overlay), "  install-hooks                    Build + deploy", "  uninstall-hooks ")
	if !strings.Contains(help, "approve guard") || !strings.Contains(help, "restart Claude Code") {
		t.Errorf("--help for install-hooks does not name the approve guard and the restart: %q", help)
	}
}
