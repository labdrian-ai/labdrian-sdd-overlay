package memorycard

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// ImportedObservation is one Engram observation memorycard imported, as memorycard holds it: the memories its
// import_map rows of source engram name, in part order. Hash is the card content hash of the observation's
// whole text, rebuilt from those parts, the hash Engram's own rows were compared by (memory.CardContentHash):
// the stored content_hash of an imported memory is not used, since it is the hash of a part, and an import
// may have merged or changed it.
type ImportedObservation struct {
	EngramID  int64
	MemoryIDs []int64
	Hash      string
}

// A part's title carries its place: the importer titles the parts of a record "<title> (part i/n)".
var partSuffix = regexp.MustCompile(` \(part \d+/\d+\)$`)

// ImportedObservations returns, in observation id order, the Engram observations imported into project, each with
// the memories of its parts and the hash of its whole text, rebuilt as the importer split it (memorycard's
// internal/memory/import.go): a record is one memory, or, when its body exceeds the cap, parts that pack whole
// paragraphs joined by a blank line, with a paragraph longer than a part cut on a rune boundary, so a part boundary
// stands for a blank line or for nothing. An observation of several parts therefore has two readings, its parts
// joined by a blank line and joined by nothing, and each is a hash it answers to (ImportedObservations returns
// the observation once per hash). A row whose memory is gone names nothing, and a key that is not a number
// names no observation.
func (s *Store) ImportedObservations(project string) ([]ImportedObservation, error) {
	rows, err := s.db.Query(`select i.source_key, m.id, m.title, m.body from import_map i join memories m on m.id = i.memory_id
		where i.source = 'engram' and m.project = ? order by i.source_key, i.part, m.id`, project)
	if err != nil {
		return nil, fmt.Errorf("memorycard: list the observations imported into project %q: %w", project, err)
	}
	type part struct {
		id          int64
		title, body string
	}
	parts := map[int64][]part{}
	for rows.Next() {
		var key string
		var p part
		if err := rows.Scan(&key, &p.id, &p.title, &p.body); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("memorycard: read an import_map row of project %q: %w", project, err)
		}
		engramID, perr := strconv.ParseInt(key, 10, 64)
		if perr != nil || engramID <= 0 || slices.ContainsFunc(parts[engramID], func(q part) bool { return q.id == p.id }) {
			continue
		}
		parts[engramID] = append(parts[engramID], p)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, fmt.Errorf("memorycard: iterate the import_map rows of project %q: %w", project, err)
	}

	ids := make([]int64, 0, len(parts))
	for id := range parts {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	var observations []ImportedObservation
	for _, id := range ids {
		ps := parts[id]
		title, separators := ps[0].title, []string{""}
		memoryIDs := make([]int64, len(ps))
		bodies := make([]string, len(ps))
		for i, p := range ps {
			memoryIDs[i], bodies[i] = p.id, p.body
		}
		if len(ps) > 1 {
			title, separators = partSuffix.ReplaceAllString(title, ""), []string{"\n\n", ""}
		}
		for _, separator := range separators {
			observations = append(observations, ImportedObservation{
				EngramID: id, MemoryIDs: memoryIDs, Hash: memory.CardContentHash(title, strings.Join(bodies, separator)),
			})
		}
	}
	return observations, nil
}
