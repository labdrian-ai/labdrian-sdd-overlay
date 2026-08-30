// Package engram gives longterm-mem read-only access to Engram's mid-term
// SQLite database. Every connection this package opens is read-only by
// construction (R-002): no code path here can write to Engram's database.
package engram

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Store is a read-only connection to Engram's SQLite database.
type Store struct {
	db            *sql.DB
	path          string
	degraded      bool
	degradedCause string
}

// Observation is one mid-term Engram observation row.
type Observation struct {
	ID      int64
	Title   string
	Content string
	Project string
}

// Open opens a read-only connection to the Engram database at dbPath. When
// dbPath is empty, it resolves to Engram's default location,
// $HOME/.engram/engram.db (R-002).
//
// If the primary read-only connection (readOnlyDSN) cannot be established --
// e.g. Engram's writer is offline and the directory is not writable, so the
// WAL shared-memory index cannot be created -- Open retries once with
// readOnlyImmutableDSN and marks the returned Store degraded (see Degraded)
// instead of failing outright.
func Open(dbPath string) (*Store, error) {
	path := dbPath
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("engram: resolve home directory: %w", err)
		}
		path = filepath.Join(home, ".engram", "engram.db")
	}

	db, primaryErr := openAndPing(readOnlyDSN(path))
	if primaryErr == nil {
		return &Store{db: db, path: path}, nil
	}

	fallbackDB, fallbackErr := openAndPing(readOnlyImmutableDSN(path))
	if fallbackErr != nil {
		return nil, fmt.Errorf("engram: connect %s: primary open failed (%v), fallback open failed: %w", path, primaryErr, fallbackErr)
	}

	return &Store{db: fallbackDB, path: path, degraded: true, degradedCause: primaryErr.Error()}, nil
}

// openAndPing opens dsn and pings it, closing the connection on any failure
// so a rejected candidate never leaks a handle.
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

// Degraded reports whether Open fell back to the immutable=1 read path
// because the primary read-only connection could not be established, and,
// when it did, the primary open error that triggered the fallback.
func (s *Store) Degraded() (bool, string) {
	return s.degraded, s.degradedCause
}

// Close releases the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// readOnlyDSN builds the modernc.org/sqlite DSN that opens path strictly
// read-only (D1): SQLite's own `mode=ro` URI parameter refuses to create or
// write the file at the OS level, `_query_only=true` refuses writes at the
// SQL level as a second line of defense, `_txlock=deferred` avoids taking a
// write lock even to begin a transaction, and `_pragma=busy_timeout(2000)`
// keeps a query from failing immediately if Engram's own writer briefly
// holds the database.
func readOnlyDSN(path string) string {
	return fmt.Sprintf("file:%s?mode=ro&_txlock=deferred&_pragma=busy_timeout(2000)&_query_only=true", path)
}

// readOnlyImmutableDSN builds the fallback DSN Open retries with when
// readOnlyDSN cannot be established. `immutable=1` tells SQLite the file
// will not change for the connection's life, so it skips WAL/locking and
// reads the main database file directly instead of requiring a writable
// -shm. This assumes no concurrent writer and may miss un-checkpointed WAL
// frames -- stale but never unsafe, since the connection stays exactly as
// read-only as the primary one.
func readOnlyImmutableDSN(path string) string {
	return readOnlyDSN(path) + "&immutable=1"
}

// Path returns the resolved database path this Store connected to.
func (s *Store) Path() string {
	return s.path
}

// ListObservations returns every observation belonging to project that has
// not been soft-deleted (R-020): rows from other projects and rows with a
// non-null deleted_at are excluded.
func (s *Store) ListObservations(project string) ([]Observation, error) {
	rows, err := s.db.Query(
		`SELECT id, title, content, project FROM observations WHERE project = ? AND deleted_at IS NULL`,
		project,
	)
	if err != nil {
		return nil, fmt.Errorf("engram: list observations for project %q: %w", project, err)
	}
	defer rows.Close()

	var observations []Observation
	for rows.Next() {
		var o Observation
		if err := rows.Scan(&o.ID, &o.Title, &o.Content, &o.Project); err != nil {
			return nil, fmt.Errorf("engram: scan observation row: %w", err)
		}
		observations = append(observations, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("engram: iterate observation rows: %w", err)
	}

	return observations, nil
}
