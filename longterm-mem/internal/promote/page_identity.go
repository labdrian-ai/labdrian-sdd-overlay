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

// fieldOf reads one field of the identity source names: the store-agnostic key when the page names source in
// `source` and that key holds a value, otherwise the legacy key of the same store. The fallback is per field, so
// a page with only some of the store-agnostic keys still gives the rest from its legacy ones, and it never
// crosses stores. key is the key the value was read from (the legacy one when neither holds a value).
func fieldOf(fields map[string]string, source, agnosticKey, legacyKey string) (value, key string) {
	if strings.TrimSpace(fields[sourceField]) == source {
		if v := fields[agnosticKey]; strings.TrimSpace(v) != "" {
			return v, agnosticKey
		}
	}
	return fields[legacyKey], legacyKey
}

// revisionOf reads the revision a page records for source, trimmed and unparsed, with the key it was read from.
// It does not need the page to name an id: the revision of a page is read as it always was, by its own key.
func revisionOf(fields map[string]string, source string) (revision, key string) {
	keys, ok := legacyKeys[source]
	if !ok {
		return "", ""
	}
	revision, key = fieldOf(fields, source, sourceRevisionField, keys.revision)
	return strings.TrimSpace(revision), key
}

// identityOf is the one reader of a page's identity, over the parsed fields of its frontmatter block: the
// identity the page names in source, or false when it names none there. The id is the store-agnostic
// `source_id` of a page whose `source` is source or, failing that, the legacy id key of that store; the
// revision and type are then read per field (fieldOf). It reads only: no page changes.
func identityOf(fields map[string]string, source string) (PageIdentity, bool) {
	keys, ok := legacyKeys[source]
	if !ok {
		return PageIdentity{}, false
	}
	id, _ := fieldOf(fields, source, sourceIDField, keys.id)
	if id == "" {
		return PageIdentity{}, false
	}
	revision, _ := revisionOf(fields, source)
	kind, _ := fieldOf(fields, source, sourceTypeField, keys.kind)
	return PageIdentity{Source: source, ID: id, Revision: revision, Type: kind}, true
}
