// Package classifierguard decides which tool calls the classifier agent may make. The classifier is the one
// agent of the loop that writes, and its input is text copied from code under review, so what it may do is a
// closed list that a pure function can decide before each call, against one root: the working directory of the
// agent. It reads with Read, Grep and Glob, only within the root; it writes one file, the evidence page, at
// <root>/docs/wiki/evidence.md; and it runs one script, in four forms, on <root>/docs/wiki. An unknown or unclean
// root blocks every call. The package has no IO and imports nothing from the engine; the verb that adapts it to
// Claude Code is engine/cmd's, and it takes the root from the hook input's cwd.
//
// It matches text and is a speed bump, not a security boundary. It does not follow symbolic links, so a path under
// the root that links elsewhere is not caught, and a file under the root that holds a secret can still be read.
package classifierguard

import (
	"path"
	"regexp"
	"strings"
)

// ScriptPath is the one copy of the path of the evidence script, as the classifier's body writes it.
const ScriptPath = "$HOME/.claude/skills/_shared/evidence/evidence.sh"

// PageSuffix is what the path of the one file the classifier writes ends with, after the root.
const PageSuffix = "/docs/wiki/evidence.md"

// wikiSuffix is what the directory argument of the script ends with, after the root.
const wikiSuffix = "/docs/wiki"

// Call is a tool call as the guard reads it: the tool's name, the root (the working directory of the agent that
// makes the call) and the fields of its input that a guard reads.
type Call struct{ Tool, Root, Command, FilePath, NotebookPath, Path, Pattern string }

// Verdict is the answer: whether to block the call, and why.
type Verdict struct {
	Block  bool
	Reason string
}

// allowed says, in one line, what the classifier may do. Every reason ends with it.
const allowed = `Allowed: the root is the classifier's working directory; Read, Grep and Glob within it; Edit and Write only on <root>` +
	PageSuffix + `; Bash only as sh "` + ScriptPath + `" check|count|index "<root>/docs/wiki" or sh "` + ScriptPath +
	`" rule <kind> "<root>/docs/wiki" with nothing before or after. This guard matches text and is a speed bump, not a security boundary.`

// The five reasons a call is blocked, each one line.
const (
	// ReasonRoot blocks every call when the working directory is not a clean absolute path.
	ReasonRoot = "labdrian classifier guard: the working directory of the classifier is not a clean absolute path, so no call is allowed. " + allowed
	// ReasonRead blocks Read, Grep and Glob outside the root.
	ReasonRead = "labdrian classifier guard: the classifier reads only within its working directory. " + allowed
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
	if !cleanRoot(c.Root) {
		return Verdict{Block: true, Reason: ReasonRoot}
	}
	switch c.Tool {
	case "Read":
		return readVerdict(within(c.Root, c.FilePath))
	case "Grep":
		return readVerdict(c.Path == "" || within(c.Root, c.Path))
	case "Glob":
		return readVerdict((c.Path == "" || within(c.Root, c.Path)) && safePattern(c.Pattern))
	case "Edit", "Write":
		if c.FilePath == c.Root+PageSuffix {
			return Verdict{}
		}
		return Verdict{Block: true, Reason: ReasonPage}
	case "Bash":
		if m := command.FindStringSubmatch(c.Command); m != nil && m[1] == c.Root+wikiSuffix {
			return Verdict{}
		}
		return Verdict{Block: true, Reason: ReasonCommand}
	}
	return Verdict{Block: true, Reason: ReasonTool}
}

func readVerdict(ok bool) Verdict {
	if ok {
		return Verdict{}
	}
	return Verdict{Block: true, Reason: ReasonRead}
}

// cleanRoot reports whether root is an absolute, clean path that is not "/" and holds no control character.
func cleanRoot(root string) bool {
	if !strings.HasPrefix(root, "/") || root == "/" || path.Clean(root) != root {
		return false
	}
	return !strings.ContainsFunc(root, func(r rune) bool { return r < 0x20 || r == 0x7f })
}

// within reports whether p is a clean absolute path that is root or lies under it.
func within(root, p string) bool {
	return strings.HasPrefix(p, "/") && path.Clean(p) == p && (p == root || strings.HasPrefix(p, root+"/"))
}

// safePattern reports whether a Glob pattern stays relative to the searched directory: it does not start with
// "/", "~" or "$" and holds no "..". Whether Glob honours an absolute pattern is unverified, so one is refused.
func safePattern(p string) bool {
	return !strings.HasPrefix(p, "/") && !strings.HasPrefix(p, "~") && !strings.HasPrefix(p, "$") && !strings.Contains(p, "..")
}
