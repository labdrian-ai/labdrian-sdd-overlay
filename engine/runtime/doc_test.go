package runtime

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strings"
	"testing"
)

// gap is what may separate two words of the package comment: spaces, or the end of one comment
// line and the start of the next.
const gap = `(?:\s|//)+`

// The package comment says where the ports of one adapter are declared, in two forms: "<A> and <B>
// are declared in <file>" and "<A> is declared in <file>".
var (
	declaredPlural   = regexp.MustCompile(`(\w+)` + gap + `and` + gap + `(\w+)` + gap + `are` + gap + `declared` + gap + `in` + gap + `(\w+\.go)`)
	declaredSingular = regexp.MustCompile(`(\w+)` + gap + `is` + gap + `declared` + gap + `in` + gap + `(\w+\.go)`)
)

// interfacesIn names the interface types a file declares.
func interfacesIn(t *testing.T, file string) map[string]bool {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("doc.go names %s as a file that declares ports, and it cannot be read: %v", file, err)
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
	return interfaces
}

// The package comment says where the ports of one adapter are declared. A reader who follows it
// must find them there, so each sentence is read and checked against the file it names; a port
// that moves, or a file that is renamed, fails here instead of leaving the comment to mislead.
func TestPackageCommentNamesTheFilesThatDeclareTheOneAdapterPorts(t *testing.T) {
	doc, err := os.ReadFile("doc.go")
	if err != nil {
		t.Fatal(err)
	}
	claims := map[string][]string{} // file -> the ports doc.go says it declares
	for _, m := range declaredPlural.FindAllSubmatch(doc, -1) {
		file := string(m[3])
		claims[file] = append(claims[file], string(m[1]), string(m[2]))
	}
	for _, m := range declaredSingular.FindAllSubmatch(doc, -1) {
		file := string(m[2])
		claims[file] = append(claims[file], string(m[1]))
	}
	if len(claims) == 0 {
		t.Fatalf("doc.go has no sentence of the form %q or %q", declaredPlural, declaredSingular)
	}
	for file, ports := range claims {
		interfaces := interfacesIn(t, file)
		for _, port := range ports {
			if !interfaces[port] {
				t.Errorf("doc.go says %s declares the port %s, and it declares no interface of that name", file, port)
			}
		}
	}
	for _, want := range []string{"pi_ports.go", "claude_ports.go"} {
		if _, ok := claims[want]; !ok {
			t.Errorf("doc.go does not say where the ports of %s are declared", strings.TrimSuffix(want, "_ports.go"))
		}
	}
}
