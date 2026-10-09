package runtime

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"testing"
)

// declaredIn names, in the package comment, the types of the one-adapter ports and the file that
// declares them: "<A> and <B> are declared in <file>".
var declaredIn = regexp.MustCompile(`(\w+) and\s+(?://\s*)?(\w+) are\s+declared in (\w+\.go)`)

// The package comment says where the ports of one adapter are declared. A reader who follows it
// must find them there, so the sentence is read and checked against the file it names; a port
// that moves, or a file that is renamed, fails here instead of leaving the comment to mislead.
func TestPackageCommentNamesTheFileThatDeclaresTheOneAdapterPorts(t *testing.T) {
	doc, err := os.ReadFile("doc.go")
	if err != nil {
		t.Fatal(err)
	}
	match := declaredIn.FindSubmatch(doc)
	if match == nil {
		t.Fatalf("doc.go has no sentence of the form %q", declaredIn)
	}
	file := string(match[3])
	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("doc.go names %s as the file that declares the ports, and it cannot be read: %v", file, err)
	}
	interfaces := map[string]bool{}
	for _, decl := range parsed.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			typeSpec := spec.(*ast.TypeSpec)
			if _, isInterface := typeSpec.Type.(*ast.InterfaceType); isInterface {
				interfaces[typeSpec.Name.Name] = true
			}
		}
	}
	for _, name := range []string{string(match[1]), string(match[2])} {
		if !interfaces[name] {
			t.Errorf("doc.go says %s declares the port %s, and it declares no interface of that name", file, name)
		}
	}
}
