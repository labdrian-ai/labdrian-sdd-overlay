package vaultfs

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/promote"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultlayout"
)

// LoadPrecedence reads the vault's precedence sidecar, longterm-mem's own record of what it last wrote for
// each promoted page. A vault with no sidecar yet answers an empty store, not an error: nothing has been
// promoted under this tracking. A sidecar that cannot be read, or is not a store, is an error that names
// the file and no store.
//
// The errors begin "promote: read" and "promote: parse", as they did when promotion read the file itself.
func (v *Vault) LoadPrecedence() (promote.PrecedenceStore, error) {
	full := v.layout.Path(vaultlayout.PrecedenceFile)
	data, err := os.ReadFile(full)
	if err != nil {
		if os.IsNotExist(err) {
			return promote.PrecedenceStore{}, nil
		}
		return nil, fmt.Errorf("promote: read %s: %w", full, err)
	}
	store := promote.PrecedenceStore{}
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, fmt.Errorf("promote: parse %s: %w", full, err)
	}
	return store, nil
}

// SavePrecedence writes store to the vault's precedence sidecar as JSON indented by two spaces and ended by a
// newline, replacing the file atomically; it creates the file owner-only, and keeps the mode of one that
// exists. A failure leaves the sidecar as it was.
func (v *Vault) SavePrecedence(store promote.PrecedenceStore) error {
	full := v.layout.Path(vaultlayout.PrecedenceFile)
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("promote: marshal %s: %w", full, err)
	}
	return writeFileAtomic(full, append(data, '\n'))
}
