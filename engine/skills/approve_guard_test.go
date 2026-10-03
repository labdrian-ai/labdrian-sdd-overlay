package skills

import (
	"strings"
	"testing"
)

// The guard decides on a call: the name of the tool, the command of a shell tool and the paths of
// the file tools. How a hook input is read is engine/hookwire's, so these tests build the call
// itself, with the field a tool uses for its path.

// bashCall is a call of the shell tool with the command.
func bashCall(command string) ApproveGuardCall {
	return ApproveGuardCall{Tool: "Bash", Command: command}
}

// fileCall is a call of a file tool with the path in the field the tool uses for it.
func fileCall(tool, field, path string) ApproveGuardCall {
	call := ApproveGuardCall{Tool: tool}
	switch field {
	case "file_path":
		call.FilePath = path
	case "notebook_path":
		call.NotebookPath = path
	default:
		panic("test bug: no such path field " + field)
	}
	return call
}

// ---- the command rule --------------------------------------------------------

// Every entry point a session would use, however it is spelled, is denied when
// it is followed by "skills approve".
func TestApproveGuardCommand_DeniesTheApproveVerbThroughEveryEntryPoint(t *testing.T) {
	const verb = "skills approve --id my-skill --approver alice"
	for name, command := range map[string]string{
		"labdrian":                        "labdrian " + verb,
		"labdrian-overlay":                "labdrian-overlay " + verb,
		"gentle-ai-overlay":               "gentle-ai-overlay " + verb,
		"absolute path":                   "/home/u/.claude/bin/gentle-ai-overlay " + verb,
		"tilde path":                      "~/.claude/bin/gentle-ai-overlay " + verb,
		"relative path":                   "./bin/labdrian-overlay " + verb,
		"repo-relative path":              "bin/labdrian-overlay " + verb,
		"quoted variable path":            `"$HOME/.claude/bin/gentle-ai-overlay" ` + verb,
		"braced variable path":            "${HOME}/.claude/bin/gentle-ai-overlay " + verb,
		"no arguments":                    "labdrian skills approve",
		"extra spaces":                    "labdrian    skills   approve --id x",
		"tabs":                            "labdrian\tskills\t\tapprove --id x",
		"line continuation":               "labdrian skills \\\n  approve --id x",
		"after cd and &&":                 "cd repo && labdrian " + verb,
		"after cd and ;":                  "cd repo; labdrian " + verb,
		"after &&, no spaces":             "cd repo&&labdrian " + verb,
		"after ||":                        "false || labdrian " + verb,
		"after a pipe":                    "yes | labdrian " + verb,
		"on the second line":              "cd repo\nlabdrian " + verb,
		"inside sh -c, single quotes":     "sh -c 'labdrian " + verb + "'",
		"inside bash -lc, double quotes":  `bash -lc "gentle-ai-overlay ` + verb + `"`,
		"inside a subshell":               "(labdrian " + verb + ")",
		"inside a command substitution":   "out=$(labdrian " + verb + ")",
		"inside a backtick substitution":  "out=`labdrian " + verb + "`",
		"inside a brace group":            "{ labdrian " + verb + "; }",
		"after an env assignment":         "FOO=1 labdrian " + verb,
		"after env":                       "env labdrian " + verb,
		"after sudo":                      "sudo -u root labdrian " + verb,
		"after time":                      "time labdrian " + verb,
		"as an xargs argument":            "echo my-skill | xargs labdrian " + verb,
		"quoted words":                    `labdrian "skills" 'approve' --id x`,
		"quotes inside a word":            `lab""drian sk''ills approve --id x`,
		"backslash-escaped entry point":   `\labdrian ` + verb,
		"after a reader in the same line": "rg -n foo docs && labdrian " + verb,
		"after a reader and a semicolon":  "rg foo; labdrian " + verb,
		"as the rg preprocessor":          "rg --pre 'labdrian " + verb + "' pattern file",
	} {
		t.Run(name, func(t *testing.T) {
			if !ApproveGuardCommandMatches(command) {
				t.Errorf("command %q was not matched", command)
			}
		})
	}
}

