package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	return root
}

// TestModuleShape asserts parity with the entry-contract-validator sibling
// module's shape, and that this module has zero external dependencies.
func TestModuleShape(t *testing.T) {
	root := repoRoot(t)
	runnerDir := filepath.Join(root, "tools", "deterministic-check-runner")
	validatorDir := filepath.Join(root, "tools", "entry-contract-validator")

	for _, name := range []string{"go.mod", "main.go", "main_test.go"} {
		if _, err := os.Stat(filepath.Join(validatorDir, name)); err != nil {
			t.Fatalf("sibling entry-contract-validator/%s missing: %v", name, err)
		}
		if _, err := os.Stat(filepath.Join(runnerDir, name)); err != nil {
			t.Errorf("%s missing: %v", name, err)
		}
	}

	info, err := os.Stat(filepath.Join(runnerDir, "testdata"))
	if err != nil {
		t.Errorf("testdata/ missing: %v", err)
	} else if !info.IsDir() {
		t.Errorf("testdata is not a directory")
	}

	data, err := os.ReadFile(filepath.Join(runnerDir, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	modContent := string(data)
	const wantModule = "module github.com/labdrian-ai/labdrian-sdd-overlay/tools/deterministic-check-runner"
	if !strings.Contains(modContent, wantModule) {
		t.Errorf("go.mod module path = %q, want it to contain %q", modContent, wantModule)
	}
	if strings.Contains(modContent, "require") {
		t.Errorf("go.mod declares a dependency, want zero external dependencies: %q", modContent)
	}
}

// TestModuleDiscovery covers discoverModules: sorted output, relative-root
// normalization against the caller cwd, and subdirectory safety.
func TestModuleDiscovery(t *testing.T) {
	writeModule := func(t *testing.T, dir string) {
		t.Helper()
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("create module dir %s: %v", dir, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/mod\n\ngo 1.21\n"), 0o644); err != nil {
			t.Fatalf("write go.mod in %s: %v", dir, err)
		}
	}
	assertModules := func(t *testing.T, root string, want []string) {
		t.Helper()
		got, err := discoverModules(root)
		if err != nil {
			t.Fatalf("discoverModules(%q): %v", root, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("discoverModules(%q) = %v, want %v", root, got, want)
		}
	}

	t.Run("sorted output across nested modules", func(t *testing.T) {
		tempDir := t.TempDir()
		moduleA, moduleB := filepath.Join(tempDir, "moduleA"), filepath.Join(tempDir, "moduleB")
		nested := filepath.Join(tempDir, "moduleC", "nested")
		writeModule(t, moduleB)
		writeModule(t, moduleA)
		writeModule(t, nested)
		assertModules(t, tempDir, []string{moduleA, moduleB, nested})
	})

	t.Run("relative root normalizes against the caller cwd", func(t *testing.T) {
		tempDir := t.TempDir()
		only := filepath.Join(tempDir, "only")
		writeModule(t, only)

		previousWD, err := os.Getwd()
		if err != nil {
			t.Fatalf("get working directory: %v", err)
		}
		if err := os.Chdir(tempDir); err != nil {
			t.Fatalf("chdir into %s: %v", tempDir, err)
		}
		defer func() {
			if err := os.Chdir(previousWD); err != nil {
				t.Fatalf("restore working directory: %v", err)
			}
		}()
		assertModules(t, ".", []string{only})
	})

	t.Run("subdirectory root only discovers modules beneath it", func(t *testing.T) {
		tempDir := t.TempDir()
		outer := filepath.Join(tempDir, "outer")
		inner := filepath.Join(outer, "inner")
		writeModule(t, inner)
		writeModule(t, filepath.Join(tempDir, "sibling"))
		assertModules(t, outer, []string{inner})
	})
}
