package memorycard

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// ContentKey is what a hand mirror and an imported observation are compared by: the title and the text, each
// lowercased and with every Unicode white-space rune removed, joined behind the length of the title, so that a title
// and a text cannot trade characters (the title "a" with the text "bc" is not the title "ab" with the text "c").
//
// What it matches: two texts that differ in white space or in letter case alone. White space, because an import
// splits a long body at a blank line or in the middle of a word (memorycard's internal/memory/import.go: splitBody,
// cutRunes), so an observation and its parts agree on their words and on nothing else. Case, because memorycard's own
// content hash folds it (save.go: contentHash lowercases) and an import reuses an existing memory of the same hash
// instead of inserting one (import.go: importRow), so an imported text may carry the case of another memory's.
//
// What it does not match: a different word or a different order of words, and a text that memorycard changed on the
// way in. It compares the stored text, which is redacted and trimmed (import.go: recordParts, redact), so a hand mirror
// that kept the unredacted text, or the title the observation had before redaction, matches nothing: no key can
// find it, and that is a limit of mirroring by text, not of this key.
func ContentKey(title, body string) string {
	t := textKey(title)
	return strconv.Itoa(len(t)) + ":" + t + textKey(body)
}

// textKey is s lowercased with every white-space rune removed.
func textKey(s string) string {
	var key strings.Builder
	key.Grow(len(s))
	for _, r := range strings.ToLower(s) {
		if !unicode.IsSpace(r) {
			key.WriteRune(r)
		}
	}
	return key.String()
}

// The importer titles the parts of a record "<title> (part i/n)", and only when it has more than one part
// (memorycard's import.go: recordParts).
var partSuffix = regexp.MustCompile(` \(part \d+/\d+\)$`)

// ImportedPart is one import_map row of source engram: part Part of Engram observation Observation. Memory is the
// memory the row names, Title its title and Hash the content hash memorycard stored for it, or 0, "" and "" when that
// memory is gone (the row still counts as a part of the observation).
type ImportedPart struct {
	Observation int64
	Part        int
	Memory      int64
	Title       string
	Hash        string
}

// ImportedParts returns the import_map rows of source engram of the observations imported into project, with each row's title and stored content hash:
// the rows whose memory belongs to project, and the rows whose memory is gone, for an observation that has a memory
// in project (import_map names no project, so a gone row can only be told by its observation; an observation with no
// memory left in project is not the project's and has nothing to match). A key that is not a number names no
// observation and is left out.
func (s *Store) ImportedParts(project string) ([]ImportedPart, error) {
	rows, err := s.db.Query(`select i.source_key, i.part, coalesce(m.id, 0), coalesce(m.title, ''), coalesce(m.content_hash, '') from import_map i
		left join memories m on m.id = i.memory_id where i.source = 'engram' and (m.project = ? or (m.id is null and exists (
			select 1 from import_map j join memories n on n.id = j.memory_id
			where j.source = 'engram' and j.source_key = i.source_key and n.project = ?)))
		order by i.source_key, i.part`, project, project)
	if err != nil {
		return nil, fmt.Errorf("memorycard: list the observations imported into project %q: %w", project, err)
	}
	var parts []ImportedPart
	for rows.Next() {
		var key string
		var p ImportedPart
		if err := rows.Scan(&key, &p.Part, &p.Memory, &p.Title, &p.Hash); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("memorycard: read an import_map row of project %q: %w", project, err)
		}
		if id, perr := strconv.ParseInt(key, 10, 64); perr == nil && id > 0 {
			p.Observation = id
			parts = append(parts, p)
		}
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, fmt.Errorf("memorycard: iterate the import_map rows of project %q: %w", project, err)
	}
	return parts, nil
}

// MemoryBodies returns the body of each of the memories ids, by memory id; a memory that is gone has no entry.
func (s *Store) MemoryBodies(ids []int64) (map[int64]string, error) {
	bodies := make(map[int64]string, len(ids))
	const chunk = 500
	for start := 0; start < len(ids); start += chunk {
		end := min(start+chunk, len(ids))
		args := make([]any, 0, end-start)
		for _, id := range ids[start:end] {
			args = append(args, id)
		}
		rows, err := s.db.Query(`select id, body from memories where id in (`+placeholders(len(args))+`)`, args...)
		if err != nil {
			return nil, fmt.Errorf("memorycard: read the bodies of imported memories: %w", err)
		}
		for rows.Next() {
			var id int64
			var body string
			if err := rows.Scan(&id, &body); err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("memorycard: read the body of an imported memory: %w", err)
			}
			bodies[id] = body
		}
		if err := errors.Join(rows.Err(), rows.Close()); err != nil {
			return nil, fmt.Errorf("memorycard: iterate the bodies of imported memories: %w", err)
		}
	}
	return bodies, nil
}

// textOf is the title and the text of a memory that holds the content of an imported observation, or may.
type textOf struct{ title, body, hash string }

// observationIndex says which Engram observations imported into one project hold a given text: those one of whose
// imported memories has the text's stored content hash, and those whose text has its ContentKey. It is built from the
// titles and stored hashes of the imported memories alone and reads a body only for an observation whose title already
// agrees with the text asked about: a ContentKey starts with the title, so an observation of another title cannot
// match by key, and what it takes to prove a match (the bodies of one observation) is read once. It is not safe for
// concurrent use; Source serializes its calls.
type observationIndex struct {
	parts   map[int64][]ImportedPart // each observation's rows, in part order
	hashes  map[string][]int64       // stored content hash of an imported memory -> observations, ascending
	titles  map[string][]int64       // textKey of a title an observation can answer to -> observations, ascending
	bodies  map[int64]string         // the bodies read so far, by memory id
	keysFor map[int64][]string       // the ContentKeys of an observation, once its bodies are read
}

