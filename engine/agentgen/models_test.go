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
