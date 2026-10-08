package runtime_test

import (
	"go/ast"
	"go/parser"
	"go/token"
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

func TestThePiFactoryBuildsUnderTheConfigRootNotInIt(t *testing.T) {
	root := t.TempDir()
	want := filepath.Join(root, "pi", "labdrian-pi")
	adapter, err := shippedRegistry(t).New(engineRuntime.TargetPi, engineRuntime.Config{ConfigRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	result := adapter.Status()
	if result.Status != engineRuntime.CapabilityUnsupported || !strings.Contains(result.Message, "package is not built at "+want+" ") {
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
	t.Setenv("LABDRIAN_PI_BIN", writeStubPiScript(t, filepath.Join(t.TempDir(), "argv.txt")))

	adapter, err := shippedRegistry(t).New(engineRuntime.TargetPi, engineRuntime.Config{Home: t.TempDir(), ConfigRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if result := adapter.Uninstall(); result.Status != engineRuntime.CapabilitySupported {
		t.Fatalf("Uninstall() = %#v, want supported", result)
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

	adapter, err := shippedRegistry(t).New(engineRuntime.TargetPi, engineRuntime.Config{Home: home, ConfigRoot: root})
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
	adapter, err := shippedRegistry(t).New(engineRuntime.TargetPi, engineRuntime.Config{ConfigRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if msg := adapter.Status().Message; !strings.Contains(msg, "cannot resolve home directory") {
		t.Fatalf("Status() without a home = %q, want it to say the home cannot be resolved", msg)
	}
}

// notRuntimeTargets are the Target constants that name something other than a runtime: a request
// for every registered runtime, and the label of the longterm-mem component's aggregate result.
var notRuntimeTargets = map[string]bool{"TargetAll": true, "TargetLongtermMem": true}

// TestEveryTargetConstantIsACapabilityTarget reads the source of the package: a constant of type
// Target is either listed in notRuntimeTargets or defined as a name of the capability vocabulary
// (capability.TargetX), never as a string of its own. A runtime added here by a literal would
// otherwise be a second vocabulary that nothing registers and nothing declares.
func TestEveryTargetConstantIsACapabilityTarget(t *testing.T) {
	fset := token.NewFileSet()
	seen := 0
	for _, path := range nonTestGoFiles(t, ".") {
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		n, problems := targetConstantProblems(file)
		seen += n
		for _, problem := range problems {
			t.Errorf("%s: %s", filepath.Base(path), problem)
		}
	}
	if seen == 0 {
		t.Fatal("no runtime Target constants found; the source scan is broken")
	}
}

// TestTargetConstantScanNamesWhatItCannotJudge feeds the scan source it must refuse without
// panicking: a literal, a constant with no value of its own (a bare declaration, or one that
// repeats the previous line), and a value that is not capability.X.
func TestTargetConstantScanNamesWhatItCannotJudge(t *testing.T) {
	const src = `package runtime
const (
	TargetGood  Target = capability.TargetClaude
	TargetLit   Target = "codex"
	TargetOther Target = other.TargetPi
	TargetImplicit
	TargetAll Target = "all"
)
const TargetBare Target
`
	file, err := parser.ParseFile(token.NewFileSet(), "scan.go", src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	n, problems := targetConstantProblems(file)
	if n != 5 {
		t.Errorf("scanned %d runtime constants, want 5 (TargetAll is not one)", n)
	}
	for _, name := range []string{"TargetLit", "TargetOther", "TargetImplicit", "TargetBare"} {
		found := false
		for _, problem := range problems {
			found = found || strings.HasPrefix(problem, name+" ")
		}
		if !found {
			t.Errorf("no problem reported for %s; got %q", name, problems)
		}
	}
	for _, problem := range problems {
		if strings.HasPrefix(problem, "TargetGood ") || strings.HasPrefix(problem, "TargetAll ") {
			t.Errorf("a valid constant was reported: %s", problem)
		}
	}
}

// targetConstantProblems judges the Target constants of one file. It returns how many runtime
// constants it looked at and one message, beginning with the constant's name, for each that is not
// defined as capability.X.
func targetConstantProblems(file *ast.File) (scanned int, problems []string) {
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		// A spec without a type repeats the previous line's type and expression, so the type is
		// the one the group last named: Go's implicit repetition.
		var lastType ast.Expr
		for _, spec := range gen.Specs {
			vs := spec.(*ast.ValueSpec)
			typ := vs.Type
			if typ != nil || len(vs.Values) > 0 {
				lastType = typ
			} else {
				typ = lastType
			}
			if id, ok := typ.(*ast.Ident); !ok || id.Name != "Target" {
				continue
			}
			for i, name := range vs.Names {
				if notRuntimeTargets[name.Name] {
					continue
				}
				scanned++
				if i >= len(vs.Values) {
					problems = append(problems, name.Name+" has no value of its own; define it explicitly as capability.Target...")
					continue
				}
				sel, ok := vs.Values[i].(*ast.SelectorExpr)
				if !ok {
					problems = append(problems, name.Name+" is not defined from the capability vocabulary; define it as capability.Target..., or list it in notRuntimeTargets if it is not a runtime")
					continue
				}
				if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "capability" {
					problems = append(problems, name.Name+" is not defined from the capability vocabulary; define it as capability.Target..., or list it in notRuntimeTargets if it is not a runtime")
				}
			}
		}
	}
	return scanned, problems
}
