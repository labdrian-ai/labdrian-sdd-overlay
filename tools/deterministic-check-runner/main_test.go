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

// TestCheckRegistry asserts the hardcoded v1 check set: exactly 4 checks,
// each declaring deterministic and blocking explicitly, each with a
// non-empty checkArgv.
func TestCheckRegistry(t *testing.T) {
	if len(registry) != 4 {
		t.Fatalf("len(registry) = %d, want 4", len(registry))
	}

	wantBlocking := map[string]bool{
		"gofmt":       true,
		"go vet":      true,
		"staticcheck": true,
		"deadcode":    false,
	}

	seen := map[string]bool{}
	for _, c := range registry {
		seen[c.name] = true
		if !c.deterministic {
			t.Errorf("check %q: deterministic = false, want true for all v1 checks", c.name)
		}
		want, ok := wantBlocking[c.name]
		if !ok {
			t.Errorf("check %q: unexpected name in registry", c.name)
			continue
		}
		if c.blocking != want {
			t.Errorf("check %q: blocking = %v, want %v", c.name, c.blocking, want)
		}
		if len(c.checkArgv) == 0 {
			t.Errorf("check %q: checkArgv is empty, want non-empty", c.name)
		}
	}
	for name := range wantBlocking {
		if !seen[name] {
			t.Errorf("registry missing expected check %q", name)
		}
	}
}

// TestClassify asserts classify is the sole enforcement point combining
// deterministic and blocking: a non-deterministic check can never classify
// as blocking, regardless of its blocking field (R-002).
func TestClassify(t *testing.T) {
	tests := []struct {
		name          string
		deterministic bool
		blocking      bool
		want          bool
	}{
		{"deterministic and blocking", true, true, true},
		{"deterministic, not blocking", true, false, false},
		{"not deterministic, blocking", false, true, false},
		{"neither deterministic nor blocking", false, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := check{name: "test", deterministic: tt.deterministic, blocking: tt.blocking, checkArgv: []string{"true"}}
			if got := classify(c); got != tt.want {
				t.Errorf("classify(%+v) = %v, want %v", c, got, tt.want)
			}
		})
	}
}

// TestRegistryInvariantNonDeterministicNeverBlocking guards the registry
// itself: no entry may declare blocking=true without deterministic=true.
// This must fail if a future edit adds a violating entry.
func TestRegistryInvariantNonDeterministicNeverBlocking(t *testing.T) {
	for _, c := range registry {
		if c.blocking && !c.deterministic {
			t.Errorf("registry invariant violated: check %q has blocking=true and deterministic=false", c.name)
		}
	}
}

// TestClassifyDeadcodeNonBlocking asserts deadcode classifies as
// non-blocking (WARNING severity) per the amended R-016 severity-
// proportional rule.
func TestClassifyDeadcodeNonBlocking(t *testing.T) {
	for _, c := range registry {
		if c.name != "deadcode" {
			continue
		}
		if classify(c) {
			t.Errorf("classify(deadcode) = true, want false (deadcode is WARNING-only)")
		}
		return
	}
	t.Fatal("deadcode not found in registry")
}
