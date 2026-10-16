package shelltest

// The layout of the script: bin/labdrian-overlay is an entry point, and what it does is in the modules in
// bin/lib, which it sources in the order of its _modules array. These are the rules of that layout.
//
//   - The entry point lists exactly the modules that are in bin/lib, once each: a file nobody sources is
//     dead, and a name with no file is a script that does not start.
//   - Every file is valid bash, and a module is a file to source: it says which shell it is for (for
//     shellcheck, which cannot tell from a name), has no shebang and is not executable.
//   - A module that is missing stops the script with a message that names it and says what to do, and
//     nothing has run by then: not when the script is run, and not when it is sourced.
//   - A module uses only what the modules before it define (and its own): the order of the array is an
//     order of dependency, so the modules are layers and none depends on one above it. That is the
//     dependency rule of the script, and it is held here the way archguard holds the one of the engine.
//   - No function is defined in two modules, where the later would silently replace the earlier.
//   - CI checks every module: the lint job runs bash -n and shellcheck on bin/lib.

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

// functionDefinition finds the functions a file defines: the name at the start of a line, then ().
var functionDefinition = regexp.MustCompile(`(?m)^([A-Za-z_][A-Za-z0-9_]*)\(\) *\{`)

// heredocStart finds the word that ends a here-document opened on a line.
var heredocStart = regexp.MustCompile(`<<-?\s*['"]?([A-Za-z_][A-Za-z0-9_]*)['"]?`)

// codeOf is the text of a bash file that can call a function: without the comment lines and without the
// body of a here-document, which is text for a person to read (the help is one).
func codeOf(text string) string {
	var code []string
	terminator := ""
	for _, line := range strings.Split(text, "\n") {
		if terminator != "" {
			if line == terminator {
				terminator = ""
			}
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if m := heredocStart.FindStringSubmatch(line); m != nil {
			terminator = m[1]
		}
		code = append(code, line)
	}
	return strings.Join(code, "\n")
}

// module is one file of bin/lib: its name, its file and its text.
type module struct {
	name, path, text string
}

// modulesOfTheScript are the modules the entry point lists, in the order it sources them.
func modulesOfTheScript(t *testing.T) []module {
	t.Helper()
	script := overlaytest.Script(t)
	var modules []module
	for _, name := range overlaytest.ScriptModules(t, script) {
		path := overlaytest.ModulePath(script, name)
		text, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read the module %s: %v", name, err)
		}
		modules = append(modules, module{name, path, string(text)})
	}
	return modules
}

func TestTheEntryPointSourcesExactlyTheModulesInLibOnceEach(t *testing.T) {
	script := overlaytest.Script(t)
	listed := overlaytest.ScriptModules(t, script)
	entries, err := os.ReadDir(overlaytest.LibDir(script))
	if err != nil {
		t.Fatalf("the script has no lib directory beside it: %v", err)
	}
	var onDisk []string
	for _, entry := range entries {
		name, isModule := strings.CutSuffix(entry.Name(), ".sh")
		if !isModule || entry.IsDir() {
			t.Errorf("bin/lib/%s is not a module (a file ending in .sh)", entry.Name())
			continue
		}
		onDisk = append(onDisk, name)
	}

	seen := map[string]bool{}
	for _, name := range listed {
		if seen[name] {
			t.Errorf("the entry point lists the module %q twice", name)
		}
		seen[name] = true
	}
	sort.Strings(onDisk)
	sortedListed := append([]string(nil), listed...)
	sort.Strings(sortedListed)
	if strings.Join(sortedListed, " ") != strings.Join(onDisk, " ") {
		t.Errorf("the entry point lists %q and bin/lib holds %q: a module nobody sources is dead, a name with no file stops the script", sortedListed, onDisk)
	}
}

func TestEveryFileOfTheScriptIsValidBash(t *testing.T) {
	overlaytest.RequireBash(t)
	for _, file := range overlaytest.ScriptFiles(t) {
		if out, err := exec.Command("bash", "-n", file).CombinedOutput(); err != nil {
			t.Errorf("bash -n %s: %v\n%s", file, err, out)
		}
	}
}

