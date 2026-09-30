package skills

import (
	"encoding/json"
	"strings"
	"testing"
)

// hookInput renders the PreToolUse hook JSON Claude Code sends for one tool
// call, with the tool input's fields as given. It goes through encoding/json so
// quotes, newlines, and backslashes in a command are escaped exactly as they
// would be on the wire.
func hookInput(t *testing.T, tool string, toolInput map[string]any) []byte {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"session_id":      "s-1",
		"hook_event_name": "PreToolUse",
		"cwd":             "/repo",
		"tool_name":       tool,
		"tool_input":      toolInput,
	})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func bashInput(t *testing.T, command string) []byte {
	t.Helper()
	return hookInput(t, "Bash", map[string]any{"command": command})
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
	v := DecideApproveGuard(bashInput(t, "cd repo && labdrian skills approve --id my-skill --approver alice"))
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
			v := DecideApproveGuard(hookInput(t, tc.tool, map[string]any{tc.field: "/repo/skills/my-skill/.approval.json", "content": "{}"}))
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

func TestDecideApproveGuard_AllowsEverythingElseSilently(t *testing.T) {
	huge := strings.Repeat("x", 1<<20)
	for name, input := range map[string][]byte{
		"a harmless Bash call":                              bashInput(t, "ls -la"),
		"another skills verb":                               bashInput(t, "labdrian skills validate --source-root skills"),
		"a Write to SKILL.md":                               hookInput(t, "Write", map[string]any{"file_path": "/repo/skills/x/SKILL.md", "content": "labdrian skills approve --id x"}),
		"a Write of documentation about approve":            hookInput(t, "Write", map[string]any{"file_path": "/repo/README.md", "content": "Run `labdrian skills approve --id <id> --approver <name>`."}),
		"an Edit of documentation":                          hookInput(t, "Edit", map[string]any{"file_path": "/repo/README.md", "old_string": "a", "new_string": "labdrian skills approve"}),
		"a Read of the record":                              hookInput(t, "Read", map[string]any{"file_path": "/repo/skills/x/.approval.json"}),
		"a Grep for the record":                             hookInput(t, "Grep", map[string]any{"pattern": ".approval.json", "path": "/repo/skills"}),
		"a Glob for the record":                             hookInput(t, "Glob", map[string]any{"pattern": "skills/*/.approval.json"}),
		"the approve text under a tool that has no command": hookInput(t, "Read", map[string]any{"command": "labdrian skills approve --id x"}),
		"a Bash call carrying only a record path":           hookInput(t, "Bash", map[string]any{"file_path": "/repo/skills/x/.approval.json"}),
		"a Write of a large unrelated file":                 hookInput(t, "Write", map[string]any{"file_path": "/repo/big.txt", "content": huge}),
		"a Bash call with an empty command":                 bashInput(t, ""),
		"an unknown tool":                                   hookInput(t, "SomethingNew", map[string]any{"command": "labdrian skills approve"}),
		"a lower-case tool name":                            hookInput(t, "bash", map[string]any{"command": "labdrian skills approve"}),
	} {
		t.Run(name, func(t *testing.T) {
			if v := DecideApproveGuard(input); v.Deny || v.Reason != "" {
				t.Errorf("verdict = %+v, want a silent allow", v)
			}
		})
	}
}

// A hook that cannot see the call cannot vouch for it, but it must not block
// it either: this guard runs on every Bash and file-edit call, so unusable
// input is an allow, never a denial. That is the opposite of the shaper guard,
// which fails closed for its narrower markers.
func TestDecideApproveGuard_UnusableInputIsAnAllow(t *testing.T) {
	for name, input := range map[string]string{
		"empty":                    "",
		"whitespace":               "  \n ",
		"not JSON":                 "labdrian skills approve",
		"truncated JSON":           `{"tool_name":"Bash","tool_input":{"command":"labdrian skills app`,
		"a JSON array":             `["Bash"]`,
		"null":                     `null`,
		"a string":                 `"labdrian skills approve"`,
		"a tool input string":      `{"tool_name":"Bash","tool_input":"labdrian skills approve --id x"}`,
		"a numeric command":        `{"tool_name":"Bash","tool_input":{"command":42}}`,
		"a numeric tool name":      `{"tool_name":7,"tool_input":{"command":"labdrian skills approve"}}`,
		"a missing tool input":     `{"tool_name":"Bash"}`,
		"a null tool input":        `{"tool_name":"Bash","tool_input":null}`,
		"an object with no fields": `{}`,
	} {
		t.Run(name, func(t *testing.T) {
			if v := DecideApproveGuard([]byte(input)); v.Deny || v.Reason != "" {
				t.Errorf("verdict = %+v, want a silent allow", v)
			}
		})
	}
}

func TestDecideApproveGuard_InputOverTheBoundIsAnAllow(t *testing.T) {
	over := bashInput(t, "labdrian skills approve --id x #"+strings.Repeat("a", ApproveGuardMaxInputBytes))
	if len(over) <= ApproveGuardMaxInputBytes {
		t.Fatalf("test bug: input is %d bytes, not over the bound", len(over))
	}
	if v := DecideApproveGuard(over); v.Deny {
		t.Errorf("verdict = %+v, want an allow for input over the bound", v)
	}
	// Just under the bound is still judged.
	fits := bashInput(t, "labdrian skills approve --id x #"+strings.Repeat("a", ApproveGuardMaxInputBytes-1024))
	if len(fits) > ApproveGuardMaxInputBytes {
		t.Fatalf("test bug: input is %d bytes, over the bound", len(fits))
	}
	if v := DecideApproveGuard(fits); !v.Deny {
		t.Errorf("verdict = %+v, want a denial for input within the bound", v)
	}
}

// The hook payload can carry fields a later Claude Code adds, in any order.
func TestDecideApproveGuard_IgnoresUnknownFieldsAndFieldOrder(t *testing.T) {
	input := `{"future_field":{"a":[1,2,3]},"tool_input":{"description":"x","command":"labdrian skills approve --id x","timeout":5},"tool_name":"Bash","tool_use_id":"t-9"}`
	if v := DecideApproveGuard([]byte(input)); !v.Deny {
		t.Errorf("verdict = %+v, want a denial", v)
	}
}
