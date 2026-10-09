package gitfs

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"sort"
	"strings"
	"testing"
)

// gitprovPath is the import path of the package that runs git.
const gitprovPath = "github.com/labdrian-ai/labdrian-sdd-overlay/engine/gitprov"

// gitfs finds a repository by reading files and must never run git (Decision 3 of Phase 7:
// no subprocess). It imports gitprov, the package that does run git, for one pure function; this
// pins that it takes nothing else from it, so the import cannot grow into a call that starts a
// process.
func TestGitfsTakesOnlyThePointerRuleFromGitprov(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(info fs.FileInfo) bool { return !strings.HasSuffix(info.Name(), "_test.go") }, 0)
	if err != nil {
		t.Fatal(err)
	}
	used := map[string]bool{}
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			names, dot := selectedFromImport(file, gitprovPath)
			if dot {
				t.Fatalf("%s dot-imports gitprov, which lets it use any of it without naming it", fset.Position(file.Pos()).Filename)
			}
			for _, name := range names {
				used[name] = true
			}
		}
	}
	var names []string
	for name := range used {
		names = append(names, name)
	}
	sort.Strings(names)
	if got := strings.Join(names, ","); got != "PointerTarget" {
		t.Fatalf("gitfs uses gitprov.{%s}, want only PointerTarget", got)
	}
}

// selectedFromImport lists the members a file selects from the package at path, by whatever name the
// file gives that package: its own name, or the alias of the import. dot is true when the file
// imports the package into its own scope, where the members are used with no selector to read.
func selectedFromImport(file *ast.File, path string) (names []string, dot bool) {
	local := ""
	for _, spec := range file.Imports {
		if strings.Trim(spec.Path.Value, `"`) != path {
			continue
		}
		switch {
		case spec.Name == nil:
			local = path[strings.LastIndex(path, "/")+1:]
		case spec.Name.Name == ".":
			return nil, true
		case spec.Name.Name == "_":
			return nil, false
		default:
			local = spec.Name.Name
		}
	}
	if local == "" {
		return nil, false
	}
	ast.Inspect(file, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == local {
				names = append(names, sel.Sel.Name)
			}
		}
		return true
	})
	return names, false
}

func parseSnippet(t *testing.T, src string) *ast.File {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "snippet.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	return file
}

func TestTheImportPinSeesAGitprovUsedUnderAnyName(t *testing.T) {
	const use = "\nvar _ = %s.Observe\n"
	cases := []struct {
		name, imports, local string
		wantNames            []string
		wantDot              bool
	}{
		{"its own name", `import "` + gitprovPath + `"`, "gitprov", []string{"Observe"}, false},
		{"an alias", `import g "` + gitprovPath + `"`, "g", []string{"Observe"}, false},
		{"a dot import", `import . "` + gitprovPath + `"`, "gitprov", nil, true},
		{"a blank import", `import _ "` + gitprovPath + `"`, "gitprov", nil, false},
		{"another package that is called gitprov", `import "example.com/other/gitprov"`, "gitprov", nil, false},
		{"no import", ``, "gitprov", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := "package p\n" + c.imports + strings.Replace(use, "%s", c.local, 1)
			names, dot := selectedFromImport(parseSnippet(t, src), gitprovPath)
			if strings.Join(names, ",") != strings.Join(c.wantNames, ",") || dot != c.wantDot {
				t.Errorf("selectedFromImport = %q, dot %v; want %q, dot %v", names, dot, c.wantNames, c.wantDot)
			}
		})
	}
}

func TestAnAliasDoesNotHideWhatTheImportTakes(t *testing.T) {
	file := parseSnippet(t, "package p\nimport g \""+gitprovPath+"\"\nvar _ = g.PointerTarget\nvar _ = g.Observe\nvar gitprov = 1\nvar _ = gitprov.X\n")
	names, _ := selectedFromImport(file, gitprovPath)
	if got := strings.Join(names, ","); got != "PointerTarget,Observe" {
		t.Errorf("members selected through the alias = %q, want PointerTarget,Observe (and nothing through an unrelated gitprov)", got)
	}
}
