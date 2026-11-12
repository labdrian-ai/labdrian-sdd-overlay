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

// The store-agnostic keys a page names its identity with (D1 of the store-agnostic promotion).
const (
	sourceField         = "source"
	sourceIDField       = "source_id"
	sourceRevisionField = "source_revision"
	sourceTypeField     = "source_type"
)

// The stores a page can name. The page-identity keys are store-agnostic, so a store is a name here, not a type.
const (
	engramSource     = "engram"
	memorycardSource = "memorycard"
)

// legacyKeys are the keys of the pages written before the store-agnostic ones: one family per store.
var legacyKeys = map[string]struct{ id, revision, kind string }{
	engramSource:     {engramIDField, engramRevisionField, engramTypeField},
	memorycardSource: {memorycardIDField, memorycardRevisionField, memorycardTypeField},
}

// identityOf is the one reader of a page's identity, over the parsed fields of its frontmatter block: the
// identity the page names in source, or false when it names none there. It reads the store-agnostic keys
// (`source` naming source, with a `source_id`) first and, only when those do not name source, the legacy keys of
// that store. It reads only: no page changes.
func identityOf(fields map[string]string, source string) (PageIdentity, bool) {
	if strings.TrimSpace(fields[sourceField]) == source {
		if id := fields[sourceIDField]; id != "" {
			return PageIdentity{Source: source, ID: id,
				Revision: strings.TrimSpace(fields[sourceRevisionField]), Type: fields[sourceTypeField]}, true
		}
	}
	if keys, ok := legacyKeys[source]; ok {
		if id := fields[keys.id]; id != "" {
			return PageIdentity{Source: source, ID: id,
				Revision: strings.TrimSpace(fields[keys.revision]), Type: fields[keys.kind]}, true
		}
	}
	return PageIdentity{}, false
}
