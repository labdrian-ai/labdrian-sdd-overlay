package app

// RegistryStore is how the use case reaches the registry of a project, the markdown file that
// lists the skills and the contracts that apply to them. The use case owns the port; an adapter at
// the edge (engine/propagator/fsstore) answers it and the composition root wires the two. A test
// of the use case hands it a fake.
//
// The caller holds whatever lock the registry needs for as long as it needs it. The port takes
// none, because what a lock protects is the whole run, not one call.
type RegistryStore interface {
	// Read returns the content of the registry at path. A registry that does not exist is
	// reported as a *NotFoundError, so the use case can tell "there is none" from "it could not
	// be read" without looking at the words of an error. Any other error is a failure to read.
	Read(path string) ([]byte, error)
	// Write replaces the registry at path with content, whole: a reader running beside the
	// write sees the old registry or the new one, never half of either.
	Write(path string, content []byte) error
}

// ContractSource is where the text of the contract comes from: a file the person named, or a
// text the engine carries. The use case asks it once per pass, not once per run, because the
// source can change between two passes and the next pass must see it as it is then.
type ContractSource interface {
	// Text returns the document of the contract, frontmatter included.
	Text() (string, error)
}

// NotFoundError is what a RegistryStore returns from Read when there is no registry. It wraps the
// error the store met, and says what that error says, so a person reading it is told the reason
// the system gave and not a word of this package's.
type NotFoundError struct{ Err error }

func (e *NotFoundError) Error() string { return e.Err.Error() }

func (e *NotFoundError) Unwrap() error { return e.Err }
