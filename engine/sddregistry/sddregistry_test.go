package sddregistry

import (
	"errors"
	"reflect"
	"testing"
)

func TestFrontmatterDescriptionReadsTheFirstDescriptionLineAsTheScriptDid(t *testing.T) {
	for _, tc := range []struct {
		name, skill, want string
	}{
		{"an unquoted description", "---\nname: x\ndescription: Apply the change\n---\n", "Apply the change"},
		{"a quoted description loses its quotes", "description: \"Archive the change\"\n", "Archive the change"},
		{"a bar is escaped", "description: \"Archive | with a pipe\"\n", `Archive \| with a pipe`},
		{"a bar that is already escaped is escaped again", "description: \\|already escaped\n", `\\|already escaped`},
		{"white space before the text is dropped and after it is kept", "description:    padded   \n", "padded   "},
		{"tabs before the text are dropped too", "description:\t\ttabbed\n", "tabbed"},
		{"a carriage return before the text is white space", "description:\r\r text\n", "text"},
		{"only the first quote and the last are dropped", "description: \"quoted\" and then more\n", `quoted" and then more`},
		{"a quote at the end only is dropped", "description: say \"hi\"\n", `say "hi`},
		{"single quotes stay", "description: 'single quotes stay'\n", "'single quotes stay'"},
		{"a lone quote leaves nothing", "description: \"\n", ""},
		{"two quotes leave nothing", "description: \"\"\n", ""},
		{"a carriage return stays", "description: with a return\r\n", "with a return\r"},
		{"no text says nothing", "description:\n", ""},
		{"the first line wins even when it says nothing", "description:\ndescription: second\n", ""},
		{"the first line wins", "description: first\ndescription: second\n", "first"},
		{"the line can be in the body", "# no frontmatter\n\ndescription: found in the body\n", "found in the body"},
		{"an indented line is not one", "  description: indented\n", ""},
		{"the word must be followed by a colon", "descriptions: no\n", ""},
		{"no line, no description", "---\nname: x\n---\n", ""},
		{"an empty file", "", ""},
		{"a last line with no line end", "description: no line end", "no line end"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := FrontmatterDescription(tc.skill); got != tc.want {
				t.Errorf("FrontmatterDescription(%q) = %q, want %q", tc.skill, got, tc.want)
			}
		})
	}
}

func TestNewExecutorDescribesASkillWithNoDescriptionByItsName(t *testing.T) {
	got := NewExecutor("sdd-design", "/s/sdd-design/SKILL.md", "---\nname: sdd-design\n---\n")
	want := Executor{Name: "sdd-design", Path: "/s/sdd-design/SKILL.md", Description: "SDD phase executor: sdd-design."}
	if got != want {
		t.Errorf("NewExecutor = %+v, want %+v", got, want)
	}
	// A description that comes to nothing once its quotes are off is no description either.
	if got := NewExecutor("sdd-verify", "/p", "description: \"\n"); got.Description != "SDD phase executor: sdd-verify." {
		t.Errorf("a description of one quote = %q, want the one the name gives", got.Description)
	}
}

func TestRowIsAnExecutorRowOfTheTable(t *testing.T) {
	got := Executor{Name: "sdd-apply", Path: "/s/sdd-apply/SKILL.md", Description: "Apply"}.Row()
	want := "| `sdd-apply` | Apply | executor | `/s/sdd-apply/SKILL.md` |"
	if got != want {
		t.Errorf("Row = %q, want %q", got, want)
	}
	if !staleRow.MatchString(got) {
		t.Errorf("a repair does not recognize its own row %q as one it wrote, so it would write it twice", got)
	}
}

func TestExecutorsAreTheElevenPhaseSkillsInTheOrderOfTheirRows(t *testing.T) {
	want := []string{
		"sdd-apply", "sdd-archive", "sdd-design", "sdd-explore", "sdd-init", "sdd-onboard",
		"sdd-propose", "sdd-spec", "sdd-tasks", "sdd-time-estimation", "sdd-verify",
	}
	if got := Executors(); !reflect.DeepEqual(got, want) {
		t.Errorf("Executors = %v, want %v", got, want)
	}
	// What a caller gets is its own: changing it changes nothing here.
	got := Executors()
	got[0] = "changed"
	if Executors()[0] != "sdd-apply" {
		t.Error("Executors hands out the list it keeps, so a caller can change it")
	}
}

