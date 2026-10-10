package gitfs

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
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
	members, dotImporters, err := gitprovUse(".")
	if err != nil {
		t.Fatal(err)
	}
	if len(dotImporters) > 0 {
		t.Fatalf("%s dot-import(s) gitprov, which lets them use any of it without naming it", strings.Join(dotImporters, ", "))
	}
	if got := strings.Join(members, ","); got != "PointerTarget" {
		t.Fatalf("gitfs uses gitprov.{%s}, want only PointerTarget", got)
	}
}

// gitprovUse reads the non-test files of the package in dir and answers which members of gitprov
// they select (sorted, each once) and which files dot-import it, by file name. The names come from the
// map of files the parser returns for the directory, so each one is the file that was parsed.
func gitprovUse(dir string) (members, dotImporters []string, err error) {
	pkgs, err := parser.ParseDir(token.NewFileSet(), dir, func(info fs.FileInfo) bool { return !strings.HasSuffix(info.Name(), "_test.go") }, 0)
	if err != nil {
		return nil, nil, err
	}
	used := map[string]bool{}
	for _, pkg := range pkgs {
		for path, file := range pkg.Files {
			names, dot := selectedFromImport(file, gitprovPath)
			if dot {
				dotImporters = append(dotImporters, filepath.Base(path))
			}
			for _, name := range names {
				used[name] = true
			}
		}
	}
	for name := range used {
		members = append(members, name)
	}
	sort.Strings(members)
	sort.Strings(dotImporters)
	return members, dotImporters, nil
}

// selectedFromImport lists the members a file selects from the package at path, by whatever name the
// file gives that package: its own name, or the alias of each import of it (a file may import the
// package under several names, and every one is read). dot is true when the file imports the package
// into its own scope, where the members are used with no selector to read.
func selectedFromImport(file *ast.File, path string) (names []string, dot bool) {
	locals := map[string]bool{}
	for _, spec := range file.Imports {
		if strings.Trim(spec.Path.Value, `"`) != path {
			continue
		}
		switch {
		case spec.Name == nil:
			locals[path[strings.LastIndex(path, "/")+1:]] = true
		case spec.Name.Name == ".":
			dot = true
		case spec.Name.Name == "_":
		default:
			locals[spec.Name.Name] = true
		}
	}
	if len(locals) == 0 {
		return nil, dot
	}
	ast.Inspect(file, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok && locals[id.Name] {
				names = append(names, sel.Sel.Name)
			}
		}
		return true
	})
	return names, dot
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

func TestAFileThatImportsGitprovTwiceUnderTwoNamesIsReadThroughBoth(t *testing.T) {
	file := parseSnippet(t, "package p\nimport (\n\ta \""+gitprovPath+"\"\n\tb \""+gitprovPath+"\"\n)\nvar _ = a.PointerTarget\nvar _ = b.Observe\n")
	names, dot := selectedFromImport(file, gitprovPath)
	if got := strings.Join(names, ","); got != "PointerTarget,Observe" || dot {
		t.Errorf("members selected through both names = %q, dot %v; want PointerTarget,Observe", got, dot)
	}
}

func TestADotImportAmongOtherImportsOfGitprovIsStillADotImport(t *testing.T) {
	file := parseSnippet(t, "package p\nimport (\n\ta \""+gitprovPath+"\"\n\t. \""+gitprovPath+"\"\n)\nvar _ = a.PointerTarget\n")
	if _, dot := selectedFromImport(file, gitprovPath); !dot {
		t.Error("a file that dot-imports gitprov next to a named import was not reported as a dot import")
	}
}

// writeSources makes a directory holding the given files, named by their keys.
func writeSources(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestTheDotImportIsReportedByTheNameOfTheFileThatHasIt(t *testing.T) {
	dir := writeSources(t, map[string]string{
		"clean.go":      "package p\nimport \"" + gitprovPath + "\"\nvar _ = gitprov.PointerTarget\n",
		"dirty.go":      "package p\nimport . \"" + gitprovPath + "\"\n",
		"dirty_test.go": "package p\nimport . \"" + gitprovPath + "\"\n",
	})
	members, dotImporters, err := gitprovUse(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(dotImporters, ","); got != "dirty.go" {
		t.Errorf("files that dot-import gitprov = %q, want dirty.go (and not the test file, which the pin does not read)", got)
	}
	if got := strings.Join(members, ","); got != "PointerTarget" {
		t.Errorf("members taken = %q, want PointerTarget", got)
	}
}

func TestAnAliasDoesNotHideWhatTheImportTakes(t *testing.T) {
	file := parseSnippet(t, "package p\nimport g \""+gitprovPath+"\"\nvar _ = g.PointerTarget\nvar _ = g.Observe\nvar gitprov = 1\nvar _ = gitprov.X\n")
	names, _ := selectedFromImport(file, gitprovPath)
	if got := strings.Join(names, ","); got != "PointerTarget,Observe" {
		t.Errorf("members selected through the alias = %q, want PointerTarget,Observe (and nothing through an unrelated gitprov)", got)
	}
}