// newObservationIndex indexes rows, the result of Store.ImportedParts. An observation none of whose memories is left
// names nothing and is left out.
func newObservationIndex(rows []ImportedPart) *observationIndex {
	ix := &observationIndex{
		parts: map[int64][]ImportedPart{}, hashes: map[string][]int64{}, titles: map[string][]int64{}, bodies: map[int64]string{}, keysFor: map[int64][]string{},
	}
	for _, p := range rows {
		ix.parts[p.Observation] = append(ix.parts[p.Observation], p)
	}
	observations := make([]int64, 0, len(ix.parts))
	for id, parts := range ix.parts {
		sort.SliceStable(parts, func(i, j int) bool { return parts[i].Part < parts[j].Part })
		if _, ok := firstSurvivor(parts); ok {
			observations = append(observations, id)
		} else {
			delete(ix.parts, id)
		}
	}
	slices.Sort(observations)
	for _, id := range observations {
		for _, p := range ix.parts[id] {
			if p.Memory != 0 {
				ix.answersTo(textKey(p.Title), id)
				if p.Hash != "" && !slices.Contains(ix.hashes[p.Hash], id) {
					ix.hashes[p.Hash] = append(ix.hashes[p.Hash], id)
				}
			}
		}
		ix.answersTo(textKey(wholeTitle(ix.parts[id])), id)
	}
	return ix
}

func (ix *observationIndex) answersTo(titleKey string, observation int64) {
	if !slices.Contains(ix.titles[titleKey], observation) {
		ix.titles[titleKey] = append(ix.titles[titleKey], observation)
	}
}

func firstSurvivor(parts []ImportedPart) (ImportedPart, bool) {
	for _, p := range parts {
		if p.Memory != 0 {
			return p, true
		}
	}
	return ImportedPart{}, false
}

// wholeTitle is the title of the whole text of an observation: the title of its first surviving part, without the
// " (part i/n)" suffix when the observation really has more than one part, that is, more than one import_map row,
// whether or not every memory is still there. A single part keeps the title it has, suffix included.
func wholeTitle(parts []ImportedPart) string {
	first, _ := firstSurvivor(parts)
	if len(parts) > 1 {
		return partSuffix.ReplaceAllString(first.Title, "")
	}
	return first.Title
}

// holders returns, for each of texts, in order, the observations that hold it, in id order, once each: the union of
//   - those one of whose imported memories has the stored content hash of the text (memorycard's own hash, which
//     joins the title and the text and splits on white space: S10 (c), "the same content hash"), found in the index
//     with no read, and
//   - those one of whose imported parts, as stored and under its own title, or whose whole text, rebuilt by joining
//     its parts in part order under wholeTitle, has the ContentKey of the text: what finds the text of a part the
//     importer cut in the middle of a word, whose stored hash is another text's.
//
// An empty hash holds the content of nobody. It reads the bodies of every candidate observation of all of texts in
// one lookup.
func (ix *observationIndex) holders(store *Store, texts []textOf) ([][]int64, error) {
	candidates := make([][]int64, len(texts))
	var missing []int64
	seen := map[int64]bool{}
	for i, t := range texts {
		candidates[i] = ix.titles[textKey(t.title)]
		for _, observation := range candidates[i] {
			if _, done := ix.keysFor[observation]; done {
				continue
			}
			for _, p := range ix.parts[observation] {
				if _, read := ix.bodies[p.Memory]; p.Memory != 0 && !read && !seen[p.Memory] {
					seen[p.Memory] = true
					missing = append(missing, p.Memory)
				}
			}
		}
	}
	if len(missing) > 0 {
		slices.Sort(missing)
		bodies, err := store.MemoryBodies(missing)
		if err != nil {
			return nil, err
		}
		for _, id := range missing {
			ix.bodies[id] = bodies[id]
		}
	}
	held := make([][]int64, len(texts))
	for i, t := range texts {
		key := ContentKey(t.title, t.body)
		if t.hash != "" {
			held[i] = append(held[i], ix.hashes[t.hash]...)
		}
		for _, observation := range candidates[i] {
			if slices.Contains(ix.keysOf(observation), key) {
				held[i] = append(held[i], observation)
			}
		}
		slices.Sort(held[i])
		held[i] = slices.Compact(held[i])
	}
	return held, nil
}

// keysOf is the ContentKey of each imported part of an observation, as stored, and of its whole text. Every part
// row counts, in part order, even when two rows name one memory.
func (ix *observationIndex) keysOf(observation int64) []string {
	if keys, ok := ix.keysFor[observation]; ok {
		return keys
	}
	var keys []string
	var whole strings.Builder
	for _, p := range ix.parts[observation] {
		if p.Memory == 0 {
			continue
		}
		whole.WriteString(ix.bodies[p.Memory])
		keys = append(keys, ContentKey(p.Title, ix.bodies[p.Memory]))
	}
	keys = append(keys, ContentKey(wholeTitle(ix.parts[observation]), whole.String()))
	ix.keysFor[observation] = keys
	return keys
}
