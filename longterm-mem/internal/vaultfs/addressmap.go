package vaultfs

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/promote"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/vaultlayout"
)

// corruptManifest is the failure to understand the manifest as an address map. It says what its cause says,
// word for word, and is promote.ErrAddressMapCorrupt to whoever asks.
type corruptManifest struct{ cause error }

func (e corruptManifest) Error() string { return e.cause.Error() }
func (e corruptManifest) Unwrap() error { return e.cause }
func (e corruptManifest) Is(target error) bool {
	return target == promote.ErrAddressMapCorrupt
}

// LoadAddressMap reads the address map out of the vault's manifest, the file wiki-ingest keeps and this
// module extends. A manifest with no address_map, or JSON null, has an empty map. One that cannot be read
// (absent, a directory) is an error that names the file and is not the error for a corrupt one; one that
// can be read and is not an address map is, for every way it can fail to be one.
//
// The errors say "read" or "parse" and name the file, behind the Vault's error prefix if it has one.
func (v *Vault) LoadAddressMap() (promote.AddressMap, error) {
	full := v.layout.Path(vaultlayout.AddressManifestFile)
	data, err := os.ReadFile(full)
	if err != nil {
		return nil, v.fail(fmt.Errorf("read %s: %w", full, err))
	}
	var manifest struct {
		AddressMap map[string]string `json:"address_map"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, v.fail(corruptManifest{fmt.Errorf("parse %s: %w", full, err)})
	}
	return manifest.AddressMap, nil
}

// RecordAddress makes the vault's address map name address for the page at path. The manifest is read as an
// open set of keys, because it is wiki-ingest's: fields this module does not know survive, keys absent from
// the file are never fabricated, and only address_map[path] changes. The result is indented by two spaces
// and ended by a newline, and replaces the manifest atomically, as the module's other files in the vault.
//
// Only a manifest that is wholly absent starts from the minimal seed a new manifest has: version 1, created
// at the day of createdAt, no sources. One that is there and cannot be read or understood is an error that
// names the file, and is left as it was.
//
// The errors say "read", "parse" or "marshal" and name the file, behind the Vault's error prefix if it has
// one.
func (v *Vault) RecordAddress(path, address string, createdAt time.Time) error {
	full := v.layout.Path(vaultlayout.AddressManifestFile)
	manifest := map[string]json.RawMessage{}

	// A manifest that holds JSON null decodes to a nil map, and the write of address_map below then panics.
	// That is how the code this was moved from has always behaved; the batch 32 ledger records it for the
	// owner's decision, and this move does not change it.
	if data, err := os.ReadFile(full); err == nil {
		if err := json.Unmarshal(data, &manifest); err != nil {
			return v.fail(fmt.Errorf("parse %s: %w", full, err))
		}
	} else if os.IsNotExist(err) {
		manifest["version"] = json.RawMessage("1")
		manifest["created"] = json.RawMessage(strconv.Quote(createdAt.Format("2006-01-02")))
		manifest["sources"] = json.RawMessage("{}")
	} else {
		return v.fail(fmt.Errorf("read %s: %w", full, err))
	}

	addressMap := map[string]string{}
	if raw, ok := manifest["address_map"]; ok {
		if err := json.Unmarshal(raw, &addressMap); err != nil {
			return v.fail(fmt.Errorf("parse %s address_map: %w", full, err))
		}
	}
	addressMap[path] = address
	encoded, err := json.Marshal(addressMap)
	if err != nil {
		return v.fail(fmt.Errorf("marshal %s address_map: %w", full, err))
	}
	manifest["address_map"] = encoded

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return v.fail(fmt.Errorf("marshal %s: %w", full, err))
	}
	if err := writeFileAtomic(full, append(data, '\n')); err != nil {
		return v.fail(err)
	}
	return nil
}
