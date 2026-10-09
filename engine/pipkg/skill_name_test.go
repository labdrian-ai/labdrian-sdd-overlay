package pipkg_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/pipkg"
)

// buildWithSkillFile builds a package whose only pi skill, named pi-skill, has the given
// SKILL.md, and returns the error of the build.
func buildWithSkillFile(t *testing.T, content string) error {
	t.Helper()
	overlayRoot, registryPath := fixtureOverlay(t)
	writeFile(t, filepath.Join(overlayRoot, "skills", "pi-skill", "SKILL.md"), content)
	return pipkg.Build(fileRegistries, overlayRoot, registryPath, filepath.Join(t.TempDir(), "labdrian-pi"))
}

// What a SKILL.md must say for the build to ship it, and the words of the refusal when it does
// not. These are the cases the build has always answered the same way.
func TestBuildReadsTheSkillNameFromTheFrontmatter(t *testing.T) {
	cases := map[string]struct {
		content string
		want    string // a substring of the error; empty means the build succeeds
	}{
		"a plain name":             {"---\nname: pi-skill\n---\nbody\n", ""},
		"a double-quoted name":     {"---\nname: \"pi-skill\"\n---\nbody\n", ""},
		"a single-quoted name":     {"---\nname: 'pi-skill'\n---\nbody\n", ""},
		"the name after others":    {"---\ndescription: d\nname: pi-skill\n---\nbody\n", ""},
		"carriage returns":         {"---\r\nname: pi-skill\r\n---\r\nbody\r\n", ""},
		"another name":             {"---\nname: other\n---\nbody\n", `SKILL.md name "other" does not match directory "pi-skill"`},
		"no frontmatter":           {"body only\n", "SKILL.md has no frontmatter block"},
		"an empty file":            {"", "SKILL.md has no frontmatter block"},
		"a block without a name":   {"---\ndescription: d\n---\nbody\n", `SKILL.md frontmatter has no "name" field`},
		"an unterminated block":    {"---\ndescription: d\n", "SKILL.md frontmatter is not terminated"},
		"a name that is not a key": {"---\nthe name: pi-skill\n---\nbody\n", `SKILL.md frontmatter has no "name" field`},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			err := buildWithSkillFile(t, c.content)
			switch {
			case c.want == "" && err != nil:
				t.Fatalf("Build refused a SKILL.md it ships: %v", err)
			case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
				t.Fatalf("Build error = %v, want it to contain %q", err, c.want)
			}
		})
	}
}

func TestBuildReportsASkillWithoutASkillFile(t *testing.T) {
	overlayRoot, registryPath := fixtureOverlay(t)
	if err := os.Remove(filepath.Join(overlayRoot, "skills", "pi-skill", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	err := pipkg.Build(fileRegistries, overlayRoot, registryPath, filepath.Join(t.TempDir(), "labdrian-pi"))
	if err == nil || !strings.Contains(err.Error(), "reading SKILL.md") {
		t.Fatalf("Build error = %v, want it to say it could not read SKILL.md", err)
	}
}

// The reading is the one `engine skills` lint uses (skills.ReadFrontmatter), so the build and the
// lint agree on what a SKILL.md says. Four inputs the build used to read differently:
func TestBuildReadsTheFrontmatterTheWayTheLintDoes(t *testing.T) {
	cases := map[string]struct {
		content string
		want    string // empty means the build succeeds
	}{
		"a byte-order mark is tolerated (it was refused as no frontmatter)": {"\xEF\xBB\xBF---\nname: pi-skill\n---\nbody\n", ""},
		"a key under another key is not the name (it was read as the name)": {"---\nmetadata:\n  name: pi-skill\n---\nbody\n", `SKILL.md frontmatter has no "name" field`},
		"an unclosed block is refused even when it names the skill":         {"---\nname: pi-skill\nbody without a closing fence\n", "SKILL.md frontmatter is not terminated"},
		"an indented fence is not a fence":                                  {"  ---\nname: pi-skill\n---\nbody\n", "SKILL.md has no frontmatter block"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			err := buildWithSkillFile(t, c.content)
			switch {
			case c.want == "" && err != nil:
				t.Fatalf("Build refused: %v", err)
			case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
				t.Fatalf("Build error = %v, want it to contain %q", err, c.want)
			}
		})
	}
}
