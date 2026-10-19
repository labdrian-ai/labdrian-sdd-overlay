package vault

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/query"
)

// PageTagReader is the adapter behind the VaultPageTags port that internal/query owns: it reads the frontmatter
// tags of a page the vault's retrieval offered, from the page file the candidate names. The composition root
// builds it beside the Retriever, over the same vault Root.
type PageTagReader struct {
	// Root is the vault checkout; a candidate's page must lie inside it.
	Root string
}

// PageTags reads candidate's page and returns the tags of its frontmatter, in their order: a block list (the one
// promote writes, `  - "memory"`), a flow list (`[a, "b"]`) or a single scalar. A page with no frontmatter, or a
// frontmatter with no tags, has none. A page that cannot be read, that lies outside Root (or is named by a relative
// path), whose frontmatter is never closed, or whose flow list is never closed is an error.
func (r PageTagReader) PageTags(_ context.Context, candidate query.VaultCandidate) ([]string, error) {
	path, err := r.pageInside(candidate.AbsolutePath)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("vault: read page %s: %w", candidate.PageAddress, err)
	}
	tags, err := frontmatterTags(strings.ReplaceAll(string(raw), "\r\n", "\n"))
	if err != nil {
		return nil, fmt.Errorf("vault: page %s: %w", candidate.PageAddress, err)
	}
	return tags, nil
}

// pageInside answers the real path of path when it is absolute and, links resolved, lies inside the real Root, and
// an error otherwise: the reader reads only the vault's own pages, whatever path a retrieval reports, and a link in
// the vault to a file outside it is not one of them.
func (r PageTagReader) pageInside(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("vault: page path %q is not absolute", path)
	}
	root, err := filepath.Abs(r.Root)
	if err != nil {
		return "", fmt.Errorf("vault: resolve the vault root %q: %w", r.Root, err)
	}
	if root, err = filepath.EvalSymlinks(root); err != nil {
		return "", fmt.Errorf("vault: resolve the vault root %q: %w", r.Root, err)
	}
	if path, err = filepath.EvalSymlinks(path); err != nil {
		return "", fmt.Errorf("vault: resolve page path: %w", err)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("vault: page path %q is outside the vault %q", path, root)
	}
	return path, nil
}

const frontmatterFence = "---"

var (
	errUnclosedFrontmatter = errors.New("the frontmatter is never closed")
	errUnclosedFlowList    = errors.New("the tags flow list is never closed")
)

// frontmatterTags returns the tags of page's frontmatter (see PageTags).
func frontmatterTags(page string) ([]string, error) {
	lines := strings.Split(page, "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], " \t") != frontmatterFence {
		return nil, nil
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], " \t") == frontmatterFence {
			end = i
			break
		}
	}
	if end < 0 {
		return nil, errUnclosedFrontmatter
	}
	fields := lines[1:end]
	for i, line := range fields {
		value, ok := strings.CutPrefix(line, "tags:")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch {
		case value == "":
			return blockListItems(fields[i+1:]), nil
		case strings.HasPrefix(value, "["):
			return flowListItems(value)
		default:
			return []string{unquote(value)}, nil
		}
	}
	return nil, nil
}

// blockListItems are the `- item` lines that open lines, unquoted, up to the first line that is not one.
func blockListItems(lines []string) []string {
	var items []string
	for _, line := range lines {
		item, ok := strings.CutPrefix(strings.TrimLeft(line, " \t"), "- ")
		if !ok {
			break
		}
		items = append(items, unquote(strings.TrimSpace(item)))
	}
	return items
}

// flowListItems are the items of a `[a, "b", 'c']` list, unquoted; a comma inside quotes does not split.
func flowListItems(value string) ([]string, error) {
	if !strings.HasSuffix(value, "]") {
		return nil, errUnclosedFlowList
	}
	inner := strings.TrimSpace(value[1 : len(value)-1])
	if inner == "" {
		return nil, nil
	}
	var items []string
	var quote rune
	start := 0
	for i, ch := range inner {
		switch {
		case quote != 0 && ch == quote:
			quote = 0
		case quote == 0 && (ch == '"' || ch == '\''):
			quote = ch
		case quote == 0 && ch == ',':
			items = append(items, unquote(strings.TrimSpace(inner[start:i])))
			start = i + 1
		}
	}
	return append(items, unquote(strings.TrimSpace(inner[start:]))), nil
}

// unquote reads one scalar: a double-quoted one with its \\ and \" escapes undone (the quoting promote writes), a
// single-quoted one with its doubled quote undone, and anything else as it is.
func unquote(v string) string {
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		var b strings.Builder
		body := v[1 : len(v)-1]
		for i := 0; i < len(body); i++ {
			if body[i] == '\\' && i+1 < len(body) && (body[i+1] == '\\' || body[i+1] == '"') {
				i++
			}
			b.WriteByte(body[i])
		}
		return b.String()
	}
	if len(v) >= 2 && v[0] == '\'' && v[len(v)-1] == '\'' {
		return strings.ReplaceAll(v[1:len(v)-1], "''", "'")
	}
	return v
}
