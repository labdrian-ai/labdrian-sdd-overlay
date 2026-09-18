package skills

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// validFrontmatter returns a frontmatter block that satisfies every hard
// rule: all required fields present and non-empty, description a single
// physical line within bounds.
func validFrontmatter() string {
	return "name: test-skill\n" +
		"description: A short, single-line description of the skill.\n" +
		"license: MIT\n" +
		"metadata:\n" +
		"  author: tester\n" +
		"  version: \"1.0\"\n"
}

// validBody returns a body that is clean under every advisory and hard
// rule: canonical headings in canonical order, no incident-log shape, no
// banned utilities, no leaked home path, well within both budgets.
func validBody() string {
	return "## Activation Contract\n" +
		"Load this skill for the smoke scenario.\n\n" +
		"## Hard Rules\n" +
		"- Keep it short.\n\n" +
		"## Execution Steps\n" +
		"1. Do the thing.\n"
}

func hasHardRule(hard []error, rule string) bool {
	for _, e := range hard {
		if le, ok := e.(*LintError); ok && le.Rule == rule {
			return true
		}
	}
	return false
}

func hasWarningRule(warnings []Warning, rule string) bool {
	for _, w := range warnings {
		if w.Rule == rule {
			return true
		}
	}
	return false
}

// --- Hard rule: required-fields ---

func TestLintSkill_RequiredFields_MissingOrEmpty(t *testing.T) {
	cases := []struct {
		name        string
		frontmatter string
		wantField   string
	}{
		{
			name: "missing name",
			frontmatter: "description: A short, single-line description.\n" +
				"license: MIT\n" +
				"metadata:\n  author: tester\n  version: \"1.0\"\n",
			wantField: "name",
		},
		{
			name: "empty name",
			frontmatter: "name: \"\"\n" +
				"description: A short, single-line description.\n" +
				"license: MIT\n" +
				"metadata:\n  author: tester\n  version: \"1.0\"\n",
			wantField: "name",
		},
		{
			name: "missing description",
			frontmatter: "name: test-skill\n" +
				"license: MIT\n" +
				"metadata:\n  author: tester\n  version: \"1.0\"\n",
			wantField: "description",
		},
		{
			name: "missing license",
			frontmatter: "name: test-skill\n" +
				"description: A short, single-line description.\n" +
				"metadata:\n  author: tester\n  version: \"1.0\"\n",
			wantField: "license",
		},
		{
			name: "missing metadata.author",
			frontmatter: "name: test-skill\n" +
				"description: A short, single-line description.\n" +
				"license: MIT\n" +
				"metadata:\n  version: \"1.0\"\n",
			wantField: "metadata.author",
		},
		{
			name: "empty metadata.author",
			frontmatter: "name: test-skill\n" +
				"description: A short, single-line description.\n" +
				"license: MIT\n" +
				"metadata:\n  author: \"\"\n  version: \"1.0\"\n",
			wantField: "metadata.author",
		},
		{
			name: "missing metadata.version",
			frontmatter: "name: test-skill\n" +
				"description: A short, single-line description.\n" +
				"license: MIT\n" +
				"metadata:\n  author: tester\n",
			wantField: "metadata.version",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hard, _ := LintSkill(tc.frontmatter, validBody())
			if len(hard) == 0 {
				t.Fatalf("expected at least one hard error, got none")
			}
			found := false
			for _, e := range hard {
				le, ok := e.(*LintError)
				if !ok {
					t.Fatalf("expected *LintError, got %T", e)
				}
				if le.Rule == "required-fields" && strings.Contains(le.Msg, tc.wantField) {
					found = true
				}
			}
			if !found {
				t.Errorf("expected a required-fields hard error naming %q, got %v", tc.wantField, hard)
			}
		})
	}
}

