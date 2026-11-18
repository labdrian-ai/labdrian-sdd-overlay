package runtime_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/roles"
	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
)

// repositoryRoot is the overlay checkout the tests of this package run in, two levels above engine/runtime.
func repositoryRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(wd, "..", "..")
}

// roleEntries are the skill entries (SKILL.md files) of the installed config whose included phases are
// exactly [role], by contract path, with how many there are of each.
func roleEntries(t *testing.T, config map[string]any, role string) map[string]int {
	t.Helper()
	out := map[string]int{}
	for _, raw := range config["prompt_config"].(map[string]any)["contracts"].([]any) {
		entry := raw.(map[string]any)
		phases := entry["included_phases"].([]any)
		if path := entry["contract_path"].(string); len(phases) == 1 && phases[0] == role && strings.HasSuffix(path, "/SKILL.md") {
			out[path]++
		}
	}
	return out
}

// The roles of OpenCode have no skill tool, so every skill a role body names as "`<id>` skill" has to
// reach the role as a path. The bodies read here are this repository's own, never a list kept in the
// test: a body that gains a mention fails this test until the adapter injects it.
func TestEveryIdEachRoleBodyNamesHasExactlyOneEntryForThatRoleAndItsFileExists(t *testing.T) {
	root := t.TempDir()
	if result := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{OverlayDir: repositoryRoot(t)}).Install(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}
	config := readOpenCodeConfig(t, root)
	dirs, err := os.ReadDir(filepath.Join(repositoryRoot(t), "roles"))
	if err != nil {
		t.Fatal(err)
	}
	named := 0
	for _, dir := range dirs {
		if !dir.IsDir() || dir.Name() == "foreman" {
			continue
		}
		body, err := os.ReadFile(filepath.Join(repositoryRoot(t), filepath.FromSlash(roles.BodyPath(roles.Role(dir.Name())))))
		if err != nil {
			t.Fatalf("read the body of %s: %v", dir.Name(), err)
		}
		got := roleEntries(t, config, dir.Name())
		ids := roles.SkillsNamed(string(body))
		for _, id := range ids {
			named++
			path := filepath.Join(repositoryRoot(t), "skills", id, "SKILL.md")
			if got[path] != 1 {
				t.Errorf("the body of %s names the `%s` skill: %d entries for %s, want exactly 1 (entries of the role: %v)", dir.Name(), id, got[path], path, got)
			}
			if _, err := os.Stat(path); err != nil {
				t.Errorf("the entry for %s does not point at a file: %v", id, err)
			}
		}
		if len(got) != len(ids) {
			t.Errorf("%s has entries %v, want exactly one per id its body names: %v", dir.Name(), got, ids)
		}
	}
	if named == 0 {
		t.Fatal("no role body names a skill: the test reads nothing")
	}
}

// A builder launch gets the paths of the skills its body names under the launch header, in a node run of
// the installed plugin; a reviewer launch, whose body names none, gets none.
func TestABuilderLaunchGetsItsSkillPathsAndAReviewerLaunchGetsNone(t *testing.T) {
	requireNode(t)
	root := t.TempDir()
	if result := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{OverlayDir: repositoryRoot(t)}).Install(); result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}
	stub := filepath.Join(root, "node_modules", "@opencode", "plugin")
	if err := os.MkdirAll(stub, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range stubPluginPackage {
		if err := os.WriteFile(filepath.Join(stub, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	script := `const builder = { tool: "subagent", input: { prompt: "work\n## Skills to load before work\n", agent: "builder" } };
const reviewer = { tool: "subagent", input: { prompt: "work\n## Skills to load before work\n", agent: "reviewer" } };
await beforeHook(builder);
let reviewerError = "";
try { await beforeHook(reviewer); } catch (error) { reviewerError = error.message; }
console.log(JSON.stringify({ builder: builder.input.prompt, reviewer: reviewer.input.prompt, reviewerError }));`
	// The installed config names a skills root, so each launch goes through `roles skill-paths`: the stand-in
	// prints the prompt back, which a prompt with no ids under the header is printed as.
	engine := newFakeEngine(t, map[string][]byte{skillsEvent: []byte("work\n## Skills to load before work\n")})
	var got struct{ Builder, Reviewer, ReviewerError string }
	if err := json.Unmarshal(runPluginWithEngine(t, openCodePluginPath(root), engine.path, script), &got); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"test-first", "odd-feature-doc"} {
		want := filepath.Join(repositoryRoot(t), "skills", id, "SKILL.md")
		if !strings.Contains(got.Builder, "\n"+want+"\n") {
			t.Errorf("the builder prompt lacks the line %s:\n%s", want, got.Builder)
		}
	}
	if strings.Contains(got.Reviewer, "SKILL.md") {
		t.Errorf("the reviewer prompt names a skill file (error %q):\n%s", got.ReviewerError, got.Reviewer)
	}
}
