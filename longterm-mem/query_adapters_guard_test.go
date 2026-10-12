// This file shares package guard with architecture_test.go (it reads the rings declared there) and with
// engram_adapter_guard_test.go and its walk test (writeTree and importing, defined there).
package guard

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/archguard"
)

// modulePath is the import path of this module.
const modulePath = "github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem"

// queryPackage is the package that reads memory, the vault and the embedding index through ports it owns.
const queryPackage = "internal/query"

// queryTestMayDriveARealStore is the one adapter a test of the query package may import: the Engram adapter,
// to put a real database behind the memory port a case drives (the engram guard allows the same). It cannot
// import the others, which import the query package: a test of the package that imports an adapter of the
// package is an import cycle, and a fake of the port is what the case wants.
const queryTestMayDriveARealStore = "internal/engram"

// TestQueryNamesNoAdapter keeps the query package free of the concrete adapters: every package of the module
// that the rings call an adapter is out of its reach, in its production files and in its tests alike (the
// dependency rule of the rings reads production code only, and a debt line would hide an import in a test as
// well). The ports it reads through are its own: VaultRetriever, EmbeddingIndexRepository,
// EmbeddingIndexBuilder and the memory ports.
func TestQueryNamesNoAdapter(t *testing.T) {
	problems, err := adapterImportsIn(".", queryPackage, adapterPackages(rings), queryTestMayDriveARealStore)
	if err != nil {
		t.Fatal(err)
	}
	for _, problem := range problems {
		t.Error(problem)
	}
}

// adapterPackages lists, as module-relative directories, the packages the rings call adapters.
func adapterPackages(rings map[string]archguard.Ring) []string {
	var adapters []string
	for dir, ring := range rings {
		if ring == archguard.Adapter {
			adapters = append(adapters, dir)
		}
	}
	sort.Strings(adapters)
	return adapters
}

// adapterImportsIn reads every Go file under pkg, tests included, and says which import an adapter: a file
// naming one of adapters (module-relative directories) is a problem, unless it is a test file and the adapter
// is testException. An import of a package below an adapter counts as the adapter. A file it cannot parse, or a
// package that is not there, is an error: the rule would guard nothing.
func adapterImportsIn(root, pkg string, adapters []string, testException string) ([]string, error) {
	var problems []string
	err := filepath.WalkDir(filepath.Join(root, filepath.FromSlash(pkg)), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		isTest := strings.HasSuffix(path, "_test.go")
		for _, spec := range file.Imports {
			imported, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			for _, adapter := range adapters {
				full := modulePath + "/" + adapter
				if imported != full && !strings.HasPrefix(imported, full+"/") {
					continue
				}
				if isTest && adapter == testException {
					continue
				}
				problems = append(problems, filepath.ToSlash(rel)+" imports "+imported+": "+pkg+" reads through the ports it owns, and "+adapter+" is an adapter")
			}
		}
		return nil
	})
	sort.Strings(problems)
	return problems, err
}

// The rule is tested on trees built for it: a production file and a test file importing an adapter (through an
// alias, a dot import or a blank import, too) are flagged; the one adapter a test may drive is let through in a
// test and not in production; an import of a domain package, of the standard library, and a file of another
// package are left alone.
func TestAdapterImportsInFlagsWhatBreaksTheRule(t *testing.T) {
	imp := func(dir string) string { return `"` + modulePath + "/" + dir + `"` }
	root := writeTree(t, map[string]string{
		"internal/query/clean.go":          importing(imp("internal/memory")),
		"internal/query/std.go":            importing(`"fmt"`),
		"internal/query/vault.go":          importing(imp("internal/vault")),
		"internal/query/aliased.go":        importing(`v ` + imp("internal/vecindex")),
		"internal/query/dot.go":            importing(`. ` + imp("internal/embed")),
		"internal/query/blank.go":          importing(`_ ` + imp("internal/vaultfs")),
		"internal/query/below.go":          importing(imp("internal/vecindex/sub")),
		"internal/query/engram.go":         importing(imp("internal/engram")),
		"internal/query/engram_test.go":    importing(imp("internal/engram")),
		"internal/query/vault_test.go":     importing(imp("internal/vault")),
		"internal/query/sub/deeper.go":     importing(imp("internal/embed")),
		"internal/query/testdata/x.go":     importing(imp("internal/vault")),
		"internal/query/notes.txt":         imp("internal/vault"),
		"internal/promote/other.go":        importing(imp("internal/vault")),
		"internal/vecindex/not_checked.go": importing(imp("internal/query")),
	})
	adapters := []string{"internal/embed", "internal/engram", "internal/vault", "internal/vaultfs", "internal/vecindex"}

	got, err := adapterImportsIn(root, "internal/query", adapters, "internal/engram")
	if err != nil {
		t.Fatalf("adapterImportsIn: %v", err)
	}

	var files []string
	for _, problem := range got {
		files = append(files, strings.SplitN(problem, " ", 2)[0])
	}
	want := []string{
		"internal/query/aliased.go",
		"internal/query/below.go",
		"internal/query/blank.go",
		"internal/query/dot.go",
		"internal/query/engram.go",
		"internal/query/sub/deeper.go",
		"internal/query/vault.go",
		"internal/query/vault_test.go",
	}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("flagged files = %v, want %v", files, want)
	}
}

func TestAdapterImportsInReportsAFileItCannotParseAndAPackageThatIsNotThere(t *testing.T) {
	root := writeTree(t, map[string]string{"internal/query/broken.go": "package p\n\nimport (\n"})
	if _, err := adapterImportsIn(root, "internal/query", nil, ""); err == nil {
		t.Fatal("adapterImportsIn accepted a file it could not parse")
	}
	if _, err := adapterImportsIn(root, "internal/gone", nil, ""); err == nil {
		t.Fatal("adapterImportsIn accepted a package that is not there")
	}
}

// The adapters the rule reads are the ones the rings declare, so a new adapter is covered the day it is added.
func TestAdapterPackagesAreTheRingsAdapters(t *testing.T) {
	got := adapterPackages(map[string]archguard.Ring{
		"a": archguard.Adapter, "b": archguard.Domain, "c": archguard.Adapter, "d": archguard.Application, "e": archguard.Root,
	})
	if want := []string{"a", "c"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("adapterPackages = %v, want %v", got, want)
	}
}
