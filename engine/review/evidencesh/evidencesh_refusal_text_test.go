package evidencesh_test

// Every refusal the adapter makes before a run is named by the README and the CHANGELOG (T4b-e, `c67f772ff05b-6` F1 and
// F2, `-5.polisher`). The hand list of the docs tests missed twice (`c9440b24b20f-6` F6, then `c67f772ff05b-6` F2), so this
// test ties the texts to the code: it reads the format string of every fmt.Errorf of Classified and checkScript, the errors
// returned before a run (the errors after it come from refusal and answers, which it does not read), and holds each to one
// fragment of a table, each fragment to some error, and each fragment to the one sentence of the README and of the CHANGELOG
// that names the refusals. A refusal added without a fragment, or a fragment a text drops, fails. It also reads every return
// of the two bodies (not a function literal in them), so an errors.New or a bare `return err` cannot pass unseen
// (`20b8b0cf75fb-6` F1): the error of a return is nil, a literal fmt.Errorf held as above, or one of three named exceptions,
// each matched once and each with its reason (the bare `err` is accepted only as the return directly in the body of the
// `if` whose init assigns `err` from a call of checkScript, `8ee5ac95b287-5` F2); a return with no result, another expression, an exception matched twice or
// not at all fails. It reads the source of two functions, so an error moved into a helper, or a harmless rewrite of an
// exception's return, fails it for a reason that is not a text; the fix is one table row or one moved line.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/overlaytest"
)

// refusalFragments are the words of each refusal before a run, as the texts name them.
var refusalFragments = []string{
	"is not an absolute path",
	"is not deployed",
	"is not a regular file",
	"cannot be checked",
	"cannot be read",
}

// errorFormats are the unquoted first arguments of every fmt.Errorf in the bodies of Classified (a method) and checkScript.
func errorFormats(t *testing.T, path string) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var formats []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		if !(fn.Name.Name == "Classified" && fn.Recv != nil) && fn.Name.Name != "checkScript" {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Errorf" {
				return true
			}
			if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "fmt" {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				t.Errorf("%s: an fmt.Errorf of %s has a format that is not a string literal", path, fn.Name.Name)
				return true
			}
			format, err := strconv.Unquote(lit.Value)
			if err != nil {
				t.Fatal(err)
			}
			formats = append(formats, format)
			return true
		})
	}
	return formats
}

// errorException is a return of Classified or checkScript whose error is not a literal fmt.Errorf and is not a refusal before
// a run that a text must name. expr is the returned error as types.ExprString prints it.
type errorException struct{ fn, expr, reason string }

var errorExceptions = []errorException{
	{"Classified", boundErr, "the error of checkScript, whose own returns this test holds"},
	{"Classified", "s.refusal(run, stderr, err)", "after the run: the package doc's row of a failed run, not a refusal before it"},
	{"Classified", "answers(run, stdout, len(reviews))", "after the run: the package doc's row of its output, not a refusal before it"},
}

// boundErr is how errorReturns writes the bare `err` that is returned directly in the body of an `if` whose init assigns
// `err` from a call of checkScript: the one place a bare `err` is accepted (`8ee5ac95b287-5` F2). Any other bare `err`
// is written `err`, which no exception names, so a pre-run check that binds and returns `err` in place of the call of
// checkScript fails as an unnamed error return.
const boundErr = "err (from checkScript)"

// returnsOfCheckScript are the returns that sit directly in the body of an `if` whose init is an assignment of the one
// name `err` from a call of the identifier checkScript (`err := checkScript(...)`).
func returnsOfCheckScript(body *ast.BlockStmt) map[*ast.ReturnStmt]bool {
	bound := map[*ast.ReturnStmt]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		stmt, ok := n.(*ast.IfStmt)
		if !ok {
			return true
		}
		assign, ok := stmt.Init.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return true
		}
		name, ok := assign.Lhs[0].(*ast.Ident)
		call, isCall := assign.Rhs[0].(*ast.CallExpr)
		if !ok || name.Name != "err" || !isCall {
			return true
		}
		if fn, ok := call.Fun.(*ast.Ident); !ok || fn.Name != "checkScript" {
			return true
		}
		for _, inner := range stmt.Body.List {
			if ret, ok := inner.(*ast.ReturnStmt); ok {
				bound[ret] = true
			}
		}
		return true
	})
	return bound
}

