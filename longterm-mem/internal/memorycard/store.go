// Package memorycard gives longterm-mem read-only access to memorycard's SQLite database, the store that
// replaces Engram. Every connection this package opens is read-only by construction, as Engram's is
// (internal/engram): no code path here can write to memorycard's database.
//
// It is an adapter: it maps memorycard's rows to the types of internal/memory (CardMemory) and defines no
// domain type of its own. Promotion reads a memory through the lookup port it owns (promote.CardLookup), which
// Store.MemoryByID satisfies.
package memorycard

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"

	_ "modernc.org/sqlite"
)

// Store is a read-only connection to memorycard's SQLite database.
type Store struct {
	db       *sql.DB
	degraded bool
}

// Open opens a read-only connection to the memorycard database at dbPath. When dbPath is empty it resolves to
// memorycard's default location, $HOME/.memorycard/memory.db.
//
// If the primary read-only connection cannot be established (the directory is not writable, so the WAL
// shared-memory index cannot be created), Open retries once with the immutable=1 DSN and marks the Store
// degraded, as the Engram adapter does.
func Open(dbPath string) (*Store, error) {
	path := dbPath
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("memorycard: resolve home directory: %w", err)
		}
		path = filepath.Join(home, ".memorycard", "memory.db")
	}
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("memorycard: %w", err)
	}

	db, primaryErr := openAndPing(readOnlyDSN(path))
	if primaryErr == nil {
		return &Store{db: db}, nil
	}
	fallback, fallbackErr := openAndPing(readOnlyDSN(path) + "&immutable=1")
	if fallbackErr != nil {
		return nil, fmt.Errorf("memorycard: connect %s: primary open failed (%v), fallback open failed: %w", path, primaryErr, fallbackErr)
	}
	return &Store{db: fallback, degraded: true}, nil
}

func openAndPing(dsn string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// readOnlyDSN opens path strictly read-only: SQLite's mode=ro refuses to create or write the file, and
// _query_only refuses writes at the SQL level as a second line of defense.
func readOnlyDSN(path string) string {
	return fmt.Sprintf("file:%s?mode=ro&_txlock=deferred&_pragma=busy_timeout(2000)&_query_only=true", path)
}

// Degraded reports whether Open fell back to the immutable=1 read path.
func (s *Store) Degraded() bool { return s.degraded }

// Close releases the underlying database connection.
func (s *Store) Close() error { return s.db.Close() }

// memorySelect is the one query shape that maps a row of memories to a memory.CardMemory: MemoryByID and
// ListMemories each append their own where clause, so the two can never map a memory differently.
const memorySelect = `select m.id, m.project, m.type, m.title, m.body, coalesce(m.topic_key, ''), m.pinned, m.status,
		coalesce(m.superseded_by, 0), coalesce(s.title, ''),
		coalesce((select max(v.version) from memory_versions v where v.memory_id = m.id), 0),
		m.content_hash, exists (select 1 from import_map i where i.memory_id = m.id)
	 from memories m left join memories s on s.id = m.superseded_by `

// rowScanner is what sql.Row and sql.Rows share.
type rowScanner interface{ Scan(dest ...any) error }

// scanMemory maps one row of memorySelect, without the Engram bridge (see bridgeOf).
func scanMemory(row rowScanner) (memory.CardMemory, error) {
	var m memory.CardMemory
	var pinned, maxVersion int
	if err := row.Scan(&m.ID, &m.Project, &m.Type, &m.Title, &m.Body, &m.TopicKey, &pinned, &m.Status, &m.SupersededBy, &m.SupersededByTitle, &maxVersion, &m.ContentHash, &m.Mapped); err != nil {
		return memory.CardMemory{}, err
	}
	m.Pinned = pinned != 0
	m.Revision = maxVersion + 1
	return m, nil
}

// bridgeOf sets m.EngramID from the memory's import_map row of source engram, when it has one.
func (s *Store) bridgeOf(m *memory.CardMemory) error {
	var key string
	err := s.db.QueryRow(`select source_key from import_map where source = 'engram' and memory_id = ? order by part limit 1`, m.ID).Scan(&key)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return fmt.Errorf("memorycard: look up the Engram id of memory %d: %w", m.ID, err)
	default:
		// A key that is not a number names no observation: the memory is not bridged to one.
		if engramID, perr := strconv.ParseInt(key, 10, 64); perr == nil && engramID > 0 {
			m.EngramID = engramID
		}
	}
	return nil
}

// MemoryByID returns the memory identified by id, whatever its status. ok is false, with a nil error, when
// there is no such memory. Revision is the memory's next version number (the highest memory_versions.version
// plus one, or 1 when it has none); EngramID is the Engram observation id named by its import_map row of source
// engram, or 0 when it has none.
func (s *Store) MemoryByID(id int64) (memory.CardMemory, bool, error) {
	m, err := scanMemory(s.db.QueryRow(memorySelect+`where m.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return memory.CardMemory{}, false, nil
	}
	if err != nil {
		return memory.CardMemory{}, false, fmt.Errorf("memorycard: look up memory %d: %w", id, err)
	}
	if err := s.bridgeOf(&m); err != nil {
		return memory.CardMemory{}, false, err
	}
	return m, true, nil
}

// ListMemories returns the memories of project that promotion reads, in id order, each mapped as MemoryByID
// maps it: the active and the superseded ones. An archived memory is left out, as Engram's ListObservations
// leaves out a soft-deleted observation (R-020): archiving is how memorycard retires a memory, and Engram's
// rows have no status of their own, so superseded memories are listed as every live Engram row is.
func (s *Store) ListMemories(project string) ([]memory.CardMemory, error) {
	rows, err := s.db.Query(memorySelect+`where m.project = ? and m.status in ('active', 'superseded') order by m.id`, project)
	if err != nil {
		return nil, fmt.Errorf("memorycard: list memories for project %q: %w", project, err)
	}
	var memories []memory.CardMemory
	for rows.Next() {
		m, err := scanMemory(rows)
		if err != nil {
			rows.Close()
			return nil, fmt.Errorf("memorycard: read a memory of project %q: %w", project, err)
		}
		memories = append(memories, m)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, fmt.Errorf("memorycard: iterate the memories of project %q: %w", project, err)
	}
	// The bridge is looked up once the rows are closed, so that no query runs while the listing holds one open.
	for i := range memories {
		if err := s.bridgeOf(&memories[i]); err != nil {
			return nil, err
		}
	}
	return memories, nil
}