// Nothing but the approve verb is denied: the other skills verbs, reading the
// record, and text that names the words without an entry point in front.
func TestApproveGuardCommand_AllowsEverythingElse(t *testing.T) {
	for name, command := range map[string]string{
		"empty":                       "",
		"another skills verb":         "labdrian skills validate --source-root skills",
		"skills add":                  "labdrian skills add my-skill",
		"skills lint":                 "gentle-ai-overlay skills lint draft/SKILL.md",
		"project verb":                "labdrian skills project-register --candidate x",
		"skills with no verb":         "labdrian skills",
		"the guard hook itself":       "gentle-ai-overlay skills guard-hook",
		"a verb that only starts so":  "labdrian skills approved",
		"a longer verb":               "labdrian skills approve-all",
		"another labdrian verb":       "labdrian install-hooks",
		"approve as another verb":     "labdrian shaper approve",
		"reading the record":          "bat skills/my-skill/.approval.json",
		"reading the record with cat": "cat skills/my-skill/.approval.json",
		"words without an entry":      "rg -n 'skills approve' engine docs",
		"echo of the words alone":     "echo skills approve",
		"a commit message":            `git commit -m "docs: describe skills approve"`,
		"an unrelated entry point":    "mytool skills approve --id x",
		"a lookalike entry point":     "labdrian-x skills approve --id x",
		"a prefixed lookalike":        "not-labdrian skills approve --id x",
		"approve without skills":      "labdrian approve --id x",
		"order reversed":              "labdrian approve skills",
		"skills on another line":      "labdrian\nskills approve --id x",
		"approve on another line":     "labdrian skills\napprove --id x",
		"ls":                          "ls -la",
		// A search pattern or a grep needle is text, not an invocation.
		"rg for the whole invocation":    "rg -n 'labdrian skills approve' README.md docs",
		"rg with the path first":         "rg 'gentle-ai-overlay skills approve --id' engine",
		"grep for the invocation":        `grep -rn "labdrian skills approve" .`,
		"egrep for the invocation":       "egrep 'labdrian skills approve|other' docs",
		"fgrep for the invocation":       "fgrep 'labdrian-overlay skills approve' README.md",
		"rg after an env assignment":     "LC_ALL=C rg 'labdrian skills approve' docs",
		"rg with a path prefix":          "/usr/bin/rg 'labdrian skills approve' docs",
		"rg at the end of a pipeline":    "cat notes.txt | rg 'labdrian skills approve'",
		"rg between other commands":      "cd repo && rg 'labdrian skills approve' docs | head -5",
		"rg on a later line":             "cd repo\nrg 'labdrian skills approve' docs",
		"rg with another flag before it": "rg --no-heading -n 'labdrian skills approve' docs",
		// Flags that merely start with the letters of a command-running flag are
		// not command-running flags: --pretty is a real rg output mode, and
		// --pre-glob only narrows the files a --pre command sees.
		"rg with --pretty":                   "rg --pretty 'labdrian skills approve' docs",
		"rg with --pre-glob":                 "rg --pre-glob '*.md' 'labdrian skills approve' docs",
		"rg with --pre-glob and a value":     "rg --pre-glob=*.md 'labdrian skills approve' docs",
		"rg with --no-pre":                   "rg --no-pre 'labdrian skills approve' docs",
		"rg with --hostname-bin-like prefix": "rg --hostname-binary 'labdrian skills approve' docs",
	} {
		t.Run(name, func(t *testing.T) {
			if ApproveGuardCommandMatches(command) {
				t.Errorf("command %q was matched, want it allowed", command)
			}
		})
	}
}

// A reader that is handed a program to run is not only reading: rg runs the
// program named by --pre (once per file) and by --hostname-bin. Each spelling of
// those two flags takes the segment out of the reader exemption, so the
// invocation text inside it is judged like any other segment's.
func TestApproveGuardCommand_ACommandRunningReaderFlagIsNotExempt(t *testing.T) {
	const needle = "labdrian skills approve"
	for name, command := range map[string]string{
		"--pre with a separate value":          "rg --pre ./prep.sh '" + needle + "' docs",
		"--pre=value":                          "rg --pre=./prep.sh '" + needle + "' docs",
		"--pre after the pattern":              "rg '" + needle + "' docs --pre ./prep.sh",
		"--pre with a quoted flag":             `rg "--pre=./prep.sh" '` + needle + `' docs`,
		"--hostname-bin with a separate value": "rg --hostname-bin ./host.sh '" + needle + "' docs",
		"--hostname-bin=value":                 "rg --hostname-bin=./host.sh '" + needle + "' docs",
		"--pre on a path-prefixed rg":          "/usr/bin/rg --pre=./prep.sh '" + needle + "' docs",
	} {
		t.Run(name, func(t *testing.T) {
			if !ApproveGuardCommandMatches(command) {
				t.Errorf("command %q was exempt as a plain search, want it judged as an invocation", command)
			}
		})
	}
}

