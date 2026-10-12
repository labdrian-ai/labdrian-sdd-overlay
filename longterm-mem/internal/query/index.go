package query

import "context"

// IndexedObservation is one row an embedding index vouches for: which observation it embeds, and the fingerprint
// that was true when it was embedded (the index's own account of the text and the contract the vector came
// from; EmbeddingIndexRepository.Fingerprint says whether it still holds).
type IndexedObservation struct {
	EngramID    int64
	Fingerprint string
}

// EmbeddingIndex is one project's embedding index as the embedding arm reads it: the contract its vectors were
// produced under (the model, how many numbers a vector has, how many characters of a row were embedded), when it
// was built, and every indexed row with its vector. Vectors[i] is the vector of Entries[i]; the repository
// answers an index in which the two agree.
type EmbeddingIndex struct {
	Model      string
	Dimension  int
	InputLimit int
	// BuiltAt is the instant the index says it was last built at, RFC 3339.
	BuiltAt string
	Entries []IndexedObservation
	Vectors [][]float32
}

// EmbeddingIndexRepository is the port through which the embedding arm reads the embedding index of a project.
// The package owns it; the embedding index adapter (internal/vecindex) satisfies it, and where an index is kept,
// how it is stored and how it is cached are the adapter's to say.
type EmbeddingIndexRepository interface {
	// LoadEmbeddingIndex returns project's index. An error means there is no index the arm can use: none was
	// ever built, or the one there cannot be trusted or read. The arm degrades the same way for every cause (no
	// rows, a Coverage that says "never built" and the command that builds one), so the words of the error are
	// not read.
	LoadEmbeddingIndex(project string) (*EmbeddingIndex, error)
	// Fingerprint is what an index built under (model, dimension, inputLimit) records for a row with this title
	// and content. A row whose recorded fingerprint differs has changed since it was embedded, and its vector no
	// longer describes it. How the text is embedded and hashed is the index's rule, not the query's.
	Fingerprint(model string, dimension, inputLimit int, title, content string) string
}

// EmbeddingIndexBuilder is the port through which the embedding arm closes a small gap between an index and the
// live corpus before it scans (see TopUpMaxRows): one incremental build of project's index under the given
// contract, which is the one the arm just read off the existing index, so a top-up never reinterprets what "the
// index" means for the project. A build that wrote makes the next LoadEmbeddingIndex answer what it wrote.
type EmbeddingIndexBuilder interface {
	BuildEmbeddingIndex(ctx context.Context, project, model string, dimension, inputLimit int) error
}
