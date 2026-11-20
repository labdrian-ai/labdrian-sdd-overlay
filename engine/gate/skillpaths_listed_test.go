package gate_test

// RewriteListedSkillPaths (labdrian-orchestrator-opencode T60 correction B, S8 L173): the header-section
// rewrite and its refusals alone, for a role that reads only its candidate, so a prose mention adds nothing.

import (
	"path/filepath"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gate"
)

func TestRewriteListedSkillPathsRewritesTheSectionAndAddsNoProse(t *testing.T) {
	root := proseRoot(t)
	p := func(id string) string { return filepath.Join(root, id, "SKILL.md") }
	for name, c := range map[string]struct{ prompt, want string }{
		"a header and a prose mention": {
			"Follow test-first and go-testing.\n" + skillsHeader + "\nsweeper\n",
			"Follow test-first and go-testing.\n" + skillsHeader + "\n" + p("sweeper") + "\n",
		},
		"no header, a prose mention": {"Use test-first.\n", "Use test-first.\n"},
		"a crlf prompt":              {"Use test-first.\r\n" + skillsHeader + "\r\nsweeper\r\n", "Use test-first.\r\n" + skillsHeader + "\r\n" + p("sweeper") + "\r\n"},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := gate.RewriteListedSkillPaths(c.prompt, root, isRegularFile)
			if err != nil || got != c.want {
				t.Fatalf("RewriteListedSkillPaths = %q, %v; want %q", got, err, c.want)
			}
		})
	}
	for name, prompt := range map[string]string{
		"an absent path": skillsHeader + "\n/nowhere/SKILL.md\n",
		"other text":     skillsHeader + "\nWhat Is This\n",
	} {
		t.Run("refuses "+name, func(t *testing.T) {
			_, want := gate.RewriteSkillPaths(prompt, root, isRegularFile)
			got, err := gate.RewriteListedSkillPaths(prompt, root, isRegularFile)
			if want == nil || err == nil || err.Error() != want.Error() || got != "" {
				t.Fatalf("RewriteListedSkillPaths = %q, %v; want no prompt and %v", got, err, want)
			}
		})
	}
	// The same eight refusals as RewriteSkillPaths, each with its exact sentence, on the same root.
	refusalRoot, _ := skillsRoot(t)
	for name, c := range refusalCases(refusalRoot) {
		t.Run("refuses as RewriteSkillPaths does: "+name, func(t *testing.T) {
			got, err := gate.RewriteListedSkillPaths(c.prompt, refusalRoot, isRegularFile)
			if err == nil || err.Error() != c.want || got != "" {
				t.Fatalf("RewriteListedSkillPaths = %q, %v; want no prompt and %q", got, err, c.want)
			}
		})
	}
}
