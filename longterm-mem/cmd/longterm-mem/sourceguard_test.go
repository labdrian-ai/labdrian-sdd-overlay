package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const (
	embedPackage   = "github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/embed"
	vaultfsPackage = "github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultfs"
)

// sourceFiles are the Go files of this package, split into the program's and the tests'.
func sourceFiles(t *testing.T) (program, tests []string) {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			tests = append(tests, name)
		} else {
			program = append(program, name)
		}
	}
	if len(program) == 0 || len(tests) == 0 {
		t.Fatalf("found %d program files and %d test files in the command package, want both: the guards below would pass over nothing", len(program), len(tests))
	}
	return program, tests
}

// usesOf lists where file refers to name in the package imported from importPath, whatever name the file
// gives that import: the declaration is read from the syntax tree, so an aliased import is still the package,
// and a comment or a string that happens to spell the call is not a use.
func usesOf(t *testing.T, file, importPath, name string) []token.Position {
	t.Helper()
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, file, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	var local string
	for _, spec := range parsed.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil || path != importPath {
			continue
		}
		local = filepath.Base(path)
		if spec.Name != nil {
			local = spec.Name.Name
		}
	}
	if local == "" || local == "_" {
		return nil
	}
	var uses []token.Position
	ast.Inspect(parsed, func(n ast.Node) bool {
		selector, ok := n.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != name {
			return true
		}
		if pkg, ok := selector.X.(*ast.Ident); ok && pkg.Name == local {
			uses = append(uses, fset.Position(selector.Pos()))
		}
		return true
	})
	return uses
}

// usesOfIdent lists where file mentions name as a plain identifier, its own declaration included.
func usesOfIdent(t *testing.T, file, name string) []token.Position {
	t.Helper()
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, file, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	var uses []token.Position
	ast.Inspect(parsed, func(n ast.Node) bool {
		if ident, ok := n.(*ast.Ident); ok && ident.Name == name {
			uses = append(uses, fset.Position(ident.Pos()))
		}
		return true
	})
	return uses
}

// Every command gets its vault files from openVault. A file of the program that built the adapter itself
// would report its failures without the prefix openVault gives them, and nothing at that site would say so.
// The syntax tree is what is read, so an aliased import of the adapter is still seen, and the comments that
// name the call are not mistaken for it.
func TestCommandsOpenTheVaultThroughOpenVault(t *testing.T) {
	program, _ := sourceFiles(t)
	for _, name := range program {
		if name == "vaultfiles.go" {
			continue
		}
		for _, at := range usesOf(t, name, vaultfsPackage, "New") {
			t.Errorf("%s uses vaultfs.New itself; commands open the vault through openVault", at)
		}
	}
	if len(usesOf(t, "vaultfiles.go", vaultfsPackage, "New")) == 0 {
		t.Error("vaultfiles.go no longer builds the adapter with vaultfs.New: the guard looks for the wrong call")
	}
}

// No command builds an embedding client itself: it asks the factory it was wired with, so that the one place
// that names the real backend is productionCommands, and a test can hand the commands a backend of its own.
func TestCommandsBuildTheirEmbeddingClientsThroughTheirFactory(t *testing.T) {
	program, _ := sourceFiles(t)
	for _, name := range program {
		if name == "commands.go" {
			continue
		}
		for _, at := range usesOf(t, name, embedPackage, "NewClient") {
			t.Errorf("%s uses embed.NewClient itself; a command builds its embedding client with the factory it was wired with", at)
		}
	}
	if len(usesOf(t, "commands.go", embedPackage, "NewClient")) == 0 {
		t.Error("commands.go no longer wires embed.NewClient: the guard looks for the wrong call")
	}
}

// The program's own wiring, productionCommands, is for main alone. A test that built it would run the
// commands against the embedding backend at the default endpoint, which is the embedding server of whoever
// runs the tests, and the tests of this package would reach it again without anything saying so.
func TestNoTestRunsTheProductionCommands(t *testing.T) {
	program, tests := sourceFiles(t)
	for _, name := range tests {
		for _, at := range usesOfIdent(t, name, "productionCommands") {
			t.Errorf("%s runs the production commands; tests run commands wired to testEmbedClient", at)
		}
	}
	var builders int
	for _, name := range program {
		builders += len(usesOfIdent(t, name, "productionCommands"))
	}
	if builders < 2 {
		t.Errorf("productionCommands is mentioned %d times in the program (its declaration and main), want at least 2: the guard looks for the wrong name", builders)
	}
}
