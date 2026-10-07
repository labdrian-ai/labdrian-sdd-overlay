package app

import (
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// RegistryError is why a use case could not work on the registry at Path: its store could not be
// read (Unreadable), or it was read and is not a registry the domain accepts. Err is the cause,
// in the words of the reader or of the domain; the CLI says which of the two it is.
type RegistryError struct {
	Path string
	Err  error
}

func (e *RegistryError) Error() string { return e.Err.Error() }
func (e *RegistryError) Unwrap() error { return e.Err }

// Unreadable reports whether the store of the registry could not be read at all (it does not
// exist, or the system refused it), as against a registry that was read and is unusable.
func (e *RegistryError) Unreadable() bool {
	return skills.IsUnreadableRegistry(e.Err)
}

// readRegistry reads the registry at path through the repository, and returns the warning that
// says what the reader left out of it, empty when it read the registry whole.
func readRegistry(registries skills.RegistryRepository, path string) (reg skills.Registry, unreadWarning string, err error) {
	reg, err = skills.ReadRegistry(registries, path)
	if err != nil {
		return skills.Registry{}, "", &RegistryError{Path: path, Err: err}
	}
	return reg, reg.UnreadWarning(), nil
}
