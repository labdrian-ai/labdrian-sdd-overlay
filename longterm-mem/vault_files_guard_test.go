// This file shares package guard with engram_adapter_guard_test.go: it uses writeTree and importing, defined
// in its walk test, and a different package name would stop it building.
package guard

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path"
	"path/filepath"
	"reflect"
	"slices"
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

// mayNameTheLayout are the files that name a part of the layout without reaching the vault through it, each with
// the names it may select and the reason. The permission is per name: a file allowed to name PagesDir may not
// join a name to a root with Layout. Naming a directory is not reading it: the text of a refusal that the
// commands' output has always carried. An entry that is not used (the file is gone, or no longer selects the
// name) is itself a problem, so the list cannot keep a permission nobody needs.
var mayNameTheLayout = map[string][]string{
	// Reconcile's refusal of an address of the wrong shape says which directory the address would have named.
	"internal/promote/reconcile.go": {"PagesDir"},
}

func TestTheVaultsFilesAreReachedOnlyThroughTheirPorts(t *testing.T) {
	problems, err := vaultAccessProblems(".", packagesThatReachTheVaultThroughPorts, fileSystemImports, layoutNamesThatArePrivateToTheAdapter, mayNameTheLayout)
	if err != nil {
		t.Fatal(err)
	}
	for _, problem := range problems {
		t.Error(problem)
	}
}

