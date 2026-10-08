package runtime_test

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
// once removed a freshly installed labdrian-pi package during `go test ./...`, because a test
// reached a real `pi`. Three nets sit under that now. The adapter reaches `pi` only through a
// CommandRunner and every test hands it a fake (the next test and the one after it keep that
// true); the PATH of the run holds a `pi` that refuses to run in front of any real one, and the
// run fails if anything started it; and HOME, STATE_DIR and OVERLAY_DIR are pinned. Individual
// tests may still override the environment with t.Setenv.
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

	code := m.Run()
	if message, ok := guard.Verdict(); !ok {
		fmt.Fprintln(os.Stderr, message)
		code = 1
	}
	guard.Close()
	os.RemoveAll(home)
	os.Exit(code)
}

// TestLiveGuard_IsolatesHomeAndPi proves the TestMain guard is in force for this package: the
// home is not a real Pi installation, and a `pi` looked up by name is the guard that refuses.
func TestLiveGuard_IsolatesHomeAndPi(t *testing.T) {
	real, _ := os.UserHomeDir()
	if _, err := os.Stat(filepath.Join(real, ".pi", "agent", "settings.json")); err == nil {
		t.Fatalf("HOME %q still points at a real Pi installation", real)
	}
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

// TestNoTestOfThisPackageStartsTheRealCLI reads the source of the tests: the Pi adapter is
// driven through a fake CommandRunner, so a test that imports the process adapter, or runs or
// runs `pi` itself, is a test that can reach the CLI of the machine with the PATH of the run.
func TestNoTestOfThisPackageStartsTheRealCLI(t *testing.T) {
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
				t.Errorf("%s imports %s: the process adapter starts real programs with the PATH of the run; hand the adapter a fake CommandRunner", path, p)
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
