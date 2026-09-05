// Package staleness finds memories that describe something the repository
// no longer has.
//
// The failure it exists for, in the maintainer's own words: a long stretch
// of work on one feature, and the agents lose track of what was already
// extirpated from the project -- so they bring it back. The memory is not
// misfiled and not in the wrong project. It is correctly stored and no
// longer true, because the addition was saved and the REMOVAL never was.
// Search returns it, an agent reads it, and the extirpated thing returns
// with it.
//
// What makes this hard is that the obvious test is wrong. Absence from the
// working tree is not evidence of removal. Measured against a real database
// of 568 observations, 53 referenced paths were absent, and they split four
// ways: genuinely removed, recorded under a different root, RENAMED, and
// belonging to another repository entirely. Only the first is staleness,
// and a detector that reported all four would mark live memory as dead --
// after which somebody deletes it. So each of the other three is closed
// here deliberately, and each has a test named after the mistake it
// prevents:
//
//   - a different root is resolved against the tree by path suffix before
//     the history is ever consulted;
//   - a rename is reported as MOVED, never removed (internal/repohistory
//     asks git in the one way that can tell them apart);
//   - a path this repository never had is ignored, because this repository
//     has nothing to say about it.
//
// The rule itself is a comparison, not a judgement about meaning: a memory
// is stale when a path it names was deleted AFTER the memory was last
// written. The inverse matters just as much -- a memory written after the
// deletion is very likely the record OF the deletion, and reporting that
// would invite deleting the one memory that stops the thing coming back.
//
// Findings are REPORTED, never acted on. longterm-mem reads Engram
// read-only (R-002), and a memory is not deleted on the word of a heuristic
// about file paths.
package staleness

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/repohistory"
)

// Finding is one observation the repository disagrees with.
type Finding struct {
	ObservationID int64
	Title         string
	// Removed are paths deleted after the observation was written: the
	// reason it may describe something that no longer exists.
	Removed []repohistory.PathFact
	// Moved are paths whose content was renamed elsewhere. The observation
	// is out of date about WHERE, never wrong about WHAT, so these are
	// reported separately and are never grounds for removing anything.
	Moved []repohistory.PathFact
}

// whereField captures the "**Where**:" line of Engram's save format, which
// is where an observation names the files it is about.
var whereField = regexp.MustCompile(`(?s)\*\*Where\*\*:(.*?)(?:\n\*\*|\z)`)

// filePath matches a repository-shaped path: at least one directory
// separator or a known extension, and an extension this repository could
// plausibly hold. It is deliberately strict. A loose pattern pulls in issue
// references (#139/#138), diff stats (+111/-1) and URLs, and every one of
// those would later be reported as a missing file -- a finding invented out
// of punctuation.
var filePath = regexp.MustCompile(`^\.?[A-Za-z0-9_-][A-Za-z0-9._/-]*\.(go|md|sh|json|toml|ya?ml|ts|js|sql)$`)

// Paths returns the repository-shaped file paths an observation names.
func Paths(content string) []string {
	m := whereField.FindStringSubmatch(content)
	if m == nil {
		return nil
	}

	seen := map[string]bool{}
	var out []string
	for _, tok := range regexp.MustCompile(`[,\s]+`).Split(m[1], -1) {
		tok = strings.Trim(tok, "`*,;:()[]\"'")
		tok = strings.TrimRight(tok, ".")
		if strings.Contains(tok, "://") || strings.ContainsAny(tok, "#+") {
			continue
		}
		if !filePath.MatchString(tok) || seen[tok] {
			continue
		}
		seen[tok] = true
		out = append(out, tok)
	}
	return out
}

// Detect reports which of observations the repository at repoRoot
// disagrees with.
func Detect(repoRoot string, observations []engram.Observation) ([]Finding, error) {
	tree, err := indexTree(repoRoot)
	if err != nil {
		return nil, err
	}

	// Only paths the tree cannot account for are worth a subprocess, and
	// resolving by suffix first is what keeps a path recorded under another
	// root from ever reaching the history as a suspected deletion.
	unresolved := map[string]bool{}
	perObs := make(map[int64][]string, len(observations))
	for _, o := range observations {
		var candidates []string
		for _, p := range Paths(o.Content) {
			if tree.holds(p) {
				continue
			}
			candidates = append(candidates, p)
			unresolved[p] = true
		}
		if len(candidates) > 0 {
			perObs[o.ID] = candidates
		}
	}
	if len(unresolved) == 0 {
		return nil, nil
	}

	facts, err := repohistory.Inspect(repoRoot, sortedKeys(unresolved))
	if err != nil {
		return nil, err
	}

	return findings(observations, perObs, facts), nil
}

// findings applies the rule to already-gathered facts. It is separated from
// the gathering so the rule can be tested against facts that are awkward to
// build on disk -- an unknown path carrying a recent date above all.
func findings(observations []engram.Observation, perObs map[int64][]string, facts map[string]repohistory.PathFact) []Finding {
	var out []Finding
	for _, o := range observations {
		written, ok := observationTime(o)
		f := Finding{ObservationID: o.ID, Title: o.Title}
		for _, p := range perObs[o.ID] {
			switch fact := facts[p]; fact.State {
			case repohistory.StateDeleted:
				// Undatable memory is not reported as stale. Assuming a
				// zero time would make every deletion look later than
				// every memory, turning an unparsed timestamp into a
				// finding -- evidence manufactured from a formatting
				// difference.
				if ok && fact.At.After(written) {
					f.Removed = append(f.Removed, fact)
				}
			case repohistory.StateRenamed:
				f.Moved = append(f.Moved, fact)
			}
		}
		if len(f.Removed) > 0 || len(f.Moved) > 0 {
			out = append(out, f)
		}
	}
	return out
}

// observationTime parses when an observation was last written. Engram
// writes its timestamps through datetime('now'), so they arrive as SQLite
// TEXT rather than RFC 3339.
func observationTime(o engram.Observation) (time.Time, bool) {
	for _, raw := range []string{o.UpdatedAt, o.CreatedAt} {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339, "2006-01-02T15:04:05"} {
			if t, err := time.Parse(layout, raw); err == nil {
				return t.UTC(), true
			}
		}
	}
	return time.Time{}, false
}

// treeIndex answers whether the repository holds a file at, or ending in, a
// recorded path.
type treeIndex struct {
	// bySuffix is keyed on each tracked path's own trailing segments, so a
	// path recorded without its module prefix still resolves.
	bySuffix map[string]bool
}

func (t treeIndex) holds(recorded string) bool {
	return t.bySuffix[strings.TrimPrefix(recorded, "./")]
}

// indexTree walks the working tree once and indexes every suffix of every
// file path, so "promote/writer.go" resolves to
// "module/promote/writer.go" -- the false positive measured on the real
// database, where a path recorded relative to a different root looked
// removed while the file was alive.
func indexTree(repoRoot string) (treeIndex, error) {
	idx := treeIndex{bySuffix: map[string]bool{}}
	err := filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(repoRoot, path)
		if relErr != nil {
			return nil
		}
		segments := strings.Split(filepath.ToSlash(rel), "/")
		for i := range segments {
			idx.bySuffix[strings.Join(segments[i:], "/")] = true
		}
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return treeIndex{}, fmt.Errorf("staleness: %s does not exist", repoRoot)
		}
		return treeIndex{}, fmt.Errorf("staleness: walking %s: %w", repoRoot, err)
	}
	return idx, nil
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