// The rule matches the command text, not what the command does, so it errs on
// the side of denying text that names an invocation. These cases are pinned so
// that widening or narrowing the rule is a decision, not an accident. Each is
// reworded by leaving out the entry point in front of "skills approve".
func TestApproveGuardCommand_DocumentedFalseDenies(t *testing.T) {
	for name, command := range map[string]string{
		"echo of the invocation":   `echo "labdrian skills approve --id x"`,
		"printf of the invocation": `printf '%s\n' "run: labdrian skills approve"`,
		"a commit message":         `git commit -m "docs: run labdrian skills approve first"`,
		"a heredoc body line": "git commit -F - <<'EOF'\n" +
			"docs: explain approval\n" +
			"\n" +
			"labdrian skills approve --id x --approver you\n" +
			"EOF",
		"a pull request body": `gh pr create --body "Then run labdrian skills approve --id x."`,
		"a quoted pipe in rg": `rg 'a|labdrian skills approve' docs`,
	} {
		t.Run(name, func(t *testing.T) {
			if !ApproveGuardCommandMatches(command) {
				t.Errorf("command %q was not matched: if the rule now allows it, update the documented false denies", command)
			}
		})
	}
}

// The rule is not a shell parser. These bypasses are known, pinned so the claim
// "a speed bump, not a security boundary" stays checkable, and are named in the
// deny reason and the documentation.
func TestApproveGuardCommand_KnownBypasses(t *testing.T) {
	for name, command := range map[string]string{
		"an alias under another name": "myalias skills approve --id x",
		"variable indirection":        "X=labdrian; $X skills approve --id x",
		"a script written to a file":  "sh ./approve-it.sh",
		"an encoded command":          "echo bGFiZHJpYW4gc2tpbGxzIGFwcHJvdmU= | base64 -d | sh",
		"a computed argument":         `labdrian skills "$(echo appr)ove" --id x`,
		"go run from the engine":      "cd engine && go run ./cmd skills approve --id x",
		// A reader's own segment is exempt, so text it prints can still reach a shell.
		"a reader feeding a shell": "rg -o 'labdrian skills approve.*' plan.md | sh",
	} {
		t.Run(name, func(t *testing.T) {
			if ApproveGuardCommandMatches(command) {
				t.Errorf("command %q was matched: if the rule now catches it, update the documented bypasses", command)
			}
		})
	}
}

// ---- the record path rule ------------------------------------------------------

func TestApproveGuardRecordPath(t *testing.T) {
	for p, want := range map[string]bool{
		"skills/my-skill/.approval.json":            true,
		"/repo/skills/my-skill/.approval.json":      true,
		".approval.json":                            true,
		"/x/.APPROVAL.JSON":                         true,
		"/x/skills/y/.approval.json/":               true,
		`C:\repo\skills\my-skill\.approval.json`:    true,
		"":                                          false,
		"skills/my-skill/SKILL.md":                  false,
		"skills/my-skill/.approval.json.bak":        false,
		"skills/my-skill/approval.json":             false,
		"skills/my-skill/notes.approval.json":       false,
		"skills/.approval.json/child.md":            false,
		"skills/my-skill/references/.approval.json": true,
	} {
		if got := ApproveGuardRecordPathMatches(p); got != want {
			t.Errorf("ApproveGuardRecordPathMatches(%q) = %v, want %v", p, got, want)
		}
	}
	if ApprovalRecordName != ".approval.json" {
		t.Fatalf("test bug: ApprovalRecordName = %q; update the table above", ApprovalRecordName)
	}
}

func TestApproveGuardFileToolsAreTheFourEditTools(t *testing.T) {
	want := []string{"Write", "Edit", "MultiEdit", "NotebookEdit"}
	got := ApproveGuardFileTools()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("ApproveGuardFileTools() = %v, want %v", got, want)
	}
	got[0] = "Changed"
	if ApproveGuardFileTools()[0] != "Write" {
		t.Error("ApproveGuardFileTools returned the list it decides from; a caller could change the guard")
	}
}

