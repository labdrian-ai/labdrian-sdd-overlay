package promote

import (
	"fmt"
	"io/fs"
	"maps"
	"time"
)

// addressRecord is one call of RecordAddress, as it was asked.
type addressRecord struct {
	path, address string
	createdAt     time.Time
}

// memAddressMap is an AddressMapRecorder and an AddressMapReader kept in memory. A map of nil is a vault with
// no manifest yet: reading it fails as a file that is not there does, and the first record creates it, dated at
// the instant the record was given. The file system adapter has its own tests (internal/vaultfs); promotion
// and the page check are tested here against this fake, which can also fail on demand and tells what it was
// asked.
type memAddressMap struct {
	// entries is the address map the manifest holds; nil means there is no manifest.
	entries AddressMap
	// created is the instant the manifest was created at; zero while there is none.
	created time.Time
	// records lists every call of RecordAddress, failed ones included.
	records []addressRecord
	// loadErr and recordErr, when set, are what the matching call answers.
	loadErr, recordErr error
}

func (m *memAddressMap) LoadAddressMap() (AddressMap, error) {
	if m.loadErr != nil {
		return nil, m.loadErr
	}
	if m.entries == nil {
		return nil, fmt.Errorf("read the manifest: %w", fs.ErrNotExist)
	}
	return maps.Clone(m.entries), nil
}

func (m *memAddressMap) RecordAddress(path, address string, createdAt time.Time) error {
	m.records = append(m.records, addressRecord{path: path, address: address, createdAt: createdAt})
	if m.recordErr != nil {
		return m.recordErr
	}
	if m.entries == nil {
		m.entries = AddressMap{}
		m.created = createdAt
	}
	m.entries[path] = address
	return nil
}

// corruptAddressMap is the error a reader answers for a manifest that is there and is not an address map.
func corruptAddressMap() error {
	return fmt.Errorf("parse the manifest: %w", ErrAddressMapCorrupt)
}
