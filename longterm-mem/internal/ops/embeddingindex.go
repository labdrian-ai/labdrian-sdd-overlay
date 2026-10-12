package ops

import "errors"

// EmbeddingIndexReader is the port through which Status and Doctor read the embedding index of the project they
// inspect. The package owns it, asks for the read half only (a diagnostic never builds, repairs or removes an
// index), and states it in plain types: what an index is made of, where it is kept and how it is stored are the
// adapter's to say (internal/vecindex's Repository satisfies it), and the failures it returns say what they could
// not read in the adapter's own words, which the checks print.
//
// An index nobody built is told by what its error says it is (NoEmbeddingIndex), found with errors.As however it
// is wrapped, and never by its words. Any other error is an index that is there and cannot be read or trusted.
type EmbeddingIndexReader interface {
	// EmbeddingIndexLocation says where project's index is kept, which the checks name when they say they looked
	// for one or could not read it.
	EmbeddingIndexLocation(project string) string
	// EmbeddingIndexBuiltAt returns the instant project's index says it was last built at. It is what Status
	// reports, and what Doctor asks to find out whether there is an index it can read.
	EmbeddingIndexBuiltAt(project string) (string, error)
	// IndexedEngramIDs returns the engram id of every row project's index vouches for, for the check that says
	// how many live rows it lacks.
	IndexedEngramIDs(project string) ([]int64, error)
}

// NoEmbeddingIndex is what an error from an EmbeddingIndexReader implements when nothing was ever built for the
// project. Doctor reports it as a missing index, naming the command that builds one, and any other failure of
// the port as an index that could not be loaded.
type NoEmbeddingIndex interface {
	error
	NoEmbeddingIndex()
}

// EmbedModelMissing is what an error from the embedding backend implements when the backend answered but the
// model the index needs is not pulled (R-070). Doctor finds it with errors.As, however the error is wrapped, and
// reports it apart from a backend that did not answer, which is every other failure of the probe. The embedding
// client's own error says it is one by this method: this package names no client.
type EmbedModelMissing interface {
	error
	EmbedModelMissing()
}

// errNoEmbeddingIndexReader is what the checks that read the index report, and Status answers, when they were
// built without an EmbeddingIndexReader. It begins with the name of its package, as every error of the package
// does; it is the output of the commands, pinned in embeddingindex_port_test.go.
var errNoEmbeddingIndexReader = errors.New("ops: no embedding index reader was wired")
