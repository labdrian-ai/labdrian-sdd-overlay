package gate_test

// RewriteSkillPaths, prose mentions (labdrian-orchestrator-opencode T60, S8 L162): a skill a launch prompt
// names outside '## Skills to load before work' and outside fenced code also reaches the role as its absolute
// SKILL.md path under that header. A mention is a whole word of the form of a skill id with a hyphen whose
// <root>/<id>/SKILL.md is a file; a word with no SKILL.md is not a mention and refuses nothing.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gate"
)

// proseRoot is a skills root holding test-first, go-testing, branch-pr and sweeper, each with a SKILL.md.
func proseRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, id := range []string{"test-first", "go-testing", "branch-pr", "sweeper"} {
		if err := os.MkdirAll(filepath.Join(root, id), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, id, "SKILL.md"), []byte("# "+id+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestRewriteSkillPathsAddsTheSkillsAPromptNamesInProse(t *testing.T) {
	root := proseRoot(t)
	p := func(id string) string { return filepath.Join(root, id, "SKILL.md") }
	for name, c := range map[string]struct{ prompt, want string }{
		"prose, backticks and a relative path under an existing header": {
			"Follow the test-first skill, and `go-testing`; read skills/branch-pr/SKILL.md.\n\n" + skillsHeader + "\nsweeper\n\n## Rules\nx\n",
			"Follow the test-first skill, and `go-testing`; read skills/branch-pr/SKILL.md.\n\n" + skillsHeader + "\n" + p("sweeper") + "\n" + p("test-first") + "\n" + p("go-testing") + "\n" + p("branch-pr") + "\n\n## Rules\nx\n",
		},
		"an id already listed as id": {
			"Use test-first.\n" + skillsHeader + "\ntest-first\n",
			"Use test-first.\n" + skillsHeader + "\n" + p("test-first") + "\n",
		},
		"an id already listed as path": {
			"Use test-first.\n" + skillsHeader + "\n" + p("test-first") + "\n",
			"Use test-first.\n" + skillsHeader + "\n" + p("test-first") + "\n",
		},
		"each once, in the order of first mention": {
			"go-testing then test-first then go-testing\n" + skillsHeader + "\n",
			"go-testing then test-first then go-testing\n" + skillsHeader + "\n" + p("go-testing") + "\n" + p("test-first") + "\n",
		},
		"an id inside a fence": {
			"Run:\n```\ntest-first\n```\nand go-testing\n",
			"Run:\n```\ntest-first\n```\nand go-testing\n\n" + skillsHeader + "\n" + p("go-testing") + "\n",
		},
		"no hyphen is no mention": {
			"Ask the sweeper.\n",
			"Ask the sweeper.\n",
		},
		"a word with no SKILL.md is neither added nor refused": {
			"Run go-test now.\n" + skillsHeader + "\n",
			"Run go-test now.\n" + skillsHeader + "\n",
		},
		"a longer word is not the id": {
			"see test-first-aid and my_test-first and test-first_x\n",
			"see test-first-aid and my_test-first and test-first_x\n",
		},
		"no header, a mention: the section is appended": {
			"Build with test-first.\n\n\n",
			"Build with test-first.\n\n" + skillsHeader + "\n" + p("test-first") + "\n",
		},
		"a section holding only blank lines": {
			"Use test-first.\n" + skillsHeader + "\n\n",
			"Use test-first.\n" + skillsHeader + "\n" + p("test-first") + "\n\n",
		},
		"a crlf prompt keeps its carriage returns": {
			"Use test-first.\r\n" + skillsHeader + "\r\nsweeper\r\n",
			"Use test-first.\r\n" + skillsHeader + "\r\n" + p("sweeper") + "\r\n" + p("test-first") + "\r\n",
		},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := gate.RewriteSkillPaths(c.prompt, root, isRegularFile)
			if err != nil || got != c.want {
				t.Fatalf("RewriteSkillPaths = %q, %v; want %q", got, err, c.want)
			}
		})
	}
}