// TestLintSkill_OneSpaceIndentedMetadataEntryIsSurfacedAsMissing documents
// the decided behavior for a malformed metadata indentation: the design's
// frontmatter subset (design.md, "LintSkill API shape") specifies a
// `metadata:` block with 2-space-indented pairs; a one-space-indented entry
// does not match that shape. Rather than being silently consumed, it is
// treated as a stray line that ends the metadata block. This is not a
// silent drop: any metadata field left unset by the truncated block
// (including a later, correctly 2-space-indented entry that never gets
// parsed because the block already ended) is reported by the required-fields
// hard rule, which names the missing field explicitly.
func TestLintSkill_OneSpaceIndentedMetadataEntryIsSurfacedAsMissing(t *testing.T) {
	frontmatter := "name: test-skill\n" +
		"description: A short, single-line description.\n" +
		"license: MIT\n" +
		"metadata:\n" +
		" author: tester\n" + // malformed: one space, not the documented two
		"  version: \"1.0\"\n" // well-formed, but unreachable once the block ended

	hard, _ := LintSkill(frontmatter, validBody())

	for _, field := range []string{"metadata.author", "metadata.version"} {
		found := false
		for _, e := range hard {
			if le, ok := e.(*LintError); ok && le.Rule == "required-fields" && strings.Contains(le.Msg, field) {
				found = true
			}
		}
		if !found {
			t.Errorf("expected required-fields to name %q as missing after the metadata block ends on a one-space-indented entry, got %v", field, hard)
		}
	}
}

// TestLintSkill_EmptyDescriptionYieldsRequiredFieldsOnly proves an empty
// `description:` line with no continuation line is reported only via
// required-fields: it must not also raise a misleading description-one-line
// hard error, since an empty value with nothing following it is genuinely
// absent, not a block scalar or a multi-line continuation.
func TestLintSkill_EmptyDescriptionYieldsRequiredFieldsOnly(t *testing.T) {
	frontmatter := "name: test-skill\n" +
		"description:\n" +
		"license: MIT\n" +
		"metadata:\n  author: tester\n  version: \"1.0\"\n"

	hard, _ := LintSkill(frontmatter, validBody())

	if !hasHardRule(hard, "required-fields") {
		t.Errorf("expected required-fields hard error for empty description, got %v", hard)
	}
	if hasHardRule(hard, "description-one-line") {
		t.Errorf("empty description with no continuation must not also trigger description-one-line, got %v", hard)
	}
}

// TestLintSkill_EmptyDescriptionWithContinuationYieldsDescriptionOneLine
// proves an empty `description:` tag line followed by one or more indented
// continuation lines (YAML plain multi-line scalar syntax) is genuinely
// present content in the wrong shape, not an absent field: it must raise the
// description-one-line hard error, and must NOT also raise a required-fields
// error naming description as missing, since that would be misleading about
// content that is actually present.
func TestLintSkill_EmptyDescriptionWithContinuationYieldsDescriptionOneLine(t *testing.T) {
	frontmatter := "name: test-skill\n" +
		"description:\n" +
		"  this continues onto an indented physical line even though the tag\n" +
		"  line itself was empty\n" +
		"license: MIT\n" +
		"metadata:\n  author: tester\n  version: \"1.0\"\n"

	hard, _ := LintSkill(frontmatter, validBody())

	if !hasHardRule(hard, "description-one-line") {
		t.Errorf("expected description-one-line hard error for empty description with an indented continuation, got %v", hard)
	}
	if hasHardRule(hard, "required-fields") {
		t.Errorf("empty description with an indented continuation must not also raise required-fields naming description as missing, got %v", hard)
	}
}

// TestLintSkill_EmptyDescriptionWithWhitespaceOnlyLineYieldsRequiredFieldsOnly
// proves a whitespace-only line following an empty `description:` tag line
// is NOT a YAML continuation: a line with no non-whitespace content carries
// no actual value, so the description is genuinely absent and must be
// reported only via required-fields, never as a misleading
// description-one-line hard error (review-b375153aa152604e carry-forward).
func TestLintSkill_EmptyDescriptionWithWhitespaceOnlyLineYieldsRequiredFieldsOnly(t *testing.T) {
	frontmatter := "name: test-skill\n" +
		"description:\n" +
		"   \n" +
		"license: MIT\n" +
		"metadata:\n  author: tester\n  version: \"1.0\"\n"

	hard, _ := LintSkill(frontmatter, validBody())

	if !hasHardRule(hard, "required-fields") {
		t.Errorf("expected required-fields hard error for empty description followed by a whitespace-only line, got %v", hard)
	}
	if hasHardRule(hard, "description-one-line") {
		t.Errorf("a whitespace-only line must not count as a continuation and must not trigger description-one-line, got %v", hard)
	}
}

