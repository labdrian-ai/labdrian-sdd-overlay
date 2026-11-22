// Package classifierguard decides which tool calls the classifier agent may make. The classifier is the one
// agent of the loop that writes, and its input is text copied from code under review, so what it may do is a
// closed list that a pure function can decide before each call: read with Read, Grep and Glob; write one file,
// the evidence page; run one script, in four forms, on one wiki directory. The package has no IO and imports
// nothing from the engine; the verb that adapts it to Claude Code is engine/cmd's.
//
// It matches text and is a speed bump, not a security boundary. It does not follow symbolic links, so a
// docs/wiki that links elsewhere is not caught.
package classifierguard

import (
	"path"
	"regexp"
	"strings"
)

// ScriptPath is the one copy of the path of the evidence script, as the classifier's body writes it.
const ScriptPath = "$HOME/.claude/skills/_shared/evidence/evidence.sh"

// PageSuffix is what the path of the one file the classifier writes ends with.
const PageSuffix = "/docs/wiki/evidence.md"

// wikiSuffix is what the directory argument of the script ends with.
const wikiSuffix = "/docs/wiki"

// Call is a tool call as the guard reads it: the tool's name and the three fields of its input that a guard
// reads.
type Call struct{ Tool, Command, FilePath, NotebookPath string }

// Verdict is the answer: whether to block the call, and why.
type Verdict struct {
	Block  bool
	Reason string
}

// allowed says, in one line, what the classifier may do. Every reason ends with it.
const allowed = `Allowed: Read, Grep, Glob; Edit and Write on one absolute path ending in ` + PageSuffix +
	`; Bash only as sh "` + ScriptPath + `" check|count|index "<root>/docs/wiki" or sh "` + ScriptPath +
	`" rule <kind> "<root>/docs/wiki" with nothing before or after. This guard matches text and is a speed bump, not a security boundary.`

// The three reasons a call is blocked, each one line.
const (
	// ReasonPage blocks Edit and Write on any path but the page.
	ReasonPage = "labdrian classifier guard: the classifier writes only the evidence page. " + allowed
	// ReasonCommand blocks every Bash command but the script's four forms.
	ReasonCommand = "labdrian classifier guard: the classifier runs only the evidence script. " + allowed
	// ReasonTool blocks every tool the classifier does not use.
	ReasonTool = "labdrian classifier guard: the classifier does not use this tool. " + allowed
)

// command is the whole of an allowed Bash command: nothing before `sh`, nothing after the closing quote. The
// quoted argument holds no quote, dollar sign, backtick, backslash or control character, so nothing in it expands.
var command = regexp.MustCompile(`^sh "` + regexp.QuoteMeta(ScriptPath) + `" ` +
	`(?:check|count|index|rule [a-z0-9]+(?:-[a-z0-9]+)*) ` +
	`"([^"$` + "`" + `\\\x00-\x1f]*)"$`)

// Decide judges one tool call of the classifier.
func Decide(c Call) Verdict {
	switch c.Tool {
	case "Read", "Grep", "Glob":
		return Verdict{}
	case "Edit", "Write":
		if isPage(c.FilePath) {
			return Verdict{}
		}
		return Verdict{Block: true, Reason: ReasonPage}
	case "Bash":
		if isScriptCall(c.Command) {
			return Verdict{}
		}
		return Verdict{Block: true, Reason: ReasonCommand}
	}
	return Verdict{Block: true, Reason: ReasonTool}
}

// isPage reports whether p is a clean absolute path, with a root, that ends in PageSuffix.
func isPage(p string) bool {
	return strings.HasPrefix(p, "/") && path.Clean(p) == p && strings.HasSuffix(p, PageSuffix) && len(p) > len(PageSuffix)
}

// isScriptCall reports whether cmd is one of the four forms of the script on a clean absolute <root>/docs/wiki.
func isScriptCall(cmd string) bool {
	m := command.FindStringSubmatch(cmd)
	if m == nil {
		return false
	}
	dir := m[1]
	return strings.HasPrefix(dir, "/") && path.Clean(dir) == dir && strings.HasSuffix(dir, wikiSuffix) && len(dir) > len(wikiSuffix)
}
