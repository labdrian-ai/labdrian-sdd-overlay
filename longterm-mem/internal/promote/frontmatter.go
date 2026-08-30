package promote

import (
	"fmt"
	"strconv"
	"strings"
)

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
