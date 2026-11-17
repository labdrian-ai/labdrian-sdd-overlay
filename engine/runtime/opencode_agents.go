package runtime

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// <config>/AGENTS.md of OpenCode (S13): v2 reads AGENTS.md and no CLAUDE.md, so the owner's rules,
// the labdrian output style (tone and language) and the memorycard block reach it as one marked
// block of that file, built from the sources this repository owns. Everything outside the block
// is the owner's and is never changed.

// OpenCodeInstructionsBlock is the block of AGENTS.md that holds the instructions below.
var OpenCodeInstructionsBlock = MarkedBlock{
	Open:  "<!-- labdrian:opencode-instructions -->",
	Close: "<!-- /labdrian:opencode-instructions -->",
}

// openCodeStyleSourcePath is the output style, a slash path relative to the overlay checkout; the
// block holds its body, without the frontmatter Claude Code reads.
const openCodeStyleSourcePath = "output-styles/labdrian.md"

// openCodeMemorycardBlock is the memory store block of the owner's rules for OpenCode: memorycard
// is the only store for new memories and its tools replace the Engram ones. It leaves out what only
// Claude Code does (the capture hook that reads a worker's Key Learnings).
const openCodeMemorycardBlock = `## memorycard is the memory store

Engram is frozen read-only history; memorycard (the MCP server ` + "`memorycard`" + `) is the only store for new memories. Never write to Engram.

- Save with ` + "`mem_save`" + ` (` + "`title`" + `, ` + "`body`" + `, ` + "`type`" + `, ` + "`topic_key`" + `, ` + "`scope`" + `, ` + "`project`" + `): the same ` + "`topic_key`" + ` updates the memory in place, and ` + "`supersedes`" + ` marks an older id as replaced.
- Find with ` + "`mem_search`" + `, read one memory in full with ` + "`mem_get`" + `, and list the pinned, stale and recent ones with ` + "`mem_context`" + `.
- Edit, pin, archive (never delete) or mark a stale memory reviewed with ` + "`mem_update`" + `, only after the user confirms a review; record a session summary with ` + "`mem_session`" + `.
- memorycard ids and Engram ids are different number spaces: never pass an Engram id to memorycard.
- A memory proven wrong is superseded, never left beside its correction. Record a supersession without asking when the evidence is in this session, and say which memory, by which, and why; ask first when it would rest on titles alone or overturn a position the user stated.`

// errNoInstructionSource says the overlay checkout holds no source of the instructions: there is
// none, or it lacks one of the files. The install is then the one without AGENTS.md, with a note,
// as the Claude adapter's is without the owner rules when no checkout is found.
var errNoInstructionSource = errors.New("no overlay checkout holds the instruction sources")

// instructions is the text of the block: the owner rules, the body of the output style and the
// memorycard block, each as its source gives it, separated by one blank line.
func (a OpenCodeAdapter) instructions() (string, error) {
	rules, err := OverlayOwnerRules{OverlayDir: a.options.OverlayDir}.OwnerRules()
	if errors.Is(err, ErrNoOverlayCheckout) || errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("%w: %v", errNoInstructionSource, err)
	}
	if err != nil {
		return "", err
	}
	root, err := locateOverlay(a.options.OverlayDir)
	if err != nil {
		return "", err
	}
	style, err := os.ReadFile(contractFile(root, openCodeStyleSourcePath))
	if errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("%w: %v", errNoInstructionSource, err)
	}
	if err != nil {
		return "", fmt.Errorf("read the output style: %w", err)
	}
	parts := []string{rules, styleBody(string(style)), openCodeMemorycardBlock}
	for i, part := range parts {
		parts[i] = strings.Trim(strings.ReplaceAll(part, "\r\n", "\n"), "\n")
	}
	return strings.Join(parts, "\n\n"), nil
}

