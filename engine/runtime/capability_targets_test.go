package runtime_test

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capability"
	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
)

// shippedRegistry registers every runtime this package ships, the way the composition root does,
// with a Pi that runs its commands through a fake nobody looks at.
func shippedRegistry(t *testing.T) *core.Registry {
	t.Helper()
	return shippedRegistryWith(t, &fakeCommands{})
}

// shippedRegistryWith is shippedRegistry with the CommandRunner the test watches.
func shippedRegistryWith(t *testing.T, commands engineRuntime.CommandRunner) *core.Registry {
	t.Helper()
	r := core.NewRegistry()
	for _, register := range []func(*core.Registry) error{
		engineRuntime.RegisterClaude,
		engineRuntime.RegisterOpenCode,
		engineRuntime.RegisterCodex,
		func(r *core.Registry) error {
			return engineRuntime.RegisterPi(r, engineRuntime.PiPorts{Commands: commands, Packages: newPiPackages()})
		},
	} {
		if err := register(r); err != nil {
			t.Fatalf("register a shipped runtime: %v", err)
		}
	}
	return r
}

// TestShippedRuntimesAreExactlyTheDeclaredTargets keeps the vocabulary total in both directions:
// a target capability declares has an adapter this package registers, and an adapter this package
// registers is a declared target. Adding a runtime to one and not the other fails here, so a
// fifth runtime cannot ship without stating what it supports.
func TestShippedRuntimesAreExactlyTheDeclaredTargets(t *testing.T) {
	var registered []string
	for _, target := range shippedRegistry(t).Targets() {
		registered = append(registered, string(target))
	}
	sort.Strings(registered)

	declared := capability.Targets()
	sort.Strings(declared)

	if strings.Join(registered, ",") != strings.Join(declared, ",") {
		t.Fatalf("registered runtimes = %v, declared targets = %v; they must be the same set", registered, declared)
	}
}

// TestALabelThatIsNotARuntimeIsNotATarget: the longterm-mem component labels its own aggregate
// result and is selected by --component, never by --target, so no registry holds it and no
// capability declares it.
func TestALabelThatIsNotARuntimeIsNotATarget(t *testing.T) {
	name := string(engineRuntime.TargetLongtermMem)
	if _, err := shippedRegistry(t).Parse(name); err == nil {
		t.Errorf("Parse(%q) accepted a label that is not a runtime", name)
	}
	if _, err := capability.Declare(name); err == nil {
		t.Errorf("capability declares %q, which is not a runtime", name)
	}
}

// TestEveryRuntimeBuildsItsOwnAdapter: each Register function binds the factory of its own
// runtime, and the factory takes the directory the adapter works in from the Config.
func TestEveryRuntimeBuildsItsOwnAdapter(t *testing.T) {
	r := shippedRegistry(t)
	for _, target := range r.Targets() {
		adapter, err := r.New(target, core.Config{ConfigRoot: t.TempDir()})
		if err != nil {
			t.Fatalf("New(%q): %v", target, err)
		}
		if adapter.Target() != target {
			t.Errorf("New(%q).Target() = %q", target, adapter.Target())
		}
	}
}

func TestTheClaudeFactoryWorksInTheConfigRoot(t *testing.T) {
	root := t.TempDir()
	adapter, err := shippedRegistry(t).New(core.TargetClaude, core.Config{ConfigRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if result := adapter.Install(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}
	if got := parseClaudeSettingsFile(t, filepath.Join(root, "settings.json")); got["hooks"] == nil {
		t.Fatalf("no hooks were written under the config root %s", root)
	}
}

func TestThePiFactoryBuildsUnderTheConfigRootNotInIt(t *testing.T) {
	root := t.TempDir()
	want := filepath.Join(root, "pi", "labdrian-pi")
	adapter, err := shippedRegistry(t).New(core.TargetPi, core.Config{ConfigRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	result := adapter.Status()
	if result.Status != core.CapabilityUnsupported || !strings.Contains(result.Message, "package is not built at "+want+" ") {
		t.Fatalf("Status() = %#v, want it to look for the package at %s, under the config root", result, want)
	}
}

// TestPiUninstallUnderASharedConfigRootLeavesTheOtherRuntimesFilesAlone: the config root may be
// the one the other runtimes keep their settings in (`--target all --config-root X`). Removing
// the Pi package removes its own directory, pi/labdrian-pi, and nothing else of the root.
func TestPiUninstallUnderASharedConfigRootLeavesTheOtherRuntimesFilesAlone(t *testing.T) {
	root := t.TempDir()
	foreign := filepath.Join(root, "settings.json")
	if err := os.WriteFile(foreign, []byte(`{"theme":"dark"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	pkg := filepath.Join(root, "pi", "labdrian-pi")
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	commands := &fakeCommands{}

	adapter, err := shippedRegistryWith(t, commands).New(core.TargetPi, core.Config{Home: t.TempDir(), ConfigRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if result := adapter.Uninstall(); result.Status != core.CapabilitySupported {
		t.Fatalf("Uninstall() = %#v, want supported", result)
	}
	if got := commands.invocations(); len(got) != 1 || got[0][0] != "remove" || got[0][1] != pkg {
		t.Errorf("the commands run = %v, want a single `pi remove %s`", got, pkg)
	}
	if _, err := os.Stat(pkg); !os.IsNotExist(err) {
		t.Errorf("the package directory %s is still there: %v", pkg, err)
	}
	if got, err := os.ReadFile(foreign); err != nil || string(got) != `{"theme":"dark"}` {
		t.Errorf("a file of the shared config root was touched: %q, %v", got, err)
	}
}

// builtPiPackage makes a config root holding a directory that looks like a built Pi package:
// Status gates on its package.json before it reads anything else. It returns the root and the
// package directory.
func builtPiPackage(t *testing.T) (root, dest string) {
	t.Helper()
	root = t.TempDir()
	dest = filepath.Join(root, "pi", "labdrian-pi")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, dest
}

// TestThePiFactoryReadsPiSettingsUnderTheConfigHome: the ~/.pi/agent files the status proves
// against are the ones under Config.Home, not under whatever home the process has.
func TestThePiFactoryReadsPiSettingsUnderTheConfigHome(t *testing.T) {
	root, dest := builtPiPackage(t)
	home := t.TempDir()
	agent := filepath.Join(home, ".pi", "agent")
	if err := os.MkdirAll(agent, 0o755); err != nil {
		t.Fatal(err)
	}
	settings := `{"packages": [` + strconv.Quote(dest) + `]}`
	if err := os.WriteFile(filepath.Join(agent, "settings.json"), []byte(settings), 0o644); err != nil {
		t.Fatal(err)
	}

	adapter, err := shippedRegistry(t).New(core.TargetPi, core.Config{Home: home, ConfigRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if msg := adapter.Status().Message; strings.Contains(msg, "settings.json packages (not listed") {
		t.Fatalf("the package is listed in the settings under the config home, yet Status says it is not: %q", msg)
	}
}

// TestThePiFactoryWithoutAHomeSaysSoAndGuessesNone: no home is reported as unproven, never
// replaced by the home of the process.
func TestThePiFactoryWithoutAHomeSaysSoAndGuessesNone(t *testing.T) {
	root, _ := builtPiPackage(t)
	adapter, err := shippedRegistry(t).New(core.TargetPi, core.Config{ConfigRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if msg := adapter.Status().Message; !strings.Contains(msg, "cannot resolve home directory") {
		t.Fatalf("Status() without a home = %q, want it to say the home cannot be resolved", msg)
	}
}
