package promote

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

// indexMarkerBegin/indexMarkerEnd delimit the idempotent master-catalog
// block RegisterIndex owns inside wiki/index.md (D7): appended once and
// replaced thereafter, never disturbing surrounding hand-authored content.
const (
	indexMarkerBegin = "<!-- longterm-mem:begin -->"
	indexMarkerEnd   = "<!-- longterm-mem:end -->"
)

// logHeaderRegexp matches a log.md entry header line (D7): the newest
// RegisterLog call inserts its own entry immediately before the first
// match, keeping the file newest-first.
var logHeaderRegexp = regexp.MustCompile(`(?m)^## \[`)

// RegisterIndex registers addr/title in the vault's master catalog,
// wiki/index.md (R-029): an idempotent marker block, sorted by address,
// appended once and replaced thereafter so re-registering the same address
// updates its entry in place instead of duplicating it.
func RegisterIndex(indexMdPath, addr, title string) error {
	content, err := readOptional(indexMdPath)
	if err != nil {
		return err
	}
	entries := parseIndexEntries(content)
	entries[addr] = title
	return writeIndexBlock(indexMdPath, content, entries)
}

// writeIndexBlock replaces or appends the marker block in indexMdPath
// (whose current content is content) with entries, sorted by address.
// Extracted from RegisterIndex (5.6 REFACTOR) so a future batch caller
// (sync, slice 7) can compute the full entries map once across every
// promoted page and write it in a single pass, instead of one
// read-modify-write per page.
func writeIndexBlock(indexMdPath, content string, entries map[string]string) error {
	block := renderIndexBlock(entries)
	return writeFileAtomic(indexMdPath, []byte(replaceOrAppendBlock(content, block)))
}

// parseIndexEntries extracts the existing marker block's address->title
// pairs from content; a content with no block yet yields an empty map.
func parseIndexEntries(content string) map[string]string {
	entries := map[string]string{}
	begin := strings.Index(content, indexMarkerBegin)
	end := strings.Index(content, indexMarkerEnd)
	if begin == -1 || end == -1 || end <= begin {
		return entries
	}
	for _, m := range wikilinkPattern.FindAllStringSubmatch(content[begin+len(indexMarkerBegin):end], -1) {
		entries[m[1]] = m[2]
	}
	return entries
}

// renderIndexBlock renders entries as the marker block, one wikilink per
// line, sorted by address (D7).
func renderIndexBlock(entries map[string]string) string {
	addrs := make([]string, 0, len(entries))
	for addr := range entries {
		addrs = append(addrs, addr)
	}
	sort.Strings(addrs)

	var b strings.Builder
	b.WriteString(indexMarkerBegin + "\n")
	for _, addr := range addrs {
		b.WriteString("- " + wikilink(addr, entries[addr]) + "\n")
	}
	b.WriteString(indexMarkerEnd + "\n")
	return b.String()
}

// replaceOrAppendBlock replaces an existing marker block in content with
// block, or appends block (after a blank-line separator from any existing
// content) when no marker block is present yet.
func replaceOrAppendBlock(content, block string) string {
	begin := strings.Index(content, indexMarkerBegin)
	end := strings.Index(content, indexMarkerEnd)
	if begin != -1 && end != -1 && end > begin {
		return content[:begin] + block + content[end+len(indexMarkerEnd):]
	}
	trimmed := strings.TrimRight(content, "\n")
	if trimmed == "" {
		return block
	}
	return trimmed + "\n\n" + block
}

// RegisterLog records the promotion event for addr/title at the given
// time in the vault's append-only promotion log, wiki/log.md (R-029): a
// `## [YYYY-MM-DD] promote | Title` header (D7) followed by a wikilink to
// the page, inserted before the first existing entry header so the file
// stays newest-first.
func RegisterLog(logMdPath, addr, title string, at time.Time) error {
	content, err := readOptional(logMdPath)
	if err != nil {
		return err
	}

	entry := fmt.Sprintf("## [%s] promote | %s\n\n%s\n\n", at.Format("2006-01-02"), title, wikilink(addr, title))

	loc := logHeaderRegexp.FindStringIndex(content)
	var newContent string
	if loc == nil {
		trimmed := strings.TrimRight(content, "\n")
		if trimmed != "" {
			trimmed += "\n\n"
		}
		newContent = trimmed + entry
	} else {
		newContent = content[:loc[0]] + entry + content[loc[0]:]
	}
	return writeFileAtomic(logMdPath, []byte(newContent))
}

// readOptional reads path's content, treating a missing file as empty
// content rather than an error.
func readOptional(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("promote: read %s: %w", path, err)
	}
	return string(data), nil
}
