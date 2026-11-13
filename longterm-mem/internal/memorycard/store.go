// Package memorycard gives longterm-mem read-only access to memorycard's SQLite database, the store that
// replaces Engram. Every connection this package opens is read-only by construction, as Engram's is
// (internal/engram): no code path here can write to memorycard's database.
//
// It is an adapter: Store maps memorycard's rows to memory.CardMemory, and Source (source.go) serves them as
// memory.Memory through the port promotion owns (promote.Source), the lineage and the mirrors of a memory read from
// memorycard's own import_map and content hashes. It defines no domain type of its own beyond what a query answers.
package memorycard

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

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
	engramID, err := s.engramIDOf(m.ID)
	m.EngramID = engramID
	return err
}

// engramIDOf is the Engram observation the import_map row of source engram of memory memoryID names (the first
// part's), or 0 when it has none or the key is not a number.
func (s *Store) engramIDOf(memoryID int64) (int64, error) {
	var key string
	err := s.db.QueryRow(`select source_key from import_map where source = 'engram' and memory_id = ? order by part limit 1`, memoryID).Scan(&key)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return 0, nil
	case err != nil:
		return 0, fmt.Errorf("memorycard: look up the Engram id of memory %d: %w", memoryID, err)
	}
	// A key that is not a number names no observation: the memory is not bridged to one.
	if engramID, perr := strconv.ParseInt(key, 10, 64); perr == nil && engramID > 0 {
		return engramID, nil
	}
	return 0, nil
}

// Twin is a memory that holds the content hash another memory holds and has an import_map row, with the Engram
// observation its row of source engram names (0 when it names none).
type Twin struct {
	MemoryID int64
	EngramID int64
}

// MappedTwins returns the memories of project that hold contentHash and have an import_map row of any source,
// whatever their status (a page outlives its memory), in id order: what a memory mirrored by hand, one with no
// row, holds the content of. An empty hash is no content, and holds it with nobody.
func (s *Store) MappedTwins(project, contentHash string) ([]Twin, error) {
	if contentHash == "" {
		return nil, nil
	}
	rows, err := s.db.Query(`select m.id from memories m where m.project = ? and m.content_hash = ?
		and exists (select 1 from import_map i where i.memory_id = m.id) order by m.id`, project, contentHash)
	if err != nil {
		return nil, fmt.Errorf("memorycard: look up the imported memories of project %q that hold a content hash: %w", project, err)
	}
	var twins []Twin
	for rows.Next() {
		var twin Twin
		if err := rows.Scan(&twin.MemoryID); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("memorycard: read a memory of project %q: %w", project, err)
		}
		twins = append(twins, twin)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, fmt.Errorf("memorycard: iterate the memories of project %q: %w", project, err)
	}
	// The observations are looked up once the rows are closed, so that no query runs while the rows are open.
	for i := range twins {
		if twins[i].EngramID, err = s.engramIDOf(twins[i].MemoryID); err != nil {
			return nil, err
		}
	}
	return twins, nil
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

// MemoryByEngramID returns the memory memorycard imported from Engram observation engramID: the one its import_map
// row of source engram names, the first part whose memory exists (a dangling row hides no later part), mapped as MemoryByID maps it. ok is
// false, with a nil error, when no row names the observation or every memory its rows name is gone.
func (s *Store) MemoryByEngramID(engramID int64) (memory.CardMemory, bool, error) {
	var memoryID int64
	err := s.db.QueryRow(`select i.memory_id from import_map i join memories m on m.id = i.memory_id where i.source = 'engram' and i.source_key = ? order by i.part limit 1`, strconv.FormatInt(engramID, 10)).Scan(&memoryID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return memory.CardMemory{}, false, nil
	case err != nil:
		return memory.CardMemory{}, false, fmt.Errorf("memorycard: look up the memory imported from Engram observation %d: %w", engramID, err)
	}
	return s.MemoryByID(memoryID)
}

