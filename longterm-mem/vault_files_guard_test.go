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

// The files of the vault are reached through the ports promote and ops own: the precedence store, the address
// map, the pages, the catalog, the log, the record of the last sync and the question of what is at the root
// (promote.PrecedenceRepository, AddressMapRecorder, PageRepository, IndexRepository, LogRepository and
// SyncStateRecorder; ops.PrecedenceReader, AddressMapReader, PageReader, IndexReader, LogReader, RootInspector
// and SyncStateReader), and the vault file system adapter (internal/vaultfs) is the one place that reads and
// writes them (L3). The architecture guard says it of `os` and the durable replacement, since the debt lines of
// promote and ops for them are gone; this guard says it of the rest of the file system and of the layout, which
// that guard cannot see: no production file of promote or ops imports any part of the file system, and none
// joins a name to a root with the layout or names the pages directory.
//
// What stays outside the adapter, and why: internal/vault runs the vault's own index scripts and reads and
// writes the markers and the sentinel of a provisioned vault, which are the scripts' files and not the
// promotion's (their names come from the layout); internal/vaultreg reads the registry of vaults, which lives
// beside the user's configuration and not in a vault.
var packagesThatReachTheVaultThroughPorts = []string{"internal/promote", "internal/ops"}

// fileSystemImports are the packages through which a file of the vault is read or written.
var fileSystemImports = []string{"os", "path/filepath", "io/ioutil", durablePackage}

// layoutNamesThatArePrivateToTheAdapter are the names of the layout whose use is the adapter's: Layout joins a
// name to the root of a vault, and PagesDir is where the pages are.
var layoutNamesThatArePrivateToTheAdapter = []string{"Layout", "PagesDir"}

// mayNameThePagesDirectory are the files that name the directory without reaching a page through it, each with
// the reason. Naming it is not reading it: the text of a refusal that the commands' output has always carried.
var mayNameThePagesDirectory = map[string]bool{
	// Reconcile's refusal of an address of the wrong shape says which directory the address would have named.
	"internal/promote/reconcile.go": true,
}

func TestTheVaultsFilesAreReachedOnlyThroughTheirPorts(t *testing.T) {
	problems, err := vaultAccessProblems(".", packagesThatReachTheVaultThroughPorts, fileSystemImports, layoutNamesThatArePrivateToTheAdapter, mayNameThePagesDirectory)
	if err != nil {
		t.Fatal(err)
	}
	for _, problem := range problems {
		t.Error(problem)
	}
}

// vaultAccessProblems reads the production files of the packages it is given, under root, and says what breaks
// the rule: a file importing a package of the file system, and a file selecting from the layout a name private
// to the adapter, unless it is among those allowed to. It reads the syntax tree, so an aliased import is still the
// package, a dot import of the layout still names what it selects, and a comment that names either is not a use.
// A package that is not there is an error: the rule would guard nothing.
func vaultAccessProblems(root string, packages, bannedImports, privateLayoutNames []string, allowedToNameTheLayout map[string]bool) ([]string, error) {
	var problems []string
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
			parsed, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, filepath.FromSlash(file)), nil, 0)
			if err != nil {
				return nil, err
			}
			for _, spec := range parsed.Imports {
				imported, err := strconv.Unquote(spec.Path.Value)
				if err != nil {
					return nil, err
				}
				for _, banned := range bannedImports {
					if imported == banned {
						problems = append(problems, file+" imports "+imported+": the vault's files are read and written through the ports the package owns, and the file system is the adapter's")
					}
				}
			}
			for _, layoutName := range privateLayoutNames {
				if selectsFromLayout(parsed, layoutName) && !allowedToNameTheLayout[file] {
					problems = append(problems, file+" names vaultlayout."+layoutName+": where the vault's files are is the file system adapter's to say, and the code reaches them through its ports")
				}
			}
		}
	}
	sort.Strings(problems)
	return problems, nil
}

// selectsFromLayout reports whether file selects name from the package imported as vaultlayout, whatever name
// it gives that import: under a dot import the name is bare, so any name that is not the selected name of
// another package's is the layout's.
func selectsFromLayout(file *ast.File, name string) bool {
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
			if pkg, ok := node.X.(*ast.Ident); ok && node.Sel.Name == name && pkg.Name == local {
				found = true
			}
		case *ast.Ident:
			// The walk reaches a selector before its parts, so a selected name is already known here.
			if local == "." && node.Name == name && !selected[node] {
				found = true
			}
		}
		return true
	})
	return found
}

