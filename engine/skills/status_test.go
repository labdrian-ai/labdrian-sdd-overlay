package skills

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
)

// buildRegistryYAML returns a valid YAML registry string with the given
// number of core and custom entries, for use in status tests.
func buildRegistryYAML(coreCount, customCount int) string {
	var sb strings.Builder
	sb.WriteString("version: \"1\"\nskills:\n")
	for i := 0; i < coreCount; i++ {
		sb.WriteString(fmt.Sprintf(
			"  - id: core-skill-%d\n    path: core-skill-%d\n    source:\n      type: core\n      upstream:\n        owner: gentle-ai\n    install:\n      defaultScope: global\n      targets:\n        - claude\n    lifecycle:\n      updateStrategy: vendor-merge\n",
			i, i,
		))
	}
	for i := 0; i < customCount; i++ {
		sb.WriteString(fmt.Sprintf(
			"  - id: custom-skill-%d\n    path: custom-skill-%d\n    source:\n      type: custom\n    install:\n      defaultScope: global\n      targets:\n        - opencode\n    lifecycle:\n      updateStrategy: overlay-only\n",
			i, i,
		))
	}
	return sb.String()
}

func TestRenderStatusCore(t *testing.T) {
	t.Run("count_report", func(t *testing.T) {
		// SC-08: 3 core + 15 custom → stdout contains "18", "3", "15", "OK"; exit 0.
		yaml := buildRegistryYAML(3, 15)
		mockReadFile := func(_ string) ([]byte, error) {
			return []byte(yaml), nil
		}
		var out, errBuf bytes.Buffer
		exitCode := 0
		RenderStatusCore(nil, mockReadFile, &out, &errBuf, func(c int) { exitCode = c })
		if exitCode != 0 {
			t.Errorf("exit code = %d, want 0; stderr=%q", exitCode, errBuf.String())
		}
		outStr := out.String()
		for _, want := range []string{"18", "3", "15", "OK"} {
			if !strings.Contains(outStr, want) {
				t.Errorf("stdout %q missing field %q", outStr, want)
			}
		}
	})

	t.Run("empty_registry", func(t *testing.T) {
		// 0 entries → stdout contains "0" total and "OK"; exit 0.
		yaml := "version: \"1\"\nskills:\n"
		mockReadFile := func(_ string) ([]byte, error) {
			return []byte(yaml), nil
		}
		var out, errBuf bytes.Buffer
		exitCode := 0
		RenderStatusCore(nil, mockReadFile, &out, &errBuf, func(c int) { exitCode = c })
		if exitCode != 0 {
			t.Errorf("exit code = %d, want 0; stderr=%q", exitCode, errBuf.String())
		}
		outStr := out.String()
		if !strings.Contains(outStr, "0") {
			t.Errorf("stdout %q should contain 0 for total count", outStr)
		}
		if !strings.Contains(outStr, "OK") {
			t.Errorf("stdout %q missing OK", outStr)
		}
	})

	t.Run("missing_file", func(t *testing.T) {
		// R-025: unreadable file → stderr non-empty, exit 1.
		mockReadFile := func(_ string) ([]byte, error) {
			return nil, os.ErrNotExist
		}
		var out, errBuf bytes.Buffer
		exitCode := 0
		RenderStatusCore(nil, mockReadFile, &out, &errBuf, func(c int) { exitCode = c })
		if exitCode != 1 {
			t.Errorf("exit code = %d, want 1", exitCode)
		}
		if errBuf.Len() == 0 {
			t.Error("stderr must be non-empty on missing file")
		}
	})

	t.Run("no_manifest_access", func(t *testing.T) {
		// R-026: status reads registry only, never overlay.manifest.
		// Inject a readFile that panics on any path other than the registry default.
		const registryDefault = "skills.registry.yaml"
		yaml := buildRegistryYAML(1, 1)
		mockReadFile := func(path string) ([]byte, error) {
			if path != registryDefault {
				panic(fmt.Sprintf("status must not read non-registry path %q", path))
			}
			return []byte(yaml), nil
		}
		var out, errBuf bytes.Buffer
		exitCode := 0
		RenderStatusCore(nil, mockReadFile, &out, &errBuf, func(c int) { exitCode = c })
		if exitCode != 0 {
			t.Errorf("exit code = %d, want 0; stderr=%q", exitCode, errBuf.String())
		}
	})
}
