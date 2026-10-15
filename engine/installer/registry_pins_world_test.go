package installer_test

// The world the registry pins run in. The pins are in registry_pins_repair_test.go (what
// `repair-sdd-registry` does to a project's .atl/skill-registry.md) and registry_pins_doctor_test.go
// (what `skill-registry refresh` and the refresh of `repair-sdd-registry` do around the repair, and
// what `doctor` says of the registry). They record what bin/labdrian-overlay did, so that where the
// script gets the answer from can change without the program changing; they were recorded from
// the script as it was before the repair moved into the engine (`registry repair-sdd`), and they
// hold what that program did as it did it, but for the lines a later commit changed on purpose
// (the mode of the registry, the registry line of doctor), whose diffs are in the history of
// testdata/registry_pins.golden. The same world serves two tests of what the script does now,
// registry_engine_absent_test.go (no engine to repair with) and registry_doctor_engine_test.go
// (what doctor asks the engine and does with its answer).
//
// The world is the routing pins' (routing_pins_world_test.go): an overlay repository and a home
// that has the engine. The project the registry belongs to is $HOME/proj.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sddExecutors are the skills the script keeps rows for, in the order of its rows.
var sddExecutors = []string{
	"sdd-apply", "sdd-archive", "sdd-design", "sdd-explore", "sdd-init", "sdd-onboard",
	"sdd-propose", "sdd-spec", "sdd-tasks", "sdd-time-estimation", "sdd-verify",
}

// skillsRoots are where each copy target keeps its skills, under the home of the world.
var skillsRoots = map[string]string{
	"claude":   ".claude/skills",
	"opencode": ".config/opencode/skills",
	"codex":    ".codex/skills",
}

// project is the directory the registry belongs to.
func (w *pinWorld) project() string { return filepath.Join(w.home, "proj") }

// registryPath is the registry of the project.
func (w *pinWorld) registryPath() string {
	return filepath.Join(w.project(), ".atl", "skill-registry.md")
}

// writeRegistry puts content in the registry of the project, with the mode a person's editor would.
func (w *pinWorld) writeRegistry(content string) {
	w.t.Helper()
	w.write(w.project(), ".atl/skill-registry.md", content)
}

// installSkill puts a skill with the given text in the skills of a target.
func (w *pinWorld) installSkill(target, name, text string) {
	w.t.Helper()
	w.write(w.home, filepath.Join(skillsRoots[target], name, "SKILL.md"), text)
}

// installExecutors installs every SDD executor skill for the target, each with the description the
// function gives it ("" for a skill with no description line).
func (w *pinWorld) installExecutors(target string, description func(name string) string) {
	w.t.Helper()
	for _, name := range sddExecutors {
		text := "---\nname: " + name + "\n"
		if d := description(name); d != "" {
			text += "description: " + d + "\n"
		}
		w.installSkill(target, name, text+"---\n\n# "+name+"\n")
	}
}

// installFew installs two of the executors, as a machine that has some of them has them.
func (w *pinWorld) installFew(target string) {
	w.t.Helper()
	for _, name := range []string{"sdd-apply", "sdd-verify"} {
		w.installSkill(target, name, "---\nname: "+name+"\ndescription: "+plainDescription(name)+"\n---\n")
	}
}

// plainDescription is the description of a skill that is installed as it should be.
func plainDescription(name string) string { return "Runs " + name + " for the team." }

// showRegistry is what a run left at path: the mode, and the content with the places of the world
// named and each carriage return spelled out, or that there is none. A symbolic link is said to be
// one.
func (w *pinWorld) showRegistry(path string) string {
	w.t.Helper()
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return "--- registry\n(absent)\n"
	}
	if err != nil {
		w.t.Fatal(err)
	}
	kind := "file"
	if info.Mode()&os.ModeSymlink != 0 {
		kind = "symbolic link"
	}
	body, err := os.ReadFile(path)
	if err != nil {
		w.t.Fatal(err)
	}
	text := strings.ReplaceAll(w.normalize(string(body)), "\r", `\r`)
	return fmt.Sprintf("--- registry (%s, mode %04o)\n%s", kind, info.Mode().Perm(), pinText(text))
}

