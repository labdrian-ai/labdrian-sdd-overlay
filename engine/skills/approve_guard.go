package skills

// The approve guard: the pure decision of whether an agent's tool call may go
// ahead, made for the Claude Code PreToolUse hook `skills guard-hook` (see
// engine/cmd). `skills approve` records that a HUMAN reviewed the exact bytes of
// a skill, so the guard denies the agent running it, and denies the agent
// writing the record file by hand.
//
// THE GUARD IS A SPEED BUMP, NOT A SECURITY BOUNDARY. It matches the text of a
// Bash command and the base name of a file tool's path. It is not a shell
// parser and it does not see what a command does, so it can be bypassed (an
// alias under another name, a variable that holds the entry point, a script
// written to a file and run, an encoded command, a Bash redirection into the
// record) and it can deny text that only mentions the verb. What it does is
// stop an agent from approving a skill in passing, and put in front of it, in
// the tool result, that approval is a human step. The record's real guarantee
// is unchanged and is tested elsewhere: it matches the exact bytes of the skill
// it sits next to.
//
// Choices, each pinned by a test:
//   - The invocation is the token sequence "<entry point> skills approve", where
//     the entry point is labdrian, labdrian-overlay, or gentle-ai-overlay by
//     base name (so any path prefix, quoting, or variable path ends up the
//     same). It is found anywhere in the command text, so sh -c, xargs, sudo,
//     env, time, and a cd in front are all caught without modeling them. The
//     verb must be exactly "approve": skills approved and skills approve-all
//     are not it.
//   - A command is cut into segments at ; & | ( ), a backtick, and a new
//     line, without regard to quoting, and quotes and backslashes are dropped
//     from each word, as the shell would when it glues 'sk''ills' together.
//   - A segment whose command is grep, egrep, fgrep, or rg is exempt, because
//     the entry point there is a search pattern, not an invocation, and an agent
//     searching the repository for the hint text is common. rg is not exempt
//     when the segment carries --pre or --hostname-bin (bare or with =value),
//     the two rg flags that make it run a program. Only those exact spellings
//     count: --pretty and --pre-glob are ordinary rg flags.
//   - Nothing else is exempt. echo and printf are not, because their output can
//     be piped to a shell; git and gh are not, so a commit message or a pull
//     request body that spells the entry point followed by "skills approve" is
//     denied, and so is a heredoc line that does. Reword the text, or leave the
//     entry point out.
//   - Input the hook cannot decode is an allow, and input over the bound the
//     hook reads is a denial (ApproveGuardOversized). The hook runs on every
//     Bash and file-edit call, and a guard that blocked what it could not
//     decode would block the session; but the agent controls the length of its
//     own command, and a guard that padding could switch off would guard
//     nothing, so what is too large to be judged is denied, as the shaper
//     clearance guard denies every call it cannot read. What the guard decides
//     on is a call (ApproveGuardCall); reading the hook input into one, and
//     answering for what cannot be read, is the hook adapter's
//     (engine/hookwire, engine/cmd).

import (
	"fmt"
	"path"
	"strings"
)

// approveGuardEntryPoints are the base names of the executables a session would
// use to run `skills approve`: the labdrian alias, the overlay script, and the
// engine binary.
var approveGuardEntryPoints = map[string]bool{
	"labdrian":          true,
	"labdrian-overlay":  true,
	"gentle-ai-overlay": true,
}

// approveGuardReaders are the commands whose arguments are a search pattern, so
// an entry point followed by "skills approve" in them is text, not an
// invocation.
var approveGuardReaders = map[string]bool{
	"rg":    true,
	"grep":  true,
	"egrep": true,
	"fgrep": true,
}

// approveGuardFileToolList is the one list of file tools the guard watches for a
// write to the approval record. The settings matcher that routes those tools to
// the hook retypes it; a test checks the copy.
var approveGuardFileToolList = []string{"Write", "Edit", "MultiEdit", "NotebookEdit"}

