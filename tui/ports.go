package main

// The ports the TUI's application logic depends on. The logic (model.go,
// run.go) talks to these interfaces and never runs a process or reads a file
// to answer them; the adapter that does lives in overlaycli.go, and main.go
// is the only place that picks it.

// TargetCatalog is where the TUI learns which targets exist. It is the single
// source: the TUI keeps no list of its own, so what it shows is what the
// backend's `--target all` acts on.
type TargetCatalog interface {
	// Targets returns the backend's targets in the order it lists them. An
	// error means the catalog is unknown, and the caller must then offer no
	// target action rather than fall back to a guess.
	Targets() ([]Target, error)
}
