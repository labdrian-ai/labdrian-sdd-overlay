package runtime_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capability"
	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
)

// pseudoTargets is the explicit registry of Target constants that are
// deliberately not runtimes. Each labels something other than a runtime the
// engine installs into, sits outside ParseTarget's and ExpandTarget's domain,
// and so needs no capability declaration.
//
// The registry is what makes the check below total. A Target constant must be
// either a runtime (in ExpandTarget(TargetAll), and therefore declared) or a
// listed pseudo-target; a new constant that is neither fails the test, so
// adding a runtime cannot skip its declaration, and adding a pseudo-target is a
// conscious edit here.
var pseudoTargets = map[string]string{
	string(engineRuntime.TargetLongtermMem): "label of the longterm-mem component's aggregate LifecycleResult (longtermmem.go)",
}

// runtimeTargetConstants returns, sorted, the value of every constant of type
// Target declared in the production files of this package, except TargetAll
// (a request for every target, not a target). It reads the source instead of
// listing the constants by hand, so a Target added in any file is seen here
// the moment it is declared.
func runtimeTargetConstants(t *testing.T) []string {
	t.Helper()
	fset := token.NewFileSet()
	var values []string
	for _, path := range nonTestGoFiles(t, ".") {
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				valueSpec := spec.(*ast.ValueSpec)
				if typ, ok := valueSpec.Type.(*ast.Ident); !ok || typ.Name != "Target" {
					continue
				}
				for i, expr := range valueSpec.Values {
					lit, ok := expr.(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						t.Fatalf("%s: constant %s of type Target is not a string literal; extend runtimeTargetConstants before adding it", filepath.Base(path), valueSpec.Names[i].Name)
					}
					value, err := strconv.Unquote(lit.Value)
					if err != nil {
						t.Fatalf("%s: constant %s: %v", filepath.Base(path), valueSpec.Names[i].Name, err)
					}
					if value != string(engineRuntime.TargetAll) {
						values = append(values, value)
					}
				}
			}
		}
	}
	sort.Strings(values)
	return values
}

// absentFrom returns the members of want that are not in have.
func absentFrom(want, have []string) []string {
	present := make(map[string]bool, len(have))
	for _, h := range have {
		present[h] = true
	}
	var absent []string
	for _, w := range want {
		if !present[w] {
			absent = append(absent, w)
		}
	}
	return absent
}

// TestCapabilityTargetsMatchRuntimeTargets is the registry check that keeps
// declarations total: every runtime.Target has a capability declaration, and
// every declared target is a runtime.Target. Adding a Target constant without
// declaring it fails here (and so does declaring a target the runtime package
// does not know), so a fifth runtime cannot ship without stating what it
// supports. It lives in this package so the TestMain isolation applies.
func TestCapabilityTargetsMatchRuntimeTargets(t *testing.T) {
	constants := runtimeTargetConstants(t)
	if len(constants) == 0 {
		t.Fatal("no Target constants found in engine/runtime; the source scan is broken")
	}

	// Runtimes are the constants that are not registered pseudo-targets.
	var runtimes []string
	for _, c := range constants {
		if _, pseudo := pseudoTargets[c]; !pseudo {
			runtimes = append(runtimes, c)
		}
	}

	var expanded []string
	for _, target := range engineRuntime.ExpandTarget(engineRuntime.TargetAll) {
		expanded = append(expanded, string(target))
	}
	sort.Strings(expanded)

	declared := capability.Targets()
	sort.Strings(declared)

	// Every Target constant is a runtime that "--target all" covers, or a
	// registered pseudo-target. A constant that is neither is a runtime the
	// CLI does not expand yet, or a label nobody registered.
	if stray := absentFrom(runtimes, expanded); len(stray) > 0 {
		t.Errorf("Target constant(s) %q are neither in ExpandTarget(TargetAll) nor registered in pseudoTargets: a new runtime must be added to ParseTarget, ExpandTarget, capability.Targets(), and the table in capability/declarations.go; a label that is not a runtime belongs in pseudoTargets", strings.Join(stray, ", "))
	}
	if stray := absentFrom(expanded, runtimes); len(stray) > 0 {
		t.Errorf("ExpandTarget(TargetAll) returns %q, which is not a runtime Target constant", strings.Join(stray, ", "))
	}

	// Total in both directions: no runtime without a declaration, and no
	// declaration for a target that is not a runtime.
	if missing := absentFrom(runtimes, declared); len(missing) > 0 {
		t.Errorf("runtime.Target %q has no capability declaration: add it to capability.Targets() and to the table in capability/declarations.go", strings.Join(missing, ", "))
	}
	if extra := absentFrom(declared, runtimes); len(extra) > 0 {
		t.Errorf("capability declares %q, which is not a runtime.Target", strings.Join(extra, ", "))
	}

	// A pseudo-target must still exist, must stay outside the CLI's target
	// domain, and must not be declared.
	for name, why := range pseudoTargets {
		if len(absentFrom([]string{name}, constants)) > 0 {
			t.Errorf("pseudoTargets lists %q (%s) but no such Target constant exists; remove the stale entry", name, why)
		}
		if parsed, err := engineRuntime.ParseTarget(name); err == nil {
			t.Errorf("pseudo-target %q is accepted by ParseTarget as %q; if it is now a runtime, remove it from pseudoTargets and declare it", name, parsed)
		}
		if _, err := capability.Declare(name); err == nil {
			t.Errorf("pseudo-target %q has a capability declaration; a pseudo-target is not a runtime", name)
		}
	}

	// Every runtime round-trips through both packages' APIs.
	for _, name := range runtimes {
		parsed, err := engineRuntime.ParseTarget(name)
		if err != nil || string(parsed) != name {
			t.Errorf("runtime.ParseTarget(%q) = %q, %v; want the same target back", name, parsed, err)
		}
		if _, err := capability.Declare(name); err != nil {
			t.Errorf("capability.Declare(%q): %v", name, err)
		}
	}
}
