package vecindex

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
)

// buildLockFileName is the advisory lock file Build takes an exclusive
// syscall.Flock on inside dir, for the lifetime of one Build call. It never
// holds index data itself.
const buildLockFileName = ".build.lock"

// dirLocks holds one *sync.Mutex per index directory Build has ever locked
// in this process. It serializes concurrent Build calls that race inside a
// single process (e.g. two MCP queries triggering a top-up for the same
// project) before either one ever reaches the filesystem, which the
// cross-process flock below cannot do on its own: flock is per-file-
// descriptor advisory locking, and a second Open+Flock from the *same*
// process on some platforms would not block a goroutine the way a second
// process does. Keyed by dir so unrelated projects never contend.
var (
	dirLocksMu sync.Mutex
	dirLocks   = map[string]*sync.Mutex{}
)

// dirLock returns the in-process mutex for dir, creating it on first use.
func dirLock(dir string) *sync.Mutex {
	dirLocksMu.Lock()
	defer dirLocksMu.Unlock()
	m, ok := dirLocks[dir]
	if !ok {
		m = &sync.Mutex{}
		dirLocks[dir] = m
	}
	return m
}

// acquireFileLock takes a blocking exclusive syscall.Flock on
// dir/.build.lock, serializing Build across separate processes (e.g. a
// top-up racing a manually invoked `longterm-mem index --embeddings` in
// another process) the same way dirLock serializes it within one process.
// The caller must release the returned file with releaseFileLock on every
// return path.
func acquireFileLock(dir string) (*os.File, error) {
	f, err := os.OpenFile(filepath.Join(dir, buildLockFileName), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("vecindex: build: open lock file: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, fmt.Errorf("vecindex: build: acquire lock: %w", err)
	}
	return f, nil
}

// releaseFileLock unlocks and closes a file opened by acquireFileLock. It is
// best-effort: an error unlocking or closing a lock file we are done with is
// not something a caller can usefully act on.
func releaseFileLock(f *os.File) {
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	_ = f.Close()
}

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
	// Serialize every Build call on dir, in both concurrency scopes this
	// package must cover: an in-process mutex (two MCP queries racing a
	// top-up for the same project inside one process) and a cross-process
	// advisory file lock (a top-up racing a manually invoked
	// `longterm-mem index --embeddings` in another process). Without this,
	// two concurrent Save calls -- each an independent atomic rename of
	// vectors.blob followed by manifest.json -- can interleave and pair one
	// build's manifest with the other's blob; Load only checks blob length
	// against entry count, so that corruption is silent.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, BuildResult{}, fmt.Errorf("vecindex: build: create %s: %w", dir, err)
	}

	mu := dirLock(dir)
	mu.Lock()
	defer mu.Unlock()

	lockFile, err := acquireFileLock(dir)
	if err != nil {
		return nil, BuildResult{}, err
	}
	defer releaseFileLock(lockFile)

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