// fakeGentle is a `gentle-ai` first on the PATH of the world that records its arguments, one call
// a line, writes the registry it is given at --cwd (as the real refresh regenerates it, whole) and
// ends with the code it is given. The world has one from the start that does nothing, so that a
// command that refreshes can never reach the real binary of the machine.
type fakeGentle struct {
	log string
	dir string // the directory the fake is in, first on the PATH it was put on
}

// installGentle puts a fake gentle-ai first on the PATH. regenerated, when it is not empty, is the
// text the fake leaves at <--cwd>/.atl/skill-registry.md; exit is the code it ends with.
func (w *pinWorld) installGentle(regenerated string, exit int) fakeGentle {
	w.t.Helper()
	dir := w.t.TempDir()
	log := filepath.Join(dir, "gentle.log")
	body := filepath.Join(dir, "regenerated.md")
	if err := os.WriteFile(body, []byte(regenerated), 0o644); err != nil {
		w.t.Fatal(err)
	}
	script := "#!/bin/sh\n" +
		"echo \"$*\" >> \"$FAKE_GENTLE_LOG\"\n" +
		"cwd=\n" +
		"while [ $# -gt 0 ]; do\n" +
		"  if [ \"$1\" = \"--cwd\" ]; then cwd=\"$2\"; fi\n" +
		"  shift\n" +
		"done\n" +
		"if [ -s \"$FAKE_GENTLE_REGISTRY\" ] && [ -n \"$cwd\" ]; then\n" +
		"  mkdir -p \"$cwd/.atl\"\n" +
		"  cp \"$FAKE_GENTLE_REGISTRY\" \"$cwd/.atl/skill-registry.md\"\n" +
		"fi\n" +
		"exit " + fmt.Sprint(exit) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "gentle-ai"), []byte(script), 0o755); err != nil {
		w.t.Fatalf("write the fake gentle-ai: %v", err)
	}
	w.env = append(w.env, "FAKE_GENTLE_LOG="+log, "FAKE_GENTLE_REGISTRY="+body)
	w.changePath(func(old string) string { return dir + string(os.PathListSeparator) + old })
	return fakeGentle{log: log, dir: dir}
}

// gentleCalls are the calls a fake gentle-ai recorded, normalized, in order; none is nothing.
func (w *pinWorld) gentleCalls(g fakeGentle) string { return w.loggedCalls(g.log) }

// loggedCalls are the calls a fake program recorded in the log at path, normalized, in order; none
// is nothing.
func (w *pinWorld) loggedCalls(log string) string {
	w.t.Helper()
	body, err := os.ReadFile(log)
	if os.IsNotExist(err) {
		return pinText("")
	}
	if err != nil {
		w.t.Fatal(err)
	}
	return pinText(w.normalize(string(body)))
}

// discoveryTools are the tools of discovery doctor looks for on the PATH (DISCOVERY_TOOLS in the
// script; a test holds that the two are the same).
var discoveryTools = []string{"bat", "rg", "fd", "sd", "eza"}

// installDiscoveryTools puts a program for each tool of discovery first on the PATH of the world, so
// that what doctor says of them is the same on every machine. A machine that lacks one gets, from
// doctor, a line about it and advice wrapped under it, which names the registry; and which tools a
// machine has is not the business of a test of what doctor says of the registry. The programs do
// nothing; doctor only looks for them.
func (w *pinWorld) installDiscoveryTools() {
	w.t.Helper()
	dir := w.t.TempDir()
	for _, tool := range discoveryTools {
		if err := os.WriteFile(filepath.Join(dir, tool), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			w.t.Fatalf("write the %s of the world: %v", tool, err)
		}
	}
	// From here every change of the PATH of the world keeps this directory first (changePath), this
	// one included: the PATH it is given is the world's own, with the directory put in front.
	w.discoveryDir = dir
	w.changePath(func(old string) string { return old })
}

// registryWorld is a world whose PATH has a gentle-ai that only records and the five tools of
// discovery, a project with no registry yet, and no skill installed.
func registryWorld(t *testing.T) *pinWorld {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	w := newPinWorld(t, "skill-a/SKILL.md managed\n", map[string]string{"skills/skill-a/SKILL.md": "# skill-a\n"})
	w.installGentle("", 0)
	w.installDiscoveryTools()
	if err := os.MkdirAll(w.project(), 0o755); err != nil {
		t.Fatal(err)
	}
	return w
}
