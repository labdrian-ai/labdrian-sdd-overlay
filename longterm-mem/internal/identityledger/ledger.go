// Package identityledger remembers every name a repository has been known
// by, so a name stops being forgettable the moment nothing derives it.
//
// Why it exists. internal/projectid derives a repository's identity from
// what the repository looks like RIGHT NOW: its declared file, its origin
// remote, the realpath of its git common directory. Derivation alone has a
// hole in it, and the hole is one-directional in the way that always costs
// memory: a repository that MOVES stops deriving its old path, so memory
// stored under that path becomes unreachable -- not wrong, unreachable, and
// silently so. Nothing in a purely derived identity can reunite it.
//
// The ledger closes that hole by writing what was derived, when it was
// derived, under the git COMMON directory: the one directory a main
// checkout and every linked worktree share, and -- the property that makes
// this work at all -- a directory bound to the repository as an OBJECT
// rather than to its location or its name. Move the repository and the
// ledger moves inside it. Put a different repository at the old path and it
// brings its own, empty, ledger; it cannot inherit a claim it never made.
// gentle-ai's review transactions live in the same place for the same
// reason.
//
// The ledger carries a digest of its own entries. A record that can no
// longer prove it describes what it recorded is the defect this module
// keeps meeting in every other form, so a ledger whose digest does not
// match its contents is REPORTED (ErrCorrupt) rather than trusted: a
// silently trusted tampered ledger would adopt a name nobody ever wrote.
package identityledger

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/durable"
)

// Schema is the ledger's schema string, following gentle-ai's own
// convention of naming and versioning every record it persists.
const Schema = "longterm-mem.identity-ledger/v1"

// ErrCorrupt is returned when a ledger exists but cannot be trusted: it does
// not parse, or its digest does not match its entries.
var ErrCorrupt = errors.New("identityledger: ledger does not match its own digest")

// Name is one name a repository has been known by.
type Name struct {
	Name string `json:"name"`
	// Rule is the projectid rule that derived it, carried so a reader can
	// tell a human's declaration from a path-derived fallback.
	Rule string `json:"rule"`
	// Adoptable is whether this name may be adopted on the ledger's word
	// alone, once nothing derives it any more.
	//
	// It is false for the loose spellings -- a repository's bare directory
	// name, say -- and the reason is asymmetric cost. A strict identity (a
	// declared name, a normalized remote, an absolute common-dir path) can
	// realistically name only this repository. A bare directory name can
	// name somebody else's, and adopting it would bind this repository to
	// another project's memory: silent, and wrong. Refusing to adopt it
	// costs a reunion that stays visible and is fixable with a declared
	// file. Prefer the visible failure.
	Adoptable bool      `json:"adoptable"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
}

// ledger is the on-disk record.
type ledger struct {
	Schema string `json:"schema"`
	// Revision is the digest of Entries, making the record self-verifying.
	Revision string `json:"revision"`
	Entries  []Name `json:"entries"`
}

// dirName is the directory the ledger lives in, under the common dir.
const dirName = "longterm-mem"

// fileName is the ledger file itself.
const fileName = "identity-ledger.json"

// Path returns the ledger file for the repository whose git common
// directory is commonDir.
func Path(commonDir string) string {
	return filepath.Join(commonDir, dirName, fileName)
}

// Names returns every name the repository has been known by, most recently
// seen first -- the most recent name being the likeliest one the memory
// actually lives under. An absent ledger is emptiness, not breakage.
func Names(commonDir string) ([]Name, error) {
	l, err := load(commonDir)
	if err != nil {
		return nil, err
	}
	return l.Entries, nil
}

// Record merges names into the repository's ledger, stamping now as when
// each was seen. A name already present keeps its FirstSeen and moves its
// LastSeen: every command records, so duplicating here would grow a record
// nobody can read.
func Record(commonDir string, names []Name, now time.Time) error {
	l, err := load(commonDir)
	if err != nil {
		return err
	}

	byName := make(map[string]int, len(l.Entries))
	for i, e := range l.Entries {
		byName[e.Name] = i
	}
	for _, n := range names {
		if n.Name == "" {
			continue
		}
		if i, ok := byName[n.Name]; ok {
			l.Entries[i].Rule = n.Rule
			l.Entries[i].Adoptable = n.Adoptable
			l.Entries[i].LastSeen = now
			continue
		}
		n.FirstSeen, n.LastSeen = now, now
		l.Entries = append(l.Entries, n)
		byName[n.Name] = len(l.Entries) - 1
	}

	sortEntries(l.Entries)
	l.Schema = Schema
	l.Revision = digest(l.Entries)
	return write(commonDir, l)
}

// sortEntries orders newest-LastSeen first, breaking ties by name so the
// file is byte-stable for an unchanged set -- a record that churns on every
// write is one nobody can diff.
func sortEntries(entries []Name) {
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].LastSeen.Equal(entries[j].LastSeen) {
			return entries[i].Name < entries[j].Name
		}
		return entries[i].LastSeen.After(entries[j].LastSeen)
	})
}

func load(commonDir string) (ledger, error) {
	raw, err := os.ReadFile(Path(commonDir))
	if os.IsNotExist(err) {
		return ledger{Schema: Schema}, nil
	}
	if err != nil {
		return ledger{}, fmt.Errorf("identityledger: reading %s: %w", Path(commonDir), err)
	}

	var l ledger
	if err := json.Unmarshal(raw, &l); err != nil {
		return ledger{}, fmt.Errorf("%w: %s does not parse: %v", ErrCorrupt, Path(commonDir), err)
	}
	if want := digest(l.Entries); l.Revision != want {
		return ledger{}, fmt.Errorf("%w: %s carries %s but its entries digest to %s", ErrCorrupt, Path(commonDir), l.Revision, want)
	}
	return l, nil
}

func write(commonDir string, l ledger) error {
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return fmt.Errorf("identityledger: marshal: %w", err)
	}
	data = append(data, '\n')

	path := Path(commonDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("identityledger: create directory for %s: %w", path, err)
	}
	if err := durable.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("identityledger: %w", err)
	}
	return nil
}

// digest hashes the entries the same way for a reader and a writer, so the
// revision proves the file was not edited behind the module's back.
func digest(entries []Name) string {
	canonical, err := json.Marshal(entries)
	if err != nil {
		// json.Marshal of this shape cannot fail; a digest that could
		// silently become "" would defeat the check entirely.
		panic(fmt.Sprintf("identityledger: digesting entries: %v", err))
	}
	return fmt.Sprintf("sha256:%x", sha256.Sum256(canonical))
}
