package vecindex

import (
	"context"
	"errors"
	"fmt"
)

// Row is one candidate observation for (re)indexing. Callers (the CLI's
// `index --embeddings` command) supply only live rows -- typically via
// engram.Store.ListObservations, which is already R-020 (soft-delete)
// scoped -- so Build never has to ask which rows are live a second time.
type Row struct {
	EngramID int64
	Title    string
	Content  string
}

// Embedder is the seam Build calls into for one row's vector. Production
// wires embed.Client.Embed (internal/embed); tests use a fake. Build never
// imports internal/embed itself: net_allowlist_test.go's allowlist would
// refuse this package importing net/http transitively if it depended on
// the client concretely, and the seam keeps Build's own tests free of any
// network dependency.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// BuildResult reports what one Build call did, so a caller (doctor,
// `index --embeddings`'s own summary line) can say something more useful
// than "done" (R-069).
type BuildResult struct {
	// Embedded counts rows that were (re-)embedded: new rows plus rows
	// whose fingerprint no longer matched the existing index.
	Embedded int
	// Reused counts rows whose fingerprint matched the existing index
	// exactly, so their vector was carried over without calling Embedder.
	Reused int
	// Removed counts index entries dropped because their row was not
	// present in rows (soft-deleted, or otherwise no longer live).
	Removed int
}

// Build performs one incremental index build (R-069): it loads dir's
// existing index when one exists, embeds only rows whose fingerprint is
// missing or has changed, reuses every other row's vector unchanged, drops
// entries for rows no longer present in rows, and persists the result.
//
// model, dim, and inputLimit describe the embedding contract that produced
// (or will produce) every vector; a changed contract makes every existing
// fingerprint mismatch and so re-embeds the whole corpus, which is the
// mechanical invalidation the fingerprint's own design is built to
// guarantee (see Fingerprint).
//
// It refuses -- wrapping ErrCorrupted, never silently rebuilding from
// scratch -- when dir already holds a corrupted index: masking that would
// defeat the whole point of Load reporting it. now is a seam for tests;
// production passes a function returning time.Now().UTC().Format(time.RFC3339).
func Build(ctx context.Context, dir, model string, dim, inputLimit int, rows []Row, embedder Embedder, now func() string) (*Index, BuildResult, error) {
	existing, err := Load(dir)
	switch {
	case errors.Is(err, ErrNoIndex):
		existing = &Index{}
	case err != nil:
		return nil, BuildResult{}, fmt.Errorf("vecindex: build: load existing index at %s: %w", dir, err)
	}

	existingByID := make(map[int64]struct {
		fingerprint string
		vector      []float32
	}, len(existing.Manifest.Entries))
	for i, e := range existing.Manifest.Entries {
		existingByID[e.EngramID] = struct {
			fingerprint string
			vector      []float32
		}{fingerprint: e.Fingerprint, vector: existing.Vectors[i]}
	}

	var result BuildResult
	newEntries := make([]ManifestEntry, 0, len(rows))
	newVectors := make([][]float32, 0, len(rows))

	for _, row := range rows {
		fp := Fingerprint(model, dim, inputLimit, row.Title, row.Content)

		if prior, ok := existingByID[row.EngramID]; ok && prior.fingerprint == fp {
			newEntries = append(newEntries, ManifestEntry{EngramID: row.EngramID, Fingerprint: fp})
			newVectors = append(newVectors, prior.vector)
			result.Reused++
			continue
		}

		vector, err := embedder.Embed(ctx, EmbedInput(row.Title, row.Content, inputLimit))
		if err != nil {
			return nil, BuildResult{}, fmt.Errorf("vecindex: build: embed engram id %d: %w", row.EngramID, err)
		}
		newEntries = append(newEntries, ManifestEntry{EngramID: row.EngramID, Fingerprint: fp})
		newVectors = append(newVectors, vector)
		result.Embedded++
	}

	// Entries present in the existing index but absent from rows are
	// removed by construction: newEntries is built only from rows, so any
	// existing entry whose EngramID never appears there is simply never
	// carried forward (R-069's "removes entries for rows no longer live").
	liveIDs := make(map[int64]bool, len(rows))
	for _, row := range rows {
		liveIDs[row.EngramID] = true
	}
	for id := range existingByID {
		if !liveIDs[id] {
			result.Removed++
		}
	}

	idx := &Index{
		Manifest: Manifest{
			Model:      model,
			Dimension:  dim,
			InputLimit: inputLimit,
			BuiltAt:    now(),
			Entries:    newEntries,
		},
		Vectors: newVectors,
	}

	if err := idx.Save(dir); err != nil {
		return nil, BuildResult{}, fmt.Errorf("vecindex: build: save index to %s: %w", dir, err)
	}
	return idx, result, nil
}