// TestLintSkill_EmptyDescriptionWithTabContinuationYieldsDescriptionOneLine
// proves a tab-indented (not just space-indented) continuation line after an
// empty `description:` tag line is still classified as a genuine
// description-one-line continuation.
func TestLintSkill_EmptyDescriptionWithTabContinuationYieldsDescriptionOneLine(t *testing.T) {
	frontmatter := "name: test-skill\n" +
		"description:\n" +
		"\tthis continues on a tab-indented physical line\n" +
		"license: MIT\n" +
		"metadata:\n  author: tester\n  version: \"1.0\"\n"

	hard, _ := LintSkill(frontmatter, validBody())

	if !hasHardRule(hard, "description-one-line") {
		t.Errorf("expected description-one-line hard error for a tab-indented continuation, got %v", hard)
	}
	if hasHardRule(hard, "required-fields") {
		t.Errorf("a tab-indented continuation must not also raise required-fields naming description as missing, got %v", hard)
	}
}

// TestLintSkill_EmptyDescriptionAsLastFrontmatterLineYieldsRequiredFieldsOnly
// proves an empty `description:` tag line that is the very last physical
// line of the frontmatter (no following line at all) is reported only via
// required-fields, exercising the i+1 >= len(lines) boundary directly.
func TestLintSkill_EmptyDescriptionAsLastFrontmatterLineYieldsRequiredFieldsOnly(t *testing.T) {
	frontmatter := "name: test-skill\n" +
		"license: MIT\n" +
		"metadata:\n  author: tester\n  version: \"1.0\"\n" +
		"description:"

	hard, _ := LintSkill(frontmatter, validBody())

	if !hasHardRule(hard, "required-fields") {
		t.Errorf("expected required-fields hard error for an empty description as the last frontmatter line, got %v", hard)
	}
	if hasHardRule(hard, "description-one-line") {
		t.Errorf("an empty description with no following line must not trigger description-one-line, got %v", hard)
	}
}

func TestLintSkill_WellFormedProducesZeroHardErrors(t *testing.T) {
	hard, _ := LintSkill(validFrontmatter(), validBody())
	if len(hard) != 0 {
		t.Fatalf("expected zero hard errors for a well-formed fixture, got %v", hard)
	}
}

func TestLintSkill_CleanFixtureHasZeroWarnings(t *testing.T) {
	hard, warnings := LintSkill(validFrontmatter(), validBody())
	if len(hard) != 0 {
		t.Fatalf("expected zero hard errors, got %v", hard)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected zero warnings for a clean fixture, got %v", warnings)
	}
}

// --- Hard rule: description-one-line ---

func TestLintSkill_DescriptionOneLine(t *testing.T) {
	cases := []struct {
		name        string
		frontmatter string
	}{
		{
			name: "block scalar",
			frontmatter: "name: test-skill\n" +
				"description: >\n" +
				"  A folded description that spans\n" +
				"  more than one physical line.\n" +
				"license: MIT\n" +
				"metadata:\n  author: tester\n  version: \"1.0\"\n",
		},
		{
			name: "indented continuation",
			frontmatter: "name: test-skill\n" +
				"description: A description that continues\n" +
				"  onto a second indented physical line.\n" +
				"license: MIT\n" +
				"metadata:\n  author: tester\n  version: \"1.0\"\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hard, _ := LintSkill(tc.frontmatter, validBody())
			if !hasHardRule(hard, "description-one-line") {
				t.Errorf("expected description-one-line hard error, got %v", hard)
			}
		})
	}
}

// --- Hard rule: description-max ---

func TestLintSkill_DescriptionMaxBoundary(t *testing.T) {
	at := strings.Repeat("a", DescriptionMaxRunes)
	over := strings.Repeat("a", DescriptionMaxRunes+1)

	fmAt := "name: test-skill\ndescription: " + at + "\nlicense: MIT\nmetadata:\n  author: tester\n  version: \"1.0\"\n"
	fmOver := "name: test-skill\ndescription: " + over + "\nlicense: MIT\nmetadata:\n  author: tester\n  version: \"1.0\"\n"

	hard, _ := LintSkill(fmAt, validBody())
	if hasHardRule(hard, "description-max") {
		t.Errorf("description exactly at %d runes must not trigger description-max, got %v", DescriptionMaxRunes, hard)
	}

	hard, _ = LintSkill(fmOver, validBody())
	if !hasHardRule(hard, "description-max") {
		t.Errorf("description at %d runes must trigger description-max, got %v", DescriptionMaxRunes+1, hard)
	}
}