func TestProjectNameIsTheDirectoryThatHoldsTheAtlDirectory(t *testing.T) {
	for registry, want := range map[string]string{
		"/home/u/proj/.atl/skill-registry.md":       "proj",
		"proj/.atl/skill-registry.md":               "proj",
		"proj//.atl/skill-registry.md":              "proj",
		"./.atl/skill-registry.md":                  ".",
		"/.atl/skill-registry.md":                   "/",
		"/home/u/my project/.atl/skill-registry.md": "my project",
	} {
		if got := ProjectName(registry); got != want {
			t.Errorf("ProjectName(%q) = %q, want %q", registry, got, want)
		}
	}
}

func TestNewRegistryHasTheTwoSectionsTheRepairWorksBetween(t *testing.T) {
	got := NewRegistry("proj")
	want := "# Skill Registry — proj\n" +
		"\n" +
		"## Skills\n" +
		"\n" +
		"| Skill | Trigger / description | Scope | Path |\n" +
		"| --- | --- | --- | --- |\n" +
		"\n" +
		"## Loading protocol\n" +
		"\n" +
		"1. Match task context and target files against the `Trigger / description` column.\n" +
		"2. Pass only the matching `Path` values to the subagent under `## Skills to load before work`.\n"
	if got != want {
		t.Errorf("NewRegistry = %q, want %q", got, want)
	}
	if _, err := Repair(got, nil); err != nil {
		t.Errorf("the registry a repair makes is refused by the next one: %v", err)
	}
}

func TestRepairPutsTheNoteAboveTheSkillsAndTheRowsAboveTheLoadingProtocol(t *testing.T) {
	rows := []Executor{
		{Name: "sdd-apply", Path: "/s/a", Description: "Apply"},
		{Name: "sdd-verify", Path: "/s/v", Description: "Verify"},
	}
	got, err := Repair("# r\n## Skills\n| a |\n## Loading protocol\nx\n", rows)
	if err != nil {
		t.Fatal(err)
	}
	want := "# r\n" + Note + "\n\n## Skills\n| a |\n" +
		"| `sdd-apply` | Apply | executor | `/s/a` |\n" +
		"| `sdd-verify` | Verify | executor | `/s/v` |\n" +
		"\n## Loading protocol\nx\n"
	if got != want {
		t.Errorf("Repair =\n%q\nwant\n%q", got, want)
	}
}

