package vaultreg

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
type Vaults struct {
	// Store is where the registry is kept.
	Store Store
	// Defaults are the rows a registry starts with: the row of the one project every machine is taken to have a
	// vault for. They name the project and the path that no rule of this package knows.
	Defaults Registry
	// Environment is what Resolve is told of the machine.
	Environment Environment
}

// Resolve is the vault of project: the flag, the environment's, or the project's row in the registry, which is
// seeded with the Defaults first when nobody has written one (D5). A registry that exists is never touched, even
// one that lacks the default row: deleting a row means "not configured", not "seed it again". When the flag or the
// environment names the vault the registry is not consulted, and so not created.
func (v Vaults) Resolve(project, flag string) (string, error) {
	if Overridden(flag, v.Environment) {
		return Resolve(flag, v.Environment, Registry{}, project)
	}
	reg, err := v.seeded()
	if err != nil {
		return "", err
	}
	return Resolve(flag, v.Environment, reg, project)
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