// --- Advisory: description-should ---

func TestLintSkill_DescriptionShouldBoundary(t *testing.T) {
	at := strings.Repeat("a", DescriptionShouldRunes)
	over := strings.Repeat("a", DescriptionShouldRunes+1)

	fmAt := "name: test-skill\ndescription: " + at + "\nlicense: MIT\nmetadata:\n  author: tester\n  version: \"1.0\"\n"
	fmOver := "name: test-skill\ndescription: " + over + "\nlicense: MIT\nmetadata:\n  author: tester\n  version: \"1.0\"\n"

	hard, warnings := LintSkill(fmAt, validBody())
	if len(hard) != 0 {
		t.Fatalf("unexpected hard errors at should boundary: %v", hard)
	}
	if hasWarningRule(warnings, "description-should") {
		t.Errorf("description exactly at %d runes must not trigger description-should, got %v", DescriptionShouldRunes, warnings)
	}

	hard, warnings = LintSkill(fmOver, validBody())
	if len(hard) != 0 {
		t.Fatalf("unexpected hard errors just over should boundary: %v", hard)
	}
	if !hasWarningRule(warnings, "description-should") {
		t.Errorf("description at %d runes must trigger description-should, got %v", DescriptionShouldRunes+1, warnings)
	}
}

// --- Hard rule: body-hard-budget ---

func TestLintSkill_BodyHardBudgetBoundary(t *testing.T) {
	atBytes := BodyHardTokens * BytesPerTokenProxy
	at := strings.Repeat("x", atBytes)
	over := strings.Repeat("x", atBytes+1)

	hard, _ := LintSkill(validFrontmatter(), at)
	if hasHardRule(hard, "body-hard-budget") {
		t.Errorf("body at exactly %d bytes must not trigger body-hard-budget, got %v", atBytes, hard)
	}

	hard, _ = LintSkill(validFrontmatter(), over)
	if !hasHardRule(hard, "body-hard-budget") {
		t.Errorf("body at %d bytes must trigger body-hard-budget, got %v", atBytes+1, hard)
	}
}

// --- Advisory: body-recommended ---

func TestLintSkill_BodyRecommendedBoundary(t *testing.T) {
	atBytes := BodyRecommendedTokens * BytesPerTokenProxy
	at := strings.Repeat("x", atBytes)
	over := strings.Repeat("x", atBytes+1)

	hard, warnings := LintSkill(validFrontmatter(), at)
	if len(hard) != 0 {
		t.Fatalf("unexpected hard errors at recommended boundary: %v", hard)
	}
	if hasWarningRule(warnings, "body-recommended") {
		t.Errorf("body at exactly %d bytes must not trigger body-recommended, got %v", atBytes, warnings)
	}

	hard, warnings = LintSkill(validFrontmatter(), over)
	if len(hard) != 0 {
		t.Fatalf("unexpected hard errors just over recommended boundary: %v", hard)
	}
	if !hasWarningRule(warnings, "body-recommended") {
		t.Errorf("body at %d bytes must trigger body-recommended, got %v", atBytes+1, warnings)
	}
}

// --- Advisory: section-order ---