func TestAModuleIsAFileToSourceThatSaysWhichShellItIsFor(t *testing.T) {
	for _, m := range modulesOfTheScript(t) {
		if first, _, _ := strings.Cut(m.text, "\n"); first != "# shellcheck shell=bash" {
			t.Errorf("%s starts with %q, want the directive %q: shellcheck cannot tell a module from its name", m.name, first, "# shellcheck shell=bash")
		}
		if strings.HasPrefix(m.text, "#!") {
			t.Errorf("%s has a shebang: a module is sourced and never run", m.name)
		}
		if info, err := os.Stat(m.path); err != nil || info.Mode().Perm()&0o111 != 0 {
			t.Errorf("%s is executable (%v, %v): a module is sourced and never run", m.name, info, err)
		}
	}
}

// copyWithout copies the script of this checkout to a place of its own and takes the module name away.
func copyWithout(t *testing.T, name string) (script, home string) {
	t.Helper()
	script = overlaytest.CopyScript(t, t.TempDir())
	if err := os.Remove(overlaytest.ModulePath(script, name)); err != nil {
		t.Fatal(err)
	}
	return script, t.TempDir()
}

func TestAMissingModuleStopsTheScriptWithAMessageThatNamesItAndNothingHasRun(t *testing.T) {
	overlaytest.RequireBash(t)
	for _, name := range overlaytest.ScriptModules(t, overlaytest.Script(t)) {
		t.Run(name, func(t *testing.T) {
			script, home := copyWithout(t, name)
			// The script says where it looked by its physical path, symlinks resolved.
			physical, err := filepath.EvalSymlinks(filepath.Dir(script))
			if err != nil {
				t.Fatal(err)
			}
			wantPath := filepath.Join(physical, "lib", name+".sh")
			check := func(how, out string, exit int) {
				t.Helper()
				if exit != 1 {
					t.Errorf("%s: exit %d, want 1", how, exit)
				}
				for _, want := range []string{"ERROR:", "'" + name + "' is missing", wantPath, "restore"} {
					if !strings.Contains(out, want) {
						t.Errorf("%s: the message %q lacks %q: it must name the module and where it should be, and say what to do", how, out, want)
					}
				}
				if lines := strings.Count(strings.TrimSpace(out), "\n") + 1; lines != 2 {
					t.Errorf("%s: wrote %d lines, want the 2 of the message and nothing else: %q", how, lines, out)
				}
			}

			run := overlaytest.Run(t, script, locationEnv(home), "", "targets")
			check("run", run.Stderr, run.Exit)
			if run.Stdout != "" {
				t.Errorf("run: stdout %q, want nothing", run.Stdout)
			}
			out, err := overlaytest.Source(t, script, overlaytest.Shell{Env: locationEnv(home)}, "echo defined")
			check("sourced", out, overlaytest.ExitCode(err))
			if strings.Contains(out, "defined") {
				t.Errorf("sourced: the shell went on after the missing module: %q", out)
			}
			if entries, err := os.ReadDir(home); err != nil || len(entries) != 0 {
				t.Errorf("the home holds %v (%v) after the script stopped: nothing should have run", entries, err)
			}
		})
	}
}

// The modules are found beside the entry point, and not from the overlay a caller names: the tests of the
// script, and a person with another checkout, point OVERLAY_DIR elsewhere.
func TestTheModulesAreFoundBesideTheScriptWhateverOverlayDirSays(t *testing.T) {
	overlaytest.RequireBash(t)
	empty := t.TempDir()

	got := overlaytest.Run(t, overlaytest.Script(t), append(locationEnv(t.TempDir()), "OVERLAY_DIR="+empty), "", "targets")

	if got.Exit != 0 || got.Stdout != theCatalog || got.Stderr != "" {
		t.Errorf("targets with OVERLAY_DIR=%s = exit %d, stdout %q, stderr %q, want exit 0 and %q", empty, got.Exit, got.Stdout, got.Stderr, theCatalog)
	}
}

