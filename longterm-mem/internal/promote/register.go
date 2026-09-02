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

// indexMdRelPath and logMdRelPath are the vault's master catalog and
// append-only promotion log (R-029, D7), vault-relative -- the paths
// Writer.Promote (task 7.10) joins onto VaultRoot before calling
// RegisterIndex/RegisterLog.
const (
	indexMdRelPath = "wiki/index.md"
	logMdRelPath   = "wiki/log.md"
)

// logHeaderDateRegexp matches a log.md entry header line and captures its
// date (D7): RegisterLog inserts a new entry immediately before the first
// existing header whose own date is on or before the new entry's date,
// keeping the file sorted by timestamp regardless of call order (hardening:
// the pre-fix version always inserted at the very top, trusting call order
// instead of the at argument).
var logHeaderDateRegexp = regexp.MustCompile(`(?m)^## \[(\d{4}-\d{2}-\d{2})\]`)

// indexEntryLineRegexp matches one rendered index.md entry line and
// captures its address/title (D7). Anchored to a full line (unlike the
// shared, more permissive wikilinkPattern lint.go scans arbitrary prose
// with), a greedy (.*) here correctly captures a title containing "]]":
// it consumes the rest of the line first and backs off only as far as the
// literal "]]$" suffix requires, landing on the LAST "]]" on the line
// rather than the first (hardening: wikilinkPattern's [^\]]* character
// class stopped at the first "]", silently truncating such a title on
// re-parse).
var indexEntryLineRegexp = regexp.MustCompile(`(?m)^- \[\[(c-\d{6})\|(.*)\]\]$`)

// RegisterIndex registers addr/title in the vault's master catalog,
// wiki/index.md (R-029): an idempotent marker block, sorted by address,
// appended once and replaced thereafter so re-registering the same address
// updates its entry in place instead of duplicating it.
func RegisterIndex(indexMdPath, addr, title string) error {
	content, err := readOptional(indexMdPath)
	if err != nil {
		return err
	}
	entries, err := parseIndexEntries(content)
	if err != nil {
		return err
	}
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
// pairs from content. Content with no block yet yields an empty map. A
// malformed block -- a begin marker present with no matching end marker,
// e.g. from a bad hand-edit or a partial prior write -- returns an error
// instead of silently treating it as "no block yet": that used to make
// writeIndexBlock append a fresh block containing only the new entry,
// discarding every entry that lived under the orphaned begin marker.
func parseIndexEntries(content string) (map[string]string, error) {
	entries := map[string]string{}
	begin := strings.Index(content, indexMarkerBegin)
	end := strings.Index(content, indexMarkerEnd)
	if begin == -1 && end == -1 {
		return entries, nil
	}
	if begin == -1 || end == -1 || end <= begin {
		return nil, fmt.Errorf("promote: index.md has a malformed longterm-mem marker block (begin present=%v, end present=%v): refusing to rewrite and risk dropping existing entries", begin != -1, end != -1)
	}
	for _, m := range indexEntryLineRegexp.FindAllStringSubmatch(content[begin+len(indexMarkerBegin):end], -1) {
		entries[m[1]] = m[2]
	}
	return entries, nil
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
// the page, inserted so the file stays sorted newest-first BY TIMESTAMP --
// not by call order. Hardening: the pre-fix version always inserted
// before the very first existing header, which only produced a correctly
// sorted file when every call happened to arrive in chronological order;
// an out-of-order call (an older at registered after a newer one already
// present) left the file unsorted.
func RegisterLog(logMdPath, addr, title string, at time.Time) error {
	content, err := readOptional(logMdPath)
	if err != nil {
		return err
	}

	entry := fmt.Sprintf("## [%s] promote | %s\n\n%s\n\n", at.Format("2006-01-02"), title, wikilink(addr, title))
	newContent := insertLogEntry(content, entry, at)
	return writeFileAtomic(logMdPath, []byte(newContent))
}

// insertLogEntry returns content with entry inserted at the position that
// keeps log.md sorted newest-first by at: immediately before the first
// existing header whose own date is on or before at's date. When every
// existing header is strictly newer than at (or there are none), entry is
// appended at the end (or, for empty content, becomes the whole file).
func insertLogEntry(content, entry string, at time.Time) string {
	atDate := at.Format("2006-01-02")
	for _, loc := range logHeaderDateRegexp.FindAllStringSubmatchIndex(content, -1) {
		headerStart, dateStart, dateEnd := loc[0], loc[2], loc[3]
		if content[dateStart:dateEnd] <= atDate {
			return content[:headerStart] + entry + content[headerStart:]
		}
	}
	trimmed := strings.TrimRight(content, "\n")
	if trimmed == "" {
		return entry
	}
	return trimmed + "\n\n" + entry
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
