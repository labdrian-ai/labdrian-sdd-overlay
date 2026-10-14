// Package embedindex is the use case behind `longterm-mem index --embeddings`: it builds, or brings up to date, the
// embedding index of a project from the project's live observations (R-069). It lists what is live, makes the
// embedder, and has the index built from the rows, in that order, and says which of the three stopped it when one
// did.
//
// The use case owns the ports it reads through. What is live is Observations, answered by the memory (the Engram
// adapter, which leaves out what was soft-deleted, R-020); the embedder is made by a function the caller hands
// in, because what it is made from (an endpoint, a model, whether a remote one is allowed) is the command's to
// decide; and the index is kept by a Builder, the adapter that owns the format and the place (internal/vecindex).
// Nothing here reaches a backend, opens a database or writes a file: the composition root hands the use case the
// adapters, and a test hands it tables.
//
// A Row is what the adapter that keeps the index takes to build from, and this package defines it: the adapter
// names the type, it does not own it. The query's top-up of an index builds from the same rows (LiveRows), so the
// mapping of an observation to a row is written once.
package embedindex

import (
	"context"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// Observations is the port through which the use case reads what to index: the live observations of a project,
// those that have not been soft-deleted (requirement R-020 in openspec/specs/longterm-mem-memory-access/spec.md),
// so that a row deleted since the last build is simply absent and the build's removal pass drops its entry.
type Observations interface {
	ListObservations(project string) ([]memory.Observation, error)
}

// Row is one live observation as an index is built from it: the id the index keys it by, and the two texts its
// vector and its fingerprint are made of.
type Row struct {
	EngramID int64
	Title    string
	Content  string
}

// RowsOf maps observations to the rows an index is built from, in the order they were given. No observation is an
// empty list, not nil.
func RowsOf(observations []memory.Observation) []Row {
	rows := make([]Row, 0, len(observations))
	for _, o := range observations {
		rows = append(rows, Row{EngramID: o.ID, Title: o.Title, Content: o.Content})
	}
	return rows
}

// LiveRows is the rows of a project's live observations, as source lists them. A failure to list is the source's
// own, untouched.
func LiveRows(source Observations, project string) ([]Row, error) {
	observations, err := source.ListObservations(project)
	if err != nil {
		return nil, err
	}
	return RowsOf(observations), nil
}

// Embedder makes the vector of a text.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// Contract is what an index is built under: the model that embeds, the number of dimensions of a vector, and how
// many characters of an observation are embedded. A row whose index entry was made under another contract is made
// again.
type Contract struct {
	Model      string
	Dimension  int
	InputLimit int
}

// Result says what a build did: how many rows it embedded (new ones, and ones whose entry no longer matched), how
// many it reused from the index as it was, and how many entries it dropped because their row is no longer live.
type Result struct {
	Embedded int
	Reused   int
	Removed  int
}

// Builder is the port through which an index is built and kept: the adapter that owns the format of the index and
// the place it lives in. It builds incrementally from the rows it is given, with the embedder it is given, and says
// what it did. A failure is its own, untouched.
type Builder interface {
	BuildRows(ctx context.Context, project string, contract Contract, rows []Row, embedder Embedder) (Result, error)
}

// UseCase is the build of an embedding index, over the ports it owns.
type UseCase struct {
	// Observations lists what is live.
	Observations Observations
	// NewEmbedder makes the embedder of this build, after the listing and before the build: a command that cannot
	// list never asks for a backend, and one whose backend is refused never builds.
	NewEmbedder func() (Embedder, error)
	// Builder keeps the index.
	Builder Builder
}

// Stage is the step of a build that stopped it.
type Stage int

const (
	// StageObservations: the live observations could not be listed.
	StageObservations Stage = iota + 1
	// StageEmbedder: the embedder could not be made.
	StageEmbedder
	// StageBuild: the index could not be built.
	StageBuild
)

// Error is a failure of a build, with the stage that stopped it. Its words are the failure's own, whatever the
// stage, because they are what an operator reads; the stage is for the one who decides what to answer.
type Error struct {
	Stage Stage
	Err   error
}

func (e *Error) Error() string { return e.Err.Error() }

func (e *Error) Unwrap() error { return e.Err }

// Build lists the live observations of project, makes the embedder, and has the index built from their rows under
// contract. A failure of any step stops the build and is an *Error naming the step; nothing after it runs.
func (u UseCase) Build(ctx context.Context, project string, contract Contract) (Result, error) {
	rows, err := LiveRows(u.Observations, project)
	if err != nil {
		return Result{}, &Error{Stage: StageObservations, Err: err}
	}
	embedder, err := u.NewEmbedder()
	if err != nil {
		return Result{}, &Error{Stage: StageEmbedder, Err: err}
	}
	result, err := u.Builder.BuildRows(ctx, project, contract, rows, embedder)
	if err != nil {
		return Result{}, &Error{Stage: StageBuild, Err: err}
	}
	return result, nil
}
