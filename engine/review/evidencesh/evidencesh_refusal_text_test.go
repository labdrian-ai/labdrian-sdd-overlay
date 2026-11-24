package evidencesh_test

// Every refusal the adapter makes before a run is named by the README and the CHANGELOG (T4b-e, `c67f772ff05b-6` F1 and
// F2, `-5.polisher`). The hand list of the docs tests missed twice (`c9440b24b20f-6` F6, then `c67f772ff05b-6` F2), so this
// test ties the texts to the code: it reads the format string of every fmt.Errorf of Classified and checkScript, the errors
// returned before a run (the errors after it come from refusal and answers, which it does not read), and holds each to one
// fragment of a table, each fragment to some error, and each fragment to the one sentence of the README and of the CHANGELOG
// that names the refusals. A refusal added without a fragment, or a fragment a text drops, fails. It reads the source of two
// functions, so an error moved into a helper fails it for a reason that is not a text; the fix is one table row or one moved
// line.

import (
	"go/ast"
	"go/parser"
	"go/token"
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

	formats := errorFormats(t, filepath.Join(root, "engine", "review", "evidencesh", "evidencesh.go"))
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
