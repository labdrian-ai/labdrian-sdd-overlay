package promote

import "strings"

// PageIdentity is the memory a page was promoted from, as the page's frontmatter names it: the store it comes
// from (Source), its id there (ID), the revision the page was promoted at (Revision) and its type (Type). The
// revision and type are the page's raw values, trimmed and unparsed: judging them belongs to the consumer that
// needs them, as it always has.
type PageIdentity struct {
	Source   string
	ID       string
	Revision string
	Type     string
}

// The store-agnostic keys a page names its identity with (D1 of the store-agnostic promotion). They are the only
// keys read: the legacy ones (engram_*, memorycard_*) of the pages written before them were retired once the live
// vault carried none (S7).
const (
	sourceField         = "source"
	sourceIDField       = "source_id"
	sourceRevisionField = "source_revision"
	sourceTypeField     = "source_type"
)

// The stores a page can name. The page-identity keys are store-agnostic, so a store is a name here, not a type.
const memorycardSource = "memorycard"

// fieldOf reads one field of the identity source names: the value of the store-agnostic key, when the page names
// source in `source` and that key holds a value, and nothing otherwise. It never crosses stores.
func fieldOf(fields map[string]string, source, key string) string {
	if strings.TrimSpace(fields[sourceField]) != source {
		return ""
	}
	if v := fields[key]; strings.TrimSpace(v) != "" {
		return v
	}
	return ""
}

// revisionOf reads the revision a page records for source, trimmed and unparsed. It does not need the page to name
// an id: the revision of a page is read by its own key.
func revisionOf(fields map[string]string, source string) string {
	return strings.TrimSpace(fieldOf(fields, source, sourceRevisionField))
}

// identityOf is the one reader of a page's identity, over the parsed fields of its frontmatter block: the
// identity the page names in source, or false when it names none there. The id is the `source_id` of a page whose
// `source` is source; the revision and type are read per field (fieldOf). It reads only: no page changes.
func identityOf(fields map[string]string, source string) (PageIdentity, bool) {
	id := fieldOf(fields, source, sourceIDField)
	if id == "" {
		return PageIdentity{}, false
	}
	return PageIdentity{Source: source, ID: id, Revision: revisionOf(fields, source), Type: fieldOf(fields, source, sourceTypeField)}, true
}
