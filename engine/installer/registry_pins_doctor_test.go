package installer_test

// The registry pins of `doctor` and of the commands that refresh the registry first (the world is
// registry_pins_world_test.go). Doctor says many things that belong to the machine it runs on (whether
// bat is installed, whether the engine is older than its source); a pin holds what it says of the
// registry, how it ends, and the line that sums it up.

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

// isCheckLine says whether line is the line of a check of doctor: it begins with the level of a check,
// PASS, WARN or FAIL, and two spaces. The lines a check wraps under its own (advice, an indented
// continuation) do not.
func isCheckLine(line string) bool {
	for _, level := range []string{"PASS  ", "WARN  ", "FAIL  "} {
		if strings.HasPrefix(line, level) {
			return true
		}
	}
	return false
}

// isRegistryCheckLine says whether line is the line of the check of the registry: a line of a check
// that speaks of the registry. Not any line that does: the advice doctor wraps under its line about
// the tools of discovery, when the machine lacks one, names the registry too, and is not the
// registry's.
func isRegistryCheckLine(line string) bool {
	return isCheckLine(line) && strings.Contains(strings.ToLower(line), "registry")
}

// doctorRegistryPin is what a run of doctor says of the registry: how it ended, the lines of its
// check, the line that sums the run up (it is on stderr when a check failed), and every
// check that failed, whatever it checked. The last tells which check made a run fail: a failed
// run whose only failed check is the registry's line is the registry's doing, and a failed check
// of anything else shows in the pin and does not hide behind the exit code and the summary.
func doctorRegistryPin(run pinRun) string {
	var registry, summary, failed []string
	for _, line := range strings.Split(run.Stdout+run.Stderr, "\n") {
		switch {
		case strings.HasPrefix(line, "doctor:"):
			summary = append(summary, line)
		case isRegistryCheckLine(line):
			registry = append(registry, line)
		}
		if strings.HasPrefix(line, "FAIL  ") {
			failed = append(failed, line)
		}
	}
	return "exit " + strconv.Itoa(run.Exit) + "\n--- registry\n" + linesText(registry) +
		"--- summary\n" + linesText(summary) + "--- failed checks\n" + linesText(failed)
}

// linesText is lines as a pin records them, one to a line.
func linesText(lines []string) string {
	if len(lines) == 0 {
		return pinText("")
	}
	return pinText(strings.Join(lines, "\n") + "\n")
}

func TestRegistryPins_DoctorSaysWhatItFindsOfTheRegistry(t *testing.T) {
	const blocks = "<!-- BEGIN: minimalism-contract-scope (auto-generated) -->\n<!-- BEGIN: anti-generic-design-scope (auto-generated) -->\n"
	for _, tc := range []struct {
		name    string
		overlay *string // the registry of the overlay repository, nil for none
		project *string // the registry of the directory doctor runs in
	}{
		{"none-anywhere", nil, nil},
		{"project-only", nil, str("# registry\n| `go-testing` |\n")},
		{"overlay-only", str("# registry\n"), nil},
		{"both-and-the-overlay-wins", str("# overlay registry\n"), str("# project registry\n")},
		{"overlay-empty-and-project-fine", str(""), str("# project registry\n")},
		{"overlay-fine-and-project-empty", str("# overlay registry\n"), str("")},
		{"zero-bytes-in-the-overlay", str(""), nil},
		{"zero-bytes-in-the-project", nil, str("")},
		{"white-space-only", nil, str(" \n\t\n")},
		{"a-single-newline", nil, str("\n")},
		{"no-scoped-block", nil, str("# registry\n## Skills\n")},
		{"both-scoped-blocks", nil, str("# registry\n" + blocks)},
		{"one-scoped-block", nil, str("# registry\n<!-- BEGIN: minimalism-contract-scope (auto-generated) -->\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := registryWorld(t)
			if tc.overlay != nil {
				overlaytest.WriteFile(w.t, w.overlay, ".atl/skill-registry.md", *tc.overlay)
			}
			if tc.project != nil {
				w.writeRegistry(*tc.project)
			}
			pin(t, "doctor/registry-"+tc.name, doctorRegistryPin(w.runIn(w.project(), "doctor")))
		})
	}
}

func str(s string) *string { return &s }

