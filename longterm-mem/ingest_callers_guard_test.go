// This file shares package guard with engram_adapter_guard_test.go and its walk test: it uses skipDirectory and
// within, defined in the first, writeTree and importing, defined in the second, and modulePath, defined in
// query_adapters_guard_test.go; a different package name would stop it building.
package guard

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// ingestImport is the import path of the ingestion package, whose SourceID derives the identity of a source
// from an origin and refuses a file or directory origin that is not absolute: ResolveOrigin is what makes one
// absolute, by asking the SourceReader where a relative path begins.
const ingestImport = modulePath + "/internal/ingest"

// TestEveryCallerOfSourceIDResolvesTheOriginFirst keeps the refusal of a relative path from being a surprise: no
// production file of the module outside internal/ingest uses ingest.SourceID unless it also uses
// ingest.ResolveOrigin. Nothing calls SourceID today (the extraction use case is task 2b of
// longterm-mem-knowledge-ingestion), so this is what makes "every caller resolves the origin first" a fact the
// build checks and not a hope: the first caller that forgets fails here, not in front of an operator who typed a
// relative path.
//
// The rule is read per file, from the syntax tree, whatever name the file gives the import. It cannot tell that
// the origin ResolveOrigin was given is the one SourceID is asked about, and a use case that resolves in one file
// and derives in another is read as a violation: it is a floor under the rule, and the tests of the use case
// (a relative path through the port) are what hold the rest.
func TestEveryCallerOfSourceIDResolvesTheOriginFirst(t *testing.T) {
	problems, err := sourceIDWithoutResolveOriginIn(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, problem := range problems {
		t.Error(problem)
	}
}

// sourceIDWithoutResolveOriginIn lists, as slash-separated paths relative to root followed by the reason, the
// production Go files outside internal/ingest that select SourceID from the ingestion package without selecting
// ResolveOrigin from it, and those that dot-import it (which would hide both). Tests are not looked at, and nor
// are directories named testdata or vendor, hidden and underscore directories, and any directory below root
// that holds a go.mod (another module). A file it cannot parse is an error, not a skip.
func sourceIDWithoutResolveOriginIn(root string) ([]string, error) {
	var problems []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			return skipDirectory(root, path, rel, d.Name())
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if within(filepath.ToSlash(filepath.Dir(rel)), "internal/ingest") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		name, imported, err := ingestName(file)
		if err != nil {
			return err
		}
		switch {
		case !imported || name == "_":
			return nil
		case name == ".":
			problems = append(problems, rel+" dot-imports the ingestion package, which would hide whether it resolves the origin before SourceID: import it by name")
			return nil
		}
		uses := map[string]bool{}
		ast.Inspect(file, func(n ast.Node) bool {
			if selector, ok := n.(*ast.SelectorExpr); ok {
				if ident, ok := selector.X.(*ast.Ident); ok && ident.Name == name {
					uses[selector.Sel.Name] = true
				}
			}
			return true
		})
		if uses["SourceID"] && !uses["ResolveOrigin"] {
			problems = append(problems, rel+" uses ingest.SourceID but never ingest.ResolveOrigin: SourceID refuses a relative file or directory origin, so the caller resolves it through the SourceReader first")
		}
		return nil
	})
	sort.Strings(problems)
	return problems, err
}

// ingestName reports the name file gives the ingestion package and whether it imports it at all.
func ingestName(file *ast.File) (name string, imported bool, err error) {
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			return "", false, err
		}
		if path != ingestImport {
			continue
		}
		if spec.Name != nil {
			return spec.Name.Name, true, nil
		}
		return filepath.Base(path), true, nil
	}
	return "", false, nil
}

// The rule is tested on a tree built for it: a file that derives an id without resolving the origin is flagged,
// through the package's own name, through an alias and through a dot import; a file that resolves first, one
// that only resolves, one that names another package's SourceID, the package itself, its tests and the files the
// walk does not look at are not.
func TestSourceIDCallersGuardFlagsOnlyAFileThatDerivesWithoutResolving(t *testing.T) {
	ingest := `"` + ingestImport + `"`
	use := func(importLine, body string) string {
		return "package p\n\nimport " + importLine + "\n\n" + body + "\n"
	}
	root := writeTree(t, map[string]string{
		"cmd/longterm-mem/resolves_first.go": use(ingest, "var _ = ingest.ResolveOrigin\nvar _ = ingest.SourceID"),
		"cmd/longterm-mem/only_resolves.go":  use(ingest, "var _ = ingest.ResolveOrigin"),
		"cmd/longterm-mem/aliased_ok.go":     use("ing "+ingest, "var _ = ing.ResolveOrigin\nvar _ = ing.SourceID"),
		"cmd/longterm-mem/blank.go":          use("_ "+ingest, ""),
		"internal/promote/other_id.go":       use(`ingest "example.com/elsewhere/ingest"`, "var _ = ingest.SourceID"),
		"internal/promote/no_import.go":      "package p\n\nfunc SourceID() {}\n",
		"internal/promote/derives.go":        use(ingest, "var _ = ingest.SourceID"),
		"internal/query/aliased.go":          use("ing "+ingest, "var _ = ing.SourceID"),
		"internal/query/shadow.go":           use("ing "+ingest, "var ingest struct{ SourceID int }\nvar _ = ingest.SourceID"),
		"internal/vaultfs/dot.go":            use(". "+ingest, "var _ = SourceID"),
		"internal/ingest/header.go":          use(ingest, "var _ = ingest.SourceID"),
		"internal/ingest/sub/helper.go":      use(ingest, "var _ = ingest.SourceID"),
		"internal/query/query_test.go":       use(ingest, "var _ = ingest.SourceID"),
		"internal/query/testdata/x.go":       use(ingest, "var _ = ingest.SourceID"),
		"other/go.mod":                       "module other\n",
		"other/x.go":                         use(ingest, "var _ = ingest.SourceID"),
	})

	got, err := sourceIDWithoutResolveOriginIn(root)
	if err != nil {
		t.Fatalf("sourceIDWithoutResolveOriginIn: %v", err)
	}

	var files []string
	for _, problem := range got {
		files = append(files, strings.SplitN(problem, " ", 2)[0])
	}
	want := []string{
		"internal/promote/derives.go",
		"internal/query/aliased.go",
		"internal/vaultfs/dot.go",
	}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("flagged = %v, want %v (the package's own files, tests, testdata, another module, and a file that resolves first, only resolves, imports another ingest or none, or selects SourceID from something that is not the package, pass)", files, want)
	}
}

func TestSourceIDCallersGuardFailsOnAFileItCannotParse(t *testing.T) {
	root := writeTree(t, map[string]string{"internal/promote/broken.go": "package p\n\nfunc (\n"})

	if _, err := sourceIDWithoutResolveOriginIn(root); err == nil {
		t.Error("a file that does not parse passed: the rule would guard nothing")
	}
}
