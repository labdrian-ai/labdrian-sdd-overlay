package skills

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// validSkillFile returns a well-formed complete SKILL.md file: fence,
// validFrontmatter, fence, validBody. It has zero hard errors and zero
// warnings under LintSkillFile.
func validSkillFile() string {
	return "---\n" + validFrontmatter() + "---\n" + validBody()
}

func TestRenderLintCore_CleanFileExitsZeroWithNoOutput(t *testing.T) {
	var out, errBuf bytes.Buffer
	exitCode := -1

	RenderLintCore(
		[]string{"skill.md"},
		func(string) ([]byte, error) { return []byte(validSkillFile()), nil },
		&out, &errBuf,
		func(c int) { exitCode = c },
	)

	if exitCode != 0 {
		t.Fatalf("exit code = %d, stderr: %q", exitCode, errBuf.String())
	}
	if out.String() != "" {
		t.Errorf("expected no stdout for a clean file, got %q", out.String())
	}
	if errBuf.String() != "" {
		t.Errorf("expected no stderr for a clean file, got %q", errBuf.String())
	}
}

func TestRenderLintCore_WarningsOnlyExitsZeroAndPrintsWarnings(t *testing.T) {
	// A body over the advisory (but not hard) token budget triggers exactly
	// the body-recommended advisory, with zero hard errors.
	longBody := strings.Repeat("word ", 750) // ~3750 bytes, over 2800, under 4000
	file := "---\n" + validFrontmatter() + "---\n" + longBody

	var out, errBuf bytes.Buffer
	exitCode := -1

	RenderLintCore(
		[]string{"skill.md"},
		func(string) ([]byte, error) { return []byte(file), nil },
		&out, &errBuf,
		func(c int) { exitCode = c },
	)

	if exitCode != 0 {
		t.Fatalf("expected exit 0 for warnings-only, got %d, stderr: %q", exitCode, errBuf.String())
	}
	if !strings.Contains(out.String(), "body-recommended") {
		t.Errorf("expected stdout to name body-recommended warning, got %q", out.String())
	}
	if errBuf.String() != "" {
		t.Errorf("expected no stderr for warnings-only, got %q", errBuf.String())
	}
}

func TestRenderLintCore_HardErrorExitsOneAndNamesMissingField(t *testing.T) {
	frontmatter := "name: test-skill\n" +
		"description: A short description.\n" +
		"license: MIT\n" +
		"metadata:\n  author: tester\n" // metadata.version missing
	file := "---\n" + frontmatter + "---\n" + validBody()

	var out, errBuf bytes.Buffer
	exitCode := -1

	RenderLintCore(
		[]string{"skill.md"},
		func(string) ([]byte, error) { return []byte(file), nil },
		&out, &errBuf,
		func(c int) { exitCode = c },
	)

	if exitCode != 1 {
		t.Fatalf("expected exit 1 for a hard error, got %d", exitCode)
	}
	if !strings.Contains(errBuf.String(), "metadata.version") {
		t.Errorf("expected stderr to name the missing field metadata.version, got %q", errBuf.String())
	}
}

func TestRenderLintCore_MissingPathExitsOne(t *testing.T) {
	var out, errBuf bytes.Buffer
	exitCode := -1

	RenderLintCore(
		[]string{},
		func(string) ([]byte, error) { return nil, errors.New("should not be called") },
		&out, &errBuf,
		func(c int) { exitCode = c },
	)

	if exitCode != 1 {
		t.Fatalf("expected exit 1 for a missing path, got %d", exitCode)
	}
}

func TestRenderLintCore_UnreadablePathExitsOne(t *testing.T) {
	var out, errBuf bytes.Buffer
	exitCode := -1

	RenderLintCore(
		[]string{"nope.md"},
		func(string) ([]byte, error) { return nil, errors.New("no such file") },
		&out, &errBuf,
		func(c int) { exitCode = c },
	)

	if exitCode != 1 {
		t.Fatalf("expected exit 1 for an unreadable path, got %d", exitCode)
	}
	if errBuf.String() == "" {
		t.Errorf("expected stderr to report the read failure")
	}
}

func TestRenderLintCore_RulesOutputEqualsRenderLintRulesByteForByte(t *testing.T) {
	var out, errBuf bytes.Buffer
	exitCode := -1

	RenderLintCore(
		[]string{"--rules"},
		func(string) ([]byte, error) { return nil, errors.New("should not be called") },
		&out, &errBuf,
		func(c int) { exitCode = c },
	)

	if exitCode != 0 {
		t.Fatalf("expected exit 0 for --rules, got %d, stderr: %q", exitCode, errBuf.String())
	}
	if out.String() != RenderLintRules() {
		t.Fatalf("lint --rules output does not equal RenderLintRules() byte-for-byte:\ngot:\n%s\nwant:\n%s", out.String(), RenderLintRules())
	}
}

// TestRenderLintCore_TolerantOfWrapperInjectedTrailingFlags proves that the
// `labdrian` wrapper's always-appended --registry/--manifest/--source-root
// flag pairs are consumed and ignored rather than misparsed as the lint
// target path.
func TestRenderLintCore_TolerantOfWrapperInjectedTrailingFlags(t *testing.T) {
	var out, errBuf bytes.Buffer
	exitCode := -1

	RenderLintCore(
		[]string{"skill.md", "--registry", "skills.registry.yaml", "--manifest", "overlay.manifest", "--source-root", "skills"},
		func(path string) ([]byte, error) {
			if path != "skill.md" {
				t.Errorf("expected readFile to be called with %q, got %q", "skill.md", path)
			}
			return []byte(validSkillFile()), nil
		},
		&out, &errBuf,
		func(c int) { exitCode = c },
	)

	if exitCode != 0 {
		t.Fatalf("exit code = %d, stderr: %q", exitCode, errBuf.String())
	}
}

// TestRenderLintCore_RulesTolerantOfWrapperInjectedTrailingFlags proves
// --rules also tolerates the wrapper's trailing flag pairs.
func TestRenderLintCore_RulesTolerantOfWrapperInjectedTrailingFlags(t *testing.T) {
	var out, errBuf bytes.Buffer
	exitCode := -1

	RenderLintCore(
		[]string{"--rules", "--registry", "skills.registry.yaml", "--manifest", "overlay.manifest", "--source-root", "skills"},
		func(string) ([]byte, error) { return nil, errors.New("should not be called") },
		&out, &errBuf,
		func(c int) { exitCode = c },
	)

	if exitCode != 0 {
		t.Fatalf("exit code = %d, stderr: %q", exitCode, errBuf.String())
	}
	if out.String() != RenderLintRules() {
		t.Fatalf("lint --rules output with trailing flags does not equal RenderLintRules() byte-for-byte")
	}
}
