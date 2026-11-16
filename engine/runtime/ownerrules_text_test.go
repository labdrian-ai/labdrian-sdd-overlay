package runtime

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The owner's rules have one source in this repository, instructions/owner-rules.md (L60), which
// the Claude adapter writes into ~/.claude/CLAUDE.md and the OpenCode and Codex adapters into their
// AGENTS.md files. These tests pin its text: the rules the owner wrote are verbatim, the reworded
// parts say what L60 approved, and nothing in it invokes gentle-ai or names an Engram tool.

func ownerRulesSourceText(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "instructions", "owner-rules.md"))
	if err != nil {
		t.Fatalf("the owner rules source must exist: %v", err)
	}
	return string(data)
}

func ownerRulesLines(text string) []string { return strings.Split(text, "\n") }

// inOrder fails when a line of want is not a whole line of the file after the previous one.
func inOrder(t *testing.T, text string, want []string) {
	t.Helper()
	lines := ownerRulesLines(text)
	at := 0
	for _, w := range want {
		found := false
		for ; at < len(lines); at++ {
			if lines[at] == w {
				found = true
				at++
				break
			}
		}
		if !found {
			t.Errorf("the line %q is missing (or out of order) in instructions/owner-rules.md", w)
		}
	}
}

func TestOwnerRulesKeepTheElevenRulesAndTheExpertiseVerbatim(t *testing.T) {
	inOrder(t, ownerRulesSourceText(t), []string{
		"## Rules",
		`- Never add "Co-Authored-By" or AI attribution to commits. Use conventional commits only.`,
		`- Prefer the agent's native file-reading, search, and editing tools. When shell commands are necessary, use tools available on the current platform; do not require a specific package manager.`,
		`- Response-length contract: default to short answers. Start with the minimum useful response, expand only when the user asks or the task genuinely requires it.`,
		`- Ask at most one question at a time. After asking it, STOP and wait.`,
		`- Do not present option menus, exhaustive lists, or multiple approaches unless there is a real fork with meaningful tradeoffs.`,
		`- If unsure about length or detail, choose the shorter response.`,
		`- When asking a question, STOP and wait for response. Never continue or assume answers.`,
		`- Never agree with user claims without verification. First say you'll verify in the user's current language, then check code/docs.`,
		`- If user is wrong, explain WHY with evidence. If you were wrong, acknowledge with proof.`,
		`- Always propose alternatives with tradeoffs when relevant.`,
		`- Verify technical claims before stating them. If unsure, investigate first.`,
		"## Expertise",
		`Clean/Hexagonal/Screaming Architecture, testing, atomic design, container-presentational pattern, LazyVim, Tmux, Zellij.`,
	})
}

func TestOwnerRulesKeepTheRemoteAuthorizationRulesVerbatim(t *testing.T) {
	inOrder(t, ownerRulesSourceText(t), []string{
		"## Remote operation authorization",
		`Permission to develop locally does not authorize remote execution or file transfer. Before remote work, require explicit user authorization for the destination, operation, and credential/session to use. If any part is missing or ambiguous, ask and remain local; do not probe the destination to resolve the ambiguity.`,
		`- Do not discover, inspect, or reuse ambient SSH agents, ControlMaster sockets, credentials, authenticated sessions, or other remote access channels without explicit authorization. Their availability is not permission to use them.`,
		`- Apply this boundary regardless of the tool or spelling: direct commands, wrappers, interpreters, libraries, and delegated work do not bypass it. Pass the authorized scope to delegates; delegation cannot expand it.`,
		`- Explicitly authorized remote work is allowed within that scope. Preserve stricter user instructions and runtime restrictions; do not weaken them or change approval settings to proceed.`,
		`- Native ask rules are an additional runtime mechanism, not authorization inferred from local-development access. Automation modes and remembered approvals may suppress prompts. This behavioral contract is not a sandbox and does not guarantee a fresh human prompt for every execution.`,
	})
}

func TestOwnerRulesCarryTheSkillLoadingRuleAndPointToTheVoiceInTheirOwnWords(t *testing.T) {
	inOrder(t, ownerRulesSourceText(t), []string{
		"## Skills",
		`When a request matches an available skill, load that skill before replying. Several can apply at once; match by the files involved and by what is asked.`,
		"## Voice",
		`Tone, reply language and teaching style come from the labdrian output style; this file sets rules, not voice.`,
	})
}

func TestOwnerRulesCarryTheShortenedCodeGraphGuidance(t *testing.T) {
	inOrder(t, ownerRulesSourceText(t), []string{
		"## CodeGraph",
		"- For structural or codebase questions (repository map, architecture, call flow, references, impact), use CodeGraph (`codegraph_explore`, or the read-only `codegraph` CLI commands) before broad Read, Glob or Grep.",
		"- In a real project with no `.codegraph/`, run `codegraph init` in the project root once, without asking; never in `$HOME`, a temporary directory or a non-project folder. Fall back to file tools only after init or use fails, and say so.",
		"- Put worktrees that need CodeGraph under the home directory, as `<repo-parent>/<repo-name>-worktrees/<name>`, never under `/tmp` or `/var/tmp`; each worktree gets its own index, never a copied or linked one.",
		"- Never run `codegraph uninit`, `install`, `uninstall` or `upgrade`; run `codegraph index` only to recover a corrupt index. After edits rely on the watcher; run `codegraph sync` only when it is off or files stay stale.",
	})
}

func TestOwnerRulesTurnEveryKnownErrorClassIntoALintCheck(t *testing.T) {
	text := ownerRulesSourceText(t)
	rule := "Every known error class (found by review, a verifier or the user) is turned into a lint check that fails on it, committed with the fix. A one-off mistake that no rule can catch gets a regression test instead."
	if !strings.Contains(text, "\n"+rule+"\n") && !strings.Contains(text, "\n- "+rule+"\n") {
		t.Errorf("the lint rule of L78 is missing as a whole line:\n%s", rule)
	}
}

func TestOwnerRulesInvokeNoGentleAIAndNameNoEngramTool(t *testing.T) {
	text := ownerRulesSourceText(t)
	for _, forbidden := range []*regexp.Regexp{
		regexp.MustCompile(`(?i)gentle-ai`),
		regexp.MustCompile(`(?i)engram`),
		regexp.MustCompile(`\bmem_[a-z_]+`),
	} {
		if loc := forbidden.FindStringIndex(text); loc != nil {
			t.Errorf("instructions/owner-rules.md must not match %s, found %q", forbidden, text[loc[0]:loc[1]])
		}
	}
}

func TestOwnerRulesHaveNoCarriageReturnsAndEndWithOneNewline(t *testing.T) {
	text := ownerRulesSourceText(t)
	if strings.Contains(text, "\r") {
		t.Error("the owner rules must use LF line endings")
	}
	if !strings.HasSuffix(text, "\n") || strings.HasSuffix(text, "\n\n") {
		t.Error("the owner rules must end with exactly one newline")
	}
}
