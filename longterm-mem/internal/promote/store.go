package promote

import "errors"

// PrecedenceEntry is one promoted page's last-written-by-longterm-mem
// fingerprint (D6): body and frontmatter hashed separately so a
// frontmatter-only patch (R-033, slice 7) can update just
// FrontmatterHash without disturbing BodyHash.
type PrecedenceEntry struct {
	BodyHash        string `json:"body_hash"`
	FrontmatterHash string `json:"frontmatter_hash"`
	// PromotedRevision is the engram_revision the fingerprinted render
	// carried. The two hashes above answer WHETHER the page still holds
	// our last write; this answers WHICH of our writes that was, and that
	// is the only evidence separating "the page is ahead of this sidecar
	// because our own write landed and the sidecar's did not" from "a
	// human edited the page" once the hashes have already diverged (see
	// isOwnUnrecordedUpdate).
	//
	// Zero means no revision was recorded -- an entry written before this
	// field existed, or one a status-only patch inherited from such an
	// entry -- and every reader treats that as no evidence and fails
	// closed. Conflating "absent" with revision 0 is safe here in the one
	// direction that matters: Engram's revision_count is NOT NULL DEFAULT
	// 1, so no promoted page legitimately carries revision 0, and reading
	// a page that somehow does as unrecorded only ever refuses a write it
	// might have adopted -- never the reverse. Kept a plain int rather
	// than a *int so a PrecedenceEntry stays comparable with ==, which is
	// how callers ask "did this entry change?"; a pointer would make two
	// decodes of the same sidecar compare unequal.
	PromotedRevision int `json:"promoted_revision,omitempty"`
}

// MatchesPage reports whether raw -- one promoted page's WHOLE file, the
// shape a scanner reads off disk -- still hashes to the two fingerprints
// this entry records. It is the same comparison UpdateInPlace makes before
// it decides whether a page has diverged, exported so a reader outside this
// package (doctor's precedence-sidecar check) asks the question exactly one
// way instead of re-deriving the frontmatter/body split and the digest.
//
// A file with no parseable frontmatter block reports false: promotion's own
// reader fails closed on it too, so it is a page this entry cannot be shown
// to still describe. That early return is a STATEMENT of that intent, not
// the thing enforcing it -- deleting it changes no outcome, because the
// degenerate split it would fall into (an empty frontmatter, a whole-file
// body) hashes the empty string, and no entry carries that digest: every
// FrontmatterHash is the digest of a rendered `---\n...` block, and a
// hand-truncated sidecar entry carries the empty STRING, which is not a
// digest at all. It is kept because a reader must not have to re-derive that
// argument to know an unparseable page is refused.
func (e PrecedenceEntry) MatchesPage(raw string) bool {
	fmBlock, ok := frontmatterBlock(raw)
	if !ok {
		return false
	}
	return e.FrontmatterHash == hashText(fmBlock) && e.BodyHash == hashText(raw[len(fmBlock):])
}

// PrecedenceStore is the sidecar precedence file's decoded form, keyed by
// page_address (D6): what longterm-mem itself last wrote for each
// promoted page, so a later re-promotion can detect a local edit (R-030).
type PrecedenceStore map[string]PrecedenceEntry

// PrecedenceRepository is the port through which promotion reads and
// persists the precedence store of one vault. The package owns it and never
// touches the sidecar file itself; the composition root hands the Writer an
// adapter (internal/vaultfs, which keeps the store in the vault's own
// sidecar, vaultlayout.PrecedenceFile), and a test hands it a fake.
//
// Both operations speak of the whole store: a Writer loads it once, changes
// it in memory as it promotes, and saves it after every promotion that wrote
// a page.
type PrecedenceRepository interface {
	// LoadPrecedence returns the persisted store. A vault with none yet
	// answers an empty store, not an error: nothing has been promoted under
	// this tracking. A store that exists and cannot be read or decoded is
	// an error and no store.
	LoadPrecedence() (PrecedenceStore, error)
	// SavePrecedence persists store, replacing what was persisted. It is
	// durable: after it returns nil the store survives the process, and a
	// failure leaves the previous store in place.
	SavePrecedence(store PrecedenceStore) error
}

// errNoPrecedenceRepository is what a Writer without a PrecedenceRepository
// answers: a port the caller forgot to wire is named, rather than nil-called
// after pages were already written.
var errNoPrecedenceRepository = errors.New("promote: the writer has no precedence repository")

// Get returns address's stored entry, if any.
func (s PrecedenceStore) Get(address string) (PrecedenceEntry, bool) {
	entry, ok := s[address]
	return entry, ok
}

// Set records address's entry.
func (s PrecedenceStore) Set(address string, entry PrecedenceEntry) {
	s[address] = entry
}
