package agentgen_test

// The tier x runtime table of skills/role-chain/references/models.md (S18): the generator reads the
// model ids from it, so a table that lacks a tier or a runtime the generator needs is refused, and the
// files of the three existing runtimes stay what they were.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/agentgen"
)

const modelsRel = "skills/role-chain/references/models.md"

func modelsPath(root string) string { return filepath.Join(root, filepath.FromSlash(modelsRel)) }

// editModels rewrites the table file of a seeded root.
func editModels(t *testing.T, root string, edit func(string) string) {
	t.Helper()
	text := readText(t, modelsPath(root))
	changed := edit(text)
	if changed == text {
		t.Fatal("the edit changed nothing: the table is not what the test expects")
	}
	if err := os.WriteFile(modelsPath(root), []byte(changed), 0o644); err != nil {
		t.Fatal(err)
	}
}

// The table of this repository holds today's Claude aliases and Pi ids and the OpenCode row.
func TestTheModelTableHoldsEveryTierForEveryRuntime(t *testing.T) {
	table, err := agentgen.ReadModelTable(repoRoot(t))
	if err != nil {
		t.Fatalf("ReadModelTable: %v", err)
	}
	want := map[string]map[string]string{
		"frontier": {"claude": "opus", "pi": "pi-claude-cli/claude-opus-5", "opencode": "openai/gpt-6.1-sol#medium"},
		"mid":      {"claude": "sonnet", "pi": "pi-claude-cli/claude-sonnet-5", "opencode": "openai/gpt-6.1-terra#medium"},
		"light":    {"claude": "haiku", "pi": "pi-claude-cli/claude-haiku-4-5", "opencode": "openai/gpt-6-luna#low"},
	}
	for tier, runtimes := range want {
		for runtime, id := range runtimes {
			got, err := table.ID(tier, runtime)
			if err != nil || got != id {
				t.Errorf("ID(%s, %s) = %q, %v; want %q", tier, runtime, got, err, id)
			}
		}
	}
}

// A table without a tier, or without a runtime, the generator needs is refused, and nothing is written.
func TestATableThatLacksATierOrARuntimeIsRefusedBeforeAnythingIsWritten(t *testing.T) {
	cases := map[string]struct {
		edit func(string) string
		want string
	}{
		"no light row": {func(s string) string {
			return strings.Replace(s, "| light | `haiku` | `pi-claude-cli/claude-haiku-4-5` | `openai/gpt-6-luna#low` |\n", "", 1)
		}, "light"},
		"no pi column": {func(s string) string {
			s = strings.Replace(s, "| tier | claude | pi | opencode |", "| tier | claude | opencode |", 1)
			s = strings.Replace(s, "| --- | --- | --- | --- |", "| --- | --- | --- |", 1)
			for _, id := range []string{"pi-claude-cli/claude-opus-5", "pi-claude-cli/claude-sonnet-5", "pi-claude-cli/claude-haiku-4-5"} {
				s = strings.Replace(s, " `"+id+"` |", "", 1)
			}
			return s
		}, "pi"},
		"no opencode column": {func(s string) string {
			s = strings.Replace(s, "| tier | claude | pi | opencode |", "| tier | claude | pi |", 1)
			for _, id := range []string{"openai/gpt-6.1-sol#medium", "openai/gpt-6.1-terra#medium", "openai/gpt-6-luna#low"} {
				s = strings.Replace(s, " `"+id+"` |", "", 1)
			}
			return strings.Replace(s, "| --- | --- | --- | --- |", "| --- | --- | --- |", 1)
		}, "opencode"},
		"an empty id": {func(s string) string {
			return strings.Replace(s, "`openai/gpt-6.1-terra#medium`", " ", 1)
		}, "mid"},
		"a duplicate tier row": {func(s string) string {
			row := "| mid | `sonnet` | `pi-claude-cli/claude-sonnet-5` | `openai/gpt-6.1-terra#medium` |\n"
			return strings.Replace(s, row, row+row, 1)
		}, "duplicate"},
		"no separator row": {func(s string) string {
			return strings.Replace(s, "| --- | --- | --- | --- |\n", "", 1)
		}, "separator"},
		"a duplicate runtime column": {func(s string) string {
			s = strings.Replace(s, "| tier | claude | pi | opencode |", "| tier | claude | pi | pi |", 1)
			return s
		}, "duplicate"},
		"an id with a space": {func(s string) string {
			return strings.Replace(s, "`openai/gpt-6.1-terra#medium`", "`openai/gpt 6#medium`", 1)
		}, "mid"},
		"the table outside its heading": {func(s string) string {
			return strings.Replace(s, "## Tier table", "## Ids", 1)
		}, "Tier table"},
		"no table at all": {func(s string) string { return s[:strings.Index(s, "## Tier table")] }, "table"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			root := seededRoot(t)
			editModels(t, root, c.edit)

			for verb, run := range map[string]func(string) error{"Generate": agentgen.Generate, "Check": agentgen.Check, "ReadModelTable": func(r string) error { _, err := agentgen.ReadModelTable(r); return err }} {
				if err := run(root); err == nil || !strings.Contains(err.Error(), c.want) {
					t.Errorf("%s error = %v, want one naming %q", verb, err, c.want)
				}
			}
			if _, err := os.Stat(filepath.Join(root, "agents")); err == nil {
				t.Error("Generate wrote files although the table was refused")
			}
		})
	}
}

