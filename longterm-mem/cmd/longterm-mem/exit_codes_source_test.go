package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	iofs "io/fs"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// exitCodeConstantsBySourceFile parses this package's own non-test sources
// and returns, per file, every `exit<Something> = <integer literal>`
// constant declared in it.
//
// It looks only at integer LITERALS on purpose: an alias of another named
// code is not a second declaration of the contract, so it is not one of the
// things this test insists lives in one place. exitDoctorChecksFailed used
// to be exactly such an alias, declared in cmd_doctor.go; it is now a code
// of its own (9) and therefore lives in exit_codes.go like every other.
func exitCodeConstantsBySourceFile(t *testing.T) map[string][]string {
	t.Helper()

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi iofs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parse package sources: %v", err)
	}

	found := map[string][]string{}
	for _, pkg := range pkgs {
		for path, file := range pkg.Files {
			for _, decl := range file.Decls {
				gen, ok := decl.(*ast.GenDecl)
				if !ok || gen.Tok != token.CONST {
					continue
				}
				for _, spec := range gen.Specs {
					value, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for i, name := range value.Names {
						if !strings.HasPrefix(name.Name, "exit") || i >= len(value.Values) {
							continue
						}
						lit, ok := value.Values[i].(*ast.BasicLit)
						if !ok || lit.Kind != token.INT {
							continue
						}
						found[path] = append(found[path], name.Name+" = "+lit.Value)
					}
				}
			}
		}
	}
	return found
}

// TestExitCodes_ContractIsDeclaredInOneFile holds exit_codes.go to its own
// stated reason for existing: "The exit codes below are this binary's
// published contract ... They live in one place because their whole value
// is that a caller can act on them without reading stderr: the moment two
// unrelated failures share a code, or one failure is spelled as a bare
// literal in six subcommands, the code stops being a contract and becomes
// a number that happens to be non-zero."
//
// A file that says the codes live in one place while a code lives
// somewhere else is worse than no claim at all: it tells the next reader
// that the list they are looking at is complete, so the next code gets
// picked by scanning that list and collides with the one the claim hid.
// exitPathUnresolvable = 8 sat in register_paths.go doing exactly that,
// with its own doc comment re-deriving the same contract from scratch to
// justify the number.
func TestExitCodes_ContractIsDeclaredInOneFile(t *testing.T) {
	const home = "exit_codes.go"

	byFile := exitCodeConstantsBySourceFile(t)
	if len(byFile[home]) == 0 {
		t.Fatalf("no exit-code constants found in %s at all; the parser or the file layout changed", home)
	}

	var strays []string
	for path, names := range byFile {
		if strings.HasSuffix(path, home) {
			continue
		}
		for _, name := range names {
			strays = append(strays, path+": "+name)
		}
	}
	sort.Strings(strays)
	if len(strays) > 0 {
		t.Fatalf("exit-code constants declared outside %s:\n  %s\n%s claims the contract lives in one place; either move these there or stop making the claim", home, strings.Join(strays, "\n  "), home)
	}
}

// TestExitCodes_NoTwoCodesShareANumber is the other half of the same
// claim, and the failure mode the centralization exists to prevent: a
// number picked by scanning a list that was missing an entry. Aliases are
// excluded by construction (they are not integer literals), so this
// catches only two SEPARATE codes that landed on one number.
func TestExitCodes_NoTwoCodesShareANumber(t *testing.T) {
	owners := map[int][]string{}
	for _, names := range exitCodeConstantsBySourceFile(t) {
		for _, decl := range names {
			parts := strings.SplitN(decl, " = ", 2)
			n, err := strconv.Atoi(parts[1])
			if err != nil {
				t.Fatalf("exit-code constant %q does not carry an integer literal: %v", decl, err)
			}
			owners[n] = append(owners[n], parts[0])
		}
	}
	for code, names := range owners {
		if len(names) > 1 {
			sort.Strings(names)
			t.Fatalf("exit code %d is claimed by %s -- two unrelated failures sharing a code is exactly what makes the code stop being a contract", code, strings.Join(names, " and "))
		}
	}
}
