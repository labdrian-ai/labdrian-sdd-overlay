package skills

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// skillsTestRegistryYAML is a minimal two-entry fixture for dispatcher tests.
const skillsTestRegistryYAML = `version: "1"
skills:
  - id: test-core
    path: test-core
    source:
      type: core
      upstream:
        owner: gentle-ai
    install:
      defaultScope: global
      targets:
        - claude
    lifecycle:
      updateStrategy: vendor-merge
  - id: test-custom
    path: test-custom
    source:
      type: custom
    install:
      defaultScope: global
      targets:
        - opencode
    lifecycle:
      updateStrategy: overlay-only
`

// skillsMockReadFile returns the two-entry fixture for any path.
func skillsMockReadFile(_ string) ([]byte, error) {
	return []byte(skillsTestRegistryYAML), nil
}

func TestSkillsCore(t *testing.T) {
	t.Run("verb_list", func(t *testing.T) {
		// "list" with valid registry → exit 0, output has entries.
		var out, errBuf bytes.Buffer
		exitCode := 0
		SkillsCore("list", nil, skillsMockReadFile, &out, &errBuf, func(c int) { exitCode = c })
		if exitCode != 0 {
			t.Errorf("exit code = %d, want 0; stderr=%q", exitCode, errBuf.String())
		}
		if out.Len() == 0 {
			t.Error("stdout must be non-empty for list")
		}
		if !strings.Contains(out.String(), "test-core") {
			t.Errorf("stdout %q missing test-core entry", out.String())
		}
	})

	t.Run("verb_status", func(t *testing.T) {
		// "status" with valid registry → exit 0, counts and OK in stdout.
		var out, errBuf bytes.Buffer
		exitCode := 0
		SkillsCore("status", nil, skillsMockReadFile, &out, &errBuf, func(c int) { exitCode = c })
		if exitCode != 0 {
			t.Errorf("exit code = %d, want 0; stderr=%q", exitCode, errBuf.String())
		}
		if !strings.Contains(out.String(), "OK") {
			t.Errorf("stdout %q missing OK", out.String())
		}
	})

	t.Run("verb_validate", func(t *testing.T) {
		// "validate" with aligned registry+manifest → exit 0, "aligned" or "OK" in stdout.
		dir := t.TempDir()
		regPath := filepath.Join(dir, "registry.yaml")
		mfPath := filepath.Join(dir, "overlay.manifest")

		const alignedReg = `version: "1"
skills:
  - id: sdd-spec
    path: sdd-spec
    source:
      type: core
      upstream:
        owner: gentle-ai
    install:
      defaultScope: global
      targets:
        - claude
    lifecycle:
      updateStrategy: vendor-merge
`
		if err := os.WriteFile(regPath, []byte(alignedReg), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(mfPath, []byte("sdd-spec/SKILL.md managed\n"), 0644); err != nil {
			t.Fatal(err)
		}

		var out, errBuf bytes.Buffer
		exitCode := 0
		SkillsCore("validate",
			[]string{"--registry", regPath, "--manifest", mfPath},
			os.ReadFile, &out, &errBuf, func(c int) { exitCode = c })
		if exitCode != 0 {
			t.Errorf("exit code = %d, want 0; stderr=%q", exitCode, errBuf.String())
		}
		outStr := out.String()
		if !strings.Contains(outStr, "aligned") && !strings.Contains(outStr, "OK") {
			t.Errorf("stdout %q should contain 'aligned' or 'OK'", outStr)
		}
	})

	t.Run("unknown_verb", func(t *testing.T) {
		// Unknown verb "nuke" → exit 1, stderr contains the verb name.
		var out, errBuf bytes.Buffer
		exitCode := 0
		SkillsCore("nuke", nil, skillsMockReadFile, &out, &errBuf, func(c int) { exitCode = c })
		if exitCode != 1 {
			t.Errorf("exit code = %d, want 1", exitCode)
		}
		if !strings.Contains(errBuf.String(), "nuke") {
			t.Errorf("stderr %q should contain verb name 'nuke'", errBuf.String())
		}
	})

	t.Run("empty_verb", func(t *testing.T) {
		// Empty verb → exit 1, stderr non-empty.
		var out, errBuf bytes.Buffer
		exitCode := 0
		SkillsCore("", nil, skillsMockReadFile, &out, &errBuf, func(c int) { exitCode = c })
		if exitCode != 1 {
			t.Errorf("exit code = %d, want 1", exitCode)
		}
		if errBuf.Len() == 0 {
			t.Error("stderr must be non-empty on empty verb")
		}
	})
}
