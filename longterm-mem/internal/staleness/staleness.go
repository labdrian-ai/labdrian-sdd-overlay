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
//   - a rename is reported as MOVED, never removed (the history behind
//     RepoEvidence asks git in the one way that can tell them apart);
//   - a path this repository never had is ignored, because this repository
//     has nothing to say about it;
//   - a path that climbs out of the project ("a/../../outside.go") is
//     unknown without a question to the history, because the repository
//     has nothing to say about a place outside it and git would refuse the
//     question, failing the whole report for one odd path.
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
//
// The rule reads the repository through RepoEvidence, a port it owns: the
// files of the working tree and what the history says of a path. It walks no
// directory and runs no process; internal/repoevidence answers the port on a
// file system and git, and the composition root hands it over.
package staleness

import (
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// Finding is one observation the repository disagrees with.
type Finding struct {
	ObservationID int64
	Title         string
	// Removed are paths deleted after the observation was written: the
	// reason it may describe something that no longer exists.
	Removed []PathFact
	// Moved are paths whose content was renamed elsewhere. The observation
	// is out of date about WHERE, never wrong about WHAT, so these are
	// reported separately and are never grounds for removing anything.
	Moved []PathFact
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

	return extractPaths(m[1])
}

// ReferencedPaths returns repository-shaped paths found only in Markdown code
// spans: inline backticks and fenced code blocks. Unlike Paths, which reads
// the structured **Where** field of an Engram observation, this function is
// for instruction bodies such as SKILL.md. Both callers share the same strict
// filePath pattern so prose, URLs, issue references and diff statistics do
// not become invented repository references.
func ReferencedPaths(text string) []string {
	var segments []string
	for i := 0; i < len(text); {
		if text[i] != '`' {
			i++
			continue
		}

		run := 1
		for i+run < len(text) && text[i+run] == '`' {
			run++
		}
		if run >= 3 {
			end := findBacktickRun(text, i+run, run)
			if end < 0 {
				segments = append(segments, text[i+run:])
				break
			}
			segments = append(segments, text[i+run:end])
			i = end + run
			continue
		}

		end := findBacktickRun(text, i+run, run)
		if end >= 0 {
			segments = append(segments, text[i+run:end])
			i = end + run
			continue
		}
		i += run
	}

	seen := map[string]bool{}
	var out []string
	for _, segment := range segments {
		for _, path := range extractPaths(segment) {
			if seen[path] {
				continue
			}
			seen[path] = true
			out = append(out, path)
		}
	}
	return out
}

func findBacktickRun(text string, start, run int) int {
	for i := start; i+run <= len(text); i++ {
		if text[i] != '`' {
			continue
		}
		matched := true
		for j := 1; j < run; j++ {
			if text[i+j] != '`' {
				matched = false
				break
			}
		}
		if matched {
			return i
		}
	}
	return -1
}

func extractPaths(text string) []string {
	seen := map[string]bool{}
	var out []string
	for _, tok := range regexp.MustCompile(`[,\s]+`).Split(text, -1) {
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

// ClassifyPaths returns the repository's best evidence for every recorded
// path. A path is resolved against the files of the working tree by its
// trailing segments first, which needs no question of the history; the ones
// the tree cannot account for are put to the history, in one question, so a
// deletion, rename, or path this repository never held remains
// distinguishable. The returned map contains one fact for every distinct
// input path.
func ClassifyPaths(evidence RepoEvidence, paths []string) (map[string]PathFact, error) {
	files, err := evidence.Files()
	if err != nil {
		return nil, err
	}
	tree := indexFiles(files)

	facts := make(map[string]PathFact, len(paths))
	unresolved := make(map[string]bool)
	for _, recorded := range paths {
		if _, seen := facts[recorded]; seen {
			continue
		}
		// Before the tree, and never a question of the history: the repository has nothing to say about a place
		// outside the project, and a history that is asked refuses it, taking the whole report with it.
		if climbsOutOfTheProject(recorded) {
			facts[recorded] = PathFact{Path: recorded, State: PathUnknown}
			continue
		}
		if tree.holds(recorded) {
			facts[recorded] = PathFact{Path: recorded, State: PathPresent}
			continue
		}
		unresolved[recorded] = true
	}
	if len(unresolved) == 0 {
		return facts, nil
	}

	history, err := evidence.History(sortedKeys(unresolved))
	if err != nil {
		return nil, err
	}
	for recorded, fact := range history {
		facts[recorded] = fact
	}
	return facts, nil
}

// climbsOutOfTheProject says whether a recorded path, once its detours are taken out ("a/../b" is "b"), leads to
// a place above the project's root. The strict pattern of the detectors admits a climb in the middle of a path
// ("a/../../outside.go"), so it is a path they can name. A detour that stays inside the project does not climb,
// and neither does a name that merely begins with dots ("..hidden/x.go").
//
// Owner's decision of 2026-10-13: such a path is unknown. Before it, the history was asked, git refused it, and
// the refusal was the answer for the whole report, which then printed nothing.
func climbsOutOfTheProject(recorded string) bool {
	cleaned := path.Clean(recorded)
	return cleaned == ".." || strings.HasPrefix(cleaned, "../")
}

// Detect reports which of observations the repository disagrees with.
//
// Only paths the tree cannot account for are worth a question of the history,
// and resolving by suffix first (what ClassifyPaths does) is what keeps a path
// recorded under another root from ever reaching the history as a suspected
// deletion.
func Detect(evidence RepoEvidence, observations []memory.Observation) ([]Finding, error) {
	named := make([][]string, len(observations))
	var all []string
	for i, o := range observations {
		named[i] = Paths(o.Content)
		all = append(all, named[i]...)
	}

	facts, err := ClassifyPaths(evidence, all)
	if err != nil {
		return nil, err
	}
	return findings(observations, named, facts), nil
}

// findings applies the rule to already-gathered facts: named[i] are the paths
// observations[i] names. It is separated from the gathering so the rule can be
// tested against facts that are awkward to build on disk -- an unknown path
// carrying a recent date above all.
func findings(observations []memory.Observation, named [][]string, facts map[string]PathFact) []Finding {
	var out []Finding
	for i, o := range observations {
		written, ok := observationTime(o)
		f := Finding{ObservationID: o.ID, Title: o.Title}
		for _, p := range named[i] {
			switch fact := facts[p]; fact.State {
			case PathDeleted:
				// Undatable memory is not reported as stale. Assuming a
				// zero time would make every deletion look later than
				// every memory, turning an unparsed timestamp into a
				// finding -- evidence manufactured from a formatting
				// difference.
				if ok && fact.At.After(written) {
					f.Removed = append(f.Removed, fact)
				}
			case PathRenamed:
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
func observationTime(o memory.Observation) (time.Time, bool) {
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

// indexFiles indexes every suffix of every file path of the working tree
// (slash-separated, relative to its root), so "promote/writer.go" resolves to
// "module/promote/writer.go" -- the false positive measured on the real
// database, where a path recorded relative to a different root looked
// removed while the file was alive.
func indexFiles(files []string) treeIndex {
	idx := treeIndex{bySuffix: map[string]bool{}}
	for _, file := range files {
		segments := strings.Split(file, "/")
		for i := range segments {
			idx.bySuffix[strings.Join(segments[i:], "/")] = true
		}
	}
	return idx
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