// ApproveGuardFileTools returns the file tools the guard watches, in a stable
// order. The result is a copy: editing it never changes the guard.
func ApproveGuardFileTools() []string {
	return append([]string(nil), approveGuardFileToolList...)
}

// ApproveGuardVerdict is the guard's decision for one hook input. Reason is set
// only for a denial and is a single line.
type ApproveGuardVerdict struct {
	Deny   bool
	Reason string
}

const approveGuardSpeedBump = "This guard matches text only, so it is a speed bump, not a security boundary"

// approveGuardBashReason and approveGuardFileReason explain a denial to the
// agent. They name the verb, say that approval is a human step, and give the
// human the exact command to run themselves.
var (
	approveGuardBashReason = "labdrian skills approve guard: \"skills approve\" records that a human reviewed the exact SKILL.md bytes, " +
		"so the agent must not run it. Ask the person to review skills/<id>/SKILL.md and run this themselves in a terminal: " +
		approveHint("<id>") + ". " + approveGuardSpeedBump + ": it matches the command text, so it can be bypassed, " +
		"and it also denies a command that only spells an entry point followed by \"skills approve\" (reword such a command)."

	approveGuardFileReason = "labdrian skills approve guard: " + ApprovalRecordName + " is the approval record that \"skills approve\" writes " +
		"for a human who reviewed the exact SKILL.md bytes, so the agent must not write it by hand. Ask the person to review " +
		"skills/<id>/SKILL.md and run this themselves in a terminal: " + approveHint("<id>") + ". " + approveGuardSpeedBump +
		": it matches the tool and the file name, so a shell command can still write the file."
)

// ApproveGuardOversized is the verdict for a call the hook did not judge because it is longer than
// the bound bytes the hook reads. It is a denial, in one line like the others: it names the verb,
// the bound, and what to do. The agent controls the length of its own command, so a call that
// size cannot be allowed unchecked.
func ApproveGuardOversized(bound int) ApproveGuardVerdict {
	return ApproveGuardVerdict{Deny: true, Reason: fmt.Sprintf("labdrian skills approve guard: this tool call is too large to be checked "+
		"(the guard reads at most %d bytes), so it could not be checked for \"skills approve\" or for a write of the approval record %s, "+
		"and it was denied. Send a smaller call, or ask the person to run what it does themselves in a terminal. %s.",
		bound, ApprovalRecordName, approveGuardSpeedBump)}
}

// ApproveGuardCall is the tool call the guard decides about: the name of the tool, the
// command of a shell tool, and the path fields of the file tools. A field the call does not
// have is empty. What is written to a file is not part of it.
type ApproveGuardCall struct {
	// Tool is the name of the tool, which decides which of the fields is read.
	Tool string
	// Command is the command of the shell tool.
	Command string
	// FilePath is the path of the file tools that write a file.
	FilePath string
	// NotebookPath is the path of the notebook tool.
	NotebookPath string
}

// DecideApproveGuard decides one tool call. It denies a Bash call whose command invokes
// `skills approve` (ApproveGuardCommandMatches) and a file-tool call whose path is an approval
// record (ApproveGuardRecordPathMatches). Everything else is a silent allow. File contents are
// never inspected, so writing documentation that mentions the verb is allowed, and reading the
// record is not a file-edit call at all.
func DecideApproveGuard(call ApproveGuardCall) ApproveGuardVerdict {
	switch {
	case call.Tool == "Bash":
		if ApproveGuardCommandMatches(call.Command) {
			return ApproveGuardVerdict{Deny: true, Reason: approveGuardBashReason}
		}
	case isApproveGuardFileTool(call.Tool):
		if ApproveGuardRecordPathMatches(call.FilePath) || ApproveGuardRecordPathMatches(call.NotebookPath) {
			return ApproveGuardVerdict{Deny: true, Reason: approveGuardFileReason}
		}
	}
	return ApproveGuardVerdict{}
}

