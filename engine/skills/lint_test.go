package skills

import (
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
	at := strings.Repeat("x", 4000)
	over := strings.Repeat("x", 4001)

	hard, _ := LintSkill(validFrontmatter(), at)
	if hasHardRule(hard, "body-hard-budget") {
		t.Errorf("body at exactly 4000 bytes must not trigger body-hard-budget, got %v", hard)
	}

	hard, _ = LintSkill(validFrontmatter(), over)
	if !hasHardRule(hard, "body-hard-budget") {
		t.Errorf("body at 4001 bytes must trigger body-hard-budget, got %v", hard)
	}
}

// --- Advisory: body-recommended ---

func TestLintSkill_BodyRecommendedBoundary(t *testing.T) {
	at := strings.Repeat("x", 2800)
	over := strings.Repeat("x", 2801)

	hard, warnings := LintSkill(validFrontmatter(), at)
	if len(hard) != 0 {
		t.Fatalf("unexpected hard errors at recommended boundary: %v", hard)
	}
	if hasWarningRule(warnings, "body-recommended") {
		t.Errorf("body at exactly 2800 bytes must not trigger body-recommended, got %v", warnings)
	}

	hard, warnings = LintSkill(validFrontmatter(), over)
	if len(hard) != 0 {
		t.Fatalf("unexpected hard errors just over recommended boundary: %v", hard)
	}
	if !hasWarningRule(warnings, "body-recommended") {
		t.Errorf("body at 2801 bytes must trigger body-recommended, got %v", warnings)
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
}

// --- Advisory: home-path-leak ---

func TestLintSkill_HomePathLeak(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "linux", body: validBody() + "\nSee /home/alice/notes.md for details.\n"},
		{name: "macos", body: validBody() + "\nSee /Users/alice/notes.md for details.\n"},
		{name: "windows", body: validBody() + `\nSee C:\Users\alice\notes.md for details.` + "\n"},
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

// --- LintSkillFile composes SplitSkillFile and LintSkill (1a.2a) ---
//
// Carry-forward from review-42be59cf3243a7ff advisory R3-lintskillfile-coverage:
// the missing/unclosed-fence scenario belongs to the "LintSkillFile Composes
// SplitSkillFile and LintSkill" requirement, not to LintSkill itself.

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