func TestRewriteSkillPathsReturnsAPromptWithNoMentionByteIdentical(t *testing.T) {
	root := proseRoot(t)
	for name, prompt := range map[string]string{
		"no header, no mention": "Build T1.\r\nnothing here\n\n\n",
		"empty":                 "",
	} {
		t.Run(name, func(t *testing.T) {
			got, err := gate.RewriteSkillPaths(prompt, root, isRegularFile)
			if err != nil || got != prompt {
				t.Fatalf("RewriteSkillPaths = %q, %v; want the prompt byte for byte", got, err)
			}
		})
	}
}

func TestRewriteSkillPathsFollowsThePromptsLineEnding(t *testing.T) {
	root := proseRoot(t)
	p := filepath.Join(root, "test-first", "SKILL.md")
	want := "Use test-first.\r\n\r\n" + skillsHeader + "\r\n" + p + "\r\n"
	for name, prompt := range map[string]string{
		"one crlf":  "Use test-first.\r\n",
		"two crlfs": "Use test-first.\r\n\r\n",
	} {
		t.Run(name, func(t *testing.T) {
			got, err := gate.RewriteSkillPaths(prompt, root, isRegularFile)
			if err != nil || got != want {
				t.Fatalf("RewriteSkillPaths = %q, %v; want %q", got, err, want)
			}
		})
	}
}

func TestRewriteSkillPathsReadsFencesAsCommonMarkOpensThem(t *testing.T) {
	root := proseRoot(t)
	p := filepath.Join(root, "go-testing", "SKILL.md")
	for name, body := range map[string]string{
		"an indented fence":                   "  ```\ntest-first\n  ```\n",
		"a tilde fence":                       "~~~\ntest-first\n~~~\n",
		"a fence in a list item":              "- item\n    ```\n    test-first\n    ```\n",
		"a four-backtick fence holding three": "````\n```\ntest-first\n```\n````\n",
	} {
		t.Run(name, func(t *testing.T) {
			prompt := "Run:\n" + body + "then go-testing\n"
			want := prompt + "\n" + skillsHeader + "\n" + p + "\n"
			got, err := gate.RewriteSkillPaths(prompt, root, isRegularFile)
			if err != nil || got != want {
				t.Fatalf("RewriteSkillPaths = %q, %v; want %q", got, err, want)
			}
		})
	}
}

func TestRewriteSkillPathsAddsNothingAfterAnUnclosedFence(t *testing.T) {
	root := proseRoot(t)
	prompt := "Run:\n```\ntest-first\nand go-testing\n"
	got, err := gate.RewriteSkillPaths(prompt, root, isRegularFile)
	if err != nil || got != prompt {
		t.Fatalf("RewriteSkillPaths = %q, %v; want the prompt byte for byte", got, err)
	}
}

func TestRewriteSkillPathsListsEntriesByTheirSkillID(t *testing.T) {
	root := proseRoot(t)
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	for name, entry := range map[string]string{
		"a dot element in the path":    root + "/./test-first/SKILL.md",
		"a path through a linked root": filepath.Join(link, "test-first", "SKILL.md"),
	} {
		t.Run(name, func(t *testing.T) {
			prompt := "Use test-first.\n" + skillsHeader + "\n" + entry + "\n"
			got, err := gate.RewriteSkillPaths(prompt, root, isRegularFile)
			if err != nil || got != prompt {
				t.Fatalf("RewriteSkillPaths = %q, %v; want the prompt byte for byte", got, err)
			}
		})
	}
}

func TestRewriteSkillPathsTakesALowerHeadingForNoHeader(t *testing.T) {
	root := proseRoot(t)
	p := filepath.Join(root, "test-first", "SKILL.md")
	prompt := "### Skills to load before work\ntest-first\n"
	want := prompt + "\n" + skillsHeader + "\n" + p + "\n"
	got, err := gate.RewriteSkillPaths(prompt, root, isRegularFile)
	if err != nil || got != want {
		t.Fatalf("RewriteSkillPaths = %q, %v; want %q", got, err, want)
	}
}

func TestRewriteSkillPathsAsksTheDiskOncePerWord(t *testing.T) {
	root := proseRoot(t)
	calls := 0
	counting := func(path string) bool {
		calls++
		return isRegularFile(path)
	}
	if _, err := gate.RewriteSkillPaths("go-test go-test go-test, test-first and test-first\n", root, counting); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("isFile was asked %d times, want 2 (one per distinct word)", calls)
	}
}