// ---- the decision ---------------------------------------------------------------

func TestDecideApproveGuard_DeniesTheApproveVerbInABashCall(t *testing.T) {
	v := DecideApproveGuard(bashCall("cd repo && labdrian skills approve --id my-skill --approver alice"))
	if !v.Deny {
		t.Fatalf("verdict = %+v, want a denial", v)
	}
	for _, want := range []string{
		"skills approve",
		"human",
		"labdrian skills approve --id <id> --approver <name>",
		"speed bump",
		"not a security boundary",
		"bypass",
	} {
		if !strings.Contains(v.Reason, want) {
			t.Errorf("reason %q does not contain %q", v.Reason, want)
		}
	}
	if strings.ContainsAny(v.Reason, "\n\r") {
		t.Errorf("reason %q must be a single line", v.Reason)
	}
}

func TestDecideApproveGuard_DeniesWritingTheRecordWithAnyFileTool(t *testing.T) {
	for _, tc := range []struct {
		tool  string
		field string
	}{
		{"Write", "file_path"},
		{"Edit", "file_path"},
		{"MultiEdit", "file_path"},
		{"NotebookEdit", "notebook_path"},
	} {
		t.Run(tc.tool, func(t *testing.T) {
			v := DecideApproveGuard(fileCall(tc.tool, tc.field, "/repo/skills/my-skill/.approval.json"))
			if !v.Deny {
				t.Fatalf("verdict = %+v, want a denial", v)
			}
			for _, want := range []string{
				".approval.json",
				"human",
				"labdrian skills approve --id <id> --approver <name>",
				"speed bump",
				"not a security boundary",
			} {
				if !strings.Contains(v.Reason, want) {
					t.Errorf("reason %q does not contain %q", v.Reason, want)
				}
			}
		})
	}
}

// Either path of a file tool names the record, whatever the tool.
func TestDecideApproveGuard_ReadsBothPathsOfAFileTool(t *testing.T) {
	record := "/repo/skills/x/.approval.json"
	for name, call := range map[string]ApproveGuardCall{
		"the file path":     {Tool: "Write", FilePath: record},
		"the notebook path": {Tool: "Write", NotebookPath: record},
		"the notebook path beside a harmless file path": {Tool: "Edit", FilePath: "/repo/README.md", NotebookPath: record},
		"the file path beside a harmless notebook path": {Tool: "NotebookEdit", FilePath: record, NotebookPath: "/repo/n.ipynb"},
	} {
		if v := DecideApproveGuard(call); !v.Deny {
			t.Errorf("%s: verdict = %+v, want a denial", name, v)
		}
	}
}

func TestDecideApproveGuard_AllowsEverythingElseSilently(t *testing.T) {
	for name, call := range map[string]ApproveGuardCall{
		"a harmless Bash call":                              bashCall("ls -la"),
		"another skills verb":                               bashCall("labdrian skills validate --source-root skills"),
		"a Write to SKILL.md":                               {Tool: "Write", FilePath: "/repo/skills/x/SKILL.md"},
		"a Write of documentation about approve":            {Tool: "Write", FilePath: "/repo/README.md"},
		"an Edit of documentation":                          {Tool: "Edit", FilePath: "/repo/README.md"},
		"a Read of the record":                              {Tool: "Read", FilePath: "/repo/skills/x/.approval.json"},
		"a Grep for the record":                             {Tool: "Grep"},
		"a Glob for the record":                             {Tool: "Glob"},
		"the approve text under a tool that has no command": {Tool: "Read", Command: "labdrian skills approve --id x"},
		"a Bash call carrying only a record path":           {Tool: "Bash", FilePath: "/repo/skills/x/.approval.json"},
		"a Bash call with an empty command":                 bashCall(""),
		"an unknown tool":                                   {Tool: "SomethingNew", Command: "labdrian skills approve"},
		"a lower-case tool name":                            {Tool: "bash", Command: "labdrian skills approve"},
		"a call with no tool name":                          {Command: "labdrian skills approve", FilePath: "/repo/skills/x/.approval.json"},
		"the zero call":                                     {},
	} {
		t.Run(name, func(t *testing.T) {
			if v := DecideApproveGuard(call); v.Deny || v.Reason != "" {
				t.Errorf("verdict = %+v, want a silent allow", v)
			}
		})
	}
}
