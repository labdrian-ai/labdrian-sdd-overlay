package main

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/archguard"
)

// modulePath is the module this guard judges.
const modulePath = "github.com/labdrian-ai/labdrian-sdd-overlay/tui"

// rings declares the layer of every package of this module (docs/architecture/
// hexagonal-target.md, section 1, row "Operator TUI"). A package missing here
// fails the guard, so a new package starts with a decision about where it sits;
// a row whose package is gone fails it too.
//
//   - domain is the vocabulary and the rules of the TUI (targets, the action
//     menu, the arguments of an invocation, the sync-check report, the scope
//     that keeps `--target all` honest) and the ports it asks the world through.
//   - app holds the use cases (running an action, planning a restore, reading
//     how far the repository is behind) over those ports.
//   - adapters/overlaycli is the driven adapter: it answers the ports by running
//     bin/labdrian-overlay, and it knows the script's wire formats.
//   - ui is the driving adapter: the bubbletea model and the screens. It is an
//     adapter because it imports bubbletea and lipgloss, which the domain and
//     the use cases may not.
//   - the module root is the composition root (main.go): the only package that
//     chooses an adapter. It also holds the pinned sessions and this guard.
//   - testhome is the home directory a test binary runs in, which the TestMain of
//     every package with tests enters: test support.
var rings = map[string]archguard.Ring{
	".":                            archguard.Root,
	"internal/adapters/overlaycli": archguard.Adapter,
	"internal/app":                 archguard.Application,
	"internal/domain":              archguard.Domain,
	"internal/testhome":            archguard.Support,
	"internal/ui":                  archguard.Adapter,
}

// knownDebt is every violation of the rule that exists today, each owed to the
// work unit that removes it. It is empty: the module follows the rule since the
// first slice of T3 cut it into the rings above, and what is added here again is
// a decision, not a habit.
//
// The guard reads imports and the members selected from a few standard packages.
// It cannot see package-level mutable variables used as test seams; the TUI has
// none (its styles are never reassigned).
var knownDebt = archguard.Debt{}

// TestArchitectureFollowsTheDependencyRule is the fitness function: it fails on
// a package without a ring, on a violation that is not known debt, and on debt
// that is no longer a violation.
func TestArchitectureFollowsTheDependencyRule(t *testing.T) {
	problems, err := archguard.Check(".", rings, knownDebt)
	if err != nil {
		t.Fatal(err)
	}
	for _, problem := range problems {
		t.Error(problem)
	}
}

// TestNoAdapterImportsAnotherAdapter closes the gap the rings leave: they let an
// adapter import any adapter, and the two this module has must not know each
// other. The screens ask the use cases through interfaces of their own and the
// script is run by an adapter that knows nothing of the screens; only main puts
// them together. Only production code counts: a test may build the real adapter
// to drive a use case.
func TestNoAdapterImportsAnotherAdapter(t *testing.T) {
	found, err := adapterImports(".", modulePath, adapterDirectories())
	if err != nil {
		t.Fatal(err)
	}
	for _, edge := range found {
		t.Errorf("%s: an adapter may not import another adapter; the composition root (main.go) is the only place that names both", edge)
	}
}

// adapterDirectories are the packages the rings declare adapters.
func adapterDirectories() []string {
	var dirs []string
	for dir, ring := range rings {
		if ring == archguard.Adapter {
			dirs = append(dirs, dir)
		}
	}
	sort.Strings(dirs)
	return dirs
}

// adapterImports lists, as "file imports package", each production Go file under
// root, in one of the adapter directories, that imports another of them (the
// module path prefixes the directory in an import). It reads the import path
// itself, so an alias, a dot import and a blank import all count. It skips test
// files and directories named testdata or vendor, hidden and underscore
// directories (the go tool's own rule). A file it cannot parse is an error.
func adapterImports(root, module string, adapters []string) ([]string, error) {
	var found []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			name := d.Name()
			if rel != "." && (name == "testdata" || name == "vendor" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		own := ""
		for _, dir := range adapters {
			if within(filepath.ToSlash(filepath.Dir(rel)), dir) {
				own = dir
			}
		}
		if own == "" {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range file.Imports {
			imported, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			local, inModule := strings.CutPrefix(imported, module+"/")
			if !inModule {
				continue
			}
			for _, dir := range adapters {
				if dir != own && within(local, dir) {
					found = append(found, rel+" imports "+dir)
				}
			}
		}
		return nil
	})
	sort.Strings(found)
	return found, err
}

// within reports whether dir is base or a directory below it.
func within(dir, base string) bool {
	return dir == base || strings.HasPrefix(dir, base+"/")
}

// The walk is the rule, so it is tested on a tree built for it: an adapter that
// imports the other is flagged (plain, aliased, dot and blank imports, and from
// a subpackage), the root and the other layers are let through, and so are test
// files, testdata, and an adapter importing itself or the standard library.
func TestAdapterImportsFlagsOneAdapterImportingAnother(t *testing.T) {
	const module = "example.com/m"
	write := func(root, rel, source string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	importing := func(spec string) string { return "package p\n\nimport " + spec + "\n" }
	root := t.TempDir()
	write(root, "main.go", importing(`"example.com/m/internal/a"`))
	write(root, "internal/a/a.go", importing(`"example.com/m/internal/b"`))
	write(root, "internal/a/sub/s.go", importing(`x "example.com/m/internal/b"`))
	write(root, "internal/b/b.go", importing(`_ "example.com/m/internal/a"`))
	write(root, "internal/b/c.go", importing(`. "example.com/m/internal/a/sub"`))
	write(root, "internal/b/self.go", importing(`"example.com/m/internal/b/more"`))
	write(root, "internal/b/std.go", importing(`"fmt"`))
	write(root, "internal/b/b_test.go", importing(`"example.com/m/internal/a"`))
	write(root, "internal/b/testdata/t.go", importing(`"example.com/m/internal/a"`))
	write(root, "internal/domain/d.go", importing(`"example.com/m/internal/a"`))
	write(root, "internal/b/foreign.go", importing(`"example.org/m/internal/a"`))

	got, err := adapterImports(root, module, []string{"internal/a", "internal/b"})
	if err != nil {
		t.Fatalf("adapterImports: %v", err)
	}

	want := []string{
		"internal/a/a.go imports internal/b",
		"internal/a/sub/s.go imports internal/b",
		"internal/b/b.go imports internal/a",
		"internal/b/c.go imports internal/a",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("adapterImports = %v, want %v", got, want)
	}
}
