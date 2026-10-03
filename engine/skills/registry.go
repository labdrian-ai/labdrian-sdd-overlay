package skills

import "errors"

// RegistryRepository is the port through which the skills domain reads and writes its own
// registry model. It is owned here and implemented by an adapter that knows where a registry is
// kept and in which format (engine/skills/registryyaml, the YAML file skills.registry.yaml): the
// domain knows neither, only that a registry has a location, can be loaded from it, and has a
// stored form that can be decoded and encoded.
//
// An adapter does not judge what it reads. Whether a registry may hold what it holds is the
// domain's rule (Registry.Validate), applied by the domain to what the adapter returns (ReadRegistry,
// DecodeRegistry); an adapter that called it would be deciding what the domain means.
//
// What an adapter says of a failure is a contract, because the domain judges before it reports:
//
//   - A store that could not be read at all is a *RegistryReadError, and the registry returned
//     with it is the zero value.
//   - A store that was read and could not be decoded returns the entries it did decode, whole and
//     in order, up to the fault, with an error that says the fault. The domain judges those
//     entries first, so what is named is the first fault in the order the store says it. A
//     decoder that stops at the fault and returns no entries makes a registry whose first entry
//     is invalid and whose second line is not understood report the second.
//
// Decode and Encode are the stored form of a registry as bytes. The verbs that write a registry
// (add, remove) encode what they will write and decode it back before they write it, so that
// what is written is what a later verb reads; the writing itself, which must be atomic and in step
// with the manifest, is not the repository's yet (Phase 9 unit H17 moves it).
type RegistryRepository interface {
	// Load reads the registry stored at location. Location is opaque to the domain: it is the
	// value of a verb's --registry flag, or what an adapter says of an overlay.
	Load(location string) (Registry, error)
	// Decode reads a registry from its stored form.
	Decode(data []byte) (Registry, error)
	// Encode returns the stored form of reg, the exact bytes Decode reads back as reg.
	Encode(reg Registry) ([]byte, error)
}

// RegistryReadError says that the store of a registry could not be read: it does not exist, it
// is not a file, the process may not read it. Nothing of the registry is known. Its text is the
// text of the failure it carries, which is how a person is told.
type RegistryReadError struct {
	Err error
}

func (e *RegistryReadError) Error() string { return e.Err.Error() }
func (e *RegistryReadError) Unwrap() error { return e.Err }

// errNoRegistryRepository is what a verb says when it was given no way to read a registry: a
// composition root that forgot it. It is a refusal, not a panic: a verb is never to crash.
var errNoRegistryRepository = errors.New("skills: no registry repository is wired")

// ReadRegistry loads the registry at location through repo and judges it. The error is a
// *RegistryReadError when the store could not be read, and otherwise the first fault of the
// registry: of an entry (Registry.Validate), or, when every entry the reading got through is
// fine, of the reading itself. The registry returned with an error is the zero value.
func ReadRegistry(repo RegistryRepository, location string) (Registry, error) {
	if repo == nil {
		return Registry{}, errNoRegistryRepository
	}
	return judged(repo.Load(location))
}

// DecodeRegistry reads the registry whose stored form is data through repo and judges it, as
// ReadRegistry does.
func DecodeRegistry(repo RegistryRepository, data []byte) (Registry, error) {
	if repo == nil {
		return Registry{}, errNoRegistryRepository
	}
	return judged(repo.Decode(data))
}

// judged applies the rule to what an adapter read, and tells the first fault: the entries came
// first in the store, so a fault in them is named before the fault of the reading that stopped
// after them.
func judged(reg Registry, readErr error) (Registry, error) {
	var unreadable *RegistryReadError
	if errors.As(readErr, &unreadable) {
		return Registry{}, readErr
	}
	if err := reg.Validate(); err != nil {
		return Registry{}, err
	}
	if readErr != nil {
		return Registry{}, readErr
	}
	return reg, nil
}
