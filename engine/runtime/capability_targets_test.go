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
)

// shippedRegistry registers every runtime this package ships, the way the composition root does.
func shippedRegistry(t *testing.T) *engineRuntime.Registry {
	t.Helper()
	r := engineRuntime.NewRegistry()
	for _, register := range []func(*engineRuntime.Registry) error{
		engineRuntime.RegisterClaude,
		engineRuntime.RegisterOpenCode,
		engineRuntime.RegisterCodex,
		func(r *engineRuntime.Registry) error { return engineRuntime.RegisterPi(r, fileRegistries) },
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
		adapter, err := r.New(target, engineRuntime.Config{ConfigRoot: t.TempDir()})
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
	adapter, err := shippedRegistry(t).New(engineRuntime.TargetClaude, engineRuntime.Config{ConfigRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if result := adapter.Install(); result.Status != engineRuntime.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}
	if got := parseClaudeSettingsFile(t, filepath.Join(root, "settings.json")); got["hooks"] == nil {
		t.Fatalf("no hooks were written under the config root %s", root)
	}
}

func TestThePiFactoryBuildsInTheConfigRootAsThePackageDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "labdrian-pi")
	adapter, err := shippedRegistry(t).New(engineRuntime.TargetPi, engineRuntime.Config{ConfigRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	result := adapter.Status()
	if result.Status != engineRuntime.CapabilityUnsupported || !strings.Contains(result.Message, "package is not built at "+root+" ") {
		t.Fatalf("Status() = %#v, want it to look for the package at the config root %s", result, root)
	}
}

// builtPiPackage makes a directory that looks like a built Pi package: Status gates on its
// package.json before it reads anything else.
func builtPiPackage(t *testing.T) string {
	t.Helper()
	dest := filepath.Join(t.TempDir(), "labdrian-pi")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dest
}

// TestThePiFactoryReadsPiSettingsUnderTheConfigHome: the ~/.pi/agent files the status proves
// against are the ones under Config.Home, not under whatever home the process has.
func TestThePiFactoryReadsPiSettingsUnderTheConfigHome(t *testing.T) {
	dest := builtPiPackage(t)
	home := t.TempDir()
	agent := filepath.Join(home, ".pi", "agent")
	if err := os.MkdirAll(agent, 0o755); err != nil {
		t.Fatal(err)
	}
	settings := `{"packages": [` + strconv.Quote(dest) + `]}`
	if err := os.WriteFile(filepath.Join(agent, "settings.json"), []byte(settings), 0o644); err != nil {
		t.Fatal(err)
	}

	adapter, err := shippedRegistry(t).New(engineRuntime.TargetPi, engineRuntime.Config{Home: home, ConfigRoot: dest})
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
	adapter, err := shippedRegistry(t).New(engineRuntime.TargetPi, engineRuntime.Config{ConfigRoot: builtPiPackage(t)})
	if err != nil {
		t.Fatal(err)
	}
	if msg := adapter.Status().Message; !strings.Contains(msg, "cannot resolve home directory") {
		t.Fatalf("Status() without a home = %q, want it to say the home cannot be resolved", msg)
	}
}
