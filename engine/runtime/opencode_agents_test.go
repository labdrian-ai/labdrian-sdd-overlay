package runtime_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	engineRuntime "github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/runtime/core"
)

// The tests below pin <config>/AGENTS.md of OpenCode (S13): one marked block built from the owner
// rules, the body of the labdrian output style and the memorycard block, and the report of labdrian
// skills found where OpenCode would read them before ~/.claude/skills.

const (
	agentsOpen  = "<!-- labdrian:opencode-instructions -->"
	agentsClose = "<!-- /labdrian:opencode-instructions -->"
)

func agentsPath(root string) string { return filepath.Join(root, "AGENTS.md") }

func readText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func writeText(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// overlayWith is a checkout that holds the contracts the plugin needs, the two instruction sources
// as given (an empty text leaves the file out), and a skills/<id>/SKILL.md for each skill id.
func overlayWith(t *testing.T, ownerRules, style string, skillIDs ...string) string {
	t.Helper()
	overlay := t.TempDir()
	for _, name := range []string{"minimalism-contract.md", "oo-quality-contract.md"} {
		writeText(t, filepath.Join(overlay, "skills", "_shared", name), readText(t, filepath.Join(shippedOverlay(t), "skills", "_shared", name)))
	}
	if ownerRules != "" {
		writeText(t, filepath.Join(overlay, "instructions", "owner-rules.md"), ownerRules)
	}
	if style != "" {
		writeText(t, filepath.Join(overlay, "output-styles", "labdrian.md"), style)
	}
	for _, id := range skillIDs {
		writeText(t, filepath.Join(overlay, "skills", id, "SKILL.md"), "# "+id+"\n")
	}
	return overlay
}

const (
	fakeRules = "## Rules\n\n- Rule one.\n- Rule two.\n"
	fakeStyle = "---\nname: labdrian\ndescription: Terse\nkeep-coding-instructions: true\n---\n\n# labdrian Output Style\n\n## Replies\n- Lead with the answer.\n"
)

func TestOpenCodeInstallWritesAGENTSFromTheRulesTheStyleBodyAndTheMemorycardBlock(t *testing.T) {
	root := t.TempDir()
	overlay := overlayWith(t, fakeRules, fakeStyle)
	adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{OverlayDir: overlay})

	result := adapter.Install()

	if result.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v", result)
	}
	text := readText(t, agentsPath(root))
	if !strings.HasPrefix(text, agentsOpen+"\n") || !strings.HasSuffix(text, agentsClose+"\n") {
		t.Fatalf("AGENTS.md is not one block between the markers:\n%s", text)
	}
	rules := strings.Index(text, "## Rules\n\n- Rule one.\n- Rule two.\n")
	style := strings.Index(text, "# labdrian Output Style\n\n## Replies\n- Lead with the answer.\n")
	memory := strings.Index(text, "## memorycard is the memory store")
	if rules < 0 || style < rules || memory < style {
		t.Errorf("want the rules, then the style body, then the memorycard block; at %d, %d, %d:\n%s", rules, style, memory, text)
	}
	for _, leaked := range []string{"name: labdrian", "keep-coding-instructions", "---"} {
		if strings.Contains(text, leaked) {
			t.Errorf("AGENTS.md holds %q: the style goes in without its frontmatter", leaked)
		}
	}
	if !strings.Contains(result.Message, agentsPath(root)) {
		t.Errorf("message = %q, want it to name %s", result.Message, agentsPath(root))
	}
}

// The shipped sources, not a fixture: the block holds the owner rules byte for byte and the style
// from its heading on, and nothing of the style's frontmatter or of gentle-ai.
func TestOpenCodeAGENTSOfTheShippedOverlayHoldsTheOwnerRulesAndTheStyleBody(t *testing.T) {
	root := t.TempDir()
	adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{OverlayDir: shippedOverlay(t)})
	adapter.Install()

	text := readText(t, agentsPath(root))
	rules := strings.TrimRight(readText(t, filepath.Join(shippedOverlay(t), "instructions", "owner-rules.md")), "\n")
	style := readText(t, filepath.Join(shippedOverlay(t), "output-styles", "labdrian.md"))
	body := strings.TrimRight(style[strings.Index(style, "# labdrian Output Style"):], "\n")
	if !strings.Contains(text, rules+"\n\n"+body+"\n\n## memorycard is the memory store") {
		t.Errorf("AGENTS.md is not the owner rules, the style body and the memorycard block in a row:\n%s", text)
	}
	for _, banned := range []string{"gentle-ai", "keep-coding-instructions"} {
		if strings.Contains(text, banned) {
			t.Errorf("AGENTS.md holds %q", banned)
		}
	}
}

