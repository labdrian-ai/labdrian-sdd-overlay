package memorycard

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// ContentKey is what a hand mirror and an imported observation are compared by: the lowercased title and text with
// every white space rune removed. An import splits a long body into parts at a blank line or in the middle of a
// word (memorycard's internal/memory/import.go: splitBody, cutRunes), so the text of an observation and the text of
// its parts agree on their words and on nothing else; a key that ignores white space is the one comparison that
// does not have to guess where the parts were joined. It can only match two texts that differ in white space alone.
func ContentKey(title, body string) string {
	var key strings.Builder
	for _, r := range strings.ToLower(title) + strings.ToLower(body) {
		if !unicode.IsSpace(r) {
			key.WriteRune(r)
		}
	}
	return key.String()
}

// A part's title carries its place: the importer titles the parts of a record "<title> (part i/n)".
var partSuffix = regexp.MustCompile(` \(part \d+/\d+\)$`)

// ObservationsByKey returns, for project, the Engram observations imported into it by the ContentKey of what they
// hold: the observation's whole text, rebuilt from its imported parts in part order under the title of its first
// part without the " (part i/n)" suffix, and each imported part as stored, under its own title. The ids of an
// observation are listed once per key, in id order. A row whose memory is gone names nothing, and a key that is not
// a number names no observation.
func (s *Store) ObservationsByKey(project string) (map[string][]int64, error) {
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
	byKey := map[string][]int64{}
	note := func(key string, id int64) {
		if !slices.Contains(byKey[key], id) {
			byKey[key] = append(byKey[key], id)
		}
	}
	for _, id := range ids {
		var whole strings.Builder
		for _, p := range parts[id] {
			whole.WriteString(p.body)
			note(ContentKey(p.title, p.body), id)
		}
		note(ContentKey(partSuffix.ReplaceAllString(parts[id][0].title, ""), whole.String()), id)
	}
	return byKey, nil
}

// ObservationsWithStoredHash returns the Engram observations, in id order, that have an imported memory of project
// whose stored content_hash is contentHash: the cheap way to find what a memory mirrors, one indexed lookup, when
// an import kept the hash of the part. It says nothing of a part whose hash differs, which ObservationsByKey finds.
func (s *Store) ObservationsWithStoredHash(project, contentHash string) ([]int64, error) {
	rows, err := s.db.Query(`select distinct i.source_key from import_map i join memories m on m.id = i.memory_id
		where i.source = 'engram' and m.project = ? and m.content_hash = ?`, project, contentHash)
	if err != nil {
		return nil, fmt.Errorf("memorycard: look up the imported memories of project %q that hold a content hash: %w", project, err)
	}
	var ids []int64
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("memorycard: read an import_map row of project %q: %w", project, err)
		}
		// A key that is not a number names no observation.
		if id, perr := strconv.ParseInt(key, 10, 64); perr == nil && id > 0 {
			ids = append(ids, id)
		}
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, fmt.Errorf("memorycard: iterate the imported memories of project %q: %w", project, err)
	}
	slices.Sort(ids)
	return slices.Compact(ids), nil
}