func isApproveGuardFileTool(name string) bool {
	for _, t := range approveGuardFileToolList {
		if name == t {
			return true
		}
	}
	return false
}

// ApproveGuardRecordPathMatches reports whether p names an approval record: its
// base name is ApprovalRecordName, compared without regard to case (a
// case-insensitive filesystem reaches the same file) and with either kind of
// path separator. Only the base name is read, so the same name anywhere else is
// denied too; an empty path is not a match.
func ApproveGuardRecordPathMatches(p string) bool {
	if p == "" {
		return false
	}
	return strings.EqualFold(path.Base(strings.ReplaceAll(p, `\`, "/")), ApprovalRecordName)
}

// ApproveGuardCommandMatches reports whether a Bash command invokes
// `skills approve` through one of the approve guard's entry points. See the
// package comment above for the rule, its exemption, and its limits: it reads
// text, it is not a shell parser, and it can be bypassed.
func ApproveGuardCommandMatches(command string) bool {
	text := strings.ReplaceAll(command, "\\\n", " ")
	for _, segment := range strings.FieldsFunc(text, isApproveGuardSeparator) {
		words := approveGuardWords(segment)
		if approveGuardIsReader(words) {
			continue
		}
		for i := 0; i+2 < len(words); i++ {
			if approveGuardEntryPoints[path.Base(words[i])] && words[i+1] == "skills" && words[i+2] == "approve" {
				return true
			}
		}
	}
	return false
}

// isApproveGuardSeparator reports the characters that end a command segment. It
// ignores quoting on purpose: a separator inside quotes splits the segment too,
// which can only add a denial, never hide one.
func isApproveGuardSeparator(r rune) bool {
	switch r {
	case ';', '&', '|', '(', ')', '`', '\n', '\r':
		return true
	}
	return false
}

// approveGuardWords splits a segment into words the way the shell would before
// it strips quotes: on white space, with quote and backslash characters removed
// from each word.
func approveGuardWords(segment string) []string {
	words := strings.Fields(segment)
	out := words[:0]
	for _, w := range words {
		w = strings.Map(func(r rune) rune {
			if r == '\'' || r == '"' || r == '\\' {
				return -1
			}
			return r
		}, w)
		if w != "" {
			out = append(out, w)
		}
	}
	return out
}

// approveGuardIsReader reports whether a segment runs one of the search
// commands, after any NAME=value assignments, so its words are a pattern and not
// an invocation. A segment that hands rg a program to run (--pre or
// --hostname-bin, see approveGuardCommandFlag) is not exempt.
func approveGuardIsReader(words []string) bool {
	i := 0
	for i < len(words) && isEnvAssignment(words[i]) {
		i++
	}
	if i >= len(words) || !approveGuardReaders[path.Base(words[i])] {
		return false
	}
	for _, w := range words[i+1:] {
		if approveGuardCommandFlag(w) {
			return false
		}
	}
	return true
}

// approveGuardCommandFlag reports whether a word is one of the rg flags that
// name a program for rg to run: --pre COMMAND (a preprocessor, run once per
// file) and --hostname-bin COMMAND (run to learn the host name for hyperlinks),
// in either the separate-value or the --flag=value spelling. rg does not accept
// abbreviated flags, so an exact match on the flag name is the whole rule;
// --pretty and --pre-glob, which only start with the same letters, do not run
// anything.
func approveGuardCommandFlag(word string) bool {
	name, _, _ := strings.Cut(word, "=")
	return name == "--pre" || name == "--hostname-bin"
}

// isEnvAssignment reports whether a word has the shape NAME=value.
func isEnvAssignment(w string) bool {
	eq := strings.IndexByte(w, '=')
	if eq <= 0 {
		return false
	}
	for i, r := range w[:eq] {
		switch {
		case r == '_', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}
