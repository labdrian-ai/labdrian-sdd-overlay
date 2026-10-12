package vecindex

import (
	"context"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/query"
)

// Repository is the adapter behind the two ports internal/query owns for its embedding arm: the repository that
// reads a project's index and says what a row's fingerprint is (query.EmbeddingIndexRepository), and the builder
// that closes a small gap before a scan (query.EmbeddingIndexBuilder). It keeps the indexes under StateDir
// (StateDir/index/<project>/, Dir), in the format this package defines, and the composition root builds it and
// hands it to the query.
type Repository struct {
	// StateDir is the directory the indexes live under.
	StateDir string
	// Cache, when set, is what an index is read through, so that a long-lived caller reads a project's index from
	// disk at most once per change; a build that wrote forgets what it cached for the project. None reads the
	// disk every time.
	Cache *LoadCache
	// LiveRows reads the project's live rows in the shape Build wants. It is asked by BuildEmbeddingIndex only.
	LiveRows func(project string) ([]Row, error)
	// NewEmbedder makes the embedder of model. It is asked by BuildEmbeddingIndex only, once per build.
	NewEmbedder func(model string) (Embedder, error)
	// Now is the clock a build stamps its index with (Build's own seam).
	Now func() string
}

// LoadEmbeddingIndex reads project's index and answers it in the terms of the query. It refuses as Load does
// (ErrNoIndex for a project nothing was built for, an error wrapping ErrCorrupted for an index that cannot be
// trusted), untouched, so that a caller who cares can tell which.
func (r *Repository) LoadEmbeddingIndex(project string) (*query.EmbeddingIndex, error) {
	dir := Dir(r.StateDir, project)
	var idx *Index
	var err error
	if r.Cache != nil {
		idx, err = r.Cache.Load(dir)
	} else {
		idx, err = Load(dir)
	}
	if err != nil {
		return nil, err
	}
	entries := make([]query.IndexedObservation, len(idx.Manifest.Entries))
	for i, e := range idx.Manifest.Entries {
		entries[i] = query.IndexedObservation{EngramID: e.EngramID, Fingerprint: e.Fingerprint}
	}
	return &query.EmbeddingIndex{
		Model: idx.Manifest.Model, Dimension: idx.Manifest.Dimension, InputLimit: idx.Manifest.InputLimit,
		BuiltAt: idx.Manifest.BuiltAt, Entries: entries, Vectors: idx.Vectors,
	}, nil
}

// Fingerprint is the package's rule for what an index records of a row (Fingerprint).
func (r *Repository) Fingerprint(model string, dimension, inputLimit int, title, content string) string {
	return Fingerprint(model, dimension, inputLimit, title, content)
}

// BuildEmbeddingIndex builds project's index incrementally (Build) under the given contract, from the project's
// live rows and the embedder of the model, into the project's directory under StateDir. When the build wrote,
// the cache forgets the project's index; when it failed, nothing on disk changed and nothing is forgotten. Every
// failure is the stage's own, untouched, and the stages after it do not run.
func (r *Repository) BuildEmbeddingIndex(ctx context.Context, project, model string, dimension, inputLimit int) error {
	rows, err := r.LiveRows(project)
	if err != nil {
		return err
	}
	embedder, err := r.NewEmbedder(model)
	if err != nil {
		return err
	}
	dir := Dir(r.StateDir, project)
	if _, _, err := Build(ctx, dir, model, dimension, inputLimit, rows, embedder, r.Now); err != nil {
		return err
	}
	if r.Cache != nil {
		r.Cache.Invalidate(dir)
	}
	return nil
}
