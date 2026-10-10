package promote

import (
	"errors"
	"time"
)

// AddressMap is the address map of a vault: for each promoted page, the vault-relative path of its file
// against the address it was allocated. It is the address_map of the manifest wiki-ingest keeps, the other
// tools' view of which addresses are spent.
type AddressMap map[string]string

// AddressMapRecorder is the port through which promotion enters a new page in the vault's address map. The
// package owns it and never touches the manifest itself; the composition root hands the Writer an adapter
// (internal/vaultfs, which keeps the map in the vault's manifest, vaultlayout.AddressManifestFile), and a
// test hands it a fake.
type AddressMapRecorder interface {
	// RecordAddress makes the map name address for the page at path, and changes nothing else in it. The
	// manifest is wiki-ingest's, not this module's: whatever else it holds survives, and a manifest the vault
	// does not have yet is created, dated at createdAt, with the fields the vault's own tools expect of a
	// new one. It is durable: after it returns nil the entry survives the process, and a failure leaves the
	// manifest as it was.
	RecordAddress(path, address string, createdAt time.Time) error
}

// AddressMapReader is the port through which a page is checked against the vault's address map: the read
// half, which the diagnostics need and a promotion does not.
type AddressMapReader interface {
	// LoadAddressMap returns the map the manifest holds. A manifest that exists but cannot be understood as
	// one is an error for which errors.Is(err, ErrAddressMapCorrupt) holds; one that does not exist, or
	// cannot be read, is an error for which it does not, and no map.
	LoadAddressMap() (AddressMap, error)
}

// ErrAddressMapCorrupt is what a reader answers for a manifest that is there and is not an address map:
// not JSON, not an object, or an address_map that is not a map of paths to addresses.
var ErrAddressMapCorrupt = errors.New("promote: the address map is corrupt")
