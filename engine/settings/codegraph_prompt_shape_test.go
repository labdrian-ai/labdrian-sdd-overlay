package settings_test

import (
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/settings"
)

// A wrapper is recognised by its shape, not by the path of the engine that installed it: the engine path
// can change between installs (T34 correction, finding 1).

const (
	oldPathWrapper    = "command -v /old/bin/engine >/dev/null 2>&1 || exec codegraph prompt-hook; exec /old/bin/engine codegraph-prompt-hook"
	oldQuotedWrapper  = "command -v '/old dir/it'\\''s' >/dev/null 2>&1 || exec codegraph prompt-hook; exec '/old dir/it'\\''s' codegraph-prompt-hook"
	mismatchedWrapper = "command -v /a/engine >/dev/null 2>&1 || exec codegraph prompt-hook; exec /b/engine codegraph-prompt-hook"
)

func countCommand(t *testing.T, doc settings.Document, command string) int {
	t.Helper()
	n := 0
	for _, c := range userPromptCommands(t, doc) {
		if c == command {
			n++
		}
	}
	return n
}

// codegraphCommands counts the commands that run codegraph, bare or wrapped.
func codegraphCommands(cmds []string) int {
	n := 0
	for _, c := range cmds {
		if strings.Contains(c, "codegraph") {
			n++
		}
	}
	return n
}

func TestInstallRewritesAnOldPathWrapperInPlaceToTheCurrentOne(t *testing.T) {
	for _, old := range []string{oldPathWrapper, oldQuotedWrapper} {
		doc := mustParse(t, codegraphSettings(old))
		if !mustMerge(t, &doc) {
			t.Fatalf("Merge reported no change over %q", old)
		}
		cmds := userPromptCommands(t, doc)
		if cmds[0] != "first-tool" || cmds[1] != wrappedCodegraph || cmds[2] != "last-tool" || codegraphCommands(cmds) != 1 {
			t.Errorf("commands = %q; want first-tool, the current wrapper in place, last-tool, one codegraph command", cmds)
		}
		if !strings.Contains(mustBytes(t, doc), `"timeout": 30`) {
			t.Error("the other fields of the entry were lost")
		}
	}
}

func TestInstallLeavesOneCurrentWrapperWhenAnOldWrapperAndABareEntryAreBothThere(t *testing.T) {
	doc := mustParse(t, `{"hooks":{"UserPromptSubmit":[`+
		`{"hooks":[{"type":"command","command":"`+oldPathWrapper+`"}]},`+
		`{"hooks":[{"type":"command","command":"`+bareCodegraph+`"}]}]}}`)
	mustMerge(t, &doc)
	if n := countCommand(t, doc, wrappedCodegraph); n != 1 || codegraphCommands(userPromptCommands(t, doc)) != 1 {
		t.Errorf("commands = %q; want exactly one current wrapper", userPromptCommands(t, doc))
	}
}

func TestUninstallRestoresTheBareCommandFromAnOldPathWrapper(t *testing.T) {
	for _, old := range []string{oldPathWrapper, oldQuotedWrapper} {
		doc := mustParse(t, codegraphSettings(old))
		if _, err := doc.Remove(documentBinary); err != nil {
			t.Fatal(err)
		}
		cmds := userPromptCommands(t, doc)
		if len(cmds) != 3 || cmds[1] != bareCodegraph {
			t.Errorf("commands = %q; want the bare command in place of %q", cmds, old)
		}
	}
}

func TestCodegraphPromptReadsAnOldPathWrapperAsWrapped(t *testing.T) {
	if got := settings.CodegraphPrompt(mustParse(t, codegraphSettings(oldPathWrapper)).Root()); got != settings.CodegraphPromptWrapped {
		t.Errorf("state = %v, want wrapped", got)
	}
}

func TestACommandThatOnlyLooksLikeAWrapperIsLeftAlone(t *testing.T) {
	for _, command := range []string{mismatchedWrapper, oldPathWrapper + " --x", "command -v  >/dev/null 2>&1 || exec codegraph prompt-hook; exec  codegraph-prompt-hook"} {
		doc := mustParse(t, codegraphSettings(command))
		mustMerge(t, &doc)
		if countCommand(t, doc, command) != 1 {
			t.Errorf("%q was changed by the install: %q", command, userPromptCommands(t, doc))
		}
		if got := settings.CodegraphPrompt(doc.Root()); got != settings.CodegraphPromptWrapped && got != settings.CodegraphPromptAbsent {
			t.Errorf("state = %v", got)
		}
		if _, err := doc.Remove(documentBinary); err != nil {
			t.Fatal(err)
		}
		if countCommand(t, doc, command) != 1 {
			t.Errorf("%q was changed by the uninstall: %q", command, userPromptCommands(t, doc))
		}
	}
}