// vaultAccessProblems reads the production files of the packages it is given, under root, and says what breaks
// the rule: a file importing a package of the file system, and a file selecting from the layout a name private
// to the adapter, unless the file is allowed that name. It reads the syntax tree, so an aliased import is still the
// package, a dot import of the layout still names what it selects, and a comment that names either is not a use.
// It walks each package's directory tree, so a subpackage is held to the rule as the package is (a directory
// named testdata is not Go to the build and is left alone). A package that is not there is an error: the rule
// would guard nothing. A permission in allowedToNameTheLayout (file to the names it may select) that nothing
// uses is a problem of its own.
func vaultAccessProblems(root string, packages, bannedImports, privateLayoutNames []string, allowedToNameTheLayout map[string][]string) ([]string, error) {
	var problems []string
	used := map[string]bool{}
	for _, pkg := range packages {
		err := filepath.WalkDir(filepath.Join(root, filepath.FromSlash(pkg)), func(full string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			name := entry.Name()
			if entry.IsDir() {
				if name == "testdata" {
					return filepath.SkipDir
				}
				return nil
			}
			if filepath.Ext(name) != ".go" || strings.HasSuffix(name, "_test.go") {
				return nil
			}
			rel, err := filepath.Rel(root, full)
			if err != nil {
				return err
			}
			file := filepath.ToSlash(rel)
			parsed, err := parser.ParseFile(token.NewFileSet(), full, nil, 0)
			if err != nil {
				return err
			}
			for _, spec := range parsed.Imports {
				imported, err := strconv.Unquote(spec.Path.Value)
				if err != nil {
					return err
				}
				for _, banned := range bannedImports {
					if imported == banned {
						problems = append(problems, file+" imports "+imported+": the vault's files are read and written through the ports the package owns, and the file system is the adapter's")
					}
				}
			}
			for _, layoutName := range privateLayoutNames {
				if !selectsFromLayout(parsed, layoutName) {
					continue
				}
				if slices.Contains(allowedToNameTheLayout[file], layoutName) {
					used[file+" "+layoutName] = true
					continue
				}
				problems = append(problems, file+" names vaultlayout."+layoutName+": where the vault's files are is the file system adapter's to say, and the code reaches them through its ports")
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	for file, names := range allowedToNameTheLayout {
		for _, layoutName := range names {
			if !used[file+" "+layoutName] {
				problems = append(problems, file+" is allowed to name vaultlayout."+layoutName+" and does not: the file is gone or no longer needs it, so remove the permission")
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
		"internal/promote/clean.go":          importing(`"fmt"`),
		"internal/promote/uses_os.go":        importing(`"os"`),
		"internal/promote/alias.go":          importing(`file "os"`),
		"internal/promote/uses_path.go":      importing(`"path/filepath"`),
		"internal/promote/uses_ioutil.go":    importing(`"io/ioutil"`),
		"internal/promote/blank.go":          importing(`_ "` + durablePackage + `"`),
		"internal/promote/names_dir.go":      layout("var dir = vl.PagesDir"),
		"internal/promote/joins_root.go":     layout("var l = vl.Layout{Root: \"/vault\"}"),
		"internal/promote/names_file.go":     layout("var f = vl.PageFile(\"c-000001\")"),
		"internal/promote/names_state.go":    layout("var f = vl.SyncStateFile"),
		"internal/promote/in_comment.go":     "package p\n\n// vaultlayout.PagesDir and vaultlayout.Layout are the adapter's, and so is \"os\".\n",
		"internal/promote/dot_dir.go":        "package p\n\nimport . \"" + vaultLayoutPackage + "\"\n\nvar dir = PagesDir\n",
		"internal/promote/dot_layout.go":     "package p\n\nimport . \"" + vaultLayoutPackage + "\"\n\nvar l = Layout{}\n",
		"internal/promote/dot_file.go":       "package p\n\nimport . \"" + vaultLayoutPackage + "\"\n\nvar f = PageFile(\"c-000001\")\n",
		"internal/promote/dot_other.go":      "package p\n\nimport . \"" + vaultLayoutPackage + "\"\n\nvar f = PageFile(\"c-000001\")\n\nvar g = other.PagesDir\n",
		"internal/promote/dir_test.go":       "package p\n\nimport \"os\"\nimport \"" + vaultLayoutPackage + "\"\n\nvar dir = vaultlayout.PagesDir\n",
		"internal/promote/sub/deeper.go":     importing(`"os"`),
		"internal/promote/testdata/x.go":     importing(`"os"`),
		"internal/promote/stale.go":          importing(`"fmt"`),
		"internal/promote/allowed_layout.go": layout("var dir = vl.PagesDir\nvar l = vl.Layout{}"),
		"internal/promote/partial.go":        layout("var dir = vl.PagesDir"),
		"internal/ops/doctor.go":             layout("var dir = vl.PagesDir"),
		"internal/ops/uses_os.go":            importing(`"os"`),
		"internal/promote/allowed.go":        layout("var dir = vl.PagesDir"),
		"internal/vaultfs/anything.go":       importing(`"os"`),
	})
	private := []string{"Layout", "PagesDir"}
	allowed := map[string][]string{
		"internal/promote/allowed.go":        {"PagesDir"},
		"internal/promote/allowed_layout.go": {"PagesDir"},
		// Allowed both names and selecting one: the permission it does not use is the problem.
		"internal/promote/partial.go": {"PagesDir", "Layout"},
		"internal/promote/stale.go":   {"PagesDir"},
		"internal/promote/gone.go":    {"PagesDir"},
	}
	imports := []string{"os", "path/filepath", "io/ioutil", durablePackage}

	got, err := vaultAccessProblems(root, []string{"internal/promote", "internal/ops"}, imports, private, allowed)
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
		"internal/promote/uses_os.go":        1,
		"internal/promote/alias.go":          1,
		"internal/promote/uses_path.go":      1,
		"internal/promote/uses_ioutil.go":    1,
		"internal/promote/blank.go":          1,
		"internal/promote/names_dir.go":      1,
		"internal/promote/joins_root.go":     1,
		"internal/promote/dot_dir.go":        1,
		"internal/promote/dot_layout.go":     1,
		"internal/ops/doctor.go":             1,
		"internal/ops/uses_os.go":            1,
		"internal/promote/clean.go":          0,
		"internal/promote/names_file.go":     0,
		"internal/promote/names_state.go":    0,
		"internal/promote/in_comment.go":     0,
		"internal/promote/dot_file.go":       0,
		"internal/promote/dot_other.go":      0,
		"internal/promote/dir_test.go":       0,
		"internal/promote/sub/deeper.go":     1,
		"internal/promote/allowed_layout.go": 1,
		"internal/promote/partial.go":        1,
		"internal/promote/allowed.go":        0,
		"internal/promote/testdata/x.go":     0,
		"internal/promote/stale.go":          1,
		"internal/promote/gone.go":           1,
		"internal/vaultfs/anything.go":       0,
	}
	counts := map[string]int{}
	for file := range want {
		counts[file] = mentions(file)
	}
	if !reflect.DeepEqual(counts, want) {
		t.Fatalf("problems per file = %v, want %v\nproblems:\n%v", counts, want, got)
	}
	// A file allowed two names that selects one: the problem is the unused permission, and it names it.
	// The problems are found by the same prefix the counts above use, and the loop fails when it finds none,
	// so a change of the message format cannot leave it checking nothing.
	partial := 0
	for _, problem := range got {
		if strings.HasPrefix(problem, "internal/promote/partial.go ") {
			partial++
			if !strings.Contains(problem, "vaultlayout.Layout") {
				t.Errorf("the problem of partial.go = %q, want the unused permission for Layout", problem)
			}
		}
	}
	if partial != 1 {
		t.Errorf("found %d problems of partial.go, want the one for the unused permission", partial)
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
