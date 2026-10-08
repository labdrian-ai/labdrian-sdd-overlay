package skills

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// allowedImports is the fixed set of stdlib packages permitted in production
// (non-test) files of the engine/skills package (ADR-15).
//
// Any import outside this set turns this test RED. Adding a new import requires
// a reviewer to consciously widen this list — making a stealth network/exec
// dependency impossible to land silently. Notably absent: net, net/http, os/exec,
// and any package whose path contains "git".
//
// "errors" joined the list with the registry port (Phase 9 unit H15): ReadRegistry tells an
// unreadable store from an unusable registry with errors.As, so an adapter that wraps its
// error still answers. It is pure and reaches nothing the list exists to keep out.
//
// The module-internal exceptions are pathguardImport, a pure path-containment helper,
// jsonstrictImport (Phase 9, H19), the strict JSON checks the two records the domain reads are
// decoded through, and capabilityImport (Phase 9, H23), the one vocabulary of runtime targets a
// registry entry names. Their own imports are held to this same allowlist by
// TestZeroFetchCoversPathguardImports, TestZeroFetchCoversJsonstrictImports and
// TestZeroFetchCoversCapabilityImports, so the exceptions cannot widen the transitive surface of
// engine/skills.
var allowedImports = map[string]bool{
	"bufio":          true,
	"bytes":          true,
	"crypto/sha256":  true,
	"encoding/hex":   true,
	"encoding/json":  true,
	"errors":         true,
	"fmt":            true,
	"io":             true,
	"io/fs":          true,
	"path":           true,
	"path/filepath":  true,
	"reflect":        true,
	"regexp":         true,
	"sort":           true,
	"strings":        true,
	pathguardImport:  true,
	jsonstrictImport: true,
	capabilityImport: true,
}

// capabilityImportExtras is what engine/capability imports beyond allowedImports: the character
// classes of its identifier checks, pure like the rest of the standard library it uses.
var capabilityImportExtras = map[string]bool{
	"unicode":      true,
	"unicode/utf8": true,
}

// capabilityImport is the third module-internal package engine/skills may import: the vocabulary
// of runtime targets (H23), which a registry entry's install.targets is validated against. Like
// the other two it is exempted from the git-package ban by exact match, never by prefix.
const capabilityImport = "github.com/labdrian-ai/labdrian-sdd-overlay/engine/capability"

// jsonstrictImportExtras is what engine/jsonstrict imports beyond allowedImports: the UTF-8 check,
// pure like the rest of the standard library it uses.
var jsonstrictImportExtras = map[string]bool{
	"unicode/utf8": true,
}

// jsonstrictImport is the second module-internal package engine/skills may import: the strict
// decoding of the approval record and the project lock (H19, decision D4). Like pathguardImport it
// is exempted from the git-package ban by exact match, never by prefix.
const jsonstrictImport = "github.com/labdrian-ai/labdrian-sdd-overlay/engine/jsonstrict"

// pathguardImport is a module-internal package engine/skills may import.
// Its path contains "git" only through the github.com module host, so it is
// exempted from the git-package ban by exact match, never by prefix.
const pathguardImport = "github.com/labdrian-ai/labdrian-sdd-overlay/engine/pathguard"

// TestZeroFetchImportAllowlist statically parses every non-test .go file in
// engine/skills/ and asserts that each import path is a member of allowedImports
// (R-131, R-132, SC-69).
func TestZeroFetchImportAllowlist(t *testing.T) {
	fset := token.NewFileSet()

	// ParseDir with a filter that excludes _test.go files.
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("go/parser.ParseDir: %v", err)
	}

	for _, pkg := range pkgs {
		for filename, file := range pkg.Files {
			base := filepath.Base(filename)
			for _, imp := range file.Imports {
				// ImportSpec.Path.Value is a double-quoted string literal.
				path := strings.Trim(imp.Path.Value, `"`)
				if !allowedImports[path] {
					t.Errorf("forbidden/unexpected import %q in %s — widen allowedImports only after reviewer approval", path, base)
				}
			}
		}
	}

	// Verify the test itself found at least one file (guard against an empty walk
	// silently passing).
	totalFiles := 0
	for _, pkg := range pkgs {
		totalFiles += len(pkg.Files)
	}
	if totalFiles == 0 {
		t.Fatal("no production .go files found under '.'; the allowlist walk may be broken")
	}

	// Sanity: confirm the AST import type is what we expect.
	var _ *ast.ImportSpec
}

