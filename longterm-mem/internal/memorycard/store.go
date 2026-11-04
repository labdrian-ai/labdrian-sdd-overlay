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

// MemoryByID returns the memory identified by id, whatever its status. ok is false, with a nil error, when
// there is no such memory. Revision is the memory's next version number (the highest memory_versions.version
// plus one, or 1 when it has none); EngramID is the Engram observation id named by its import_map row of source
// engram, or 0 when it has none.
func (s *Store) MemoryByID(id int64) (memory.CardMemory, bool, error) {
	var m memory.CardMemory
	var pinned int
	var maxVersion int
	err := s.db.QueryRow(
		`select m.id, m.project, m.type, m.title, m.body, coalesce(m.topic_key, ''), m.pinned, m.status,
			coalesce(m.superseded_by, 0), coalesce(s.title, ''),
			coalesce((select max(v.version) from memory_versions v where v.memory_id = m.id), 0),
			m.content_hash, exists (select 1 from import_map i where i.memory_id = m.id)
		 from memories m left join memories s on s.id = m.superseded_by
		 where m.id = ?`, id,
	).Scan(&m.ID, &m.Project, &m.Type, &m.Title, &m.Body, &m.TopicKey, &pinned, &m.Status, &m.SupersededBy, &m.SupersededByTitle, &maxVersion, &m.ContentHash, &m.Mapped)
	if errors.Is(err, sql.ErrNoRows) {
		return memory.CardMemory{}, false, nil
	}
	if err != nil {
		return memory.CardMemory{}, false, fmt.Errorf("memorycard: look up memory %d: %w", id, err)
	}
	m.Pinned = pinned != 0
	m.Revision = maxVersion + 1

	var key string
	err = s.db.QueryRow(`select source_key from import_map where source = 'engram' and memory_id = ? order by part limit 1`, id).Scan(&key)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return memory.CardMemory{}, false, fmt.Errorf("memorycard: look up the Engram id of memory %d: %w", id, err)
	default:
		// A key that is not a number names no observation: the memory is not bridged to one.
		if engramID, perr := strconv.ParseInt(key, 10, 64); perr == nil && engramID > 0 {
			m.EngramID = engramID
		}
	}
	return m, true, nil
}
