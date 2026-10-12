package query

import (
	"context"
	"errors"
)

// ErrNoEmbeddingIndex is what an EmbeddingIndexRepository answers for a project nothing was built for: a typed
// outcome, found with errors.Is, as the arm tells an index nobody built (nothing to say) from one that is there
// and cannot be read (said beside the answer, DiagnosticEmbeddingIndexUnreadable). The words of an error are
// never read to tell the two apart.
var ErrNoEmbeddingIndex = errors.New("query: no embedding index was built for this project")

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
//
// An index is read-only to whoever receives it. A repository that caches may answer every caller the same slices
// (the vecindex adapter shares its Vectors), not copies, so the arm only reads them
// (TestTheEmbeddingArmOnlyReadsTheIndexItIsGiven) and so must any other reader.
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
	// LoadEmbeddingIndex returns project's index. An error means there is no index the arm can use. A project
	// nothing was built for is answered with an error that is ErrNoEmbeddingIndex (wrapped or not); any other
	// error is an index that is there and cannot be trusted or read. The arm degrades the same way for both (no
	// rows, a Coverage that says "never built" and the command that builds one), and says the second beside
	// the answer with the error's own text. The words of the error are never read to tell them apart.
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
