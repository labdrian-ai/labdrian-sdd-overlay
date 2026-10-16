package installer_test

// What `doctor` says of the registry is what these tests look at, and doctor says many other things
// that belong to the machine it runs on. One of them speaks of the registry too: when a tool of
// discovery (bat, rg, fd, sd, eza) is not on the PATH, doctor wraps advice under its line about the
// tools ("Skill discovery MUST use the registry (.atl/skill-registry.md or engram), ..."). A runner of
// CI has not all of them and a developer's machine has, so the tests of the registry line failed there
// and passed here. Two things keep that from happening: the world pins the tools (it puts a program for
// each first on the PATH of the run, so doctor sees the same five on every machine), and the line of the
// registry check is the line that begins with the level of a check, not any line that names the registry.

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

// The output of doctor on a machine without some of the tools of discovery, with the line of the
// registry check among the rest.
const doctorWithoutDiscoveryTools = "labdrian doctor — host-toolchain preflight\n" +
	"\n" +
	"PASS  go on PATH\n" +
	"PASS  gentle-ai on PATH\n" +
	"WARN  not found: bat sd eza. Without these tools agents that run\n" +
	"      e.g. 'fd ... 2>/dev/null' get empty output (command not found, swallowed)\n" +
	"      and may misread it as zero skills — the silent-empty footgun.\n" +
	"      Run 'labdrian doctor --fix' to auto-install via Homebrew (if available).\n" +
	"      Skill discovery MUST use the registry (.atl/skill-registry.md or engram),\n" +
	"      never a bare filesystem scan. An empty result is INCONCLUSIVE, not zero skills.\n" +
	"PASS  engine binary present and current at $HOME/.claude/bin/gentle-ai-overlay\n" +
	"WARN  registry: $HOME/proj/.atl/skill-registry.md — present but scoped block(s) missing: anti-generic-design-scope\n" +
	"\n" +
	"doctor: OK — required checks passed (review any WARN above).\n"

func TestTheLineOfTheRegistryCheckIsACheckLineAndNotAdviceThatNamesTheRegistry(t *testing.T) {
	run := pinRun{Stdout: doctorWithoutDiscoveryTools}
	want := []string{"WARN  registry: $HOME/proj/.atl/skill-registry.md — present but scoped block(s) missing: anti-generic-design-scope"}

	if got := registryLines(run); !reflect.DeepEqual(got, want) {
		t.Errorf("registryLines = %q, want %q: the advice under another check names the registry and is not its line", got, want)
	}
	pin := doctorRegistryPin(run)
	if strings.Contains(pin, "Skill discovery") || !strings.Contains(pin, "--- registry\n"+want[0]+"\n--- summary\n") {
		t.Errorf("doctorRegistryPin = %q, want the registry section to hold only the line of the registry check", pin)
	}
}

// The line of a check that did not come out is a check line too, whatever its level.
func TestTheLineOfTheRegistryCheckHasAnyLevelOfACheck(t *testing.T) {
	for _, line := range []string{
		"PASS  registry: r — fine",
		"WARN  registry not checked: the engine at e did not answer",
		"FAIL  registry not checked: the engine at e ended with code 139",
		"FAIL  registry: r — present but EMPTY",
	} {
		if got := registryLines(pinRun{Stdout: "PASS  go on PATH\n" + line + "\n"}); !reflect.DeepEqual(got, []string{line}) {
			t.Errorf("registryLines of a run that says %q = %q", line, got)
		}
	}
	// And what is not a line of a check is not, indented or not.
	for _, line := range []string{
		"      Skill discovery MUST use the registry (.atl/skill-registry.md or engram),",
		"registry: r — fine",
		"doctor: FAIL — one or more required checks did not pass (see above).",
		"PASS engine binary (a single space is not the separator of a check line)",
	} {
		if got := registryLines(pinRun{Stdout: line + "\n"}); len(got) != 0 {
			t.Errorf("registryLines of a run that says %q = %q, want none", line, got)
		}
	}
}

// discoveryToolsOfTheScript is DISCOVERY_TOOLS as the script spells it.
func discoveryToolsOfTheScript(t *testing.T) []string {
	t.Helper()
	m := regexp.MustCompile(`(?m)^DISCOVERY_TOOLS=\(([^)]*)\)`).FindStringSubmatch(overlaytest.ScriptText(t))
	if m == nil {
		t.Fatal("the overlay script has no DISCOVERY_TOOLS")
	}
	return strings.Fields(m[1])
}

// envValue is the value a run gets for name: the last entry of it.
func envValue(env []string, name string) string {
	value := ""
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, name+"="); ok {
			value = v
		}
	}
	return value
}

// firstInPath is where a lookup of name on path finds it: the first directory of path that holds an
// executable file of that name, or "".
func firstInPath(path, name string) string {
	for _, dir := range filepath.SplitList(path) {
		candidate := filepath.Join(dir, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return candidate
		}
	}
	return ""
}