func TestRegistryPins_DoctorLooksPastARegistryThatIsADirectory(t *testing.T) {
	w := registryWorld(t)
	if err := os.MkdirAll(filepath.Join(w.overlay, ".atl", "skill-registry.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	w.writeRegistry("# project registry\n")
	pin(t, "doctor/registry-overlay-one-is-a-directory", doctorRegistryPin(w.runIn(w.project(), "doctor")))
}

// A registry that cannot be read fails doctor. The script used to stop where it read it (the shell
// stopped on the failed read, with bash's own words and the line of the script, so what it said was
// not pinned); the engine now says "cannot read" and doctor goes on to sum up.
func TestRegistryPins_DoctorFailsOnARegistryItCannotRead(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("a registry cannot be made unreadable to root")
	}
	w := registryWorld(t)
	w.writeRegistry("# registry\n")
	if err := os.Chmod(w.registryPath(), 0o000); err != nil {
		t.Fatal(err)
	}
	run := w.runIn(w.project(), "doctor")
	if run.Exit != 1 || strings.Contains(run.Stdout+run.Stderr, "doctor: OK") {
		t.Errorf("doctor of a registry it cannot read = %s, want it to fail and not to say the checks passed", pinRunText(run))
	}
	if lines := registryLines(run); len(lines) != 1 || !strings.HasPrefix(lines[0], "FAIL  registry: $HOME/proj/.atl/skill-registry.md — cannot read: ") {
		t.Errorf("registry lines = %q, want one FAIL that says the registry cannot be read", lines)
	}
}

func TestRegistryPins_RefreshRepairsTheRegistryItRegenerates(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"by-default", nil},
		{"for-claude", []string{"--target", "claude"}},
		{"with-force-and-quiet", []string{"--force", "--quiet"}},
		{"with-no-gitignore-said", []string{"--no-gitignore"}},
		{"without-repair", []string{"--no-repair"}},
		{"with-cwd-twice", []string{"--cwd", "$HOME/other", "--cwd", "$PROJECT"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := registryWorld(t)
			w.installFew("codex")
			w.installFew("claude")
			g := w.installGentle(gentleRegistry, 0)
			args := []string{"skill-registry", "refresh"}
			if tc.args == nil || tc.args[0] != "--cwd" {
				args = append(args, "--cwd", w.project())
			}
			for _, a := range tc.args {
				args = append(args, strings.NewReplacer("$HOME", w.home, "$PROJECT", w.project()).Replace(a))
			}
			if err := os.MkdirAll(filepath.Join(w.home, "other"), 0o755); err != nil {
				t.Fatal(err)
			}
			run := w.runIn(w.home, args...)
			pin(t, "refresh/"+tc.name, pinRunText(run)+"--- gentle-ai\n"+w.gentleCalls(g)+w.showRegistry(w.registryPath()))
		})
	}
}

func TestRegistryPins_RepairRefreshesFirstUnlessItIsToldNotTo(t *testing.T) {
	w := registryWorld(t)
	w.installFew("codex")
	g := w.installGentle(gentleRegistry, 0)
	run := w.runIn(w.project(), "repair-sdd-registry")
	pin(t, "refresh/repair-refreshes-first", pinRunText(run)+"--- gentle-ai\n"+w.gentleCalls(g)+w.showRegistry(w.registryPath()))

	w = registryWorld(t)
	w.installFew("codex")
	g = w.installGentle(gentleRegistry, 0)
	run = w.runIn(w.project(), "repair-sdd-registry", "--target", "codex", "--cwd", w.project())
	pin(t, "refresh/repair-refreshes-first-with-cwd", pinRunText(run)+"--- gentle-ai\n"+w.gentleCalls(g)+w.showRegistry(w.registryPath()))
}

func TestRegistryPins_ARefreshThatFailsLeavesTheRegistryAlone(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"repair-sdd-registry", []string{"repair-sdd-registry"}},
		{"skill-registry-refresh", []string{"skill-registry", "refresh"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := registryWorld(t)
			w.installFew("codex")
			w.writeRegistry(gentleRegistry)
			g := w.installGentle("# regenerated, and then it failed\n", 3)
			args := append(append([]string(nil), tc.args...), "--cwd", w.project())
			run := w.runIn(w.project(), args...)
			pin(t, "refresh/fails-"+tc.name, pinRunText(run)+"--- gentle-ai\n"+w.gentleCalls(g)+w.showRegistry(w.registryPath()))
		})
	}
}

func TestRegistryPins_WithoutGentleAiTheRefreshStopsAndNoRefreshIsNotAsked(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"repair-sdd-registry", []string{"repair-sdd-registry"}},
		{"skill-registry-refresh", []string{"skill-registry", "refresh"}},
		{"repair-sdd-registry-no-refresh", []string{"repair-sdd-registry", "--no-refresh"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := registryWorld(t)
			w.installFew("codex")
			path := pathWithout(t, "gentle-ai")
			w.changePath(func(string) string { return path })
			run := w.runIn(w.project(), tc.args...)
			pinRepair(t, w, "refresh/without-gentle-ai-"+tc.name, run)
		})
	}
}

func TestRegistryPins_RefreshRefusesWhatItIsNotAskedRight(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"no-verb", []string{"skill-registry"}},
		{"unknown-verb", []string{"skill-registry", "regenerate"}},
		{"unknown-option", []string{"skill-registry", "refresh", "--frobnicate"}},
		{"unknown-target", []string{"skill-registry", "refresh", "--target", "vim"}},
		{"package-target", []string{"skill-registry", "refresh", "--target", "pi"}},
		{"target-without-a-value", []string{"skill-registry", "refresh", "--target"}},
		{"cwd-without-a-value", []string{"skill-registry", "refresh", "--cwd"}},
		{"cwd-that-is-not-a-directory", []string{"skill-registry", "refresh", "--cwd", "$HOME/not-there"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := registryWorld(t)
			g := w.installGentle(gentleRegistry, 0)
			args := make([]string, len(tc.args))
			for i, a := range tc.args {
				args[i] = strings.ReplaceAll(a, "$HOME", w.home)
			}
			run := w.runIn(w.project(), args...)
			pin(t, "refresh/refused-"+tc.name, pinRunText(run)+"--- gentle-ai\n"+w.gentleCalls(g)+w.showRegistry(w.registryPath()))
		})
	}
}
