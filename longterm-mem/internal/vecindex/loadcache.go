package vecindex

import (
	"os"
	"path/filepath"
	"sync"
	"time"
)

// cacheEntry is one directory's cached *Index, plus the on-disk manifest
// state (mtime, size) that was true when it was loaded -- the cheapest
// signal available that the manifest may have changed since, without
// re-reading and re-digesting it on every call.
type cacheEntry struct {
	idx     *Index
	modTime time.Time
	size    int64
}

// LoadCache caches loaded *Index values keyed by directory (R-066/R-067's
// own state layout: one directory per project), so a long-lived process
// -- the MCP server, in particular -- reads a project's manifest.json and
// vectors.blob from disk at most once per change, rather than on every
// query that names or defaults to engram-embed.
//
// It is deliberately not keyed by Manifest.Revision: reading Revision
// still requires reading and parsing manifest.json, which is the smaller
// of the two files this cache exists to avoid re-reading (vectors.blob
// measured at 1.9MB on this repository's own corpus, issue #286). Stat'ing
// the manifest file's mtime and size costs one syscall and no parsing, and
// is exactly wrong only if a manifest is rewritten with an unchanged mtime
// and size within the same second on a filesystem with second-granularity
// mtimes -- a caller that writes its own fresh index through this same
// process (query's bounded top-up, embedarm.go) does not rely on that
// signal at all and calls Invalidate explicitly instead.
type LoadCache struct {
	mu      sync.Mutex
	entries map[string]cacheEntry
	// load is the underlying loader, Load by default. It is a field
	// rather than a direct call to the package function only so this
	// package's own tests can wrap it to count real disk reads; no
	// production caller ever sets it.
	load func(dir string) (*Index, error)
}

// NewLoadCache returns an empty LoadCache backed by the package's own
// Load.
func NewLoadCache() *LoadCache {
	return &LoadCache{entries: make(map[string]cacheEntry), load: Load}
}

// Load returns dir's index, reading it from disk only when this is the
// first call for dir, the on-disk manifest's mtime or size has changed
// since the cached entry was made, or the caller has since called
// Invalidate(dir). It returns the cached *Index without touching disk
// otherwise.
//
// A failed load is never cached: an empty dir (ErrNoIndex) or a corrupted
// one (ErrCorrupted) must not poison every later Load for the lifetime of
// the process once a real index appears there.
func (c *LoadCache) Load(dir string) (*Index, error) {
	info, statErr := os.Stat(filepath.Join(dir, manifestFileName))

	c.mu.Lock()
	if statErr == nil {
		if e, ok := c.entries[dir]; ok && e.modTime.Equal(info.ModTime()) && e.size == info.Size() {
			c.mu.Unlock()
			return e.idx, nil
		}
	}
	c.mu.Unlock()

	idx, err := c.load(dir)
	if err != nil {
		c.mu.Lock()
		delete(c.entries, dir)
		c.mu.Unlock()
		return nil, err
	}

	c.mu.Lock()
	if statErr == nil {
		c.entries[dir] = cacheEntry{idx: idx, modTime: info.ModTime(), size: info.Size()}
	} else {
		// info could not be stat'd even though load just succeeded --
		// vanishingly unlikely (a race with something deleting the
		// manifest between the two), but caching an entry with no
		// mtime/size to compare against would make every later Load
		// treat it as unconditionally fresh. Simplest is correct: leave
		// it uncached, so the next Load just reads again.
		delete(c.entries, dir)
	}
	c.mu.Unlock()
	return idx, nil
}

// Invalidate forces the next Load(dir) to read from disk, regardless of
// what an mtime/size comparison would otherwise decide. A caller that
// writes a fresh index for dir out of band -- query's bounded top-up
// (embedarm.go) -- calls this right after a successful build, rather than
// trusting filesystem mtime resolution to have visibly changed within one
// query.
func (c *LoadCache) Invalidate(dir string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, dir)
}
