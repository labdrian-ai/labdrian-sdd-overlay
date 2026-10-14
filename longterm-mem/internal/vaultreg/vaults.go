package vaultreg

import (
	"errors"
	"io/fs"
)

// Store is where the registry is kept: the port through which Vaults reads it and, when a command that writes asks
// for it, seeds it. internal/vaultregfs is the adapter that answers it on a file system.
type Store interface {
	// Exists says whether a registry has been written.
	Exists() (bool, error)
	// Load reads the registry. A registry that has not been written is an error that wraps fs.ErrNotExist.
	Load() (Registry, error)
	// Save writes the registry, so that a reader never meets half of it.
	Save(Registry) error
}

// Vaults finds the vault of a project on a machine: the Store it keeps its registry in, the Defaults that registry
// holds when nobody has written one, and what the machine's environment says. The composition root builds one for
// the run of a command; it holds no state of its own.
//
// The registry is made by a command that writes (ToWrite) and by none that only reads (ToRead): a machine that has
// no registry is taken to hold the Defaults, to the reader and to whoever asks what the registry knows (Known), so
// that nothing a reader says depends on whether a writer has been there first.
type Vaults struct {
	// Store is where the registry is kept.
	Store Store
	// Defaults are the rows a registry starts with: the row of the one project every machine is taken to have a
	// vault for. They name the project and the path that no rule of this package knows, and where the registry
	// would be kept (Origin), for the words of a project that has no row.
	Defaults Registry
	// Environment is what Resolve is told of the machine.
	Environment Environment
}

// ToWrite is the vault of project for a command that writes into it: the flag, the environment's, or the project's
// row in the registry, which is seeded with the Defaults first when nobody has written one (D5). A registry that
// exists is never touched, even one that lacks the default row: deleting a row means "not configured", not "seed it
// again". When the flag or the environment names the vault the registry is not consulted, and so not created.
func (v Vaults) ToWrite(project, flag string) (string, error) {
	return v.resolve(project, flag, v.seeded)
}

// ToRead is the vault of project for a command that only reads: the same answer ToWrite gives, without making the
// registry. On a machine that has none, the Defaults are the registry, so the answer is the one the first writer
// would have had, and the machine is as it was.
func (v Vaults) ToRead(project, flag string) (string, error) {
	return v.resolve(project, flag, v.read)
}

// resolve is Resolve over the registry that registry answers, asked for only when the flag and the environment do
// not name the vault.
func (v Vaults) resolve(project, flag string, registry func() (Registry, error)) (string, error) {
	if Overridden(flag, v.Environment) {
		return Resolve(flag, v.Environment, Registry{}, project)
	}
	reg, err := registry()
	if err != nil {
		return "", err
	}
	return Resolve(flag, v.Environment, reg, project)
}

// Known is the registry as far as the program knows it, for whoever asks which names it holds a row for: the
// registry that is kept, or the Defaults on a machine where none is. A registry that cannot be read is the error
// of the read, never an absence.
func (v Vaults) Known() (Registry, error) {
	reg, err := v.Store.Load()
	if errors.Is(err, fs.ErrNotExist) {
		return v.Defaults, nil
	}
	return reg, err
}

// seeded is the registry, written from the Defaults first when there is none.
func (v Vaults) seeded() (Registry, error) {
	exists, err := v.Store.Exists()
	if err != nil {
		return Registry{}, err
	}
	if !exists {
		if err := v.Store.Save(v.Defaults); err != nil {
			return Registry{}, err
		}
	}
	return v.Store.Load()
}

// read is the registry that is kept, or the Defaults when there is none, and writes nothing.
func (v Vaults) read() (Registry, error) {
	exists, err := v.Store.Exists()
	if err != nil {
		return Registry{}, err
	}
	if !exists {
		return v.Defaults, nil
	}
	return v.Store.Load()
}
