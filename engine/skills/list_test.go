package skills

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// update enables golden-file regeneration: go test -run TestRenderListCore -update
var update = flag.Bool("update", false, "update golden files")

// listTestRegistryYAML is a 3-entry fixture with ids in non-alphabetical order.
const listTestRegistryYAML = `version: "1"
skills:
  - id: zebra-skill
    path: zebra-skill
    source:
      type: custom
    install:
      defaultScope: global
      targets:
        - claude
    lifecycle:
      updateStrategy: overlay-only
  - id: alpha-skill
    path: alpha-skill
    source:
      type: core
      upstream:
        owner: gentle-ai
    install:
      defaultScope: global
      targets:
        - claude
        - opencode
    lifecycle:
      updateStrategy: vendor-merge
  - id: middle-skill
    path: middle-skill
    source:
      type: custom
    install:
      defaultScope: global
      targets:
        - codex
    lifecycle:
      updateStrategy: overlay-only
`

func TestRenderListCore(t *testing.T) {
	t.Run("deterministic_output", func(t *testing.T) {
		// SC-06: 3 entries (non-sorted ids) → two calls produce byte-identical output.
		mockReadFile := func(_ string) ([]byte, error) {
			return []byte(listTestRegistryYAML), nil
		}

		var out1, err1 bytes.Buffer
		exitCode1 := 0
		RenderListCore(nil, mockReadFile, &out1, &err1, func(c int) { exitCode1 = c })
		if exitCode1 != 0 {
			t.Fatalf("first call exit %d; stderr=%q", exitCode1, err1.String())
		}

		var out2, err2 bytes.Buffer
		exitCode2 := 0
		RenderListCore(nil, mockReadFile, &out2, &err2, func(c int) { exitCode2 = c })
		if exitCode2 != 0 {
			t.Fatalf("second call exit %d; stderr=%q", exitCode2, err2.String())
		}

		if !bytes.Equal(out1.Bytes(), out2.Bytes()) {
			t.Error("outputs are not byte-identical (non-deterministic)")
		}

		lines := strings.Split(strings.TrimRight(out1.String(), "\n"), "\n")
		if len(lines) != 3 {
			t.Errorf("line count = %d, want 3", len(lines))
		}

		// Verify sorted order: alpha-skill < middle-skill < zebra-skill (R-021).
		if !strings.HasPrefix(lines[0], "alpha-skill") {
			t.Errorf("line[0] = %q, want prefix alpha-skill", lines[0])
		}
		if !strings.HasPrefix(lines[1], "middle-skill") {
			t.Errorf("line[1] = %q, want prefix middle-skill", lines[1])
		}
		if !strings.HasPrefix(lines[2], "zebra-skill") {
			t.Errorf("line[2] = %q, want prefix zebra-skill", lines[2])
		}

		// Verify each line contains id + source type + updateStrategy + targets (R-020).
		checks := []struct{ id, srcType, strategy, targets string }{
			{"alpha-skill", "core", "vendor-merge", "claude,opencode"},
			{"middle-skill", "custom", "overlay-only", "codex"},
			{"zebra-skill", "custom", "overlay-only", "claude"},
		}
		for i, c := range checks {
			for _, field := range []string{c.id, c.srcType, c.strategy, c.targets} {
				if !strings.Contains(lines[i], field) {
					t.Errorf("line[%d] %q missing field %q", i, lines[i], field)
				}
			}
		}

		// Golden file comparison.
		goldenPath := filepath.Join("testdata", "golden", "list_deterministic_output.txt")
		if *update {
			_ = os.MkdirAll(filepath.Dir(goldenPath), 0755)
			if err := os.WriteFile(goldenPath, out1.Bytes(), 0644); err != nil {
				t.Fatalf("update golden: %v", err)
			}
			t.Logf("updated golden: %s", goldenPath)
			return
		}
		golden, err := os.ReadFile(goldenPath)
		if err != nil {
			t.Fatalf("read golden %s: %v (run with -update to create)", goldenPath, err)
		}
		if !bytes.Equal(out1.Bytes(), golden) {
			t.Errorf("output differs from golden:\ngot:\n%swant:\n%s", out1.String(), string(golden))
		}
	})

	t.Run("missing_registry_file", func(t *testing.T) {
		// SC-07: missing file → exit 1, stderr non-empty, stdout empty.
		mockReadFile := func(_ string) ([]byte, error) {
			return nil, os.ErrNotExist
		}
		var out, errBuf bytes.Buffer
		exitCode := 0
		RenderListCore(nil, mockReadFile, &out, &errBuf, func(c int) { exitCode = c })
		if exitCode != 1 {
			t.Errorf("exit code = %d, want 1", exitCode)
		}
		if errBuf.Len() == 0 {
			t.Error("stderr must be non-empty on missing file")
		}
		if out.Len() != 0 {
			t.Errorf("stdout must be empty, got %q", out.String())
		}
	})

	t.Run("invalid_registry", func(t *testing.T) {
		// Parse error → exit 1, stderr non-empty.
		mockReadFile := func(_ string) ([]byte, error) {
			return []byte("{invalid: yaml: flow: content}"), nil
		}
		var out, errBuf bytes.Buffer
		exitCode := 0
		RenderListCore(nil, mockReadFile, &out, &errBuf, func(c int) { exitCode = c })
		if exitCode != 1 {
			t.Errorf("exit code = %d, want 1", exitCode)
		}
		if errBuf.Len() == 0 {
			t.Error("stderr must be non-empty on parse error")
		}
	})
}
