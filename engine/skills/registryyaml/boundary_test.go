package registryyaml_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// The adapter decodes and encodes, and the domain judges. Nothing in the production code of this
// package may call the domain's rule (skills.Registry.Validate), or name one of its own to judge
// an entry with: an adapter that did would be deciding what the registry means, and the rule would
// run at the moment the adapter chooses, not the domain. This reads the package as a parser does
// and fails on a call or a reference to Validate, or to a function whose name says it validates an
// entry; the words "validate" in a comment are not what it looks for.
func TestTheAdapterNeverJudgesWhatItReads(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		checked++
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		ast.Inspect(parsed, func(n ast.Node) bool {
			var name string
			switch x := n.(type) {
			case *ast.SelectorExpr:
				name = x.Sel.Name
			case *ast.Ident:
				name = x.Name
			default:
				return true
			}
			if strings.HasPrefix(strings.ToLower(name), "validate") || strings.HasPrefix(name, "judge") {
				t.Errorf("%s uses %s: the adapter does not judge what it reads, the domain does (skills.ReadRegistry applies skills.Registry.Validate)", file, name)
			}
			return true
		})
	}
	if checked < 4 {
		t.Fatalf("only %d production files were read: the glob is broken", checked)
	}
}

// The adapter imports the domain it serves and nothing else of the module: not the other
// adapters, and not the composition root.
func TestTheAdapterImportsOnlyTheDomainItServes(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	const module = "github.com/labdrian-ai/labdrian-sdd-overlay/"
	const domain = module + "engine/skills"
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		for _, imp := range parsed.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if strings.HasPrefix(path, module) && path != domain {
				t.Errorf("%s imports %s: registryyaml reaches no package of the module but the skills domain", file, path)
			}
		}
	}
}
