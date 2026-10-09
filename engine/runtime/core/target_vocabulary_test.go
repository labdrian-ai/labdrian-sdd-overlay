package core_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// notRuntimeTargets are the Target constants that name something other than a runtime: a request
// for every registered runtime. (The label of the longterm-mem component's aggregate result is a
// Target constant of engine/runtime, outside this package.)
var notRuntimeTargets = map[string]bool{"TargetAll": true}

// nonTestGoFiles returns the regular, non-test .go files directly inside dir, sorted.
func nonTestGoFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read directory %s: %v", dir, err)
	}
	var files []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.Type().IsRegular() && strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
			files = append(files, filepath.Join(dir, name))
		}
	}
	sort.Strings(files)
	return files
}

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
	const src = `package core
const (
	TargetGood  Target = capability.TargetClaude
	TargetLit   Target = "codex"
	TargetOther Target = other.TargetPi
	TargetImplicit
	TargetAll Target = "all"
)
const TargetBare Target

// Go wants as many values as names: the scan names the constant that has none, and the spec that
// has too many.
const TargetA, TargetB Target = capability.TargetClaude
const TargetC Target = capability.TargetCodex, capability.TargetPi

// An untyped spec, and the ones that repeat it, are not Target constants.
const (
	TargetTyped Target = capability.TargetPi
	Counter0           = iota
	Counter1
)
`
	file, err := parser.ParseFile(token.NewFileSet(), "scan.go", src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	n, problems := targetConstantProblems(file)
	if n != 9 {
		t.Errorf("scanned %d runtime constants, want 9 (TargetAll, Counter0 and Counter1 are not)", n)
	}
	for _, name := range []string{"TargetLit", "TargetOther", "TargetImplicit", "TargetBare", "TargetB", "TargetC"} {
		found := false
		for _, problem := range problems {
			found = found || strings.HasPrefix(problem, name+" ")
		}
		if !found {
			t.Errorf("no problem reported for %s; got %q", name, problems)
		}
	}
	for _, problem := range problems {
		for _, accepted := range []string{"TargetGood ", "TargetAll ", "TargetA ", "TargetTyped ", "Counter0 ", "Counter1 "} {
			if strings.HasPrefix(problem, accepted) {
				t.Errorf("a constant Go accepts, or that is not a Target, was reported: %s", problem)
			}
		}
	}
}

// isCapabilityName says whether expr is a name taken from the capability package: capability.X.
func isCapabilityName(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "capability"
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
				if !isCapabilityName(vs.Values[i]) {
					problems = append(problems, name.Name+" is not defined from the capability vocabulary; define it as capability.Target..., or list it in notRuntimeTargets if it is not a runtime")
				}
			}
			if len(vs.Values) > len(vs.Names) {
				problems = append(problems, vs.Names[0].Name+" is declared with more values than names; give each constant the one value of its own")
			}
		}
	}
	return scanned, problems
}
