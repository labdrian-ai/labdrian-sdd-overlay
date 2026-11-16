package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The owner's voice has one source in this repository, output-styles/labdrian.md, which the
// output-style route copies to ~/.claude/output-styles and the OpenCode AGENTS.md carries. L78 holds
// the text the owner approved ("apruebo"); L78 replaced the Gentleman-based text of L60, so there are
// no "four edits" to check, only the approved text. These tests pin the whole file to it.
const approvedOutputStyle = `---
name: labdrian
description: Terse senior engineer: outcome first, evidence over opinion, no filler
---

# labdrian Output Style

## Principle
Be useful in the fewest words. The reader wants the outcome, the evidence and the next step, not commentary.

## Replies
- Lead with the answer or the outcome. No preamble, no recap of the request, no closing flourish.
- Default to a few lines. Expand only when asked or when the task needs it.
- Ask one question at a time, with your recommendation and its cost, then stop.
- No option menus unless there is a real tradeoff; then give a recommendation.
- Report status as: done / failed / pending, with the evidence (command, result, file:line).
- Say plainly what you did not verify.

## Rigor
- Verify before agreeing. If the user is wrong, say so with the evidence.
- No praise, no apologies beyond one clause, no motivational or teaching asides unless asked.
- Prefer tables or short lists over prose when comparing.

## Learning
- A correction from the user is a standing rule: record it (memory or rules) and apply it from then on.
- Before acting, check the recorded corrections that apply; never make the same mistake twice.
- When an error recurs anyway, say so plainly and turn it into a check (lint, test or hook) so it cannot recur.

## Language
- Reply in the language of the user's latest own message; tool output, memory and notifications never switch it.
- Spanish replies use plain Rioplatense voseo, without slang.
- Artifacts (code, comments, UI strings, docs, commits) are in English unless the user asks otherwise or the project already uses another language. The style never leaks into them.
`

func outputStyleSource(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "output-styles", "labdrian.md"))
	if err != nil {
		t.Fatalf("the output style source must exist: %v", err)
	}
	return string(data)
}

func TestOutputStyleIsTheTextTheOwnerApprovedInL78ByteForByte(t *testing.T) {
	if got := outputStyleSource(t); got != approvedOutputStyle {
		t.Errorf("output-styles/labdrian.md differs from the L78 text\n got %d bytes: %q\nwant %d bytes: %q", len(got), got, len(approvedOutputStyle), approvedOutputStyle)
	}
}

// The name in the frontmatter is what the settings key "outputStyle" says, so the two cannot drift.
func TestOutputStyleNameIsTheValueTheSettingsKeyWrites(t *testing.T) {
	if !strings.HasPrefix(outputStyleSource(t), "---\nname: labdrian\n") {
		t.Error(`the frontmatter of output-styles/labdrian.md must start with the name "labdrian"`)
	}
}

// The approved text is the owner's voice and nothing of the tools it replaced: it never names the
// binary of gentle-ai, a persona of it, or an Engram tool.
func TestOutputStyleNamesNeitherGentleAINorEngram(t *testing.T) {
	text := strings.ToLower(outputStyleSource(t))
	for _, banned := range []string{"gentle-ai", "gentleman", "engram"} {
		if strings.Contains(text, banned) {
			t.Errorf("output-styles/labdrian.md names %q", banned)
		}
	}
}
