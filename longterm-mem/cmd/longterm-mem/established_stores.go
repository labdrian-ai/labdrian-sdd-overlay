package main

import (
	"fmt"
	"os"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/adoption"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
)

// machineStores are the two stores longterm-mem is allowed to consult to learn whether memory already lives
// under a name: its own vault registry, and Engram's database, read-only. It is the adoption use case's Stores
// (adoption.Stores) on this machine; it lives here because only the composition root may name Engram's adapter.
type machineStores struct{}

var _ adoption.Stores = machineStores{}

// Open opens both, returning a note for each one that could not be consulted rather than an error. A store that
// cannot be read must not stop the command -- most subcommands never touch Engram -- but it must not read as
// "nothing is established" in silence either, so the caller reports every note it gets back.
func (machineStores) Open() (adoption.Established, []string) {
	s := &establishedNames{vaults: map[string]bool{}}
	var notes []string

	// Known, never seeded: consulting the registry must not create it. A machine where no command that writes
	// has been is taken to hold the row every machine starts with, as it is by the commands that read, so what a
	// repository's name resolves to does not depend on which command ran first.
	if reg, err := machineVaults().Known(); err == nil {
		for name := range reg.Vaults {
			s.vaults[name] = true
		}
	} else {
		notes = append(notes, fmt.Sprintf("the vault registry could not be read (%v)", err))
	}

	if store, err := engram.Open(os.Getenv(engramDBEnvVar)); err == nil {
		s.store = store
	} else {
		notes = append(notes, fmt.Sprintf("Engram's database could not be opened (%v)", err))
	}
	return s, notes
}

// establishedNames answers "does memory already live under this name?" from what Open could consult.
type establishedNames struct {
	vaults map[string]bool
	store  *engram.Store
}

func (s *establishedNames) HasMemory(name string) (bool, error) {
	if s.vaults[name] {
		return true, nil
	}
	if s.store == nil {
		return false, nil
	}
	return s.store.HasMemory(name)
}

func (s *establishedNames) Close() {
	if s.store != nil {
		s.store.Close()
	}
}
