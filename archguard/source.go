package archguard

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type refKind int

const (
	refImport    refKind = iota // the file imports pkg
	refMember                   // the file selects member from pkg
	refDotImport                // the file dot-imports pkg
)

// reference is one thing a file takes from another package.
type reference struct {
	kind   refKind
	pkg    string // the import path as written
	member string // refMember only
	file   string // slash-separated, relative to the module root
}

// sourcePackage is the directory of a package and everything its production files
// reference.
type sourcePackage struct {
	dir  string // slash-separated, relative to the module root; "." is the root
	refs []reference
}

func readModulePath(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("archguard: %w (root must be a module root with a go.mod)", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.Trim(strings.TrimSpace(rest), `"`), nil
		}
	}
	return "", fmt.Errorf("archguard: no module line in %s", filepath.Join(root, "go.mod"))
}

// loadPackages parses every production .go file under root. A directory with Go
// files of any kind is a package, so a test-only directory must declare a ring
// too. It skips what the go tool skips (hidden and underscore directories,
// vendor) and a nested module. It does not skip testdata: a Go package that lives
// there is code like any other.
func loadPackages(root string) ([]sourcePackage, error) {
	fset := token.NewFileSet()
	byDir := map[string]*sourcePackage{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDirectory(root, p, d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		dir := path.Dir(rel)
		pkg := byDir[dir]
		if pkg == nil {
			pkg = &sourcePackage{dir: dir}
			byDir[dir] = pkg
		}
		if strings.HasSuffix(p, "_test.go") {
			return nil // a test file makes its directory a package, but is not read
		}
		file, err := parser.ParseFile(fset, p, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		pkg.refs = append(pkg.refs, collectReferences(file, rel)...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	packages := make([]sourcePackage, 0, len(byDir))
	for _, pkg := range byDir {
		packages = append(packages, *pkg)
	}
	sort.Slice(packages, func(i, j int) bool { return packages[i].dir < packages[j].dir })
	return packages, nil
}

func skipDirectory(root, p, name string) bool {
	if p == root {
		return false
	}
	if name == "vendor" || name == "node_modules" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
		return true
	}
	_, err := os.Stat(filepath.Join(p, "go.mod"))
	return err == nil
}

// collectReferences reads the imports of a file and, for the guarded standard
// packages, the members it selects.
func collectReferences(file *ast.File, rel string) []reference {
	var refs []reference
	localNames := map[string]string{} // the name a file gives a guarded import -> its path
	for _, spec := range file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		refs = append(refs, reference{kind: refImport, pkg: importPath, file: rel})
		name := importName(importPath)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		switch {
		case name == "_":
		case name == ".":
			if isAmbientPackage(importPath) {
				refs = append(refs, reference{kind: refDotImport, pkg: importPath, file: rel})
			}
		case isAmbientPackage(importPath):
			localNames[name] = importPath
		}
	}
	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if id, ok := sel.X.(*ast.Ident); ok {
			if importPath, ok := localNames[id.Name]; ok {
				refs = append(refs, reference{kind: refMember, pkg: importPath, member: sel.Sel.Name, file: rel})
			}
		}
		return true
	})
	return refs
}

var majorVersion = regexp.MustCompile(`^v[0-9]+$`)

// importName is the name a package gives itself when the import does not rename
// it: the last path element, or the one before a /vN major-version suffix.
func importName(importPath string) string {
	elements := strings.Split(importPath, "/")
	name := elements[len(elements)-1]
	if len(elements) > 1 && majorVersion.MatchString(name) {
		name = elements[len(elements)-2]
	}
	return name
}