// The rule is tested on trees built for it: a file that imports the file system (through an alias, a blank import
// or the durable replacement, too), a file that joins a name to a root or names the pages directory (through an
// alias, or by a dot import, too), and what is left alone.
func TestVaultAccessProblemsFlagsWhatBreaksTheRule(t *testing.T) {
	layout := func(body string) string {
		return "package p\n\nimport vl \"" + vaultLayoutPackage + "\"\n\n" + body + "\n"
	}
	root := writeTree(t, map[string]string{
		"internal/promote/clean.go":       importing(`"fmt"`),
		"internal/promote/uses_os.go":     importing(`"os"`),
		"internal/promote/alias.go":       importing(`file "os"`),
		"internal/promote/uses_path.go":   importing(`"path/filepath"`),
		"internal/promote/uses_ioutil.go": importing(`"io/ioutil"`),
		"internal/promote/blank.go":       importing(`_ "` + durablePackage + `"`),
		"internal/promote/names_dir.go":   layout("var dir = vl.PagesDir"),
		"internal/promote/joins_root.go":  layout("var l = vl.Layout{Root: \"/vault\"}"),
		"internal/promote/names_file.go":  layout("var f = vl.PageFile(\"c-000001\")"),
		"internal/promote/names_state.go": layout("var f = vl.SyncStateFile"),
		"internal/promote/in_comment.go":  "package p\n\n// vaultlayout.PagesDir and vaultlayout.Layout are the adapter's, and so is \"os\".\n",
		"internal/promote/dot_dir.go":     "package p\n\nimport . \"" + vaultLayoutPackage + "\"\n\nvar dir = PagesDir\n",
		"internal/promote/dot_layout.go":  "package p\n\nimport . \"" + vaultLayoutPackage + "\"\n\nvar l = Layout{}\n",
		"internal/promote/dot_file.go":    "package p\n\nimport . \"" + vaultLayoutPackage + "\"\n\nvar f = PageFile(\"c-000001\")\n",
		"internal/promote/dot_other.go":   "package p\n\nimport . \"" + vaultLayoutPackage + "\"\n\nvar f = PageFile(\"c-000001\")\n\nvar g = other.PagesDir\n",
		"internal/promote/dir_test.go":    "package p\n\nimport \"os\"\nimport \"" + vaultLayoutPackage + "\"\n\nvar dir = vaultlayout.PagesDir\n",
		"internal/promote/sub/deeper.go":  importing(`"os"`),
		"internal/ops/doctor.go":          layout("var dir = vl.PagesDir"),
		"internal/ops/uses_os.go":         importing(`"os"`),
		"internal/promote/allowed.go":     layout("var dir = vl.PagesDir"),
		"internal/vaultfs/anything.go":    importing(`"os"`),
	})
	private := []string{"Layout", "PagesDir"}
	imports := []string{"os", "path/filepath", "io/ioutil", durablePackage}

	got, err := vaultAccessProblems(root, []string{"internal/promote", "internal/ops"}, imports, private, map[string]bool{"internal/promote/allowed.go": true})
	if err != nil {
		t.Fatalf("vaultAccessProblems: %v", err)
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
		"internal/promote/uses_os.go":     1,
		"internal/promote/alias.go":       1,
		"internal/promote/uses_path.go":   1,
		"internal/promote/uses_ioutil.go": 1,
		"internal/promote/blank.go":       1,
		"internal/promote/names_dir.go":   1,
		"internal/promote/joins_root.go":  1,
		"internal/promote/dot_dir.go":     1,
		"internal/promote/dot_layout.go":  1,
		"internal/ops/doctor.go":          1,
		"internal/ops/uses_os.go":         1,
		"internal/promote/clean.go":       0,
		"internal/promote/names_file.go":  0,
		"internal/promote/names_state.go": 0,
		"internal/promote/in_comment.go":  0,
		"internal/promote/dot_file.go":    0,
		"internal/promote/dot_other.go":   0,
		"internal/promote/dir_test.go":    0,
		"internal/promote/sub/deeper.go":  0,
		"internal/promote/allowed.go":     0,
		"internal/vaultfs/anything.go":    0,
	}
	counts := map[string]int{}
	for file := range want {
		counts[file] = mentions(file)
	}
	if !reflect.DeepEqual(counts, want) {
		t.Fatalf("problems per file = %v, want %v\nproblems:\n%v", counts, want, got)
	}
}

func TestVaultAccessProblemsReportsAFileItCannotParseAndAPackageThatIsNotThere(t *testing.T) {
	root := writeTree(t, map[string]string{"internal/promote/broken.go": "package p\n\nimport (\n"})
	if _, err := vaultAccessProblems(root, []string{"internal/promote"}, nil, nil, nil); err == nil {
		t.Fatal("vaultAccessProblems accepted a file it could not parse; a guard that skips what it cannot read guards nothing")
	}
	if _, err := vaultAccessProblems(root, []string{"internal/gone"}, nil, nil, nil); err == nil {
		t.Fatal("vaultAccessProblems accepted a package that is not there; the rule would guard nothing")
	}
}
