package settings_test

import (
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings"
)

// The codegraph 1.2.0 installer writes a UserPromptSubmit hook whose command is exactly
// `codegraph prompt-hook`. Merge replaces that command, in place, with the labdrian wrapper (S19, L87),
// and Remove writes the bare command back. Nothing else of the settings changes.

const (
	bareCodegraph    = "codegraph prompt-hook"
	wrappedCodegraph = "command -v " + documentBinary + " >/dev/null 2>&1 || exec codegraph prompt-hook; exec " + documentBinary + " codegraph-prompt-hook"
)

// codegraphSettings is a settings file with the codegraph entry the installer writes, plus a foreign
// entry on each side of it, as a machine that has run /codegraph has.
func codegraphSettings(command string) string {
	return `{"hooks":{"UserPromptSubmit":[` +
		`{"hooks":[{"type":"command","command":"first-tool"}]},` +
		`{"matcher":"*","hooks":[{"type":"command","command":"` + command + `","timeout":30,"statusMessage":"cg"}]},` +
		`{"hooks":[{"type":"command","command":"last-tool"}]}]}}`
}

func userPromptCommands(t *testing.T, doc settings.Document) []string {
	t.Helper()
	hooks, _ := doc.Root()["hooks"].(map[string]interface{})
	entries, _ := hooks["UserPromptSubmit"].([]interface{})
	var out []string
	for _, e := range entries {
		inner, _ := e.(map[string]interface{})["hooks"].([]interface{})
		for _, h := range inner {
			c, _ := h.(map[string]interface{})["command"].(string)
			out = append(out, c)
		}
	}
	return out
}

func TestMergeWrapsTheBareCodegraphPromptHookInPlace(t *testing.T) {
	doc := mustParse(t, codegraphSettings(bareCodegraph))
	if !mustMerge(t, &doc) {
		t.Fatal("Merge reported no change")
	}
	cmds := userPromptCommands(t, doc)
	if len(cmds) < 3 || cmds[0] != "first-tool" || cmds[1] != wrappedCodegraph {
		t.Fatalf("commands = %q; want first-tool, then the wrapper, in place", cmds)
	}
	for _, c := range cmds {
		if c == bareCodegraph {
			t.Errorf("commands = %q; the bare command must be gone", cmds)
		}
	}
	// every other field of the hook object and of its entry is kept
	got := mustBytes(t, doc)
	for _, keep := range []string{`"matcher": "*"`, `"timeout": 30`, `"statusMessage": "cg"`, `"command": "last-tool"`} {
		if !strings.Contains(got, keep) {
			t.Errorf("the merged settings lost %s:\n%s", keep, got)
		}
	}
}

func TestMergeQuotesTheBinaryPathInTheWrapper(t *testing.T) {
	doc := mustParse(t, codegraphSettings(bareCodegraph))
	if _, err := doc.Merge("/opt/my dir/bin"); err != nil {
		t.Fatal(err)
	}
	want := "command -v '/opt/my dir/bin' >/dev/null 2>&1 || exec codegraph prompt-hook; exec '/opt/my dir/bin' codegraph-prompt-hook"
	var found bool
	for _, c := range userPromptCommands(t, doc) {
		found = found || c == want
	}
	if !found {
		t.Errorf("commands = %q; want %q", userPromptCommands(t, doc), want)
	}
}

func TestMergeTwiceKeepsTheWrapperAndChangesNothing(t *testing.T) {
	doc := mustParse(t, codegraphSettings(bareCodegraph))
	mustMerge(t, &doc)
	first := mustBytes(t, doc)
	if mustMerge(t, &doc) {
		t.Error("the second Merge reported a change")
	}
	if got := mustBytes(t, doc); got != first {
		t.Errorf("the second Merge changed the bytes:\n%s\nwant\n%s", got, first)
	}
}

func TestMergeLeavesAnEntryThatOnlyContainsTheCommandAlone(t *testing.T) {
	for _, command := range []string{"codegraph prompt-hook --verbose", "env X=1 codegraph prompt-hook", "codegraph  prompt-hook", wrappedCodegraph} {
		doc := mustParse(t, codegraphSettings(command))
		mustMerge(t, &doc)
		var found bool
		for _, c := range userPromptCommands(t, doc) {
			found = found || c == command
		}
		if !found {
			t.Errorf("command %q was changed: %q", command, userPromptCommands(t, doc))
		}
	}
}

func TestMergeRemovesABareEntryBesideTheWrapper(t *testing.T) {
	doc := mustParse(t, `{"hooks":{"UserPromptSubmit":[`+
		`{"hooks":[{"type":"command","command":"`+wrappedCodegraph+`"}]},`+
		`{"hooks":[{"type":"command","command":"`+bareCodegraph+`"}]},`+
		`{"hooks":[{"type":"command","command":"last-tool"}]}]}}`)
	if !mustMerge(t, &doc) {
		t.Fatal("Merge reported no change")
	}
	var wrapped, bare int
	for _, c := range userPromptCommands(t, doc) {
		switch c {
		case wrappedCodegraph:
			wrapped++
		case bareCodegraph:
			bare++
		}
	}
	if wrapped != 1 || bare != 0 {
		t.Errorf("wrapper entries = %d, bare entries = %d, want 1 and 0 (codegraph runs once per prompt): %q", wrapped, bare, userPromptCommands(t, doc))
	}
}

