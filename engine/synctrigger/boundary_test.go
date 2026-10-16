package synctrigger

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

// The trigger starts no program and reads no clock: it asks the Runner and the clock it is given.
// A second way of starting a process in this package would be a second place to kill a group, and
// a reading of the machine's clock a line no test could date, so the source is read for both. (The
// import rules of the engine cannot say it: the package is an adapter, which may reach the
// operating system, and this is the one thing it must not.)
//
// The words "os/exec", "syscall" and the names of the functions below are the machine's own and
// are written as literals on purpose: they are what is looked for.
func TestTheTriggerStartsNoProgramAndReadsNoClockItself(t *testing.T) {
	forbiddenImports := map[string]bool{"os/exec": true, "syscall": true, "os/signal": true}
	forbiddenMembers := map[string]map[string]bool{
		"time": {"Now": true, "Since": true, "Until": true, "Sleep": true, "After": true, "AfterFunc": true, "Tick": true, "NewTicker": true, "NewTimer": true},
		"os":   {"Executable": true, "StartProcess": true, "FindProcess": true, "Getpid": true},
	}

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(info os.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parse the package: %v", err)
	}
	if len(pkgs) != 1 {
		t.Fatalf("want one package in the directory, got %d", len(pkgs))
	}
	for _, pkg := range pkgs {
		for name, file := range pkg.Files {
			// The name each forbidden package goes by in this file.
			names := map[string]string{}
			for _, imp := range file.Imports {
				path, _ := strconv.Unquote(imp.Path.Value)
				if forbiddenImports[path] {
					t.Errorf("%s imports %s: starting a program is the Runner's", name, path)
				}
				if _, watched := forbiddenMembers[path]; watched {
					local := path
					if imp.Name != nil {
						local = imp.Name.Name
					}
					names[local] = path
				}
			}
			ast.Inspect(file, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if id, ok := sel.X.(*ast.Ident); ok {
					if path, watched := names[id.Name]; watched && forbiddenMembers[path][sel.Sel.Name] {
						t.Errorf("%s: %s.%s: the trigger takes the clock and the process table from its Options", fset.Position(sel.Pos()), id.Name, sel.Sel.Name)
					}
				}
				return true
			})
		}
	}
}
