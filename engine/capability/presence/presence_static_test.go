package presence_test

// Static guard for Decision 4: the presence prober only stats. The test parses
// the prober's source with go/parser and fails on any way to open, read, list,
// or run anything: an import of os/exec, net, io/ioutil, or syscall, a call to
// os.Open, os.OpenFile, os.ReadFile, os.ReadDir, or anything else on the os
// package that is not Lstat or Stat, or any use of the ioutil package. It runs
// nothing and touches no file but the source it parses.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// presenceSources are the files under the presence contract.
var presenceSources = []string{"presence.go"}

// allowedOSSelectors are the only members of package os the prober may use.
var allowedOSSelectors = map[string]bool{"Lstat": true, "Stat": true}

// presenceViolations parses one Go source (src is its text; name labels the
// messages) and returns why it breaks the contract, one message per finding.
func presenceViolations(t *testing.T, name, src string) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, name, src, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	var out []string
	// The local name each forbidden or watched package is imported under.
	local := map[string]string{}
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			t.Fatalf("unquote import in %s: %v", name, err)
		}
		switch {
		case path == "os/exec":
			out = append(out, name+" imports os/exec: the prober must not run programs")
		case path == "net" || strings.HasPrefix(path, "net/"):
			out = append(out, name+" imports "+path+": the prober must not use the network")
		case path == "io/ioutil":
			out = append(out, name+" imports io/ioutil: the prober must not read files")
		case path == "syscall":
			out = append(out, name+" imports syscall: the prober must go through os.Lstat and os.Stat only")
		}
		if path == "os" || path == "io/ioutil" {
			short := path[strings.LastIndex(path, "/")+1:]
			if spec.Name != nil {
				short = spec.Name.Name
			}
			local[short] = path
		}
	}
	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		id, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		switch local[id.Name] {
		case "os":
			if !allowedOSSelectors[sel.Sel.Name] {
				out = append(out, name+": "+fset.Position(sel.Pos()).String()+" uses os."+sel.Sel.Name+"; only os.Lstat and os.Stat are allowed")
			}
		case "io/ioutil":
			out = append(out, name+": "+fset.Position(sel.Pos()).String()+" uses ioutil."+sel.Sel.Name)
		}
		return true
	})
	sort.Strings(out)
	return out
}

// readSource returns the text of a production source of this package. The
// guard's own tests read source files; only the files in presenceSources answer
// to the contract.
func readSource(t *testing.T, file string) string {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	return string(data)
}

func TestPresenceProberSourceOnlyStats(t *testing.T) {
	for _, file := range presenceSources {
		src := readSource(t, file)
		for _, v := range presenceViolations(t, file, src) {
			t.Error(v)
		}
	}
}

// TestPresenceGuardCatchesEachForbiddenConstruct proves the guard itself: each
// snippet breaks the contract in one way and must be reported, and a clean
// snippet must pass. A guard that cannot fail proves nothing.
func TestPresenceGuardCatchesEachForbiddenConstruct(t *testing.T) {
	const clean = `package p
import "os"
func f() { _, _ = os.Lstat("x"); _, _ = os.Stat("x") }`
	if got := presenceViolations(t, "clean.go", clean); len(got) != 0 {
		t.Fatalf("clean source reported %v", got)
	}
	for name, src := range map[string]string{
		"os.Open":         "package p\nimport \"os\"\nfunc f() { _, _ = os.Open(\"x\") }",
		"os.OpenFile":     "package p\nimport \"os\"\nfunc f() { _, _ = os.OpenFile(\"x\", 0, 0) }",
		"os.ReadFile":     "package p\nimport \"os\"\nfunc f() { _, _ = os.ReadFile(\"x\") }",
		"os.ReadDir":      "package p\nimport \"os\"\nfunc f() { _, _ = os.ReadDir(\"x\") }",
		"os.Getenv":       "package p\nimport \"os\"\nfunc f() { _ = os.Getenv(\"HOME\") }",
		"aliased os":      "package p\nimport o \"os\"\nfunc f() { _, _ = o.Open(\"x\") }",
		"os as a value":   "package p\nimport \"os\"\nvar f = os.ReadFile",
		"os/exec":         "package p\nimport _ \"os/exec\"",
		"net":             "package p\nimport _ \"net\"",
		"net/http":        "package p\nimport _ \"net/http\"",
		"io/ioutil":       "package p\nimport \"io/ioutil\"\nfunc f() { _, _ = ioutil.ReadFile(\"x\") }",
		"io/ioutil alias": "package p\nimport u \"io/ioutil\"\nfunc f() { _, _ = u.ReadDir(\"x\") }",
		"syscall":         "package p\nimport _ \"syscall\"",
	} {
		if got := presenceViolations(t, "bad.go", src); len(got) == 0 {
			t.Errorf("%s: the guard reported nothing", name)
		}
	}
}