func TestSettingsWithoutACodegraphEntryKeepTheirOtherCommands(t *testing.T) {
	// with no codegraph entry the step has nothing to replace: install and uninstall leave the other
	// UserPromptSubmit commands as they were.
	const in = `{"hooks":{"UserPromptSubmit":[{"hooks":[{"type":"command","command":"first-tool"}]}]}}`
	doc := mustParse(t, in)
	mustMerge(t, &doc)
	if cmds := userPromptCommands(t, doc); cmds[0] != "first-tool" {
		t.Errorf("commands = %q; the foreign entry must stay first and unchanged", cmds)
	}
	for _, c := range userPromptCommands(t, doc) {
		if strings.Contains(c, "codegraph") {
			t.Errorf("an install invented a codegraph command: %q", c)
		}
	}
	if _, err := doc.Remove(documentBinary); err != nil {
		t.Fatal(err)
	}
	if got := mustBytes(t, doc); !strings.Contains(got, `"command": "first-tool"`) || strings.Contains(got, "codegraph") {
		t.Errorf("after Remove:\n%s", got)
	}
}

func TestRemoveGivesTheBareCommandBackInPlaceWithItsOtherFields(t *testing.T) {
	original := mustParse(t, codegraphSettings(bareCodegraph))
	doc := mustParse(t, codegraphSettings(bareCodegraph))
	mustMerge(t, &doc)
	changed, err := doc.Remove(documentBinary)
	if err != nil || !changed {
		t.Fatalf("Remove = %v, %v; want a change", changed, err)
	}
	if got, want := userPromptCommands(t, doc), userPromptCommands(t, original); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("commands after the round trip = %q, want %q", got, want)
	}
	for _, keep := range []string{`"matcher": "*"`, `"timeout": 30`, `"statusMessage": "cg"`} {
		if !strings.Contains(mustBytes(t, doc), keep) {
			t.Errorf("the round trip lost %s:\n%s", keep, mustBytes(t, doc))
		}
	}
}

func TestRemoveLeavesTheBareCommandAlone(t *testing.T) {
	doc := mustParse(t, codegraphSettings(bareCodegraph))
	before := mustBytes(t, doc)
	if changed, err := doc.Remove(documentBinary); err != nil || changed {
		t.Errorf("Remove = %v, %v; want no change", changed, err)
	}
	if got := mustBytes(t, doc); got != before {
		t.Errorf("Remove changed the bytes:\n%s", got)
	}
}

func TestCodegraphPromptStateNamesBareWrappedAndAbsent(t *testing.T) {
	cases := []struct {
		name string
		text string
		want settings.CodegraphPromptState
	}{
		{"absent", `{"hooks":{"UserPromptSubmit":[{"hooks":[{"type":"command","command":"x"}]}]}}`, settings.CodegraphPromptAbsent},
		{"no hooks", `{}`, settings.CodegraphPromptAbsent},
		{"bare", codegraphSettings(bareCodegraph), settings.CodegraphPromptBare},
		{"wrapped", codegraphSettings(wrappedCodegraph), settings.CodegraphPromptWrapped},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := settings.CodegraphPrompt(mustParse(t, c.text).Root(), documentBinary); got != c.want {
				t.Errorf("CodegraphPrompt = %v, want %v", got, c.want)
			}
		})
	}
}

func TestCodegraphPromptReplacedNoticeNamesTheFileAndTheCommands(t *testing.T) {
	got := settings.CodegraphPromptReplacedNotice("/h/.claude/settings.json")
	for _, want := range []string{"/h/.claude/settings.json", "codegraph prompt-hook", "task notifications", "install-hooks"} {
		if !strings.Contains(got, want) {
			t.Errorf("notice %q does not mention %q", got, want)
		}
	}
}

// The lifecycle state `engine runtime status --target claude` answers from is the families', so a
// codegraph entry, bare or wrapped, never changes it.
func TestTheCodegraphStepDoesNotChangeTheSupportedLifecycleAnswer(t *testing.T) {
	for _, text := range []string{`{}`, codegraphSettings(bareCodegraph)} {
		doc := mustParse(t, text)
		if settings.HasSupportedClaudeLifecycleState(doc.Root(), documentBinary) {
			t.Fatalf("settings %s are supported before the install", text)
		}
		mustMerge(t, &doc)
		if !settings.HasSupportedClaudeLifecycleState(doc.Root(), documentBinary) {
			t.Errorf("settings %s are not supported after the install", text)
		}
		if _, err := doc.Remove(documentBinary); err != nil {
			t.Fatal(err)
		}
		if settings.HasSupportedClaudeLifecycleState(doc.Root(), documentBinary) {
			t.Errorf("settings %s are still supported after the uninstall", text)
		}
	}
}
