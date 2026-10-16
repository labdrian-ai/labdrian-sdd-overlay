package gadu_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gadu"
)

// Artifacts is the one table of what the generator emits: where each file goes under the
// repository root and what it holds. Generate writes it and Check compares with it, and they are
// the same four files in the same order.
func TestArtifactsAreTheFourFilesInTheOrderTheyAreWrittenAndReported(t *testing.T) {
	got := gadu.Artifacts()

	if len(got) != len(artifactPaths) {
		t.Fatalf("%d artifacts, want %d", len(got), len(artifactPaths))
	}
	for i, p := range artifactPaths {
		if got[i].Path != p {
			t.Errorf("artifact %d is %q, want %q", i, got[i].Path, p)
		}
		if got[i].Content == "" {
			t.Errorf("%s has no content", p)
		}
	}
}

func TestGenerateWritesExactlyWhatTheTableHolds(t *testing.T) {
	root := t.TempDir()
	if err := gadu.Generate(root); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	for _, a := range gadu.Artifacts() {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(a.Path)))
		if err != nil || string(data) != a.Content {
			t.Errorf("%s on disk is not the table's content (err %v)", a.Path, err)
		}
	}
}

// The table is made for each caller: changing the slice one call returns is not seen by the next.
func TestArtifactsHandsOutATableNoCallerCanChangeForTheNext(t *testing.T) {
	first := gadu.Artifacts()
	first[0].Path, first[0].Content = "changed", "changed"

	second := gadu.Artifacts()

	if second[0].Path != artifactPaths[0] || second[0].Content == "changed" {
		t.Errorf("the second call returned %q, which the first caller changed", second[0].Path)
	}
}

// Check reads the files that are committed and compares them with the table, in memory. It writes
// nothing, not even a scratch directory: it holds with no place to write one.
func TestCheckWritesNothingAnywhere(t *testing.T) {
	root := t.TempDir()
	if err := gadu.Generate(root); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	t.Setenv("TMPDIR", filepath.Join(root, "no-such-scratch-directory"))

	if err := gadu.Check(root); err != nil {
		t.Errorf("Check: %v", err)
	}

	before := listing(t, root)
	gadu.Check(root)
	if after := listing(t, root); after != before {
		t.Errorf("Check changed the files under the root:\nbefore\n%s\nafter\n%s", before, after)
	}
	if _, err := os.Stat(filepath.Join(root, "no-such-scratch-directory")); err == nil {
		t.Error("Check made a scratch directory")
	}
}

// listing is the paths, sizes and modification times under dir, one per line.
func listing(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		b.WriteString(path + " " + info.ModTime().String() + "\n")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// The model each agent file declares is a named constant, the way PiAgentModel always was, and the
// frontmatter says it.
func TestTheModelIdsAreNamedAndEachAgentFileDeclaresItsOwn(t *testing.T) {
	if gadu.ClaudeAgentModel != "opus" || gadu.OpenCodeAgentModel != "openai/gpt-5.5" || gadu.PiAgentModel != "pi-claude-cli/claude-sonnet-5" {
		t.Errorf("model ids = %q, %q, %q", gadu.ClaudeAgentModel, gadu.OpenCodeAgentModel, gadu.PiAgentModel)
	}
	declared := map[string]string{}
	for _, a := range gadu.Artifacts() {
		declared[a.Path] = extractFrontmatter(a.Content)["model"]
	}
	want := map[string]string{
		"agents/GADU.md":                gadu.ClaudeAgentModel,
		"opencode/agents/GADU.md":       gadu.OpenCodeAgentModel,
		"pi/agents/GADU.md":             gadu.PiAgentModel,
		"skills/gadu-operator/SKILL.md": "",
	}
	for path, model := range want {
		if declared[path] != model {
			t.Errorf("%s declares model %q, want %q", path, declared[path], model)
		}
	}
}

// The three agent files say the same thing about who GADU is: one description.
func TestTheThreeAgentFilesCarryTheSameDescription(t *testing.T) {
	var descriptions []string
	for _, a := range gadu.Artifacts() {
		if strings.HasSuffix(a.Path, "agents/GADU.md") {
			descriptions = append(descriptions, extractFrontmatter(a.Content)["description"])
		}
	}
	if len(descriptions) != 3 || descriptions[0] == "" || descriptions[0] != descriptions[1] || descriptions[1] != descriptions[2] {
		t.Errorf("descriptions = %q, want one non-empty text three times", descriptions)
	}
}