func TestLintSkill_SectionOrder(t *testing.T) {
	inOrder := "## Activation Contract\ntext\n## Hard Rules\ntext\n## Execution Steps\ntext\n"
	outOfOrder := "## Activation Contract\ntext\n## Execution Steps\ntext\n## Hard Rules\ntext\n"
	missingHeadingAllowed := "## Activation Contract\ntext\n## Execution Steps\ntext\n"
	unknownHeadingIgnored := "## Activation Contract\ntext\n## Unrelated Notes\ntext\n## Hard Rules\ntext\n"

	_, warnings := LintSkill(validFrontmatter(), inOrder)
	if hasWarningRule(warnings, "section-order") {
		t.Errorf("canonical order must not trigger section-order, got %v", warnings)
	}

	_, warnings = LintSkill(validFrontmatter(), outOfOrder)
	if !hasWarningRule(warnings, "section-order") {
		t.Errorf("reversed canonical headings must trigger section-order, got %v", warnings)
	}

	_, warnings = LintSkill(validFrontmatter(), missingHeadingAllowed)
	if hasWarningRule(warnings, "section-order") {
		t.Errorf("a missing heading must not by itself trigger section-order, got %v", warnings)
	}

	_, warnings = LintSkill(validFrontmatter(), unknownHeadingIgnored)
	if hasWarningRule(warnings, "section-order") {
		t.Errorf("an unknown heading must be ignored for section-order, got %v", warnings)
	}
}

// --- Advisory: incident-log-shape ---

func TestLintSkill_IncidentLogShape(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "ISO date", body: validBody() + "\nThis happened on 2026-09-18.\n"},
		{name: "engram reference", body: validBody() + "\nSee engram:1234 for context.\n"},
		{name: "PR number", body: validBody() + "\nFixed in PR #351.\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, warnings := LintSkill(validFrontmatter(), tc.body)
			if !hasWarningRule(warnings, "incident-log-shape") {
				t.Errorf("expected incident-log-shape warning, got %v", warnings)
			}
		})
	}
}

// --- Advisory: banned-shell-utility ---

func TestLintSkill_BannedShellUtility(t *testing.T) {
	utilities := []string{"cat", "grep", "find", "sed", "ls"}

	for _, u := range utilities {
		t.Run(u+" inline", func(t *testing.T) {
			body := validBody() + "\nRun `" + u + " file.txt` to inspect it.\n"
			_, warnings := LintSkill(validFrontmatter(), body)
			if !hasWarningRule(warnings, "banned-shell-utility") {
				t.Errorf("expected banned-shell-utility warning for inline %q, got %v", u, warnings)
			}
		})

		t.Run(u+" fenced", func(t *testing.T) {
			body := validBody() + "\n```\n" + u + " file.txt\n```\n"
			_, warnings := LintSkill(validFrontmatter(), body)
			if !hasWarningRule(warnings, "banned-shell-utility") {
				t.Errorf("expected banned-shell-utility warning for fenced %q, got %v", u, warnings)
			}
		})
	}

	t.Run("near miss does not match", func(t *testing.T) {
		body := validBody() + "\nRun `catalog list` to inspect it.\n"
		_, warnings := LintSkill(validFrontmatter(), body)
		if hasWarningRule(warnings, "banned-shell-utility") {
			t.Errorf("catalog must not be treated as the banned utility cat, got %v", warnings)
		}
	})

	t.Run("near miss inside fenced block does not match", func(t *testing.T) {
		body := validBody() + "\n```\ncatalog list\n```\n"
		_, warnings := LintSkill(validFrontmatter(), body)
		if hasWarningRule(warnings, "banned-shell-utility") {
			t.Errorf("catalog inside a fenced block must not be treated as the banned utility cat, got %v", warnings)
		}
	})

	t.Run("banned word in plain prose outside code is ignored", func(t *testing.T) {
		body := validBody() + "\nDo not cat the file; use the approved replacement instead.\n"
		_, warnings := LintSkill(validFrontmatter(), body)
		if hasWarningRule(warnings, "banned-shell-utility") {
			t.Errorf("a banned word in plain prose (outside a code span or fence) must be ignored, got %v", warnings)
		}
	})
}

// --- Advisory: home-path-leak ---

func TestLintSkill_HomePathLeak(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "linux", body: validBody() + "\nSee /home/alice/notes.md for details.\n"},
		{name: "macos", body: validBody() + "\nSee /Users/alice/notes.md for details.\n"},
		{name: "windows", body: validBody() + "\n" + `See C:\Users\alice\notes.md for details.` + "\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, warnings := LintSkill(validFrontmatter(), tc.body)
			if !hasWarningRule(warnings, "home-path-leak") {
				t.Errorf("expected home-path-leak warning, got %v", warnings)
			}
		})
	}
}

// --- Determinism ---

