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

// warningsOnlyBodyWordRepeat returns the repeat count for
// strings.Repeat("word ", n) that lands the body's byte length strictly
// between the body-recommended and body-hard token budgets (converted to
// bytes via BytesPerTokenProxy, the same estimator LintSkill uses). This
// keeps the warnings-only fixture derived from the named lint constants
// instead of unexplained literal byte thresholds that would silently turn
// into a no-warning or hard-error case if the estimator or the budgets
// change (review-b75e4a27b9494ff8 R2-unexplained-test-constants).
func warningsOnlyBodyWordRepeat() int {
	const word = "word "
	recommendedBytes := BodyRecommendedTokens * BytesPerTokenProxy
	hardBytes := BodyHardTokens * BytesPerTokenProxy
	midpointBytes := (recommendedBytes + hardBytes) / 2
	return midpointBytes / len(word)
}

func TestRenderLintCore_WarningsOnlyExitsZeroAndPrintsWarnings(t *testing.T) {
	// A body over the advisory (but not hard) token budget triggers exactly
	// the body-recommended advisory, with zero hard errors.
	longBody := strings.Repeat("word ", warningsOnlyBodyWordRepeat())
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
	if !strings.Contains(errBuf.String(), "requires a path or --rules") {
		t.Errorf("expected stderr to carry the missing-path diagnostic, got %q", errBuf.String())
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

// TestRenderLintCore_ExtraPositionalArgumentExitsOneWithUsageError proves
// RenderLintCore rejects a second positional argument instead of silently
// linting only the first path and exiting 0 (review-b75e4a27b9494ff8
// R4-001/R2-silent-extra-args/R3-lint-extra-positional-ignored: a multi-path
// call like `lint a.md b.md` must not become a false-clean gate).
func TestRenderLintCore_ExtraPositionalArgumentExitsOneWithUsageError(t *testing.T) {
	var out, errBuf bytes.Buffer
	exitCode := -1

	RenderLintCore(
		[]string{"a.md", "b.md"},
		func(string) ([]byte, error) { return nil, errors.New("should not be called") },
		&out, &errBuf,
		func(c int) { exitCode = c },
	)

	if exitCode != 1 {
		t.Fatalf("expected exit 1 for a second positional argument, got %d, stderr: %q", exitCode, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "b.md") {
		t.Errorf("expected stderr to name the offending extra argument %q, got %q", "b.md", errBuf.String())
	}
}

// TestRenderLintCore_UnknownFlagExitsOneWithUsageError proves RenderLintCore
// rejects an unrecognized flag instead of silently treating it as the lint
// path.
func TestRenderLintCore_UnknownFlagExitsOneWithUsageError(t *testing.T) {
	var out, errBuf bytes.Buffer
	exitCode := -1

	RenderLintCore(
		[]string{"skill.md", "--rule"},
		func(string) ([]byte, error) { return []byte(validSkillFile()), nil },
		&out, &errBuf,
		func(c int) { exitCode = c },
	)

	if exitCode != 1 {
		t.Fatalf("expected exit 1 for an unknown flag, got %d, stderr: %q", exitCode, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "--rule") {
		t.Errorf("expected stderr to name the offending unknown flag %q, got %q", "--rule", errBuf.String())
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
