package main

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// TestMain_BuildsIndependentModule asserts that longterm-mem/ compiles as an
// independent Go module — capable of declaring its own third-party
// dependencies — separate from engine/'s zero-dependency module (R-001).
// The build output goes to a scratch directory (-o) so this test never
// leaves a compiled binary inside the module's own source tree.
func TestMain_BuildsIndependentModule(t *testing.T) {
	moduleRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve module root: %v", err)
	}

	cmd := exec.Command("go", "build", "-o", t.TempDir()+string(filepath.Separator), "./...")
	cmd.Dir = moduleRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build ./... failed in %s: %v\n%s", moduleRoot, err, out)
	}
}