func TestLintSkill_Deterministic(t *testing.T) {
	frontmatter := validFrontmatter()
	body := validBody() + "\nRun `cat file.txt` on 2026-09-18, see engram:1.\n"

	hard1, warnings1 := LintSkill(frontmatter, body)
	hard2, warnings2 := LintSkill(frontmatter, body)

	if !reflect.DeepEqual(hard1, hard2) {
		t.Errorf("LintSkill hard errors are not deterministic:\n%v\n%v", hard1, hard2)
	}
	if !reflect.DeepEqual(warnings1, warnings2) {
		t.Errorf("LintSkill warnings are not deterministic:\n%v\n%v", warnings1, warnings2)
	}
}

// --- LintSkillFile composes SplitSkillFile and LintSkill ---
//
// The missing/unclosed-fence scenario belongs to the "LintSkillFile Composes
// SplitSkillFile and LintSkill" requirement, not to LintSkill itself: a split
// failure must short-circuit before LintSkill ever runs on the body.

func TestLintSkillFile_MissingFrontmatterFence(t *testing.T) {
	data := []byte("no frontmatter fence here\njust body text\n")

	hard, warnings := LintSkillFile(data)
	if !hasHardRule(hard, "frontmatter-fence") {
		t.Fatalf("expected frontmatter-fence hard error, got hard=%v warnings=%v", hard, warnings)
	}
}

func TestLintSkillFile_UnclosedFrontmatterFence(t *testing.T) {
	data := []byte("---\nname: test-skill\ndescription: no closing fence\n")

	hard, warnings := LintSkillFile(data)
	if !hasHardRule(hard, "frontmatter-fence") {
		t.Fatalf("expected frontmatter-fence hard error, got hard=%v warnings=%v", hard, warnings)
	}
}

// TestLintSkillFile_SplitFailureSkipsLintSkill proves that when
// SplitSkillFile fails, LintSkill is never invoked: the only reported
// finding is the fence error itself, never a body/frontmatter finding
// (such as required-fields, which this broken input would otherwise
// trigger many times over if LintSkill ran on garbage input).
func TestLintSkillFile_SplitFailureSkipsLintSkill(t *testing.T) {
	data := []byte("this is not a skill file at all, no fences, no fields\n")

	hard, warnings := LintSkillFile(data)

	if len(hard) != 1 {
		t.Fatalf("expected exactly one hard error (the fence failure) when split fails, got %v", hard)
	}
	if hard[0].(*LintError).Rule != "frontmatter-fence" {
		t.Fatalf("expected the sole hard error to be frontmatter-fence, got %v", hard[0])
	}
	if len(warnings) != 0 {
		t.Fatalf("expected zero warnings when split fails (LintSkill never ran), got %v", warnings)
	}
}

func TestLintSkillFile_WellFormedFileHasZeroHardErrors(t *testing.T) {
	data := []byte("---\n" + validFrontmatter() + "---\n" + validBody())

	hard, warnings := LintSkillFile(data)
	if len(hard) != 0 {
		t.Fatalf("expected zero hard errors for a well-formed file, got %v", hard)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected zero warnings for a well-formed file, got %v", warnings)
	}
}

// --- SplitSkillFile ---