func TestOpenCodeMemorycardBlockNamesTheToolsAndNeverTheOldStore(t *testing.T) {
	root := t.TempDir()
	adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{OverlayDir: overlayWith(t, fakeRules, fakeStyle)})
	adapter.Install()

	text := readText(t, agentsPath(root))
	start := strings.Index(text, "## memorycard is the memory store")
	if start < 0 {
		t.Fatalf("AGENTS.md has no memorycard block:\n%s", text)
	}
	block := text[start:]
	for _, want := range []string{"memorycard", "`mem_save`", "`mem_search`", "`mem_get`", "`mem_update`", "`topic_key`", "`supersedes`", "is the only memory store"} {
		if !strings.Contains(block, want) {
			t.Errorf("the memorycard block does not say %s:\n%s", want, block)
		}
	}
	// The owner approved the block without any mention of the retired store (L83).
	if strings.Contains(strings.ToLower(block), "engram") {
		t.Errorf("the memorycard block names Engram:\n%s", block)
	}
	for _, banned := range []string{"gentle-ai", "mem_capture_passive", "mem_get_observation", "mem_session_summary"} {
		if strings.Contains(block, banned) {
			t.Errorf("the memorycard block names %s, which is not a memorycard tool", banned)
		}
	}
}

func TestOpenCodeAGENTSKeepsEveryByteOutsideTheBlockAndReplacesItInPlace(t *testing.T) {
	root := t.TempDir()
	overlay := overlayWith(t, fakeRules, fakeStyle)
	writeText(t, agentsPath(root), "# Mine\n\nMy own rule.\n")
	adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{OverlayDir: overlay})
	adapter.Install()
	installed := readText(t, agentsPath(root))
	if !strings.HasPrefix(installed, "# Mine\n\nMy own rule.\n\n"+agentsOpen) {
		t.Fatalf("the owner's text must stay first, the block appended after a blank line:\n%s", installed)
	}
	writeText(t, agentsPath(root), installed+"\nText after the block.\n")
	writeText(t, filepath.Join(overlay, "instructions", "owner-rules.md"), "## Rules\n\n- Rule three.\n")

	result := adapter.Update()

	updated := readText(t, agentsPath(root))
	if !strings.HasPrefix(updated, "# Mine\n\nMy own rule.\n\n"+agentsOpen) || !strings.HasSuffix(updated, "\nText after the block.\n") {
		t.Errorf("text outside the block changed:\n%s", updated)
	}
	if !strings.Contains(updated, "- Rule three.") || strings.Contains(updated, "Rule one") || strings.Count(updated, agentsOpen) != 1 {
		t.Errorf("the block was not replaced in place:\n%s", updated)
	}
	if !strings.Contains(result.Message, "updated") {
		t.Errorf("message = %q, want it to say the block was updated", result.Message)
	}
}

func TestOpenCodeAGENTSIsNotRewrittenWhenCurrent(t *testing.T) {
	root := t.TempDir()
	adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{OverlayDir: overlayWith(t, fakeRules, fakeStyle)})
	adapter.Install()
	before, err := os.Stat(agentsPath(root))
	if err != nil {
		t.Fatalf("Install() left no AGENTS.md: %v", err)
	}

	result := adapter.Update()

	after, _ := os.Stat(agentsPath(root))
	if !before.ModTime().Equal(after.ModTime()) || !strings.Contains(result.Message, "AGENTS.md already current") {
		t.Errorf("a current AGENTS.md must not be written (mtimes %v, %v); message = %q", before.ModTime(), after.ModTime(), result.Message)
	}
}

func TestOpenCodeAGENTSWithBrokenMarkersIsRefusedAndKept(t *testing.T) {
	root := t.TempDir()
	broken := "Mine\n" + agentsOpen + "\nno close line\n"
	writeText(t, agentsPath(root), broken)
	adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{OverlayDir: overlayWith(t, fakeRules, fakeStyle)})

	result := adapter.Install()

	if result.Status != core.CapabilityPartial || !strings.Contains(result.Message, "AGENTS.md") {
		t.Errorf("Install() = %#v, want partial naming AGENTS.md", result)
	}
	if got := readText(t, agentsPath(root)); got != broken {
		t.Errorf("AGENTS.md changed to %q", got)
	}
	if _, err := os.Stat(openCodePluginPath(root)); err != nil {
		t.Errorf("the plugin is installed whatever happened to AGENTS.md: %v", err)
	}
}

func TestOpenCodeStatusIsPartialWhileAGENTSIsMissingOrStale(t *testing.T) {
	root := t.TempDir()
	overlay := overlayWith(t, fakeRules, fakeStyle)
	adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{OverlayDir: overlay})
	adapter.Install()
	writeMatchingActiveMarker(t, root)
	if status := adapter.Status(); status.Status != core.CapabilitySupported {
		t.Fatalf("Status() after install = %#v, want supported", status)
	}

	writeText(t, filepath.Join(overlay, "instructions", "owner-rules.md"), "## Rules\n\n- Changed.\n")
	stale := adapter.Status()
	if stale.Status != core.CapabilityPartial || !strings.Contains(stale.Message, "stale") || !strings.Contains(stale.Message, "engine runtime update --target opencode") {
		t.Errorf("Status() = %#v, want partial, stale, and the update verb", stale)
	}

	if err := os.Remove(agentsPath(root)); err != nil {
		t.Fatal(err)
	}
	missing := adapter.Status()
	if missing.Status != core.CapabilityPartial || !strings.Contains(missing.Message, "missing") || !strings.Contains(missing.Message, "engine runtime install --target opencode") {
		t.Errorf("Status() = %#v, want partial, missing, and the install verb", missing)
	}
}