func TestTheWorldOfTheDoctorTestsHasTheToolsOfDiscoveryWhateverTheMachineHas(t *testing.T) {
	if got := discoveryTools; !reflect.DeepEqual(got, discoveryToolsOfTheScript(t)) {
		t.Fatalf("the world pins %v and the script looks for %v: the world must pin the tools doctor looks for", got, discoveryToolsOfTheScript(t))
	}
	w := registryWorld(t)
	path := envValue(w.env, "PATH")
	for _, tool := range discoveryTools {
		if got, want := firstInPath(path, tool), filepath.Join(w.discoveryDir, tool); got != want {
			t.Errorf("%s resolves to %q on the PATH of the world, want the program of the world, %q: the machine's own would decide what doctor says", tool, got, want)
		}
	}
	run := w.runIn(w.project(), "doctor")
	if !strings.Contains(run.Stdout, "PASS  bat/rg/fd/sd/eza on PATH\n") || strings.Contains(run.Stdout, "Skill discovery MUST use") {
		t.Errorf("doctor in the world says %s, want it to find the five tools and give no advice about them", pinRunText(run))
	}
}

// A world changes its PATH again and again after it is built: a second fake gentle-ai, a PATH made of
// links to everything on the machine's but one tool, a fake go. The tools of discovery are pinned once
// (installDiscoveryTools), so every one of those changes must leave them on the world's programs, ahead of
// the machine's: the pin is a property of the world and not of the order its helpers were called in. Each
// case is a change that a doctor or registry test of this package makes, or one of the shapes a change can
// have (a directory put in front, a PATH replaced whole, another one altogether).
func TestEveryChangeOfThePathOfAWorldKeepsTheToolsOfDiscoveryOnTheWorldsPrograms(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(t *testing.T, w *pinWorld)
	}{
		{"the world as registryWorld makes it", func(t *testing.T, w *pinWorld) {}},
		{"a second fake gentle-ai (the refresh tests)", func(t *testing.T, w *pinWorld) {
			w.installGentle(gentleRegistry, 0)
		}},
		{"a third, that fails", func(t *testing.T, w *pinWorld) {
			w.installGentle(gentleRegistry, 0)
			w.installGentle("# regenerated, and then it failed\n", 3)
		}},
		{"a PATH without gentle-ai, whole (the tests of no gentle-ai)", func(t *testing.T, w *pinWorld) {
			path := pathWithout(t, "gentle-ai")
			w.changePath(func(string) string { return path })
		}},
		{"a fake gentle-ai first on a PATH without go (the tests of no engine)", func(t *testing.T, w *pinWorld) {
			g := w.installGentle("", 0)
			path := g.dir + string(os.PathListSeparator) + pathWithout(t, "go")
			w.changePath(func(string) string { return path })
		}},
		{"neither the engine nor go (withoutEngineOrGo)", func(t *testing.T, w *pinWorld) {
			w.withoutEngineOrGo()
		}},
		{"a fake go first (the bootstrap tests)", func(t *testing.T, w *pinWorld) {
			w.installFakeGo()
		}},
		{"a directory put in front", func(t *testing.T, w *pinWorld) {
			dir := t.TempDir()
			w.putFirst(dir)
		}},
		{"a directory put behind", func(t *testing.T, w *pinWorld) {
			dir := t.TempDir()
			w.changePath(func(old string) string { return old + string(os.PathListSeparator) + dir })
		}},
		{"a PATH that is another one altogether", func(t *testing.T, w *pinWorld) {
			w.changePath(func(string) string { return "/no/such/directory" })
		}},
		{"an empty PATH", func(t *testing.T, w *pinWorld) {
			w.changePath(func(string) string { return "" })
		}},
		{"a PATH that has the directory of the tools in the middle", func(t *testing.T, w *pinWorld) {
			w.changePath(func(old string) string {
				return "/a" + string(os.PathListSeparator) + w.discoveryDir + string(os.PathListSeparator) + old
			})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := registryWorld(t)

			tc.change(t, w)

			path := envValue(w.env, "PATH")
			for _, tool := range discoveryTools {
				if got, want := firstInPath(path, tool), filepath.Join(w.discoveryDir, tool); got != want {
					t.Errorf("after the change, %s resolves to %q on the PATH of the world, want the program of the world, %q (PATH %q)", tool, got, want, path)
				}
			}
			if dirs := filepath.SplitList(path); len(dirs) == 0 || dirs[0] != w.discoveryDir {
				t.Errorf("after the change the directory of the tools is not the first of the PATH of the world: %q", path)
			}
			count := 0
			for _, dir := range filepath.SplitList(path) {
				if dir == w.discoveryDir {
					count++
				}
			}
			if count != 1 {
				t.Errorf("after the change the directory of the tools is %d times on the PATH of the world, want once: %q", count, path)
			}
		})
	}
}

// A world that has no tools of discovery (the worlds of the routing and bootstrap pins) has its PATH as the
// change made it: no directory is put in front, not even an empty one, which would put the working directory
// on the PATH.
func TestAWorldWithoutToolsOfDiscoveryKeepsThePathItsChangeMade(t *testing.T) {
	w := newPinWorld(t, "x managed\n", map[string]string{"skills/x/SKILL.md": "# x\n"})
	if w.discoveryDir != "" {
		t.Fatalf("a world of the routing pins has tools of discovery in %q", w.discoveryDir)
	}
	want := "/a" + string(os.PathListSeparator) + "/b"

	w.changePath(func(string) string { return want })

	if got := envValue(w.env, "PATH"); got != want {
		t.Errorf("PATH = %q after a change to %q, want it as the change made it", got, want)
	}
}