// A table of the same shape outside the `## Tier table` section is not the tier table.
func TestOnlyTheTableUnderItsHeadingIsRead(t *testing.T) {
	decoy := "Earlier prose.\n\n| tier | claude | pi | opencode |\n| --- | --- | --- | --- |\n| frontier | `bad` | `bad` | `bad` |\n| mid | `bad` | `bad` | `bad` |\n| light | `bad` | `bad` | `bad` |\n\n"
	root := seededRoot(t)
	editModels(t, root, func(s string) string { return strings.Replace(s, "## Tier table", decoy+"## Tier table", 1) })

	table, err := agentgen.ReadModelTable(root)
	if err != nil {
		t.Fatalf("ReadModelTable: %v", err)
	}
	if got, _ := table.ID("frontier", "claude"); got != "opus" {
		t.Errorf("ID(frontier, claude) = %q, want opus: the table before the heading was read", got)
	}
}

// A table that cannot be used is a problem of parsing it, not of reading the file.
func TestAMalformedTableIsReportedAsParsingNotReading(t *testing.T) {
	root := seededRoot(t)
	editModels(t, root, func(s string) string { return strings.Replace(s, "| --- | --- | --- | --- |\n", "", 1) })

	err := agentgen.Generate(root)

	if err == nil || !strings.HasPrefix(err.Error(), "parsing "+modelsRel+":") {
		t.Errorf("Generate error = %v, want one that starts with \"parsing %s:\"", err, modelsRel)
	}
}

// A table that cannot be used does not hide a role source that cannot be read: Check names both. The roles
// whose sources were read have nothing to report of their own, since the table is why they have no files.
func TestAMalformedTableAndAMissingSourceAreReportedTogether(t *testing.T) {
	root := seededRoot(t)
	editModels(t, root, func(s string) string { return strings.Replace(s, "| --- | --- | --- | --- |\n", "", 1) })
	if err := os.Remove(filepath.Join(root, "roles", "builder", "body.md")); err != nil {
		t.Fatal(err)
	}

	// In this order: Generate first, so that a Generate that wrote files would change what Check sees.
	for _, run := range []struct {
		verb string
		fn   func(string) error
	}{{"Generate", agentgen.Generate}, {"Check", agentgen.Check}} {
		err := run.fn(root)
		if err == nil {
			t.Errorf("%s returned nil, want an error", run.verb)
			continue
		}
		// Both problems are in the one error, and no role whose source was read is named.
		for _, want := range []string{"roles/builder/body.md", modelsRel, "separator"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("%s error = %v, want it to name %q", run.verb, err, want)
			}
		}
		for _, r := range wantRoles {
			if r.name != "builder" && strings.Contains(err.Error(), "roles/"+r.name+"/") {
				t.Errorf("%s error = %v, names %s, whose source was read", run.verb, err, r.name)
			}
		}
	}
	// Every directory the generator writes a role file into (Files: agents/, pi/agents/, codex/agents/).
	for _, dir := range []string{"agents", "pi", "codex"} {
		if _, err := os.Stat(filepath.Join(root, dir)); err == nil {
			t.Errorf("Generate wrote %s/ although the table was refused", dir)
		}
	}
}

// The heading is found whatever its line ending or trailing space, as the rows are.
func TestTheHeadingAndRowsAreReadWithWindowsLineEndingsAndTrailingSpace(t *testing.T) {
	for name, edit := range map[string]func(string) string{
		"CRLF":           func(s string) string { return strings.ReplaceAll(s, "\n", "\r\n") },
		"trailing space": func(s string) string { return strings.Replace(s, "## Tier table\n", "## Tier table \t\n", 1) },
	} {
		t.Run(name, func(t *testing.T) {
			root := seededRoot(t)
			editModels(t, root, edit)

			table, err := agentgen.ReadModelTable(root)
			if err != nil {
				t.Fatalf("ReadModelTable: %v", err)
			}
			if got, _ := table.ID("light", "opencode"); got != "openai/gpt-6-luna#low" {
				t.Errorf("ID(light, opencode) = %q", got)
			}
		})
	}
}