// definitions maps each function of the modules to the position, in the order of the array, of the module
// that defines it. It fails the test for a function two modules define.
func definitions(t *testing.T, modules []module) map[string]int {
	t.Helper()
	at := map[string]int{}
	for i, m := range modules {
		for _, d := range functionDefinition.FindAllStringSubmatch(m.text, -1) {
			if first, twice := at[d[1]]; twice {
				t.Errorf("%s is defined in %s and in %s: the later would silently replace the earlier", d[1], modules[first].name, m.name)
				continue
			}
			at[d[1]] = i
		}
	}
	return at
}

func TestNoFunctionIsDefinedInTwoModules(t *testing.T) {
	definitions(t, modulesOfTheScript(t))
}

// referencesToLater is the functions of the modules after the one at position i that its code names. A function
// name is a word of its own: not part of a longer name, a path or an option.
func referencesToLater(modules []module, at map[string]int, i int) []string {
	code := codeOf(modules[i].text)
	var later []string
	for fn, defined := range at {
		if defined <= i {
			continue
		}
		if regexp.MustCompile(`(^|[^A-Za-z0-9_./-])` + regexp.QuoteMeta(fn) + `($|[^A-Za-z0-9_-])`).MatchString(code) {
			later = append(later, fn+" ("+modules[defined].name+")")
		}
	}
	sort.Strings(later)
	return later
}

func TestAModuleUsesNothingThatALaterModuleDefines(t *testing.T) {
	modules := modulesOfTheScript(t)
	at := definitions(t, modules)
	for i, m := range modules {
		if later := referencesToLater(modules, at, i); len(later) != 0 {
			t.Errorf("%s uses %s, which a module sourced after it defines: reorder the modules or move the function, so that the order of the array stays an order of dependency", m.name, strings.Join(later, ", "))
		}
	}
}

// The rule has teeth: it finds a use of a later module when there is one, in code and not in a comment or a
// here-document, and a word that only contains the name is not a use.
func TestTheDependencyRuleSeesALaterUseInCodeAndOnlyThere(t *testing.T) {
	modules := []module{
		{name: "first", text: "first_fn() {\n  # second_fn is not called here\n  cat <<'EOF'\nsecond_fn in the help\nEOF\n  my_second_fn_extra\n  echo ./second_fn --second_fn second_fn_x\n}\n"},
		{name: "second", text: "second_fn() {\n  first_fn\n}\n"},
	}
	at := definitions(t, modules)

	if got := referencesToLater(modules, at, 0); len(got) != 0 {
		t.Errorf("first uses %v: a comment, a here-document, a longer name, a path and an option are not uses", got)
	}
	modules[0].text += "again() {\n  second_fn\n}\n"
	if got := referencesToLater(modules, at, 0); len(got) != 1 || got[0] != "second_fn (second)" {
		t.Errorf("first, with a call of second_fn = %v, want it found", got)
	}
}

func TestTheLintJobOfCIChecksEveryModule(t *testing.T) {
	ci, err := os.ReadFile(filepath.Join(overlaytest.RepoRoot(t), ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"bash -n", "shellcheck"} {
		var runs []string
		for _, line := range strings.Split(string(ci), "\n") {
			if strings.Contains(line, "run: ") && strings.Contains(line, tool+" ") && strings.Contains(line, "bin/labdrian-overlay") {
				runs = append(runs, line)
			}
		}
		if len(runs) != 1 {
			t.Errorf("found %d lines of ci.yml that run %s on bin/labdrian-overlay, want 1: %q", len(runs), tool, runs)
			continue
		}
		if !strings.Contains(runs[0], "bin/lib/*.sh") {
			t.Errorf("the CI line %q does not run %s on bin/lib/*.sh: the modules would not be checked", strings.TrimSpace(runs[0]), tool)
		}
	}
}