func TestSplitSkillFile_RoundTrip(t *testing.T) {
	data := []byte("---\n" + validFrontmatter() + "---\n" + validBody())

	frontmatter, body, err := SplitSkillFile(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if frontmatter != strings.TrimRight(validFrontmatter(), "\n") {
		t.Errorf("frontmatter mismatch:\ngot:  %q\nwant: %q", frontmatter, strings.TrimRight(validFrontmatter(), "\n"))
	}
	if body != validBody() {
		t.Errorf("body mismatch:\ngot:  %q\nwant: %q", body, validBody())
	}
}

// --- SplitSkillFile tolerates a leading BOM and trailing fence whitespace:
// an editor-inserted UTF-8 BOM or trailing horizontal whitespace on a fence
// line must never falsely trip the frontmatter-fence hard rule ---

func TestSplitSkillFile_LeadingBOMDoesNotFalselyFailFence(t *testing.T) {
	data := append([]byte{0xEF, 0xBB, 0xBF}, []byte("---\n"+validFrontmatter()+"---\n"+validBody())...)

	frontmatter, body, err := SplitSkillFile(data)
	if err != nil {
		t.Fatalf("expected a leading UTF-8 BOM not to cause a fence error, got: %v", err)
	}
	if frontmatter != strings.TrimRight(validFrontmatter(), "\n") {
		t.Errorf("frontmatter mismatch:\ngot:  %q\nwant: %q", frontmatter, strings.TrimRight(validFrontmatter(), "\n"))
	}
	if body != validBody() {
		t.Errorf("body mismatch:\ngot:  %q\nwant: %q", body, validBody())
	}
}

func TestSplitSkillFile_TrailingSpacesOnOpeningFence(t *testing.T) {
	data := []byte("---   \n" + validFrontmatter() + "---\n" + validBody())

	_, _, err := SplitSkillFile(data)
	if err != nil {
		t.Fatalf("expected trailing spaces on the opening fence not to cause a fence error, got: %v", err)
	}
}

func TestSplitSkillFile_TrailingTabOnClosingFence(t *testing.T) {
	data := []byte("---\n" + validFrontmatter() + "---\t\n" + validBody())

	_, _, err := SplitSkillFile(data)
	if err != nil {
		t.Fatalf("expected trailing whitespace on the closing fence not to cause a fence error, got: %v", err)
	}
}

// --- RenderLintRules: the rendered rule table has a stable header and row
// shape, and every row names its rule id and stays within the header's
// column count even when a Summary itself contains an escaped `|` ---

func TestRenderLintRules_HeaderAndRowCount(t *testing.T) {
	out := RenderLintRules()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")

	if len(lines) != len(lintRules)+2 {
		t.Fatalf("expected header + separator + %d rule rows, got %d lines:\n%s", len(lintRules), len(lines), out)
	}
	if lines[0] != "| ID | Severity | Check |" {
		t.Errorf("unexpected header line: %q", lines[0])
	}
	if lines[1] != "|---|---|---|" {
		t.Errorf("unexpected separator line: %q", lines[1])
	}

	headerCells := countUnescapedPipes(lines[0])
	for i, r := range lintRules {
		row := lines[i+2]
		if !strings.Contains(row, "`"+r.ID+"`") {
			t.Errorf("row %d does not name rule id %q: %q", i, r.ID, row)
		}
		if got := countUnescapedPipes(row); got != headerCells {
			t.Errorf("row %d has %d unescaped (cell-splitting) pipes, header has %d: %q", i, got, headerCells, row)
		}
	}
}

// countUnescapedPipes counts `|` characters that are NOT preceded by a
// backslash, i.e. the pipes that actually split a GFM table row into cells.
// An escaped `\|` renders as a literal pipe inside one cell and must not be
// counted as a separator.
func countUnescapedPipes(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '|' && (i == 0 || s[i-1] != '\\') {
			n++
		}
	}
	return n
}

// --- RenderLintRules derives summaries from source data ---
//
// TestRenderLintRules_DescriptionOneLineListsEveryBlockScalarIndicator proves
// the rendered description-one-line summary names exactly the indicator set
// isBlockScalarIndicator checks against, and does so non-vacuously: it
// extracts the delimited code-span tokens from the row's summary cell (not a
// raw substring search, which would pass trivially for "|" because the row's
// own table-column separators are literal `|` characters, and for ">"
// because it is a substring of the rendered ">-" token) and compares that
// exact set against a literal expected set that is deliberately independent
// of blockScalarIndicators (see the "want" comment below for why).
func TestRenderLintRules_DescriptionOneLineListsEveryBlockScalarIndicator(t *testing.T) {
	out := RenderLintRules()
	row := lintRuleRow(t, out, "description-one-line")
	cells := splitTableRowCells(row)
	if len(cells) != 3 {
		t.Fatalf("expected 3 table cells in the description-one-line row, got %d: %v", len(cells), cells)
	}
	summaryCell := cells[2]
	indicatorList := parenthesizedContent(t, summaryCell)

	got := codeSpanTokenSet(indicatorList)
	// want is a literal set, independent of blockScalarIndicators: deriving
	// it from that same production variable would make this assertion
	// compare the source data against itself, which can never fail no
	// matter what the variable's contents are.
	want := map[string]bool{">": true, "|": true, ">-": true, "|-": true, ">+": true, "|+": true}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("description-one-line summary cell code-span tokens = %v, want exactly %v", got, want)
	}
}