func TestOpenCodeUninstallRemovesOnlyTheBlockAndGivesBackTheFile(t *testing.T) {
	root := t.TempDir()
	adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{OverlayDir: overlayWith(t, fakeRules, fakeStyle)})
	adapter.Install()
	adapter.Uninstall()
	if _, err := os.Stat(agentsPath(root)); !os.IsNotExist(err) {
		t.Errorf("AGENTS.md held only the block and should be gone, stat err: %v", err)
	}

	writeText(t, agentsPath(root), "# Mine\n\nMy own rule.\n")
	adapter.Install()
	adapter.Uninstall()
	if got := readText(t, agentsPath(root)); got != "# Mine\n\nMy own rule.\n" {
		t.Errorf("AGENTS.md = %q, want the owner's bytes back", got)
	}
}

func TestOpenCodeInstallOverACheckoutWithNoInstructionSourcesIsTheBaseResultWithANote(t *testing.T) {
	root := t.TempDir()
	adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{OverlayDir: overlayWith(t, "", "")})

	result := adapter.Install()

	if result.Status != core.CapabilityRestartRequired || !strings.Contains(result.Message, "AGENTS.md not written") {
		t.Errorf("Install() = %#v, want restart_required and a note that AGENTS.md was not written", result)
	}
	if _, err := os.Stat(agentsPath(root)); !os.IsNotExist(err) {
		t.Errorf("AGENTS.md was written from no source, stat err: %v", err)
	}
	writeMatchingActiveMarker(t, root)
	if status := adapter.Status(); status.Status != core.CapabilitySupported {
		t.Errorf("Status() = %#v, want the base result (supported) when no checkout holds the sources", status)
	}
}

func TestOpenCodeReportsLabdrianSkillsFoundWhereOpenCodeReadsThemFirst(t *testing.T) {
	root, agentsSkills := t.TempDir(), t.TempDir()
	overlay := overlayWith(t, fakeRules, fakeStyle, "role-chain", "odd-feature-doc")
	writeText(t, filepath.Join(root, "skills", "role-chain", "SKILL.md"), "copy")
	writeText(t, filepath.Join(root, "skills", "someone-elses", "SKILL.md"), "mine")
	writeText(t, filepath.Join(agentsSkills, "odd-feature-doc", "SKILL.md"), "copy")
	adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{OverlayDir: overlay, AgentsSkillsDir: agentsSkills})

	install := adapter.Install()

	if install.Status != core.CapabilityRestartRequired {
		t.Fatalf("Install() = %#v: a report is not a failure", install)
	}
	for _, want := range []string{filepath.Join(root, "skills") + ": role-chain", agentsSkills + ": odd-feature-doc"} {
		if !strings.Contains(install.Message, want) {
			t.Errorf("message = %q, want it to report %q", install.Message, want)
		}
	}
	if strings.Contains(install.Message, "someone-elses") {
		t.Errorf("message = %q, a skill that is not labdrian's is not reported", install.Message)
	}
	writeMatchingActiveMarker(t, root)
	if status := adapter.Status(); !strings.Contains(status.Message, "role-chain") {
		t.Errorf("Status() = %#v, want it to report the skills too", status)
	}
	for _, kept := range []string{filepath.Join(root, "skills", "role-chain", "SKILL.md"), filepath.Join(agentsSkills, "odd-feature-doc", "SKILL.md")} {
		if got := readText(t, kept); got != "copy" {
			t.Errorf("%s was changed to %q: the report touches nothing", kept, got)
		}
	}
}

func TestOpenCodeInstallWritesNoSkillAndSaysNothingWhenThereAreNoCopies(t *testing.T) {
	root := t.TempDir()
	adapter := engineRuntime.NewOpenCodeAdapter(root, engineRuntime.OpenCodeOptions{OverlayDir: overlayWith(t, fakeRules, fakeStyle, "role-chain"), AgentsSkillsDir: t.TempDir()})

	result := adapter.Install()

	if strings.Contains(result.Message, "role-chain") {
		t.Errorf("message = %q, nothing was found to report", result.Message)
	}
	if _, err := os.Stat(filepath.Join(root, "skills")); !os.IsNotExist(err) {
		t.Errorf("the adapter created %s, stat err: %v", filepath.Join(root, "skills"), err)
	}
}
