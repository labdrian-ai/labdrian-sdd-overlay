package runtime_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Static import policy for Phase 7 (runtime adapters). These tests only parse
// import blocks; they run nothing and touch no runtime state, so the TestMain
// isolation of this package is not needed for them, only inherited.
//
// The policy is Decision 3 of the Phase 7 design: no os/exec in any new Phase
// 7 code, enforced statically, with the existing Pi adapter as the one named
// exception. It is modelled on engine/skills' zero-fetch import allowlist.

const (
	engineModulePath  = "github.com/labdrian-ai/labdrian-sdd-overlay/engine"
	longtermMemModule = "github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem"
)

// execImportAllowed names the only non-test files of engine/runtime that may
// import os/exec, each with the reason it is allowed.
//
// pi.go is the named Decision 3 exception. PiAdapter shells out to the pi CLI
// with a fixed argv (pi install, pi remove) and cannot do otherwise: the CLI
// owns ~/.pi/agent/settings.json. That boundary has already caused an
// incident: a test that reached it without a stub ran a real `pi remove` and
// deleted a live labdrian-pi package during `go test ./...`. It is why every
// test in this package runs under the TestMain guard in live_guard_test.go,
// with LABDRIAN_PI_BIN pointing at a path that does not exist. Pi installation
// is deliberately not refactored in Phase 7; the exception stays explicit and
// bounded to this one file.
var execImportAllowed = map[string]string{
	"pi.go": "Decision 3 exception: the Pi CLI adapter runs pi install and pi remove with a fixed argv",
}

// phase7Sources lists, relative to the engine root, the production files of
// Phase 7 code: a glob per package directory and an explicit filename for each
// Phase 7 file that lives in a shared package (engine/cmd). Later tasks
// append their own entries; an entry that matches no file fails the test, so
// a rename cannot silently drop a file out of the policy.
var phase7Sources = []string{
	"capability/*.go",
	"projection/*.go",
	"projection/fsstore/*.go",
	"cmd/projection_hook.go",
	"cmd/runtime_capabilities.go",
	"cmd/runtime_probe.go",
	"cmd/workflow_bind.go",
	// workflow_provenance.go is a Phase 6 file that now also holds the Phase 7
	// repository key (observeRepoKey), so it answers to the same policy: it
	// promises to find the repository by walking the filesystem, never by
	// running git.
	"cmd/workflow_provenance.go",
}

// forbiddenPhase7Import returns why path may not be imported by Phase 7 code,
// or the empty string when it may.
func forbiddenPhase7Import(path string) string {
	switch {
	case path == "os/exec":
		return "starts subprocesses; Decision 3 forbids os/exec in Phase 7 code (the named Pi exception is engine/runtime/pi.go)"
	case path == "net" || strings.HasPrefix(path, "net/"):
		return "network access; Phase 7 code makes no network calls"
	case path == longtermMemModule || strings.HasPrefix(path, longtermMemModule+"/"):
		return "depends on the longterm-mem module, which Phase 7 code must not import"
	case strings.Contains(path, "gentle-ai") || strings.Contains(path, "gentle-pi"):
		return "depends on Gentle AI or gentle-pi, which Phase 7 code must not import"
	case path == engineModulePath+"/runtime":
		return "imports engine/runtime, whose Pi adapter imports os/exec; Phase 7 packages sit below it (runtime may import them, never the reverse)"
	}
	return ""
}

// phase7ImportViolations returns one message per forbidden import, ordered by
// file and then by import path. imports maps a file (as it should be shown to
// the reader) to the import paths it declares.
func phase7ImportViolations(imports map[string][]string) []string {
	files := make([]string, 0, len(imports))
	for file := range imports {
		files = append(files, file)
	}
	sort.Strings(files)

	var violations []string
	for _, file := range files {
		paths := append([]string(nil), imports[file]...)
		sort.Strings(paths)
		for _, path := range paths {
			if reason := forbiddenPhase7Import(path); reason != "" {
				violations = append(violations, file+" imports "+strconv.Quote(path)+": "+reason)
			}
		}
	}
	return violations
}

// nonTestGoFiles returns the regular, non-test .go files directly inside dir,
// sorted, as paths joined onto dir.
func nonTestGoFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read directory %s: %v", dir, err)
	}
	var files []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.Type().IsRegular() && strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
			files = append(files, filepath.Join(dir, name))
		}
	}
	sort.Strings(files)
	return files
}

// importsOf returns the import paths declared by one Go source file. It
// parses only the import block.
func importsOf(t *testing.T, path string) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse imports of %s: %v", path, err)
	}
	imports := make([]string, 0, len(file.Imports))
	for _, spec := range file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			t.Fatalf("unquote import %s in %s: %v", spec.Path.Value, path, err)
		}
		imports = append(imports, importPath)
	}
	return imports
}

