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

// Save writes s to vaultRoot's sidecar precedence file via tmp+fsync+rename
// (D6, address.go's writeFileAtomic).
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
