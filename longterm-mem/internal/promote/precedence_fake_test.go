package promote

import (
	"errors"
	"maps"
)

// memPrecedence is a PrecedenceRepository kept in memory: what it persists is a copy of the store it was
// given, as a file is, so a store changed after a save does not change what was saved. The file system
// adapter has its own tests (internal/vaultfs); promotion is tested here against this fake, which can also
// fail on demand and report the world at the instant the store is persisted.
type memPrecedence struct {
	// persisted is what the last successful save wrote; nil means nothing was ever persisted.
	persisted PrecedenceStore
	// saves counts the saves asked for, failed ones included.
	saves int
	// loadErr and saveErr, when set, are what the matching call answers.
	loadErr, saveErr error
	// atSave, when set, runs at the start of every save with the store being saved, before the save
	// succeeds or fails: a test looks at the vault at the instant the store is persisted.
	atSave func(store PrecedenceStore)
}

func (m *memPrecedence) LoadPrecedence() (PrecedenceStore, error) {
	if m.loadErr != nil {
		return nil, m.loadErr
	}
	return orEmpty(maps.Clone(m.persisted)), nil
}

func (m *memPrecedence) SavePrecedence(store PrecedenceStore) error {
	m.saves++
	if m.atSave != nil {
		m.atSave(store)
	}
	if m.saveErr != nil {
		return m.saveErr
	}
	m.persisted = orEmpty(maps.Clone(store))
	return nil
}

// orEmpty is store, or an empty store when it is nil: a load never answers a nil store without an error.
func orEmpty(store PrecedenceStore) PrecedenceStore {
	if store == nil {
		return PrecedenceStore{}
	}
	return store
}

var errTestSave = errors.New("the precedence store cannot be persisted")