// TestZeroFetchAllowlistExcludesExecAndNet makes task 3a-ii.4's confirmation
// executable rather than a one-time reading: after the project-lock-format
// widening (encoding/json) and the project-lock-ownership widening
// (crypto/sha256, encoding/hex), os/exec, every net package and anything
// git-related must still be absent from the allowlist itself — so no future
// widening can smuggle one in without turning this test RED.
func TestZeroFetchAllowlistExcludesExecAndNet(t *testing.T) {
	for imp := range allowedImports {
		switch {
		case imp == "os/exec":
			t.Errorf("allowedImports must never contain %q", imp)
		case imp == "net" || strings.HasPrefix(imp, "net/"):
			t.Errorf("allowedImports must never contain the net package %q", imp)
		case imp != pathguardImport && imp != jsonstrictImport && imp != capabilityImport && strings.Contains(imp, "git"):
			t.Errorf("allowedImports must never contain a git-related package %q", imp)
		}
	}
	// The two ownership widenings are present and are the only ones this
	// slice added.
	for _, want := range []string{"crypto/sha256", "encoding/hex"} {
		if !allowedImports[want] {
			t.Errorf("expected %q in allowedImports after the project-lock-ownership widening", want)
		}
	}
	// 15 stdlib packages (errors is the one the registry port added, os the one Phase 9 unit H17
	// took away) plus the reviewer-approved pathguardImport, jsonstrictImport and capabilityImport
	// exceptions.
	if len(allowedImports) != 18 {
		t.Errorf("len(allowedImports) = %d, want 18 — widen it only after reviewer approval", len(allowedImports))
	}
	if allowedImports["os"] {
		t.Error(`"os" is in allowedImports: engine/skills reaches the file system through its ports (Phase 9 unit H17), and engine/skills/skillsfs is the one place that imports os`)
	}
	if !allowedImports["errors"] {
		t.Error(`expected "errors" in allowedImports: the registry port tells an unreadable store from an unusable registry with errors.As`)
	}
}

// TestZeroFetchCoversPathguardImports extends the zero-fetch guarantee through
// the pathguardImport exception: every import in the production files of
// engine/pathguard must itself be an allowlisted stdlib package, so no
// network, exec, git, or further internal dependency can reach engine/skills
// transitively through it.
func TestZeroFetchCoversPathguardImports(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, filepath.Join("..", "pathguard"), func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("go/parser.ParseDir(../pathguard): %v", err)
	}

	totalFiles := 0
	for _, pkg := range pkgs {
		for filename, file := range pkg.Files {
			totalFiles++
			base := filepath.Base(filename)
			for _, imp := range file.Imports {
				path := strings.Trim(imp.Path.Value, `"`)
				if path == pathguardImport || path == capabilityImport || !allowedImports[path] {
					t.Errorf("engine/pathguard imports %q in %s; it may import only allowlisted stdlib packages", path, base)
				}
			}
		}
	}
	if totalFiles == 0 {
		t.Fatal("no production .go files found under ../pathguard; the transitive guard walk may be broken")
	}
}

// TestZeroFetchCoversJsonstrictImports is TestZeroFetchCoversPathguardImports for jsonstrictImport:
// every import in the production files of engine/jsonstrict must be an allowlisted stdlib package or
// one of its pure extras, so nothing else can reach engine/skills transitively through it.
func TestZeroFetchCoversJsonstrictImports(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, filepath.Join("..", "jsonstrict"), func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("go/parser.ParseDir(../jsonstrict): %v", err)
	}

	totalFiles := 0
	for _, pkg := range pkgs {
		for filename, file := range pkg.Files {
			totalFiles++
			base := filepath.Base(filename)
			for _, imp := range file.Imports {
				path := strings.Trim(imp.Path.Value, `"`)
				if jsonstrictImportExtras[path] {
					continue
				}
				if path == jsonstrictImport || path == pathguardImport || path == capabilityImport || !allowedImports[path] {
					t.Errorf("engine/jsonstrict imports %q in %s; it may import only allowlisted stdlib packages", path, base)
				}
			}
		}
	}
	if totalFiles == 0 {
		t.Fatal("no production .go files found under ../jsonstrict; the transitive guard walk may be broken")
	}
}

// TestZeroFetchCoversCapabilityImports is TestZeroFetchCoversJsonstrictImports for capabilityImport:
// every import in the production files of engine/capability must be an allowlisted stdlib package
// or one of its pure extras, so nothing else can reach engine/skills transitively through it.
func TestZeroFetchCoversCapabilityImports(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, filepath.Join("..", "capability"), func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("go/parser.ParseDir(../capability): %v", err)
	}

	totalFiles := 0
	for _, pkg := range pkgs {
		for filename, file := range pkg.Files {
			totalFiles++
			base := filepath.Base(filename)
			for _, imp := range file.Imports {
				path := strings.Trim(imp.Path.Value, `"`)
				if capabilityImportExtras[path] {
					continue
				}
				if path == capabilityImport || path == pathguardImport || path == jsonstrictImport || !allowedImports[path] {
					t.Errorf("engine/capability imports %q in %s; it may import only allowlisted stdlib packages", path, base)
				}
			}
		}
	}
	if totalFiles == 0 {
		t.Fatal("no production .go files found under ../capability; the transitive guard walk may be broken")
	}
}