// splitTableRowCells splits a rendered GFM table row into its cells on
// unescaped `|` separators (an escaped `\|` stays inside its cell), trimming
// surrounding whitespace and dropping only the empty leading and trailing
// cells produced by a row that starts and ends with `|`. A genuinely empty
// cell in the middle of the row is kept, since dropping it would silently
// shift every later cell's index.
func splitTableRowCells(s string) []string {
	var cells []string
	var cur strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '|' && (i == 0 || s[i-1] != '\\') {
			cells = append(cells, cur.String())
			cur.Reset()
			continue
		}
		cur.WriteByte(s[i])
	}
	cells = append(cells, cur.String())

	for i := range cells {
		cells[i] = strings.TrimSpace(cells[i])
	}
	if len(cells) > 0 && cells[0] == "" {
		cells = cells[1:]
	}
	if len(cells) > 0 && cells[len(cells)-1] == "" {
		cells = cells[:len(cells)-1]
	}
	return cells
}

// TestSplitTableRowCells_KeepsMiddleEmptyCell proves splitTableRowCells
// drops only the leading and trailing empty cells produced by a row that
// starts and ends with `|`, never a genuinely empty cell in the middle of
// the row (review-b375153aa152604e carry-forward).
func TestSplitTableRowCells_KeepsMiddleEmptyCell(t *testing.T) {
	got := splitTableRowCells("| a | | c |")
	want := []string{"a", "", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitTableRowCells(%q) = %v, want %v", "| a | | c |", got, want)
	}
}

// parenthesizedContent returns the content of the first `(...)` group in s,
// failing the test if none is found. The description-one-line summary lists
// its rejected block-scalar indicators inside exactly one such group, so
// extracting it isolates the indicator list from the rest of the sentence
// (which also names `description` as an unrelated code span).
func parenthesizedContent(t *testing.T, s string) string {
	t.Helper()
	open := strings.Index(s, "(")
	closeIdx := strings.Index(s, ")")
	if open < 0 || closeIdx < 0 || closeIdx < open {
		t.Fatalf("expected a parenthesized indicator list in %q", s)
	}
	return s[open+1 : closeIdx]
}

// codeSpanTokenSet returns the set of backtick-delimited token contents in
// s, unescaping a table-escaped `\|` back to a literal `|` so a rendered
// pipe indicator compares equal to the indicator data itself.
func codeSpanTokenSet(s string) map[string]bool {
	tokens := map[string]bool{}
	for _, m := range inlineCodeRe.FindAllStringSubmatch(s, -1) {
		tokens[strings.ReplaceAll(m[1], `\|`, "|")] = true
	}
	return tokens
}

func TestRenderLintRules_SectionOrderListsEveryCanonicalSection(t *testing.T) {
	out := RenderLintRules()
	row := lintRuleRow(t, out, "section-order")

	for _, section := range canonicalSections {
		if !strings.Contains(row, section) {
			t.Errorf("section-order row must list canonical section %q, got: %q", section, row)
		}
	}
}

// lintRuleRow returns the rendered row for the given rule id, failing the
// test if the id is not present.
func lintRuleRow(t *testing.T, rendered, ruleID string) string {
	t.Helper()
	for _, line := range strings.Split(rendered, "\n") {
		if strings.Contains(line, "`"+ruleID+"`") {
			return line
		}
	}
	t.Fatalf("no rendered row found for rule %q in:\n%s", ruleID, rendered)
	return ""
}

func TestRenderLintRules_RendersNumericConstants(t *testing.T) {
	out := RenderLintRules()

	wantSubstrings := []string{
		fmt.Sprintf("%d", DescriptionMaxRunes),
		fmt.Sprintf("%d", DescriptionShouldRunes),
		fmt.Sprintf("%d", BodyHardTokens),
		fmt.Sprintf("%d", BodyRecommendedTokens),
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(out, want) {
			t.Errorf("expected rendered rule table to contain %q, got:\n%s", want, out)
		}
	}
}