// A header row cut short is a table that is refused, not a panic.
func TestAHeaderRowWithoutItsClosingPipeIsRefusedNotAPanic(t *testing.T) {
	root := seededRoot(t)
	editModels(t, root, func(s string) string {
		return strings.Replace(s, "| tier | claude | pi | opencode |", "| tier | claude", 1)
	})

	if _, err := agentgen.ReadModelTable(root); err == nil || !strings.Contains(err.Error(), "separator") {
		t.Errorf("ReadModelTable error = %v, want one that names the separator row", err)
	}
}

// The section ends at the next heading: a table under a later heading is not the tier table.
func TestATableUnderALaterHeadingIsNotTheTierTable(t *testing.T) {
	root := seededRoot(t)
	editModels(t, root, func(s string) string {
		return strings.Replace(s, "## Tier table\n", "## Tier table\n\nNo table here.\n\n## Other\n", 1)
	})

	if _, err := agentgen.ReadModelTable(root); err == nil || !strings.Contains(err.Error(), "Tier table") {
		t.Errorf("ReadModelTable error = %v, want one that says no table is under the Tier table heading", err)
	}
}

// The model of a generated file is the table's, not a constant of the generator: another id in the
// table reaches the Claude and Pi files of exactly the roles of that tier, and the Codex files, which
// carry no model, do not change.
func TestTheModelsOfTheFilesAreReadFromTheTable(t *testing.T) {
	root := seededRoot(t)
	before := artifactsByPath(t, root)
	editModels(t, root, func(s string) string {
		s = strings.Replace(s, "| mid | `sonnet` |", "| mid | `sonnet-next` |", 1)
		return strings.Replace(s, "| `opus` | `pi-claude-cli/claude-opus-5` |", "| `opus` | `pi-claude-cli/claude-opus-next` |", 1)
	})
	after := artifactsByPath(t, root)

	for _, r := range wantRoles {
		claude, pi, codex := "agents/"+r.name+".md", "pi/agents/"+r.name+".md", "codex/agents/"+r.name+".toml"
		if after[codex] != before[codex] {
			t.Errorf("%s: the Codex file changed with the table", r.name)
		}
		wantClaude, wantPi := before[claude], before[pi]
		switch r.claudeModel {
		case "sonnet":
			wantClaude = strings.Replace(wantClaude, "model: sonnet\n", "model: sonnet-next\n", 1)
		case "opus":
			wantPi = strings.Replace(wantPi, "claude-opus-5\n", "claude-opus-next\n", 1)
		}
		if after[claude] != wantClaude {
			t.Errorf("%s: the Claude file is\n%s\nwant\n%s", r.name, after[claude], wantClaude)
		}
		if after[pi] != wantPi {
			t.Errorf("%s: the Pi file is\n%s\nwant\n%s", r.name, after[pi], wantPi)
		}
	}
}

// An id with brackets or an at sign (`opus[1m]`, `model@20250929`) is a real model id and is written
// verbatim after `model: `, where YAML reads it as one plain scalar, so the table accepts it.
func TestAnIdWithBracketsOrAnAtSignIsReadAndWrittenVerbatim(t *testing.T) {
	root := seededRoot(t)
	editModels(t, root, func(s string) string {
		s = strings.Replace(s, "| frontier | `opus` |", "| frontier | `opus[1m]` |", 1)
		return strings.Replace(s, "| mid | `sonnet` | `pi-claude-cli/claude-sonnet-5` |", "| mid | `sonnet` | `pi-claude-cli/model@20250929` |", 1)
	})

	table, err := agentgen.ReadModelTable(root)
	if err != nil {
		t.Fatalf("ReadModelTable: %v", err)
	}
	if got, _ := table.ID("frontier", "claude"); got != "opus[1m]" {
		t.Errorf("ID(frontier, claude) = %q", got)
	}
	files := artifactsByPath(t, root)
	if !strings.Contains(files["agents/shaper.md"], "\nmodel: opus[1m]\n") {
		t.Errorf("the Claude file of the shaper does not carry the id verbatim:\n%s", files["agents/shaper.md"])
	}
	if !strings.Contains(files["pi/agents/builder.md"], "\nmodel: pi-claude-cli/model@20250929\n") {
		t.Errorf("the Pi file of the builder does not carry the id verbatim:\n%s", files["pi/agents/builder.md"])
	}
}

func artifactsByPath(t *testing.T, root string) map[string]string {
	t.Helper()
	table, err := agentgen.Artifacts(root)
	if err != nil {
		t.Fatalf("Artifacts: %v", err)
	}
	out := map[string]string{}
	for _, a := range table {
		out[a.Path] = a.Content
	}
	return out
}
