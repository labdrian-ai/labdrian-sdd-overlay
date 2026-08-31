package promote

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// PatchStatusFields rewrites only the status: and related: lines inside
// path's frontmatter block to status/related, leaving every other
// frontmatter line -- and the entire body -- byte-identical (R-033: a
// frontmatter-only patch never rewrites a page body, even on a page a
// human has since edited by hand). It returns the patched frontmatter
// block's own hash and the untouched body's hash, so a caller
// (propagate.go) can record both halves of the precedence sidecar without
// re-reading the file. 7.8 REFACTOR: extracted into frontmatter.go so a
// future frontmatter-only editor shares this one line-patcher instead of
// re-implementing the frontmatter/body split (frontmatterBlock, address.go)
// and the fixed-format field layout (writeField/writeListField, below).
func PatchStatusFields(path, status string, related []string) (frontmatterHash, bodyHash string, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", "", fmt.Errorf("promote: read %s: %w", path, err)
	}
	fmBlock, ok := frontmatterBlock(string(raw))
	if !ok {
		return "", "", fmt.Errorf("promote: %s has no parseable frontmatter block", path)
	}
	body := string(raw)[len(fmBlock):]

	newBlock := setScalarField(fmBlock, "status", status)
	newBlock = setListField(newBlock, "related", related)

	if newBlock != fmBlock {
		if err := writeFileAtomic(path, []byte(newBlock+body)); err != nil {
			return "", "", err
		}
	}
	return hashText(newBlock), hashText(body), nil
}

// setScalarField replaces block's "key: ..." line with key: value,
// leaving every other line untouched. A block with no key: line at all --
// a legacy or hand-authored page predating this field, never produced by
// Render(), which always emits it -- gets one inserted just before the
// block's closing delimiter, the same way setListField handles its own
// absent-field case. Returning the block unchanged instead would let a
// patch report success having written nothing, which is exactly wrong for
// status: (the field a caller patches to record supersession or archival).
func setScalarField(block, key, value string) string {
	lines := strings.Split(block, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, key+": ") {
			lines[i] = key + ": " + value
			return strings.Join(lines, "\n")
		}
	}
	return insertBeforeClosingDelimiter(lines, []string{key + ": " + value})
}

// setListField replaces block's key: list section (either the inline
// "key: []" form, or "key:" followed by "  - ..." lines) with a freshly
// rendered one for items, via writeListField -- the same renderer Render()
// itself uses, so the patched section matches EmitPage's own format
// exactly. A block with no key: line at all -- a legacy or hand-authored
// page predating this field, never produced by Render() itself, which
// always emits it -- gets one inserted just before the block's closing
// delimiter, rather than silently leaving the field missing (task 7.10
// Gap 2 finding: the pre-fix version returned block unchanged here).
func setListField(block, key string, items []string) string {
	lines := strings.Split(block, "\n")
	start := -1
	for i, line := range lines {
		if strings.HasPrefix(line, key+":") {
			start = i
			break
		}
	}

	var rendered strings.Builder
	writeListField(&rendered, key, items)
	renderedLines := strings.Split(strings.TrimRight(rendered.String(), "\n"), "\n")

	if start == -1 {
		return insertBeforeClosingDelimiter(lines, renderedLines)
	}

	end := start + 1
	if lines[start] != key+": []" {
		for end < len(lines) && strings.HasPrefix(lines[end], "  - ") {
			end++
		}
	}

	newLines := make([]string, 0, len(lines)-(end-start)+len(renderedLines))
	newLines = append(newLines, lines[:start]...)
	newLines = append(newLines, renderedLines...)
	newLines = append(newLines, lines[end:]...)
	return strings.Join(newLines, "\n")
}

// insertBeforeClosingDelimiter inserts newLines immediately before lines's
// last "---" element (block's closing frontmatter delimiter, per
// frontmatterBlock's `---\n...\n---\n` shape), so a freshly rendered
// field section lands inside the block rather than after it.
func insertBeforeClosingDelimiter(lines, newLines []string) string {
	closing := len(lines) - 1
	for closing >= 0 && lines[closing] != "---" {
		closing--
	}
	if closing < 0 {
		closing = len(lines)
	}
	result := make([]string, 0, len(lines)+len(newLines))
	result = append(result, lines[:closing]...)
	result = append(result, newLines...)
	result = append(result, lines[closing:]...)
	return strings.Join(result, "\n")
}

// vaultType is the single vault type-contract enum value every Engram
// observation maps onto (D7): the only value with no type-specific
// required fields (author, url, ...) promotion would otherwise fabricate.
const vaultType = "concept"

// frontmatter is the flat-YAML page header EmitPage renders.
type frontmatter struct {
	Title          string
	Address        string
	Aliases        []string
	Created        string
	Updated        string
	Tags           []string
	Status         string
	Related        []string
	EngramID       int64
	EngramSyncID   string
	EngramType     string
	EngramRevision int
	Project        string
}

// Render renders fm as flat YAML frontmatter, in the fixed field order
// design-notes #3133 specifies: type, title, address, aliases, created,
// updated, tags, status, related, sources, engram_id, engram_sync_id,
// engram_type, engram_revision, project.
func (fm frontmatter) Render() string {
	var b strings.Builder
	b.WriteString("---\n")
	writeField(&b, "type", vaultType)
	writeField(&b, "title", quoteYAML(fm.Title))
	writeField(&b, "address", fm.Address)
	writeListField(&b, "aliases", fm.Aliases)
	writeField(&b, "created", fm.Created)
	writeField(&b, "updated", fm.Updated)
	writeListField(&b, "tags", fm.Tags)
	writeField(&b, "status", fm.Status)
	writeListField(&b, "related", fm.Related)
	writeListField(&b, "sources", nil)
	writeField(&b, "engram_id", strconv.FormatInt(fm.EngramID, 10))
	writeField(&b, "engram_sync_id", fm.EngramSyncID)
	writeField(&b, "engram_type", fm.EngramType)
	writeField(&b, "engram_revision", strconv.Itoa(fm.EngramRevision))
	writeField(&b, "project", fm.Project)
	b.WriteString("---\n")
	return b.String()
}

func writeField(b *strings.Builder, key, value string) {
	fmt.Fprintf(b, "%s: %s\n", key, value)
}

// writeListField renders a flat YAML list; an empty/nil list renders as
// the inline `key: []` form (WIKI.md's own universal-fields example).
func writeListField(b *strings.Builder, key string, items []string) {
	if len(items) == 0 {
		fmt.Fprintf(b, "%s: []\n", key)
		return
	}
	fmt.Fprintf(b, "%s:\n", key)
	for _, item := range items {
		fmt.Fprintf(b, "  - %s\n", quoteYAML(item))
	}
}

// quoteYAML wraps v in double quotes, escaping backslashes first and then
// embedded double quotes, so free text (a title, a wikilink alias) stays a
// valid flat scalar even if it contains a colon, a backslash — including a
// trailing one — or another YAML-significant character.
func quoteYAML(v string) string {
	escaped := strings.ReplaceAll(v, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}
