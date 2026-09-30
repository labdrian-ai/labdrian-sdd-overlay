package main

// The documentation of the skills locks: README, the engine usage text, and the
// wrapper's help. They pin what a user must be told: which verbs lock, where the
// lock file is, and that a busy lock is exit 2 (retry) and not exit 1 (refused).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocsDescribeTheSkillsLockAndTheBusyExit(t *testing.T) {
	readme, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	overlay, err := os.ReadFile(filepath.Join("..", "..", "bin", "labdrian-overlay"))
	if err != nil {
		t.Fatalf("read bin/labdrian-overlay: %v", err)
	}
	// Each document's own description of the skills verbs, not the rest of the text.
	sections := map[string]string{
		"README":  docSection(t, string(readme), "overlay skills <verb>\n", "overlay shaper <verb>\n"),
		"usage()": docSection(t, captureUsage(t), "  engine skills <verb>", "  engine skills guard-hook\n"),
		"help":    docSection(t, string(overlay), "  skills <verb>                    Forward", "  shaper <verb>                    Forward"),
	}
	for name, text := range sections {
		for _, want := range []string{
			".skills.registry.yaml.lock",
			"exclusive",
			"shared",
			"another skills command is in progress",
			"exit 2",
			"retry",
			"never removed",
			// The project lock: which verbs, on what, and that no file is created.
			"project-register, project-revise, project-retire, install and adopt",
			"project root directory itself",
			"no file is created in the project",
			"overlay lock first",
			"project-status takes a shared lock",
			// Two locks can each be waited for, so the wait can be twice the bound.
			"twice the lock bound",
			// What install does with what it does not own, and adopt.
			".agents/skills",
			"only what it installed",
			"hand-edited",
			"skills adopt",
			"unchanged: <id>",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("%s does not say %q", name, want)
			}
		}
	}
}