func TestPhase7ForbiddenImports(t *testing.T) {
	tests := []struct {
		path      string
		forbidden bool
	}{
		// Subprocesses.
		{"os/exec", true},
		// Network: the net package and everything under it.
		{"net", true},
		{"net/http", true},
		{"net/url", true},
		{"net/http/httptest", true},
		// The longterm-mem module, by exact path or as a parent of a package.
		{"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem", true},
		{"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/store", true},
		// Gentle AI and gentle-pi, wherever they are hosted.
		{"github.com/Gentleman-Programming/gentle-ai", true},
		{"github.com/Gentleman-Programming/gentle-ai/internal/review", true},
		{"example.com/vendor/gentle-pi/client", true},
		// engine/runtime carries the Pi adapter's os/exec dependency.
		{"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime", true},

		// Allowed: the standard library a declaration package needs...
		{"os", false},
		{"encoding/json", false},
		{"go/parser", false},
		{"path/filepath", false},
		{"unicode/utf8", false},
		// ...other engine packages, which Phase 7 code may build on...
		{"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow", false},
		{"github.com/labdrian-ai/labdrian-sdd-overlay/engine/capability", false},
		// ...and near misses that are not what the rules name.
		{"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-memory-notes", false},
		{"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtimeutil", false},
		{"example.com/internet", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			reason := forbiddenPhase7Import(tt.path)
			if tt.forbidden && reason == "" {
				t.Errorf("forbiddenPhase7Import(%q) = \"\", want a reason", tt.path)
			}
			if !tt.forbidden && reason != "" {
				t.Errorf("forbiddenPhase7Import(%q) = %q, want it allowed", tt.path, reason)
			}
		})
	}
}

// TestPhase7ImportViolationsNameTheFileAndTheImport pins the failure message
// contract: a reviewer reading a red run sees which file imported what.
func TestPhase7ImportViolationsNameTheFileAndTheImport(t *testing.T) {
	violations := phase7ImportViolations(map[string][]string{
		"capability/clean.go": {"encoding/json", "fmt"},
		"capability/exec.go":  {"fmt", "os/exec"},
		"cmd/net.go":          {"net/http", "os/exec"},
	})
	want := []string{
		`capability/exec.go imports "os/exec"`,
		`cmd/net.go imports "net/http"`,
		`cmd/net.go imports "os/exec"`,
	}
	if len(violations) != len(want) {
		t.Fatalf("got %d violations, want %d:\n%s", len(violations), len(want), strings.Join(violations, "\n"))
	}
	for i, w := range want {
		if !strings.HasPrefix(violations[i], w) {
			t.Errorf("violations[%d] = %q, want it to start with %q", i, violations[i], w)
		}
	}
}

// TestRuntimeExecAllowlist pins that within engine/runtime the only
// production file that imports os/exec is the one named in execImportAllowed.
// It also fails when the exception goes stale (pi.go stops importing os/exec),
// so the allowlist cannot outlive the reason it records.
func TestRuntimeExecAllowlist(t *testing.T) {
	files := nonTestGoFiles(t, ".")
	if len(files) == 0 {
		t.Fatal("no production .go files found in engine/runtime; the allowlist walk is broken")
	}

	allowed := make([]string, 0, len(execImportAllowed))
	for name := range execImportAllowed {
		allowed = append(allowed, name)
	}
	sort.Strings(allowed)

	importers := make(map[string]bool)
	for _, file := range files {
		name := filepath.Base(file)
		for _, imp := range importsOf(t, file) {
			if imp != "os/exec" {
				continue
			}
			importers[name] = true
			if _, ok := execImportAllowed[name]; !ok {
				t.Errorf("engine/runtime/%s imports os/exec; within engine/runtime only %s may (Decision 3). Widen execImportAllowed only after reviewer approval, with the reason", name, strings.Join(allowed, ", "))
			}
		}
	}
	for _, name := range allowed {
		if !importers[name] {
			t.Errorf("engine/runtime/%s is allowlisted to import os/exec but no longer does; remove the stale exception from execImportAllowed", name)
		}
	}
}

// TestPhase7PackagesImportNoExecOrNetwork pins that no Phase 7 production
// file imports os/exec, any net package, the longterm-mem module, Gentle AI or
// gentle-pi, or engine/runtime. _test.go files are exempt: later acceptance
// tests build and run the engine binary.
func TestPhase7PackagesImportNoExecOrNetwork(t *testing.T) {
	imports := make(map[string][]string)
	for _, pattern := range phase7Sources {
		matches, err := filepath.Glob(filepath.Join("..", filepath.FromSlash(pattern)))
		if err != nil {
			t.Fatalf("phase7Sources entry %q is not a valid glob: %v", pattern, err)
		}
		matched := 0
		for _, match := range matches {
			if strings.HasSuffix(match, "_test.go") {
				continue
			}
			rel, err := filepath.Rel("..", match)
			if err != nil {
				t.Fatalf("relative path of %s: %v", match, err)
			}
			imports[filepath.ToSlash(rel)] = importsOf(t, match)
			matched++
		}
		if matched == 0 {
			t.Errorf("phase7Sources entry %q matches no production .go file; the list is stale or the walk is broken", pattern)
		}
	}
	for _, violation := range phase7ImportViolations(imports) {
		t.Error(violation)
	}
}