// styleBody is the output style without its frontmatter, the lines between the opening --- line and
// the next one. A text that does not start with a frontmatter is all body.
func styleBody(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return text
	}
	rest := text[len("---\n"):]
	switch {
	case strings.HasPrefix(rest, "---\n"):
		return rest[len("---\n"):]
	case rest == "---":
		return ""
	}
	if end := strings.Index(rest, "\n---\n"); end >= 0 {
		return rest[end+len("\n---\n"):]
	}
	if strings.HasSuffix(rest, "\n---") {
		return ""
	}
	return text
}

func (a OpenCodeAdapter) agentsPath() string { return filepath.Join(a.root, "AGENTS.md") }

// writeAgents makes AGENTS.md hold the block and says what it did. partial is true when it could not.
func (a OpenCodeAdapter) writeAgents() (message string, partial bool) {
	path := a.agentsPath()
	body, err := a.instructions()
	if errors.Is(err, errNoInstructionSource) {
		return "AGENTS.md not written: " + err.Error(), false
	}
	if err != nil {
		return "AGENTS.md could not be read from its sources: " + err.Error(), true
	}
	change, err := OpenCodeInstructionsBlock.WriteFile(path, body)
	if err != nil {
		return "AGENTS.md could not be written: " + err.Error(), true
	}
	switch change {
	case BlockAdded:
		return "AGENTS.md written to " + path, false
	case BlockReplaced:
		return "AGENTS.md updated in " + path, false
	}
	return "AGENTS.md already current in " + path, false
}

// removeAgents takes the block out of AGENTS.md; a file left with nothing in it is removed.
func (a OpenCodeAdapter) removeAgents() error {
	_, err := OpenCodeInstructionsBlock.RemoveFile(a.agentsPath())
	return err
}

// agentsStatus is the status result for AGENTS.md when its block is not current: partial, naming
// the file and the verb that fixes it. ok is false when the block is current or no checkout holds
// the sources, which is not a defect of this machine's install.
func (a OpenCodeAdapter) agentsStatus() (message string, ok bool) {
	path := a.agentsPath()
	body, err := a.instructions()
	if errors.Is(err, errNoInstructionSource) {
		return "", false
	}
	if err != nil {
		return "OpenCode instructions could not be read from their sources: " + err.Error(), true
	}
	state, err := OpenCodeInstructionsBlock.StateOfFile(path, body)
	if err != nil {
		return "OpenCode AGENTS.md block could not be checked: " + err.Error(), true
	}
	switch state {
	case BlockMissing:
		return "OpenCode AGENTS.md block is missing in " + path + "; run 'engine runtime install --target opencode'", true
	case BlockStale:
		return "OpenCode AGENTS.md block is stale in " + path + "; run 'engine runtime update --target opencode'", true
	}
	return "", false
}

// shadowingSkillsNote reports the labdrian skills found where OpenCode reads skills before
// ~/.claude/skills (<config>/skills, then ~/.agents/skills): a copy there shadows the Claude one,
// so none is deployed (S13) and one found is the owner's to remove. It is empty when there is none
// or the overlay checkout cannot be read; nothing is changed either way.
func (a OpenCodeAdapter) shadowingSkillsNote() string {
	root, err := locateOverlay(a.options.OverlayDir)
	if err != nil {
		return ""
	}
	entries, err := os.ReadDir(filepath.Join(root, "skills"))
	if err != nil {
		return ""
	}
	labdrian := map[string]bool{}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), "_") {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, "skills", entry.Name(), "SKILL.md")); err == nil {
			labdrian[entry.Name()] = true
		}
	}
	var note strings.Builder
	for _, dir := range []string{filepath.Join(a.root, "skills"), a.options.AgentsSkillsDir} {
		if dir == "" {
			continue
		}
		found, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		var ids []string
		for _, entry := range found {
			if labdrian[entry.Name()] {
				ids = append(ids, entry.Name())
			}
		}
		if len(ids) == 0 {
			continue
		}
		sort.Strings(ids)
		fmt.Fprintf(&note, "; labdrian skills found in %s: %s (OpenCode reads them before ~/.claude/skills, so they shadow the Claude copy; remove them by hand)", dir, strings.Join(ids, ", "))
	}
	return note.String()
}
