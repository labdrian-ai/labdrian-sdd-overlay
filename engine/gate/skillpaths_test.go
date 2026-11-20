package gate_test

// RewriteSkillPaths (labdrian-orchestrator-opencode T43 commit a, S8 L117 b): the skill list of a launch
// prompt, the section under the exact header '## Skills to load before work', is made of absolute SKILL.md
// paths. A bare id of the skills root becomes its path; an absolute path to an existing file and a blank line
// stay; anything else is refused with one sentence that says what to write. Gate.Rewrite is not involved.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gate"
)

// isRegularFile is the port the gate is given: the disk of the test.
func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// skillsRoot makes a skills root holding the skills test-first and go-testing, each with a SKILL.md, and
// the directory no-file with none. It returns the root and the path of test-first's SKILL.md.
func skillsRoot(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	for _, id := range []string{"test-first", "go-testing"} {
		if err := os.MkdirAll(filepath.Join(root, id), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, id, "SKILL.md"), []byte("# "+id+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "no-file"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root, filepath.Join(root, "test-first", "SKILL.md")
}

func TestRewriteSkillPathsMakesABareIdItsAbsolutePath(t *testing.T) {
	root, testFirst := skillsRoot(t)
	goTesting := filepath.Join(root, "go-testing", "SKILL.md")
	prompt := "Build T1.\n\n" + skillsHeader + "\ntest-first\n  go-testing  \n\n## Rules\ntest-first\n"

	got, err := gate.RewriteSkillPaths(prompt, root, isRegularFile)

	want := "Build T1.\n\n" + skillsHeader + "\n" + testFirst + "\n  " + goTesting + "  \n\n## Rules\ntest-first\n"
	if err != nil || got != want {
		t.Fatalf("RewriteSkillPaths = %q, %v; want %q (the ids under the header become paths, the text after the next heading stays)", got, err, want)
	}
}

func TestRewriteSkillPathsKeepsAnAbsolutePathAndABlankLine(t *testing.T) {
	root, testFirst := skillsRoot(t)
	prompt := skillsHeader + "\n" + testFirst + "\n\n   \n"

	got, err := gate.RewriteSkillPaths(prompt, root, isRegularFile)

	if err != nil || got != prompt {
		t.Fatalf("RewriteSkillPaths = %q, %v; want the prompt as it was", got, err)
	}
}

func TestRewriteSkillPathsReturnsAPromptWithNoHeaderUnchanged(t *testing.T) {
	root, _ := skillsRoot(t)
	for name, prompt := range map[string]string{
		"empty":            "",
		"no header":        "Build T1.\nplain text\n",
		"a lower heading":  "### Skills to load before work\nplain text\n",
		"header in a text": "Write ## Skills to load before work on its own line.\n",
	} {
		t.Run(name, func(t *testing.T) {
			got, err := gate.RewriteSkillPaths(prompt, root, isRegularFile)
			if err != nil || got != prompt {
				t.Fatalf("RewriteSkillPaths = %q, %v; want the prompt unchanged", got, err)
			}
		})
	}
}

// refusalCases are the prompts the header-section rewrite refuses and the one line it says for each. Both
// entry points, RewriteSkillPaths and RewriteListedSkillPaths, are ranged over it, so they cannot drift.
func refusalCases(root string) map[string]struct{ prompt, want string } {
	missing := filepath.Join(root, "absent", "SKILL.md")
	return map[string]struct{ prompt, want string }{
		"the colon form": {
			"Build.\n" + skillsHeader + ": test-first, go-test\n",
			`line 2 "## Skills to load before work: test-first, go-test" is not the header alone: write "## Skills to load before work" alone on its line, then one skill id or absolute SKILL.md path per line`,
		},
		"a variant after the header": {
			skillsHeader + "\ntest-first\n" + skillsHeader + " (extra)\n",
			`line 3 "## Skills to load before work (extra)" is not the header alone: write "## Skills to load before work" alone on its line, then one skill id or absolute SKILL.md path per line`,
		},
		"an unknown id": {
			skillsHeader + "\nno-such\n",
			`line 2 "no-such" names no skill: write a skill id of ` + root + ` (a directory with a SKILL.md) or the absolute path of its SKILL.md`,
		},
		"an id with no file": {
			skillsHeader + "\nno-file\n",
			`line 2 "no-file" names no skill: write a skill id of ` + root + ` (a directory with a SKILL.md) or the absolute path of its SKILL.md`,
		},
		"an absolute path to no file": {
			skillsHeader + "\n" + missing + "\n",
			`line 2 "` + missing + `" is an absolute path to no file: write the absolute path of an existing SKILL.md or a skill id of ` + root,
		},
		"a list marker": {
			skillsHeader + "\n- test-first\n",
			`line 2 "- test-first" is neither a skill id nor an absolute path: a skill id is lowercase letters, digits and hyphens; write one skill id or absolute SKILL.md path per line`,
		},
		"a relative path": {
			skillsHeader + "\nskills/test-first/SKILL.md\n",
			`line 2 "skills/test-first/SKILL.md" is neither a skill id nor an absolute path: a skill id is lowercase letters, digits and hyphens; write one skill id or absolute SKILL.md path per line`,
		},
		"a comma list": {
			skillsHeader + "\ntest-first, go-testing\n",
			`line 2 "test-first, go-testing" is neither a skill id nor an absolute path: a skill id is lowercase letters, digits and hyphens; write one skill id or absolute SKILL.md path per line`,
		},
	}
}

func TestRewriteSkillPathsRefusesWhatItCannotMakeAPathOf(t *testing.T) {
	root, _ := skillsRoot(t)
	for name, c := range refusalCases(root) {
		t.Run(name, func(t *testing.T) {
			got, err := gate.RewriteSkillPaths(c.prompt, root, isRegularFile)
			if err == nil || err.Error() != c.want {
				t.Fatalf("RewriteSkillPaths error = %v; want %q", err, c.want)
			}
			if got != "" || strings.Contains(err.Error(), "\n") {
				t.Fatalf("a refusal returns no prompt and one line, got %q and %q", got, err)
			}
		})
	}
}

func TestRewriteSkillPathsKeepsTheIndentAndTheLineEndOfAnIdLine(t *testing.T) {
	root, testFirst := skillsRoot(t)
	prompt := skillsHeader + "\r\n  test-first\r\n\ttest-first  \r\n"

	got, err := gate.RewriteSkillPaths(prompt, root, isRegularFile)

	want := skillsHeader + "\r\n  " + testFirst + "\r\n\t" + testFirst + "  \r\n"
	if err != nil || got != want {
		t.Fatalf("RewriteSkillPaths = %q, %v; want %q (the id becomes its path, the indent and the carriage return stay)", got, err, want)
	}
}

func TestRewriteSkillPathsStopsAtTheNextHeading(t *testing.T) {
	root, testFirst := skillsRoot(t)
	prompt := skillsHeader + "\ntest-first\n# Notes\nfree text, with: anything\n" + skillsHeader + "\nno-such\n"

	got, err := gate.RewriteSkillPaths(prompt, root, isRegularFile)

	want := skillsHeader + "\n" + testFirst + "\n# Notes\nfree text, with: anything\n" + skillsHeader + "\nno-such\n"
	if err != nil || got != want {
		t.Fatalf("RewriteSkillPaths = %q, %v; want only the section under the first exact header made paths, %q", got, err, want)
	}
}
