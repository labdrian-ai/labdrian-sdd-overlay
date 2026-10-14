package vecindex

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
)

// buildLockFileName is the advisory lock file a build takes an exclusive
// syscall.Flock on inside the index directory, for the lifetime of one build.
// It never holds index data itself.
const buildLockFileName = ".build.lock"

// Builder builds embedding indexes, and owns the locks that keep two builds of
// one index directory apart.
//
// A build is a read of the existing index, the embedding of what changed and
// two atomic replacements (vectors.blob, then manifest.json), and two builds
// that interleave pair one build's manifest with the other's blob, a
// corruption Load cannot see. Builds are therefore serialized per directory,
// in both scopes a process meets: within the process, by a mutex the Builder
// keeps for each directory (two MCP queries triggering a top-up for the same
// project) and, across processes, by an advisory file lock (a top-up racing a
// manually invoked `longterm-mem index --embeddings`). The mutex is the
// Builder's and not a global: what must exclude one another is built through
// one Builder, which the composition root makes for the run of a command or
// for a whole MCP session and hands to every repository it makes.
//
// The zero Builder is ready to use. It holds a mutex, so it is not copied
// after first use.
type Builder struct {
	mu sync.Mutex
	// locks holds one *sync.Mutex per index directory the Builder has ever
	// built, so unrelated projects never contend.
	locks map[string]*sync.Mutex
}

// mutexFor returns the in-process mutex for dir, creating it on first use.
func (b *Builder) mutexFor(dir string) *sync.Mutex {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.locks == nil {
		b.locks = map[string]*sync.Mutex{}
	}
	m, ok := b.locks[dir]
	if !ok {
		m = &sync.Mutex{}
		b.locks[dir] = m
	}
	return m
}

// lock takes both locks of dir, the Builder's mutex first and then the file
// lock, and returns the function that releases them in the opposite order. It
// serializes the in-process builds before either reaches the filesystem, which
// the file lock cannot do on its own: flock is per-file-descriptor advisory
// locking, and a second Open+Flock from the *same* process on some platforms
// would not block a goroutine the way a second process does. A failure to take
// the file lock releases the mutex again, so a directory that cannot be locked
// does not stay locked.
func (b *Builder) lock(dir string) (unlock func(), err error) {
	mu := b.mutexFor(dir)
	mu.Lock()

	file, err := acquireFileLock(dir)
	if err != nil {
		mu.Unlock()
		return nil, err
	}
	return func() {
		releaseFileLock(file)
		mu.Unlock()
	}, nil
}

// acquireFileLock takes a blocking exclusive syscall.Flock on
// dir/.build.lock, serializing builds across separate processes the way the
// Builder's mutex serializes them within one. The caller must release the
// returned file with releaseFileLock on every return path.
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
