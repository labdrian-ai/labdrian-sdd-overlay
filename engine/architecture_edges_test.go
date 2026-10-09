package guard

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// moduleReach returns every package of the module that the package at start imports, directly or
// through another package of the module, as slash-separated directories relative to the module
// root. It reads the imports of the production files with go/parser, as the ring guard does, so a
// build constraint hides nothing. A directory with no Go file is an error, so a wrong name cannot
// pass for a package that reaches nothing.
func moduleReach(root, modulePath, start string) (map[string]bool, error) {
	reached := map[string]bool{}
	pending := []string{start}
	for len(pending) > 0 {
		dir := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		files, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(dir), "*.go"))
		if err != nil {
			return nil, err
		}
		if len(files) == 0 {
			return nil, fmt.Errorf("no Go file in %s", dir)
		}
		for _, file := range files {
			if strings.HasSuffix(file, "_test.go") {
				continue
			}
			parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
			if err != nil {
				return nil, err
			}
			for _, spec := range parsed.Imports {
				imported, err := strconv.Unquote(spec.Path.Value)
				if err != nil {
					return nil, err
				}
				rel, inModule := strings.CutPrefix(imported, modulePath+"/")
				if !inModule || reached[rel] {
					continue
				}
				reached[rel] = true
				pending = append(pending, rel)
			}
		}
	}
	return reached, nil
}

func TestModuleReachFollowsImportsThroughOtherPackages(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a/a.go", "package a\nimport _ \"example.com/m/b\"\nimport _ \"os\"\n")
	write("b/b.go", "package b\nimport _ \"example.com/m/c/d\"\n")
	write("b/b_test.go", "package b\nimport _ \"example.com/m/test_only\"\n")
	write("c/d/d.go", "package d\n")
	write("test_only/t.go", "package test_only\n")

	got, err := moduleReach(root, "example.com/m", "a")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for name := range got {
		names = append(names, name)
	}
	sort.Strings(names)
	if want := "b c/d"; strings.Join(names, " ") != want {
		t.Fatalf("reach of a = %v, want %q: the standard library and a test file's imports are not part of it", names, want)
	}

	if _, err := moduleReach(root, "example.com/m", "no-such-package"); err == nil {
		t.Fatal("a package with no Go file was read as one that reaches nothing")
	}
}

// The package that holds the hook entries the overlay writes into settings.json holds nothing that
// decides about a tool call. It reached the shaper for two strings (H27), and through the shaper
// the goal and strict-JSON packages; the strings are the constants of guardmarkers now, and this
// keeps the edge from coming back. The ring guard cannot say it: an adapter may import a domain
// package, and the shaper is one. The git adapter is named too, because the shaper once imported
// it and the plan of H27 still counts it.
func TestSettingsDoesNotReachTheShaperOrGit(t *testing.T) {
	const modulePath = "github.com/labdrian-ai/labdrian-sdd-overlay/engine"
	forbidden := []string{"shaper", "gitprov"}
	for _, name := range forbidden {
		// A forbidden package that is renamed or moved would make the check pass for nothing: each
		// name must still be a package of the module.
		if files, _ := filepath.Glob(filepath.Join(name, "*.go")); len(files) == 0 {
			t.Errorf("%q is not a package of the module any more: rename it in this test", name)
		}
	}
	for _, start := range []string{"settings", "settings/settingsfile"} {
		reached, err := moduleReach(".", modulePath, start)
		if err != nil {
			t.Fatal(err)
		}
		for pkg := range reached {
			for _, name := range forbidden {
				if pkg == name || strings.HasPrefix(pkg, name+"/") {
					t.Errorf("%s reaches %s: the words of the guard are guardmarkers', not the package that decides with them", start, pkg)
				}
			}
		}
	}
}
