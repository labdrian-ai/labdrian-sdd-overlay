package app

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// IDRequiredError is a verb that works on one skill, and was not told which.
type IDRequiredError struct{ Verb string }

func (e *IDRequiredError) Error() string {
	return fmt.Sprintf("skills %s requires an <id> argument", e.Verb)
}

// ErrRoundTripMismatch is a registry that, written and read back, is not the registry that was
// written: it is never put on disk.
var ErrRoundTripMismatch = errors.New("validate-before-write: serialize→parse round-trip mismatch")

// EncodeError is a registry that could not be written in the form the store keeps it.
type EncodeError struct{ Err error }

func (e *EncodeError) Error() string { return fmt.Sprintf("serializing registry: %v", e.Err) }
func (e *EncodeError) Unwrap() error { return e.Err }

// ReparseError is a registry that was written, and could not be read back: it is never put on
// disk.
type ReparseError struct{ Err error }

func (e *ReparseError) Error() string {
	return fmt.Sprintf("validate-before-write re-parse failed: %v", e.Err)
}
func (e *ReparseError) Unwrap() error { return e.Err }

// ManifestSyntaxError is a manifest, or the one a verb is about to write, that cannot be read.
type ManifestSyntaxError struct{ Err error }

func (e *ManifestSyntaxError) Error() string { return fmt.Sprintf("parsing manifest: %v", e.Err) }
func (e *ManifestSyntaxError) Unwrap() error { return e.Err }

// DivergenceError is a registry and a manifest, as a verb is about to write them, that disagree:
// nothing is written.
type DivergenceError struct{ Divergences []skills.Divergence }

func (e *DivergenceError) Error() string {
	return fmt.Sprintf("the registry and the manifest would diverge (%d)", len(e.Divergences))
}

// encodeVerified encodes newReg in the form the store keeps it and proves the store reads back
// what was written (ADR-9, validate-before-write): it returns the bytes to write and the registry
// as the store reads them. Nothing is written here.
func encodeVerified(registries skills.RegistryRepository, newReg skills.Registry) ([]byte, skills.Registry, error) {
	regBytes, err := registries.Encode(newReg)
	if err != nil {
		return nil, skills.Registry{}, &EncodeError{Err: err}
	}
	reread, err := skills.DecodeRegistry(registries, regBytes)
	if err != nil {
		return nil, skills.Registry{}, &ReparseError{Err: err}
	}
	if !reflect.DeepEqual(newReg, reread) {
		return nil, skills.Registry{}, ErrRoundTripMismatch
	}
	return regBytes, reread, nil
}

// crossCheck proves the registry and the manifest agree: a manifest that cannot be read is a
// *ManifestSyntaxError, and one that disagrees a *DivergenceError.
func crossCheck(reg skills.Registry, manifest []byte) error {
	divs, err := skills.ValidateAgainstManifest(reg, manifest)
	switch {
	case len(divs) > 0:
		return &DivergenceError{Divergences: divs}
	case err != nil:
		return &ManifestSyntaxError{Err: err}
	}
	return nil
}
