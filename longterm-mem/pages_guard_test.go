// This file shares package guard with engram_adapter_guard_test.go: it uses writeTree and importing, defined
// in its walk test, and a different package name would stop it building.
package guard

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// vaultLayoutPackage is the import path of the list of where the vault keeps its files.
const vaultLayoutPackage = "github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultlayout"

// durablePackage is the import path of the module's atomic file replacement.
const durablePackage = "github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/durable"

// The pages of the vault, wiki/memory/<address>.md, are reached through the ports promote and ops own
// (promote.PageRepository, ops.PageReader), satisfied by the file system adapter (L3 slice 3). The package
// level debt of the architecture guard cannot say it, because promote still imports os for the files of
// slices to come (the catalog, the log and the sync state), so this guard says it by file: the files that
// read, patch, scan or write pages import no part of the file system, and no file of promote or ops names the
// pages directory. A file that is added to the pages' code starts under the rule by being listed.
var pageFilesWithoutTheFileSystem = map[string][]string{
	"internal/promote/address.go":     {"os", "path/filepath", durablePackage},
	"internal/promote/frontmatter.go": {"os", "path/filepath", durablePackage},
	"internal/promote/page.go":        {"os", "path/filepath", durablePackage},
	"internal/promote/pages.go":       {"os", "path/filepath", durablePackage},
	"internal/promote/propagate.go":   {"os", "path/filepath", durablePackage},
	"internal/promote/reconcile.go":   {"os", "path/filepath", durablePackage},
	"internal/promote/update.go":      {"os", "path/filepath", durablePackage},
	// The Writer still joins the vault root to the catalog and the log (slice 4), so it may name filepath; the
	// pages it reads and writes are the port's.
	"internal/promote/writer.go": {"os", durablePackage},
}

// packagesThatMustNotNameThePagesDirectory are the directories whose production files never select
// vaultlayout.PagesDir: where the pages are is the adapter's to say.
var packagesThatMustNotNameThePagesDirectory = []string{"internal/promote", "internal/ops"}

// mayNameThePagesDirectory are the files that name the directory without reaching a page through it, each with
// the reason. Naming it is not reading it: the text of a refusal that the commands' output has always carried.
var mayNameThePagesDirectory = map[string]bool{
	// Reconcile's refusal of an address of the wrong shape says which directory the address would have named.
	"internal/promote/reconcile.go": true,
}

func TestThePagesAreReachedOnlyThroughTheirPorts(t *testing.T) {
	problems, err := pageAccessProblems(".", pageFilesWithoutTheFileSystem, packagesThatMustNotNameThePagesDirectory, mayNameThePagesDirectory)
	if err != nil {
		t.Fatal(err)
	}
	for _, problem := range problems {
		t.Error(problem)
	}
}

// pageAccessProblems reads the files it is given and the production files of the packages it is given, under
// root, and says what breaks the rule: a listed file that is not there (the list would guard nothing), a listed
// file importing what it must not, and a production file of a package selecting vaultlayout.PagesDir that is
// not among those allowed to. It reads the syntax tree, so an aliased import is still the package and a comment
// that names it is not a use.
func pageAccessProblems(root string, banned map[string][]string, packages []string, allowedToNameTheDirectory map[string]bool) ([]string, error) {
	var problems []string

	files := make([]string, 0, len(banned))
	for file := range banned {
		files = append(files, file)
	}
	sort.Strings(files)
	for _, file := range files {
		parsed, err := parseFile(root, file)
		if os.IsNotExist(err) {
			problems = append(problems, file+" is listed as a file that reaches the pages through their port, and is not there: the guard looks at nothing")
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, spec := range parsed.Imports {
			imported, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return nil, err
			}
			for _, bannedImport := range banned[file] {
				if imported == bannedImport {
					problems = append(problems, file+" imports "+imported+": it reads and writes pages through the port the package owns, and the file system is the adapter's")
				}
			}
		}
	}

	for _, pkg := range packages {
		entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(pkg)))
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || filepath.Ext(name) != ".go" || strings.HasSuffix(name, "_test.go") {
				continue
			}
			file := path.Join(pkg, name)
			parsed, err := parseFile(root, file)
			if err != nil {
				return nil, err
			}
			if selectsPagesDir(parsed) && !allowedToNameTheDirectory[file] {
				problems = append(problems, file+" names vaultlayout.PagesDir: where the pages are is the file system adapter's to say, and the code reaches them through its port")
			}
		}
	}
	return problems, nil
}

func parseFile(root, file string) (*ast.File, error) {
	full := filepath.Join(root, filepath.FromSlash(file))
	if _, err := os.Stat(full); err != nil {
		return nil, err
	}
	return parser.ParseFile(token.NewFileSet(), full, nil, 0)
}

