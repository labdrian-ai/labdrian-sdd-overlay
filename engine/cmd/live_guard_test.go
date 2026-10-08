package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/piguard"
)

// TestMain isolates every test in this package from the developer's live machine: the Pi adapter
// shells out to a real `pi` CLI, which once removed a freshly installed labdrian-pi package
// during `go test ./...`. The cores of the commands take their CommandRunner as a parameter and
// every test hands them a fake (noPi); under that, the PATH of the run holds a `pi` that refuses
// to run in front of any real one, and the run fails if anything started it, the built program
// that the golden tests start included. Individual tests may still override the environment with
// t.Setenv. HOME, STATE_DIR and OVERLAY_DIR are pinned under a temporary directory, and
// XDG_STATE_HOME and XDG_CONFIG_HOME under the temporary HOME too, so no test can write the real
// shaper clearance store or the real XDG config.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "engine-test-home-*")
	if err != nil {
		panic(err)
	}
	guard, err := piguard.Install()
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", home)
	os.Setenv("STATE_DIR", filepath.Join(home, ".labdrian-overlay"))
	os.Unsetenv("OVERLAY_DIR")
	os.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	os.Unsetenv("GENTLE_PI_AGENTS_CHILD")
	// os.Exit does not run deferred calls, so what the run made is removed before it.
	code := m.Run()
	if message, ok := guard.Verdict(); !ok {
		fmt.Fprintln(os.Stderr, message)
		code = 1
	}
	guard.Close()
	os.RemoveAll(home)
	removeReviewReceiptBinary()
	os.Exit(code)
}

// TestLiveGuard_PiOnThePathIsTheGuard proves the TestMain guard is in force: a `pi` looked up by
// name is the one that refuses to run, not a real one.
func TestLiveGuard_PiOnThePathIsTheGuard(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("the pi guard is a shell script")
	}
	path, err := exec.LookPath("pi")
	if err != nil {
		t.Fatalf("no pi guard on the PATH of the run: %v", err)
	}
	if !strings.HasPrefix(filepath.Base(filepath.Dir(path)), "pi-guard-") {
		t.Fatalf("pi resolves to %s on the PATH of the run, want the refusing guard", path)
	}
}

// TestNoTestOfThisPackageUsesTheProcessAdapter reads the source of the tests: the cores of the
// commands are driven with a fake CommandRunner, so a test that imports the process adapter, or
// runs `pi` itself, can reach the CLI of the machine with the PATH of the run.
func TestNoTestOfThisPackageUsesTheProcessAdapter(t *testing.T) {
	files, err := filepath.Glob("*_test.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("no test files found: %v", err)
	}
	fset := token.NewFileSet()
	for _, path := range files {
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, spec := range file.Imports {
			if p, err := strconv.Unquote(spec.Path.Value); err == nil && strings.HasSuffix(p, "/engine/execrunner") {
				t.Errorf("%s imports %s: the process adapter starts real programs with the PATH of the run; hand the core a fake CommandRunner", path, p)
			}
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "exec" {
				return true
			}
			// exec.Command(name, ...) and exec.CommandContext(ctx, name, ...) start a program;
			// looking one up (exec.LookPath, which the guard test above does) starts nothing.
			nameArg := 0
			switch sel.Sel.Name {
			case "Command":
			case "CommandContext":
				nameArg = 1
			default:
				return true
			}
			if nameArg >= len(call.Args) {
				return true
			}
			if lit, ok := call.Args[nameArg].(*ast.BasicLit); ok && lit.Kind == token.STRING {
				if name, err := strconv.Unquote(lit.Value); err == nil && name == "pi" {
					t.Errorf("%s:%d: exec.%s(%q) starts the real CLI", path, fset.Position(call.Pos()).Line, sel.Sel.Name, name)
				}
			}
			return true
		})
	}
}
