package runtime_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"testing"
)

// The names of the runtimes are the capability vocabulary's, and the constants that carry them
// live in runtime/core, where target_vocabulary_test.go reads every one of them. This package
// holds the adapters, so the only Target constant it may declare is the label of the longterm-mem
// component, which names no runtime. A constant of type core.Target declared here under any other
// name would be a runtime named outside the vocabulary and outside the scan; this test is the
// guard on that side.
const adapterTargetConstant = "TargetLongtermMem"

// targetConstantsDeclared names every constant of type core.Target that file declares.
func targetConstantsDeclared(file *ast.File) []string {
	var names []string
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs := spec.(*ast.ValueSpec)
			sel, ok := vs.Type.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Target" {
				continue
			}
			if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "core" {
				continue
			}
			for _, name := range vs.Names {
				names = append(names, name.Name)
			}
		}
	}
	return names
}

// targetConstantProblems judges the names found across the package: each one other than the
// label is a problem, and so is anything but exactly one label (none means the scan is broken).
func targetConstantProblems(found []string) []string {
	var problems []string
	labels := 0
	for _, name := range found {
		if name == adapterTargetConstant {
			labels++
			continue
		}
		problems = append(problems, name+" is a Target constant of the adapters: a runtime is named from the capability vocabulary in runtime/core, where the scan of every Target constant reads it")
	}
	if labels != 1 {
		problems = append(problems, "the scan found the label "+adapterTargetConstant+" "+strconv.Itoa(labels)+" times, want once: the scan is broken, or the label moved")
	}
	return problems
}

func TestTheAdaptersDeclareNoTargetConstantButTheLongtermMemLabel(t *testing.T) {
	fset := token.NewFileSet()
	var found []string
	for _, path := range nonTestGoFiles(t, ".") {
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, name := range targetConstantsDeclared(file) {
			found = append(found, name)
			t.Logf("%s declares %s", filepath.Base(path), name)
		}
	}
	for _, problem := range targetConstantProblems(found) {
		t.Error(problem)
	}
}

func TestTheJudgementOfTheAdaptersTargetConstants(t *testing.T) {
	cases := []struct {
		name  string
		found []string
		bad   int
	}{
		{"only the label", []string{adapterTargetConstant}, 0},
		{"nothing found: the scan is broken", nil, 1},
		{"a stray constant beside the label", []string{adapterTargetConstant, "TargetStray"}, 1},
		{"a stray constant without the label", []string{"TargetStray"}, 2},
		{"the label twice", []string{adapterTargetConstant, adapterTargetConstant}, 1},
	}
	for _, tc := range cases {
		if got := len(targetConstantProblems(tc.found)); got != tc.bad {
			t.Errorf("%s: %d problems, want %d", tc.name, got, tc.bad)
		}
	}
}

// The scan reads what it should: a constant of type core.Target under any name, alone or in a
// group, and nothing else.
func TestTheTargetConstantScanOfTheAdaptersSeesWhatItShould(t *testing.T) {
	const src = `package runtime
const TargetSolo core.Target = "x"
const (
	TargetGrouped, TargetPair core.Target = "y", "z"
	NotATarget string = "s"
	OtherPackage other.Target = "o"
	OtherType core.CapabilityStatus = "c"
	Untyped = "u"
)
`
	file, err := parser.ParseFile(token.NewFileSet(), "scan.go", src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	got := targetConstantsDeclared(file)
	want := []string{"TargetSolo", "TargetGrouped", "TargetPair"}
	if len(got) != len(want) {
		t.Fatalf("scan found %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("scan found %v, want %v", got, want)
		}
	}
}
