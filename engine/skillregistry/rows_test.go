package skillregistry

import (
	"reflect"
	"testing"
)

func fm(name, desc string) string { return "---\nname: " + name + "\ndescription: " + desc + "\n---\n" }

func TestRowLineIsTheTableRow(t *testing.T) {
	got := Row{Name: "plain", Description: "A plain one.", Scope: ScopeProject, Path: "/p/skills/plain/SKILL.md"}.Line()
	want := "| `plain` | A plain one. | project | `/p/skills/plain/SKILL.md` |"
	if got != want {
		t.Fatalf("Line = %q, want %q", got, want)
	}
}

func TestRowLineEscapesBarsAndDashesAnEmptyDescription(t *testing.T) {
	bars := Row{Name: "pipe|name", Description: "a | b || c", Scope: ScopeUser, Path: "/p"}.Line()
	if want := "| `pipe\\|name` | a \\| b \\|\\| c | user | `/p` |"; bars != want {
		t.Fatalf("Line = %q, want %q", bars, want)
	}
	empty := Row{Name: "n", Scope: ScopeProject, Path: "/p"}.Line()
	if want := "| `n` | — | project | `/p` |"; empty != want {
		t.Fatalf("Line = %q, want %q", empty, want)
	}
}

func TestRowsTheFirstSourceWinsAFileOfTheSameName(t *testing.T) {
	got := Rows([]Candidate{
		{ScopeProject, "plain", "/p/skills/plain/SKILL.md", fm("plain", "project one")},
		{ScopeProject, "plain", "/p/.claude/skills/plain/SKILL.md", fm("plain", "loser in claude dir")},
		{ScopeUser, "plain", "/h/.claude/skills/plain/SKILL.md", fm("plain", "loser in user dir")},
	})
	want := []Row{{"plain", "project one", ScopeProject, "/p/skills/plain/SKILL.md"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Rows = %+v, want %+v", got, want)
	}
}

func TestRowsKeyOnTheFrontmatterNameAndTakeTheDirectoryWhenThereIsNone(t *testing.T) {
	got := Rows([]Candidate{
		{ScopeProject, "dirdiff", "/p/skills/dirdiff/SKILL.md", fm("realname", "d")},
		{ScopeProject, "noname", "/p/skills/noname/SKILL.md", "---\ndescription: has no name\n---\n"},
		{ScopeProject, "nofm", "/p/skills/nofm/SKILL.md", "# heading\n"},
	})
	want := []Row{
		{"nofm", "", ScopeProject, "/p/skills/nofm/SKILL.md"},
		{"noname", "has no name", ScopeProject, "/p/skills/noname/SKILL.md"},
		{"realname", "d", ScopeProject, "/p/skills/dirdiff/SKILL.md"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Rows = %+v, want %+v", got, want)
	}
}

func TestRowsSkipSharedTheRegistrySkillAndTheSDDSkillsByTheirName(t *testing.T) {
	got := Rows([]Candidate{
		{ScopeProject, "_shared", "/p/skills/_shared/SKILL.md", fm("_shared", "no")},
		{ScopeProject, "skill-registry", "/p/skills/skill-registry/SKILL.md", fm("skill-registry", "no")},
		{ScopeProject, "dirsdd", "/p/skills/dirsdd/SKILL.md", fm("sdd-bar", "no")},
		{ScopeProject, "sdd-foo", "/p/skills/sdd-foo/SKILL.md", "# no frontmatter, the directory is the name\n"},
		// The directory's name decides nothing when the skill names itself.
		{ScopeProject, "sdd-baz", "/p/skills/sdd-baz/SKILL.md", fm("baz", "dir sdd")},
		{ScopeProject, "u1", "/p/skills/u1/SKILL.md", fm("shared", "dir _shared is kept by its name")},
		{ScopeProject, "sddx", "/p/skills/sddx/SKILL.md", fm("sddx", "no hyphen")},
	})
	want := []Row{
		{"baz", "dir sdd", ScopeProject, "/p/skills/sdd-baz/SKILL.md"},
		{"sddx", "no hyphen", ScopeProject, "/p/skills/sddx/SKILL.md"},
		{"shared", "dir _shared is kept by its name", ScopeProject, "/p/skills/u1/SKILL.md"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Rows = %+v, want %+v", got, want)
	}
}

func TestRowsAreSortedByNameInByteOrder(t *testing.T) {
	got := Rows([]Candidate{
		{ScopeUser, "emptyname", "/h/e", fm("emptyname", "")},
		{ScopeUser, "_under", "/h/_", fm("_under", "")},
		{ScopeUser, "alpha", "/h/a", fm("alpha", "")},
		{ScopeUser, "Zed", "/h/z", fm("Zed", "")},
		{ScopeUser, "Alpha", "/h/A", fm("Alpha", "")},
	})
	var names []string
	for _, row := range got {
		names = append(names, row.Name)
	}
	if want := []string{"Alpha", "Zed", "_under", "alpha", "emptyname"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("names = %v, want %v", names, want)
	}
}

func TestRowsAreNoneWhenNothingIsFound(t *testing.T) {
	if got := Rows(nil); len(got) != 0 {
		t.Fatalf("Rows(nil) = %+v, want none", got)
	}
}

func TestSourcesAreTheProjectDirectoriesThenTheUserDirectoriesInGentlesOrder(t *testing.T) {
	got := Sources("/p", "/h")
	var labels []string
	for _, s := range got {
		labels = append(labels, s.Label)
	}
	want := []string{
		"skills", ".opencode/skills", ".claude/skills", ".gemini/skills", ".cursor/skills",
		".github/skills", ".codex/skills", ".qwen/skills", ".kiro/skills", ".openclaw/skills",
		".pi/skills", ".agent/skills", ".agents/skills", ".atl/skills", ".hermes/skills",
		"/h/.pi/agent/skills", "/h/.config/agents/skills", "/h/.agents/skills", "/h/.kimi/skills",
		"/h/.config/opencode/skills", "/h/.config/kilo/skills", "/h/.claude/skills", "/h/.gemini/skills",
		"/h/.gemini/antigravity-cli/skills", "/h/.cursor/skills", "/h/.copilot/skills", "/h/.codex/skills",
		"/h/.codeium/windsurf/skills", "/h/.qwen/skills", "/h/.kiro/skills", "/h/.openclaw/skills",
		"/h/.hermes/skills", "/h/.trae/skills",
	}
	if !reflect.DeepEqual(labels, want) {
		t.Fatalf("labels = %v\nwant %v", labels, want)
	}
	if got[0] != (Source{ScopeProject, "skills", "/p/skills"}) {
		t.Fatalf("first source = %+v", got[0])
	}
	if got[15] != (Source{ScopeUser, "/h/.pi/agent/skills", "/h/.pi/agent/skills"}) {
		t.Fatalf("first user source = %+v", got[15])
	}
}

func TestSourcesHaveNoUserDirectoriesWithoutAHome(t *testing.T) {
	for _, s := range Sources("/p", "") {
		if s.Scope == ScopeUser {
			t.Fatalf("user source %+v without a home", s)
		}
	}
}
