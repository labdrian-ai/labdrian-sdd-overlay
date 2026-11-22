package classifierguard_test

import (
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/classifierguard"
)

const script = `sh "` + classifierguard.ScriptPath + `" `

func bash(cmd string) classifierguard.Call {
	return classifierguard.Call{Tool: "Bash", Command: cmd}
}

func write(tool, path string) classifierguard.Call {
	return classifierguard.Call{Tool: tool, FilePath: path}
}

// The classifier may read, write its one page and run its one script in four forms; everything else is blocked.
func TestDecideAllowsOnlyTheClassifiersOwnCalls(t *testing.T) {
	allowed := []classifierguard.Call{
		{Tool: "Read", FilePath: "/anything"},
		{Tool: "Grep", Command: "whatever"},
		{Tool: "Glob"},
		write("Write", "/r/docs/wiki/evidence.md"),
		write("Edit", "/r/docs/wiki/evidence.md"),
		bash(script + `check "/r/docs/wiki"`),
		bash(script + `count "/r/docs/wiki"`),
		bash(script + `index "/r/docs/wiki"`),
		bash(script + `rule test-passes-when-broken "/r/docs/wiki"`),
	}
	for _, c := range allowed {
		if v := classifierguard.Decide(c); v.Block {
			t.Errorf("Decide(%+v) blocked: %s", c, v.Reason)
		}
	}
	ok := script + `check "/r/docs/wiki"`
	blocked := []struct {
		name string
		call classifierguard.Call
		want string
	}{
		{"backup of the page", write("Write", "/r/docs/wiki/evidence.md.bak"), classifierguard.ReasonPage},
		{"dot-dot in the page", write("Write", "/r/docs/wiki/../wiki/evidence.md"), classifierguard.ReasonPage},
		{"dot-dot before the page", write("Write", "/r/../docs/wiki/evidence.md"), classifierguard.ReasonPage},
		{"relative page", write("Write", "docs/wiki/evidence.md"), classifierguard.ReasonPage},
		{"no root", write("Write", "/docs/wiki/evidence.md"), classifierguard.ReasonPage},
		{"readme", write("Edit", "/r/README.md"), classifierguard.ReasonPage},
		{"git config", write("Write", "/r/.git/config"), classifierguard.ReasonPage},
		{"MultiEdit", write("MultiEdit", "/r/docs/wiki/evidence.md"), classifierguard.ReasonTool},
		{"NotebookEdit", classifierguard.Call{Tool: "NotebookEdit", NotebookPath: "/r/docs/wiki/evidence.md"}, classifierguard.ReasonTool},
		{"Agent", classifierguard.Call{Tool: "Agent"}, classifierguard.ReasonTool},
		{"mcp tool", classifierguard.Call{Tool: "mcp__memorycard__mem_save"}, classifierguard.ReasonTool},
		{"empty tool", classifierguard.Call{}, classifierguard.ReasonTool},
		{"git commit", bash("git commit -m x"), classifierguard.ReasonCommand},
		{"semicolon", bash(ok + "; git push"), classifierguard.ReasonCommand},
		{"and", bash(ok + " && rm -rf /r"), classifierguard.ReasonCommand},
		{"redirect", bash(ok + " > /r/x"), classifierguard.ReasonCommand},
		{"stderr redirect", bash(ok + " 2>&1"), classifierguard.ReasonCommand},
		{"second line", bash(ok + "\ngit push"), classifierguard.ReasonCommand},
		{"env prefix", bash("HOME=/x " + ok), classifierguard.ReasonCommand},
		{"rules mode", bash(script + `rules "/r/docs/wiki"`), classifierguard.ReasonCommand},
		{"bad kind", bash(script + `rule Bad_Kind "/r/docs/wiki"`), classifierguard.ReasonCommand},
		{"substitution", bash(script + `check "/r/$(id)/docs/wiki"`), classifierguard.ReasonCommand},
		{"backtick", bash(script + "check \"/r/`id`/docs/wiki\""), classifierguard.ReasonCommand},
		{"dot-dot root", bash(script + `check "/r/../docs/wiki"`), classifierguard.ReasonCommand},
		{"quote in root", bash(script + `check "/r/a"b/docs/wiki"`), classifierguard.ReasonCommand},
		{"tilde script", bash(`sh ~/.claude/skills/_shared/evidence/evidence.sh check "/r/docs/wiki"`), classifierguard.ReasonCommand},
		{"no wiki argument", bash(script + "check"), classifierguard.ReasonCommand},
	}
	for _, tc := range blocked {
		v := classifierguard.Decide(tc.call)
		if !v.Block {
			t.Errorf("%s: Decide(%+v) allowed", tc.name, tc.call)
			continue
		}
		if v.Reason != tc.want {
			t.Errorf("%s: reason = %q, want %q", tc.name, v.Reason, tc.want)
		}
	}
}

// Each reason is one line, names what is allowed and says it is a speed bump.
func TestEveryReasonIsOneLineAndNamesWhatIsAllowed(t *testing.T) {
	for name, reason := range map[string]string{
		"page": classifierguard.ReasonPage, "command": classifierguard.ReasonCommand, "tool": classifierguard.ReasonTool,
	} {
		if reason == "" || strings.Contains(reason, "\n") {
			t.Errorf("%s reason is empty or has a newline: %q", name, reason)
		}
		for _, want := range []string{"labdrian classifier guard", "speed bump", "Read, Grep, Glob", classifierguard.PageSuffix} {
			if !strings.Contains(reason, want) {
				t.Errorf("%s reason lacks %q: %q", name, want, reason)
			}
		}
	}
	if !strings.Contains(classifierguard.ReasonCommand, classifierguard.ScriptPath) {
		t.Errorf("the command reason does not quote the script path: %q", classifierguard.ReasonCommand)
	}
	for _, mode := range []string{"check", "count", "index", "rule <kind>"} {
		if !strings.Contains(classifierguard.ReasonCommand, mode) {
			t.Errorf("the command reason lacks the mode %q", mode)
		}
	}
}
