package classifierguard_test

import (
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/classifierguard"
)

const script = `sh "` + classifierguard.ScriptPath + `" `

func bash(cmd string) classifierguard.Call {
	return classifierguard.Call{Tool: "Bash", Root: "/r", Command: cmd}
}

func write(tool, path string) classifierguard.Call {
	return classifierguard.Call{Tool: tool, Root: "/r", FilePath: path}
}

func read(path string) classifierguard.Call {
	return classifierguard.Call{Tool: "Read", Root: "/r", FilePath: path}
}

// The classifier may read under its root, write its one page and run its one script in four forms on its own
// wiki; everything else is blocked.
func TestDecideAllowsOnlyTheClassifiersOwnCalls(t *testing.T) {
	allowed := []classifierguard.Call{
		read("/r/odd/review-results/x.json"),
		read("/r"),
		{Tool: "Grep", Root: "/r", Command: "whatever"},
		{Tool: "Grep", Root: "/r", Path: "/r/docs"},
		{Tool: "Glob", Root: "/r"},
		{Tool: "Glob", Root: "/r", Pattern: "docs/**/*.md"},
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
		{"page of another root", write("Write", "/s/docs/wiki/evidence.md"), classifierguard.ReasonPage},
		{"readme", write("Edit", "/r/README.md"), classifierguard.ReasonPage},
		{"git config", write("Write", "/r/.git/config"), classifierguard.ReasonPage},
		{"MultiEdit", write("MultiEdit", "/r/docs/wiki/evidence.md"), classifierguard.ReasonTool},
		{"NotebookEdit", classifierguard.Call{Tool: "NotebookEdit", Root: "/r", NotebookPath: "/r/docs/wiki/evidence.md"}, classifierguard.ReasonTool},
		{"Agent", classifierguard.Call{Tool: "Agent", Root: "/r"}, classifierguard.ReasonTool},
		{"mcp tool", classifierguard.Call{Tool: "mcp__memorycard__mem_save", Root: "/r"}, classifierguard.ReasonTool},
		{"empty tool", classifierguard.Call{Root: "/r"}, classifierguard.ReasonTool},
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
		{"wiki of another root", bash(script + `check "/s/docs/wiki"`), classifierguard.ReasonCommand},
		{"root holding a quote runs no script", classifierguard.Call{Tool: "Bash", Root: `/r"x`, Command: script + `check "/r"x/docs/wiki"`}, classifierguard.ReasonCommand},
		{"read outside the root", read("/etc/passwd"), classifierguard.ReasonRead},
		{"read dot-dot", read("/r/../s/x"), classifierguard.ReasonRead},
		{"read of a sibling prefix", read("/rs/x"), classifierguard.ReasonRead},
		{"read relative", read("docs/x"), classifierguard.ReasonRead},
		{"grep outside", classifierguard.Call{Tool: "Grep", Root: "/r", Path: "/home"}, classifierguard.ReasonRead},
		{"grep dot-dot", classifierguard.Call{Tool: "Grep", Root: "/r", Path: "/r/../x"}, classifierguard.ReasonRead},
		{"glob path /", classifierguard.Call{Tool: "Glob", Root: "/r", Path: "/"}, classifierguard.ReasonRead},
		{"glob absolute pattern", classifierguard.Call{Tool: "Glob", Root: "/r", Pattern: "/etc/*"}, classifierguard.ReasonRead},
		{"glob home pattern", classifierguard.Call{Tool: "Glob", Root: "/r", Pattern: "~/x"}, classifierguard.ReasonRead},
		{"glob dollar pattern", classifierguard.Call{Tool: "Glob", Root: "/r", Pattern: "$HOME/x"}, classifierguard.ReasonRead},
		{"glob leading dot-dot", classifierguard.Call{Tool: "Glob", Root: "/r", Pattern: "../x"}, classifierguard.ReasonRead},
		{"glob inner dot-dot", classifierguard.Call{Tool: "Glob", Root: "/r", Pattern: "**/../x"}, classifierguard.ReasonRead},
	}
	for _, root := range []string{"", "docs", "/r/", "/", "/r/../r", "/r\n"} {
		blocked = append(blocked,
			struct {
				name string
				call classifierguard.Call
				want string
			}{"read, root " + root, classifierguard.Call{Tool: "Read", Root: root, FilePath: "/r/x"}, classifierguard.ReasonRoot},
			struct {
				name string
				call classifierguard.Call
				want string
			}{"script, root " + root, classifierguard.Call{Tool: "Bash", Root: root, Command: ok}, classifierguard.ReasonRoot},
		)
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
		"root": classifierguard.ReasonRoot, "read": classifierguard.ReasonRead, "page": classifierguard.ReasonPage,
		"command": classifierguard.ReasonCommand, "tool": classifierguard.ReasonTool,
	} {
		if reason == "" || strings.Contains(reason, "\n") {
			t.Errorf("%s reason is empty or has a newline: %q", name, reason)
		}
		for _, want := range []string{"labdrian classifier guard", "speed bump", "working directory", "Read, Grep and Glob", classifierguard.PageSuffix} {
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