// errorReturns are "<function>: <expr>" for the last result of every return of Classified and checkScript, not entering a
// function literal, but nil and a literal fmt.Errorf call, which errorFormats holds. A return with no result fails here.
// A bare `err` is written boundErr when it is returned where returnsOfCheckScript finds it, and `err` anywhere else.
func errorReturns(t *testing.T, path string) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil || !(fn.Name.Name == "Classified" && fn.Recv != nil) && fn.Name.Name != "checkScript" {
			continue
		}
		bound := returnsOfCheckScript(fn.Body)
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if _, ok := n.(*ast.FuncLit); ok {
				return false
			}
			ret, ok := n.(*ast.ReturnStmt)
			if !ok {
				return true
			}
			if len(ret.Results) == 0 {
				t.Errorf("%s: a return with no result hides its error: name the result", fn.Name.Name)
				return true
			}
			last := ret.Results[len(ret.Results)-1]
			if id, ok := last.(*ast.Ident); ok && id.Name == "nil" {
				return true
			}
			if call, ok := last.(*ast.CallExpr); ok {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Errorf" {
					if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "fmt" {
						return true
					}
				}
			}
			expr := types.ExprString(last)
			if expr == "err" && bound[ret] {
				expr = boundErr
			}
			out = append(out, fn.Name.Name+": "+expr)
			return true
		})
	}
	return out
}

func TestTheTextsNameEveryRefusalBeforeARun(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := overlaytest.RepoRoot(t)
	read := func(parts ...string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(append([]string{root}, parts...)...))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}

	source := filepath.Join(root, "engine", "review", "evidencesh", "evidencesh.go")
	matched := make([]int, len(errorExceptions))
	for _, ret := range errorReturns(t, source) {
		named := false
		for i, e := range errorExceptions {
			if ret == e.fn+": "+e.expr {
				matched[i]++
				named = true
			}
		}
		if !named {
			t.Errorf("%s is an error return that is not nil or a literal fmt.Errorf: give it a fragment of the table with an fmt.Errorf, or name it in errorExceptions with its reason", ret)
		}
	}
	for i, e := range errorExceptions {
		if matched[i] != 1 {
			t.Errorf("the exception %s: %s (%s) is matched by %d returns, want exactly 1: drop a stale row or give the extra return a fragment", e.fn, e.expr, e.reason, matched[i])
		}
	}

	formats := errorFormats(t, source)
	if len(formats) < len(refusalFragments) {
		t.Fatalf("found %d errors before a run in Classified and checkScript, want at least %d: the parse no longer finds them", len(formats), len(refusalFragments))
	}
	for _, format := range formats {
		var held []string
		for _, fragment := range refusalFragments {
			if strings.Contains(format, fragment) {
				held = append(held, fragment)
			}
		}
		if len(held) != 1 {
			t.Errorf("the error %q holds %d fragments of the table %q, want exactly one: add a row for a refusal, or reword it", format, len(held), held)
		}
	}
	for _, fragment := range refusalFragments {
		found := false
		for _, format := range formats {
			found = found || strings.Contains(format, fragment)
		}
		if !found {
			t.Errorf("no error before a run holds the fragment %q: drop the row or put the words back", fragment)
		}
	}

	readme := textBetween(t, "README.md", read("README.md"), "gentle-ai-overlay review stop-hook\n", "gentle-ai-overlay roles builder-guard\n")
	changelog := textBetween(t, "CHANGELOG.md", read("CHANGELOG.md"), "- The Stop hook (`labdrian review stop-hook`)", "\n- ")
	for _, tc := range []struct{ name, text string }{
		{"the README sentence of the refusals", textBetween(t, "the README section", readme, "A script path that", "and not run")},
		{"the CHANGELOG sentence of the refusals", textBetween(t, "the CHANGELOG entry", changelog, "An evidence script path that", "not run")},
	} {
		for _, fragment := range refusalFragments {
			if !strings.Contains(tc.text, fragment) {
				t.Errorf("%s does not say %q", tc.name, fragment)
			}
		}
	}
}
