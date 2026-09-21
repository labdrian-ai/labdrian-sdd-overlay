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

// TestRenderLintCore_UnknownFlagExitsOneWithUsageError protects one exact
// behavior: an argument that starts with "-" and is not one of the four
// recognized flags (--rules, --registry, --manifest, --source-root) exits 1
// with a usage error NAMING that argument, and lints nothing. It is neither
// treated as the lint path nor silently dropped. Dropping it is the real
// hazard: a mistyped `--rule` would then leave `lint skill.md` looking like
// a clean run of a flag that was never honoured, which is a false-clean gate
// result (review-b75e4a27b9494ff8 R4-001). The `--` escape covers the
// legitimate case this rejection would otherwise block, a path that really
// does begin with a dash (see
// TestRenderLintCore_EndOfOptionsEscapeBindsDashPrefixedPath).
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

// TestRenderLintCore_UnknownFlagBeforePathExitsOne is the position-
// independence half of the unknown-flag rule (task 4.0 item 4). The parser is
// a single pass over args, so the rejection must not depend on the flag
// appearing AFTER the positional has already been bound: an unknown flag in
// the leading position is the likelier operator typo.
func TestRenderLintCore_UnknownFlagBeforePathExitsOne(t *testing.T) {
	var out, errBuf bytes.Buffer
	exitCode := -1

	RenderLintCore(
		[]string{"--rule", "skill.md"},
		func(string) ([]byte, error) { return []byte(validSkillFile()), nil },
		&out, &errBuf,
		func(c int) { exitCode = c },
	)

	if exitCode != 1 {
		t.Fatalf("expected exit 1 for an unknown flag before the path, got %d, stderr: %q", exitCode, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "--rule") {
		t.Errorf("expected stderr to name the offending unknown flag %q, got %q", "--rule", errBuf.String())
	}
}

// TestRenderLintCore_RulesWithOnePositionalIgnoresIt pins the CURRENT,
// deliberate asymmetry of the --rules form (task 4.0 item 4): --rules wins
// over a single positional and the path is never read. It is pinned rather
// than changed because --rules is a pure report and reading a file the
// operator also named would make the exit code depend on that file's lint
// result, silently turning a report into a gate.
func TestRenderLintCore_RulesWithOnePositionalIgnoresIt(t *testing.T) {
	var out, errBuf bytes.Buffer
	exitCode := -1

	RenderLintCore(
		[]string{"--rules", "a.md"},
		func(p string) ([]byte, error) {
			t.Errorf("readFile(%q) must not be called: --rules never reads a file", p)
			return nil, errors.New("should not be called")
		},
		&out, &errBuf,
		func(c int) { exitCode = c },
	)

	if exitCode != 0 {
		t.Fatalf("expected exit 0 for --rules with one positional, got %d, stderr: %q", exitCode, errBuf.String())
	}
	if out.String() != RenderLintRules() {
		t.Fatalf("lint --rules a.md must still print RenderLintRules() byte-for-byte")
	}
}

// TestRenderLintCore_RulesWithExtraPositionalExitsOne is the other half of
// that asymmetry (task 4.0 item 4): the one-positional tolerance above does
// NOT extend to a second one. The extra-argument rejection runs during
// parsing, before --rules is honoured, so `lint --rules a.md b.md` fails
// loud naming b.md instead of printing a clean rule table over a malformed
// command line.
func TestRenderLintCore_RulesWithExtraPositionalExitsOne(t *testing.T) {
	var out, errBuf bytes.Buffer
	exitCode := -1

	RenderLintCore(
		[]string{"--rules", "a.md", "b.md"},
		func(string) ([]byte, error) { return nil, errors.New("should not be called") },
		&out, &errBuf,
		func(c int) { exitCode = c },
	)

	if exitCode != 1 {
		t.Fatalf("expected exit 1 for --rules with a second positional, got %d, stderr: %q", exitCode, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "b.md") {
		t.Errorf("expected stderr to name the offending extra argument %q, got %q", "b.md", errBuf.String())
	}
}

// TestRenderLintCore_EndOfOptionsEscapeBindsDashPrefixedPath pins the `--`
// escape (task 4.0 item 3). A SKILL.md path that legitimately begins with a
// dash is otherwise unlintable: the unknown-flag rejection above — correctly
// — refuses it. After `--`, every remaining argument is a positional.
func TestRenderLintCore_EndOfOptionsEscapeBindsDashPrefixedPath(t *testing.T) {
	var out, errBuf bytes.Buffer
	exitCode := -1
	got := ""

	RenderLintCore(
		[]string{"--", "-weird-skill.md"},
		func(p string) ([]byte, error) { got = p; return []byte(validSkillFile()), nil },
		&out, &errBuf,
		func(c int) { exitCode = c },
	)

	if got != "-weird-skill.md" {
		t.Errorf("after `--` the path must bind verbatim, readFile saw %q", got)
	}
	if exitCode != 0 {
		t.Fatalf("expected exit 0 for a lint-clean dash-prefixed path, got %d, stderr: %q", exitCode, errBuf.String())
	}
}

// TestRenderLintCore_EndOfOptionsEscapeStopsFlagParsing proves `--` ends
// OPTION parsing rather than merely allowing one dash-prefixed path: a
// recognized flag after it is a positional too, so a second one is the
// extra-argument error and not a silently honoured flag.
func TestRenderLintCore_EndOfOptionsEscapeStopsFlagParsing(t *testing.T) {
	var out, errBuf bytes.Buffer
	exitCode := -1

	RenderLintCore(
		[]string{"--", "skill.md", "--rules"},
		func(string) ([]byte, error) { return []byte(validSkillFile()), nil },
		&out, &errBuf,
		func(c int) { exitCode = c },
	)

	if exitCode != 1 {
		t.Fatalf("expected exit 1 for a second positional after `--`, got %d, stderr: %q", exitCode, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "--rules") {
		t.Errorf("expected stderr to name %q as an extra argument, got %q", "--rules", errBuf.String())
	}
}
