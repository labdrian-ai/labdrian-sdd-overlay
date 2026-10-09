package app

import "fmt"

// The errors below say what the use case refused or could not do. Each carries what a caller
// needs to tell a person (the registry, the number of attempts, the error underneath) and none of
// the words of a terminal: the command that prints them chooses its own. Error() is a plain
// sentence for a log or a test failure.

// ContractReadError: the text of the contract could not be had.
type ContractReadError struct{ Err error }

func (e *ContractReadError) Error() string { return fmt.Sprintf("reading the contract: %v", e.Err) }
func (e *ContractReadError) Unwrap() error { return e.Err }

// ContractParseError: the text of the contract is not a contract the use case can scope a row
// from. Err is the parser's error, which names what is wrong with the document.
type ContractParseError struct{ Err error }

func (e *ContractParseError) Error() string { return fmt.Sprintf("parsing the contract: %v", e.Err) }
func (e *ContractParseError) Unwrap() error { return e.Err }

// RegistryReadError: the registry exists, or may, and could not be read.
type RegistryReadError struct {
	Path string
	Err  error
}

func (e *RegistryReadError) Error() string {
	return fmt.Sprintf("reading the registry %s: %v", e.Path, e.Err)
}
func (e *RegistryReadError) Unwrap() error { return e.Err }

// RegistryRequiredError: the request said the registry must exist, and it does not.
type RegistryRequiredError struct{ Path string }

func (e *RegistryRequiredError) Error() string {
	return fmt.Sprintf("the registry %s is required and not found", e.Path)
}

// RewriteError: the domain could not scope the row into the registry it was given.
type RewriteError struct{ Err error }

func (e *RewriteError) Error() string { return fmt.Sprintf("scoping the row: %v", e.Err) }
func (e *RewriteError) Unwrap() error { return e.Err }

// RegistryWriteError: the registry could not be written.
type RegistryWriteError struct {
	Path string
	Err  error
}

func (e *RegistryWriteError) Error() string {
	return fmt.Sprintf("writing the registry %s: %v", e.Path, e.Err)
}
func (e *RegistryWriteError) Unwrap() error { return e.Err }

// EmptyRegistryError: the registry was empty on every one of the attempts, and nothing else was
// seen. Writing over it was refused each time.
type EmptyRegistryError struct {
	Path     string
	Attempts int
}

func (e *EmptyRegistryError) Error() string {
	return fmt.Sprintf("the registry %s was empty on all %d attempts", e.Path, e.Attempts)
}

// InconsistentRegistryError: the attempts did not agree on what the registry is. It was seen
// absent and empty in turn, or it was seen to exist (a write landed on it) and then to be absent
// or empty. It exists, so the run does not conclude that the project is without a registry, and
// it does not conclude what the registry holds either.
type InconsistentRegistryError struct {
	Path     string
	Attempts int
}

func (e *InconsistentRegistryError) Error() string {
	return fmt.Sprintf("the registry %s was in an inconsistent state across %d attempts", e.Path, e.Attempts)
}

// UnverifiedWriteError: a write was made and the registry could not be read back to confirm it,
// on every attempt that made one. Err is the error of the last read-back.
type UnverifiedWriteError struct {
	Path     string
	Attempts int
	Err      error
}

func (e *UnverifiedWriteError) Error() string {
	return fmt.Sprintf("the write to %s could not be verified after %d attempts: %v", e.Path, e.Attempts, e.Err)
}
func (e *UnverifiedWriteError) Unwrap() error { return e.Err }

// LostWriteError: a write was made, the registry was read back, and it held other bytes, on every
// attempt that made a write. A writer outside the lock is replacing the registry as fast as this
// one writes it.
type LostWriteError struct {
	Path     string
	Attempts int
}

func (e *LostWriteError) Error() string {
	return fmt.Sprintf("the write to %s did not persist after %d attempts", e.Path, e.Attempts)
}
