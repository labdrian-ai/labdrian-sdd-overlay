package promote

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// precedenceManifestRelPath is longterm-mem's own sidecar file (D6),
// vault-relative -- unlike .raw/.manifest.json (wiki-ingest-owned,
// address.go), this file has exactly one writer (this package), so a
// closed struct is safe here.
const precedenceManifestRelPath = ".raw/.longterm-mem-manifest.json"

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

// PrecedenceStore is the sidecar precedence file's decoded form, keyed by
// page_address (D6): what longterm-mem itself last wrote for each
// promoted page, so a later re-promotion can detect a local edit (R-030).
type PrecedenceStore map[string]PrecedenceEntry

// LoadPrecedenceStore reads vaultRoot's sidecar precedence file. A missing
// file returns an empty store, not an error -- nothing has been promoted
// under this tracking yet.
func LoadPrecedenceStore(vaultRoot string) (PrecedenceStore, error) {
	full := filepath.Join(vaultRoot, precedenceManifestRelPath)
	data, err := os.ReadFile(full)
	if err != nil {
		if os.IsNotExist(err) {
			return PrecedenceStore{}, nil
		}
		return nil, fmt.Errorf("promote: read %s: %w", full, err)
	}
	store := PrecedenceStore{}
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, fmt.Errorf("promote: parse %s: %w", full, err)
	}
	return store, nil
}

// Save writes s to vaultRoot's sidecar precedence file through
// address.go's writeFileAtomic (D6), which durably replaces it.
func (s PrecedenceStore) Save(vaultRoot string) error {
	full := filepath.Join(vaultRoot, precedenceManifestRelPath)
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("promote: marshal %s: %w", full, err)
	}
	return writeFileAtomic(full, append(data, '\n'))
}

// Get returns address's stored entry, if any.
func (s PrecedenceStore) Get(address string) (PrecedenceEntry, bool) {
	entry, ok := s[address]
	return entry, ok
}

// Set records address's entry.
func (s PrecedenceStore) Set(address string, entry PrecedenceEntry) {
	s[address] = entry
}