// selectsPagesDir reports whether file selects PagesDir from the package imported as vaultlayout, whatever
// name it gives that import: under a dot import the name is bare, so any PagesDir that is not the selected
// name of another package's is the layout's.
func selectsPagesDir(file *ast.File) bool {
	var local string
	for _, spec := range file.Imports {
		imported, err := strconv.Unquote(spec.Path.Value)
		if err != nil || imported != vaultLayoutPackage {
			continue
		}
		local = path.Base(imported)
		if spec.Name != nil {
			local = spec.Name.Name
		}
	}
	if local == "" || local == "_" {
		return false
	}
	found := false
	selected := map[*ast.Ident]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.SelectorExpr:
			selected[node.Sel] = true
			if pkg, ok := node.X.(*ast.Ident); ok && node.Sel.Name == "PagesDir" && pkg.Name == local {
				found = true
			}
		case *ast.Ident:
			// The walk reaches a selector before its parts, so a selected name is already known here.
			if local == "." && node.Name == "PagesDir" && !selected[node] {
				found = true
			}
		}
		return true
	})
	return found
}

// The rule is tested on trees built for it: a file that imports the file system, an aliased import of it, a
// file that names the pages directory (through an alias, or by a dot import, too), and what is left alone.
func TestPageAccessProblemsFlagsWhatBreaksTheRule(t *testing.T) {
	root := writeTree(t, map[string]string{
		"internal/promote/clean.go":      importing(`"fmt"`),
		"internal/promote/uses_os.go":    importing(`"os"`),
		"internal/promote/alias.go":      importing(`file "os"`),
		"internal/promote/blank.go":      importing(`_ "` + durablePackage + `"`),
		"internal/promote/names_dir.go":  "package p\n\nimport vl \"" + vaultLayoutPackage + "\"\n\nvar dir = vl.PagesDir\n",
		"internal/promote/names_file.go": "package p\n\nimport \"" + vaultLayoutPackage + "\"\n\nvar f = vaultlayout.PageFile(\"c-000001\")\n",
		"internal/promote/in_comment.go": "package p\n\n// vaultlayout.PagesDir is the adapter's, and so is \"os\".\n",
		"internal/promote/dot_dir.go":    "package p\n\nimport . \"" + vaultLayoutPackage + "\"\n\nvar dir = PagesDir\n",
		"internal/promote/dot_file.go":   "package p\n\nimport . \"" + vaultLayoutPackage + "\"\n\nvar f = PageFile(\"c-000001\")\n",
		"internal/promote/dot_other.go":  "package p\n\nimport . \"" + vaultLayoutPackage + "\"\n\nvar f = PageFile(\"c-000001\")\n\nvar g = other.PagesDir\n",
		"internal/promote/dir_test.go":   "package p\n\nimport \"" + vaultLayoutPackage + "\"\n\nvar dir = vaultlayout.PagesDir\n",
		"internal/ops/doctor.go":         "package p\n\nimport \"" + vaultLayoutPackage + "\"\n\nvar dir = vaultlayout.PagesDir\n",
		"internal/promote/allowed.go":    "package p\n\nimport \"" + vaultLayoutPackage + "\"\n\nvar dir = vaultlayout.PagesDir\n",
	})
	banned := map[string][]string{
		"internal/promote/clean.go":   {"os"},
		"internal/promote/uses_os.go": {"os"},
		"internal/promote/alias.go":   {"os"},
		"internal/promote/blank.go":   {durablePackage},
		"internal/promote/gone.go":    {"os"},
	}

	got, err := pageAccessProblems(root, banned, []string{"internal/promote", "internal/ops"}, map[string]bool{"internal/promote/allowed.go": true})
	if err != nil {
		t.Fatalf("pageAccessProblems: %v", err)
	}

	mentions := func(file string) int {
		n := 0
		for _, problem := range got {
			if strings.HasPrefix(problem, file+" ") {
				n++
			}
		}
		return n
	}
	want := map[string]int{
		"internal/promote/uses_os.go":    1,
		"internal/promote/alias.go":      1,
		"internal/promote/blank.go":      1,
		"internal/promote/gone.go":       1,
		"internal/promote/names_dir.go":  1,
		"internal/promote/dot_dir.go":    1,
		"internal/promote/dot_file.go":   0,
		"internal/promote/dot_other.go":  0,
		"internal/ops/doctor.go":         1,
		"internal/promote/clean.go":      0,
		"internal/promote/names_file.go": 0,
		"internal/promote/in_comment.go": 0,
		"internal/promote/dir_test.go":   0,
		"internal/promote/allowed.go":    0,
	}
	counts := map[string]int{}
	for file := range want {
		counts[file] = mentions(file)
	}
	if !reflect.DeepEqual(counts, want) {
		t.Fatalf("problems per file = %v, want %v\nproblems:\n%v", counts, want, got)
	}
}

func TestPageAccessProblemsReportsAFileItCannotParse(t *testing.T) {
	root := writeTree(t, map[string]string{"internal/promote/broken.go": "package p\n\nimport (\n"})
	if _, err := pageAccessProblems(root, nil, []string{"internal/promote"}, nil); err == nil {
		t.Fatal("pageAccessProblems accepted a file it could not parse; a guard that skips what it cannot read guards nothing")
	}
}
