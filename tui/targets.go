package main

import (
	"errors"
	"fmt"
)

// TargetKind says how the backend deploys to a target. It is the backend's
// own classification (the second column of `labdrian-overlay targets`), not a
// guess made here.
type TargetKind string

const (
	// KindCopy targets receive individual files copied into a directory
	// (claude, opencode, codex today). Actions that work file by file, such as
	// capture and restore, only apply to these.
	KindCopy TargetKind = "copy"
	// KindPackage targets are built and installed as a whole package (pi
	// today); the backend refuses the file-by-file actions for them.
	KindPackage TargetKind = "package"
)

// Target is one deployment target, exactly as the backend's catalog lists it.
// The TUI keeps no list of targets of its own: every Target comes from a
// TargetCatalog.
type Target struct {
	Name string
	Kind TargetKind
}

// targetScope is the catalog the operator was shown, together with the means
// of checking it is still the backend's. It exists for one invariant: the TUI
// never acts on a target the operator did not see. Every explicit
// `--target <name>` names a target from the shown catalog; `--target all` is
// the only argument that can name more than the selection, so it is allowed
// only when it provably means the shown catalog and nothing else.
type targetScope struct {
	shown   []Target
	catalog TargetCatalog
}

// coversAll reports whether `--target all` may stand for selected: the
// selection must be the whole catalog that was shown. An empty catalog never
// qualifies, since two empty sets are equal and `all` would then act on every
// target the backend has.
func (s targetScope) coversAll(selected []Target) bool {
	return len(s.shown) > 0 && sameTargetNames(selected, s.shown)
}

// confirmUnchanged asks the backend for its catalog again and fails unless it
// is the one that was shown. It runs right before each `--target all`: the
// backend can change under a running TUI (self-update moves the checkout the
// script lives in, and the apply chained after it then runs the new script),
// and `all` is defined by the backend at the moment it runs, not by what the
// operator saw earlier.
func (s targetScope) confirmUnchanged() error {
	if s.catalog == nil {
		return errors.New("no hay catálogo de destinos con el que verificar --target all")
	}
	current, err := s.catalog.Targets()
	if err != nil {
		return fmt.Errorf("no se pudo releer el catálogo de destinos del backend: %w", err)
	}
	if !sameTargetNames(current, s.shown) {
		return errors.New("el catálogo de destinos del backend cambió desde que se mostró: " +
			"--target all alcanzaría destinos que no viste")
	}
	return nil
}

// sameTargetNames reports whether a and b name exactly the same targets, in
// any order. It is the test for "this selection is the whole catalog": the
// only case in which `--target all` is allowed to stand for the selection. A
// name repeated on either side makes the two unequal, since neither a catalog
// nor a selection of catalog entries ever repeats one.
func sameTargetNames(a, b []Target) bool {
	if len(a) != len(b) {
		return false
	}
	pending := make(map[string]bool, len(a))
	for _, t := range a {
		pending[t.Name] = true
	}
	for _, t := range b {
		if !pending[t.Name] {
			return false
		}
		delete(pending, t.Name)
	}
	return len(pending) == 0
}