// MemoryIDsByEngramIDs returns, for each of the Engram observations engramIDs, the ids of the memories its
// import_map rows of source engram name: one per part, in part order, without repeats. A row whose memory is
// gone names nothing and hides no other part. An observation with no such row has no entry.
func (s *Store) MemoryIDsByEngramIDs(engramIDs []int64) (map[int64][]int64, error) {
	found := map[int64][]int64{}
	const chunk = 500
	for start := 0; start < len(engramIDs); start += chunk {
		end := min(start+chunk, len(engramIDs))
		args := make([]any, 0, end-start)
		for _, id := range engramIDs[start:end] {
			args = append(args, strconv.FormatInt(id, 10))
		}
		rows, err := s.db.Query(`select i.source_key, i.memory_id from import_map i join memories m on m.id = i.memory_id
			where i.source = 'engram' and i.source_key in (`+placeholders(len(args))+`) order by i.source_key, i.part, i.memory_id`, args...)
		if err != nil {
			return nil, fmt.Errorf("memorycard: look up the memories imported from Engram observations: %w", err)
		}
		for rows.Next() {
			var key string
			var memoryID int64
			if err := rows.Scan(&key, &memoryID); err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("memorycard: read an import_map row: %w", err)
			}
			// A key that is not a number names no observation.
			engramID, perr := strconv.ParseInt(key, 10, 64)
			if perr != nil || slices.Contains(found[engramID], memoryID) {
				continue
			}
			found[engramID] = append(found[engramID], memoryID)
		}
		if err := errors.Join(rows.Err(), rows.Close()); err != nil {
			return nil, fmt.Errorf("memorycard: iterate the import_map rows: %w", err)
		}
	}
	return found, nil
}

// UnmappedMemoryIDsByHash returns, for each of the content hashes, the ids of the memories of project that hold
// it and have no import_map row of any source, whatever their status (a page outlives its memory): the memories
// that were mirrored to memorycard by hand. A hash no such memory holds has no entry.
func (s *Store) UnmappedMemoryIDsByHash(project string, hashes []string) (map[string][]int64, error) {
	found := map[string][]int64{}
	const chunk = 500
	for start := 0; start < len(hashes); start += chunk {
		end := min(start+chunk, len(hashes))
		args := []any{project}
		for _, h := range hashes[start:end] {
			args = append(args, h)
		}
		rows, err := s.db.Query(`select m.content_hash, m.id from memories m
			where m.project = ? and m.content_hash in (`+placeholders(end-start)+`)
			and not exists (select 1 from import_map i where i.memory_id = m.id) order by m.id`, args...)
		if err != nil {
			return nil, fmt.Errorf("memorycard: look up the memories mirrored by hand in project %q: %w", project, err)
		}
		for rows.Next() {
			var hash string
			var id int64
			if err := rows.Scan(&hash, &id); err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("memorycard: read a memory of project %q: %w", project, err)
			}
			found[hash] = append(found[hash], id)
		}
		if err := errors.Join(rows.Err(), rows.Close()); err != nil {
			return nil, fmt.Errorf("memorycard: iterate the memories of project %q: %w", project, err)
		}
	}
	return found, nil
}

// placeholders is n comma-separated question marks, for an in list whose values are bound arguments.
func placeholders(n int) string { return strings.TrimSuffix(strings.Repeat("?, ", n), ", ") }

// ListMemories returns the memories of project that promotion reads, in id order, each mapped as MemoryByID
// maps it: the active and the superseded ones. An archived memory is left out, as Engram's ListObservations
// leaves out a soft-deleted observation (R-020): archiving is how memorycard retires a memory, and Engram's
// rows have no status of their own, so superseded memories are listed as every live Engram row is.
func (s *Store) ListMemories(project string) ([]memory.CardMemory, error) {
	return s.listWithStatus(project, `m.status in ('active', 'superseded')`)
}

// ListAllMemories returns every memory of project whatever its status, in id order, each mapped as MemoryByID maps
// it. It is the listing status propagation reads, and the only one that shows an archived memory: ListMemories
// leaves those out because promotion does. A page can be behind its memory in either direction (the memory was
// retired, or restored to active), so propagation needs them all.
func (s *Store) ListAllMemories(project string) ([]memory.CardMemory, error) {
	return s.listWithStatus(project, `m.status in ('active', 'superseded', 'archived')`)
}

// listWithStatus is the one listing of a project's memories: it appends statusClause, a constant of this file and
// never caller input, to memorySelect's query, so every listing maps a row as MemoryByID does.
func (s *Store) listWithStatus(project, statusClause string) ([]memory.CardMemory, error) {
	rows, err := s.db.Query(memorySelect+`where m.project = ? and `+statusClause+` order by m.id`, project)
	if err != nil {
		return nil, fmt.Errorf("memorycard: list memories for project %q: %w", project, err)
	}
	var memories []memory.CardMemory
	for rows.Next() {
		m, err := scanMemory(rows)
		if err != nil {
			// The scan error is the one to report; a close failure after it adds nothing the caller can act on.
			_ = rows.Close()
			return nil, fmt.Errorf("memorycard: read a memory of project %q: %w", project, err)
		}
		memories = append(memories, m)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
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
