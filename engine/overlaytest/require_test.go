package overlaytest_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

// helperRecorder stands in for the test a Require helper is given: it embeds the real one, and keeps what the
// helper skipped or failed with instead of skipping or failing the real test. Every call that ends a
// test stops the helper, as the real one would, with runtime.Goexit.
type helperRecorder struct {
	testing.TB
	skipped, failed string
}

func (*helperRecorder) Helper() {}

func (r *helperRecorder) skip(text string) { r.skipped = text; runtime.Goexit() }
func (r *helperRecorder) fail(text string) { r.failed = text; runtime.Goexit() }

func (r *helperRecorder) Skip(args ...any)                 { r.skip(fmt.Sprint(args...)) }
func (r *helperRecorder) Skipf(format string, args ...any) { r.skip(fmt.Sprintf(format, args...)) }
func (r *helperRecorder) SkipNow()                         { r.skip("") }
func (r *helperRecorder) Fatal(args ...any)                { r.fail(fmt.Sprint(args...)) }
func (r *helperRecorder) Fatalf(format string, args ...any) {
	r.fail(fmt.Sprintf(format, args...))
}
func (r *helperRecorder) FailNow() { r.fail("") }

// callHelper calls helper with a helperRecorder, in a goroutine of its own, since a helper that skips or fails
// ends the goroutine it runs in.
func callHelper(t testing.TB, helper func(testing.TB)) *helperRecorder {
	t.Helper()
	r := &helperRecorder{TB: t}
	done := make(chan struct{})
	go func() {
		defer close(done)
		helper(r)
	}()
	<-done
	return r
}

// requireRows is one row for each exported Require helper of the package: the helper called with its
// tool missing. The PATH of the test is an empty directory, and absent is the path of a program that
// is not in it.
var requireRows = map[string]func(t testing.TB, absent string){
	"RequireBash":     func(t testing.TB, _ string) { overlaytest.RequireBash(t) },
	"RequireShAndAwk": func(t testing.TB, _ string) { overlaytest.RequireShAndAwk(t) },
	"RequirePrograms": func(t testing.TB, absent string) { overlaytest.RequirePrograms(t, absent) },
}

// requireHelpers lists the exported functions whose name starts with Require in the non-test files of
// this package.
func requireHelpers(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(".", e.Name()), nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", e.Name(), err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if ok && fn.Recv == nil && fn.Name.IsExported() && strings.HasPrefix(fn.Name.Name, "Require") {
				names = append(names, fn.Name.Name)
			}
		}
	}
	sort.Strings(names)
	return names
}

// A tool a test needs that is missing skips the test on a developer machine, and fails it when CI is
// "true", so a runner that lost the tool cannot pass by skipping. Every exported Require helper of the
// package is held to it: a new helper with no row here fails this test, which is how a helper that
// only skips (the class of NewGrandchild) cannot come back.
func TestEveryRequireHelperFailsOnCI(t *testing.T) {
	helpers := requireHelpers(t)
	if len(helpers) == 0 {
		t.Fatal("no exported Require helper found in the package: the parse reads nothing")
	}
	for _, name := range helpers {
		if requireRows[name] == nil {
			t.Errorf("%s has no row in requireRows: add one that runs it with its tool missing", name)
		}
	}
	for name := range requireRows {
		found := false
		for _, h := range helpers {
			found = found || h == name
		}
		if !found {
			t.Errorf("requireRows has a row for %s, which is no exported Require helper of the package", name)
		}
	}
	for _, name := range helpers {
		row := requireRows[name]
		if row == nil {
			continue
		}
		t.Run(name, func(t *testing.T) {
			empty := t.TempDir()
			t.Setenv("PATH", empty)
			absent := filepath.Join(empty, "no-such-program")

			t.Setenv("CI", "true")
			got := callHelper(t, func(r testing.TB) { row(r, absent) })
			if got.failed == "" || got.skipped != "" {
				t.Errorf("with CI=true and the tool missing, %s skipped %q and failed %q, want a failure and no skip", name, got.skipped, got.failed)
			}

			t.Setenv("CI", "")
			got = callHelper(t, func(r testing.TB) { row(r, absent) })
			if got.skipped == "" || got.failed != "" {
				t.Errorf("with CI empty and the tool missing, %s skipped %q and failed %q, want a skip and no failure", name, got.skipped, got.failed)
			}
		})
	}
}

// With every program present, RequirePrograms neither skips nor fails, on CI or off it.
func TestRequireProgramsAcceptsPresentPrograms(t *testing.T) {
	present := filepath.Join(t.TempDir(), "tool")
	if err := os.WriteFile(present, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, ci := range []string{"true", ""} {
		t.Setenv("CI", ci)
		got := callHelper(t, func(r testing.TB) { overlaytest.RequirePrograms(r, present, present) })
		if got.skipped != "" || got.failed != "" {
			t.Errorf("CI=%q: RequirePrograms of a present program skipped %q and failed %q, want neither", ci, got.skipped, got.failed)
		}
	}
}

// The message of the failure names the program that is missing, so a red CI run says which.
func TestRequireProgramsNamesTheMissingProgram(t *testing.T) {
	t.Setenv("CI", "true")
	absent := filepath.Join(t.TempDir(), "no-such-program")

	got := callHelper(t, func(r testing.TB) { overlaytest.RequirePrograms(r, absent) })

	if !strings.Contains(got.failed, absent) || !strings.Contains(got.failed, "CI") {
		t.Errorf("failure = %q, want it to name %s and say that CI needs it", got.failed, absent)
	}
}