func TestRepairDropsTheNoteAndTheRowsOfAnEarlierRepairAndKeepsTheBlankLinesItLeft(t *testing.T) {
	registry := "## Skills\n" +
		"| `sdd-spec` | a row of the overlay | executor | `/x` |\n" + // dropped
		"| `sdd-ghost` | a skill the overlay has not got | executor | `/x` |\n" + // dropped
		"| `sdd-spec`| no space before the bar | executor | `/x` |\n" + // kept
		"| sdd-spec | no backticks | executor | `/x` |\n" + // kept
		"|  `sdd-spec` | two spaces before | executor | `/x` |\n" + // kept
		"| `sdd-` | no name | executor | `/x` |\n" + // kept
		"  | `sdd-spec` | indented | executor | `/x` |\n" + // kept
		Note + "\n" + // dropped
		"## Loading protocol\n"
	got, err := Repair(registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := Note + "\n\n## Skills\n" +
		"| `sdd-spec`| no space before the bar | executor | `/x` |\n" +
		"| sdd-spec | no backticks | executor | `/x` |\n" +
		"|  `sdd-spec` | two spaces before | executor | `/x` |\n" +
		"| `sdd-` | no name | executor | `/x` |\n" +
		"  | `sdd-spec` | indented | executor | `/x` |\n" +
		"\n## Loading protocol\n"
	if got != want {
		t.Errorf("Repair =\n%q\nwant\n%q", got, want)
	}

	// Not idempotent, as the script was: the note is dropped and written again below the blank line
	// the first repair put under it, and the rows below the one it put above the heading, so each
	// repair adds a blank line above the note and one above the loading protocol.
	again, err := Repair(got, nil)
	if err != nil {
		t.Fatal(err)
	}
	wantAgain := "\n" + Note + "\n\n## Skills\n" +
		"| `sdd-spec`| no space before the bar | executor | `/x` |\n" +
		"| sdd-spec | no backticks | executor | `/x` |\n" +
		"|  `sdd-spec` | two spaces before | executor | `/x` |\n" +
		"| `sdd-` | no name | executor | `/x` |\n" +
		"  | `sdd-spec` | indented | executor | `/x` |\n" +
		"\n\n## Loading protocol\n"
	if again != wantAgain {
		t.Errorf("a second Repair =\n%q\nwant\n%q", again, wantAgain)
	}
}

func TestRepairKeepsTheShapeOfTheRegistryItIsGiven(t *testing.T) {
	for _, tc := range []struct{ name, registry, want string }{
		{"a last line with no line end gets one", "## Skills\n## Loading protocol", Note + "\n\n## Skills\n\n## Loading protocol\n"},
		{"blank lines at the end stay", "## Skills\n## Loading protocol\n\n\n", Note + "\n\n## Skills\n\n## Loading protocol\n\n\n"},
		{"the headings twice are worked twice", "## Skills\n## Loading protocol\n## Skills\n", Note + "\n\n## Skills\n\n## Loading protocol\n" + Note + "\n\n## Skills\n"},
		{"a heading in a code fence is a heading", "```\n## Skills\n```\n## Loading protocol\n", "```\n" + Note + "\n\n## Skills\n```\n\n## Loading protocol\n"},
		{"carriage returns stay on the lines they end", "# a\r\n## Skills\n## Loading protocol\n", "# a\r\n" + Note + "\n\n## Skills\n\n## Loading protocol\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Repair(tc.registry, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("Repair(%q) = %q, want %q", tc.registry, got, tc.want)
			}
		})
	}
}

func TestRepairRefusesARegistryWithoutOneOfItsHeadingsAndSaysWhich(t *testing.T) {
	for _, tc := range []struct{ name, registry, heading string }{
		{"no skills heading", "# r\n## Loading protocol\n", "## Skills"},
		{"no loading protocol heading", "# r\n## Skills\n", "## Loading protocol"},
		{"neither heading says the first", "", "## Skills"},
		{"headings with carriage returns", "## Skills\r\n## Loading protocol\r\n", "## Skills"},
		{"headings with a trailing space", "## Skills \n## Loading protocol \n", "## Skills"},
		{"white space only", " \n\n", "## Skills"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Repair(tc.registry, nil)
			var missing *MissingSectionError
			if !errors.As(err, &missing) || missing.Heading != tc.heading {
				t.Errorf("Repair(%q) = %q, %v, want a refusal that names %q", tc.registry, got, err, tc.heading)
			}
			if got != "" {
				t.Errorf("Repair returned %q with its refusal, want nothing", got)
			}
		})
	}
	if msg := (&MissingSectionError{Heading: "## Skills"}).Error(); msg != "registry has no '## Skills' section" {
		t.Errorf("the refusal says %q", msg)
	}
}

func TestCountRowsCountsTheLinesThatBeginLikeARow(t *testing.T) {
	registry := "## Skills\n| `sdd-a` | x |\n| `sdd-b`x\n| sdd-c |\n  | `sdd-d` |\n| `sdd` | no hyphen |\n| `sdd-"
	if got := CountRows(registry); got != 3 {
		t.Errorf("CountRows = %d, want 3: a line is counted when it begins with \"| `sdd-\", whatever follows", got)
	}
	if got := CountRows(""); got != 0 {
		t.Errorf("CountRows of nothing = %d", got)
	}
}
