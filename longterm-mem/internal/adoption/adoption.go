// Package adoption is the use case that chooses the project a command acts on when the operator did not name one:
// it resolves the working directory's identity (internal/projectid) and lets what is already STORED decide which
// of the names the repository derives is the one used, so that memory is never split across two names of one
// repository. It also keeps the repository's identity ledger: it reads the names the repository was known by
// before, and records the names it derives now.
//
// The use case owns the ports it reads through. Whether a name already holds memory is Established (answered
// from the stores longterm-mem is allowed to consult, behind Stores); the names a repository was known by are
// the Ledger's; and the time a name is seen at is the Clock's. Nothing here opens a store, reads a file or asks
// the machine for the time: the composition root hands the use case the adapters, and a test hands it tables.
//
// Adoption runs only where longterm-mem is choosing the name itself. An explicit --project is the operator
// choosing it, and there the correspondence warning (projectid.Correspondence) already says when that choice
// disagrees with the directory. And it is for the CLI and only for the CLI: a tool of the MCP server must not
// adopt from the working directory of whatever host launched it, which has no relationship to the project a call
// is asking about, so adopting from it would bind observations to wherever the host happened to start.
package adoption

import (
	"fmt"
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/projectid"
)

// RecordPolicy decides whether an adoption records what the repository derives.
type RecordPolicy bool

const (
	// RecordDerived writes today's derived names into the repository's ledger.
	RecordDerived RecordPolicy = true
	// DoNotRecordDerived reads the ledger and writes nothing: for a command that promises not to change the
	// repository it is inspecting. Only the WRITE is skipped; the ledger is still READ, because reading is what
	// makes an already-established name win over a freshly derived one.
	DoNotRecordDerived RecordPolicy = false
)

// Established answers "does memory already live under this name?" from the stores longterm-mem may consult.
type Established interface {
	// HasMemory reports whether name already holds memory. The error says why that could not be told, which
	// is not the same as "no": it must never be read as an empty store.
	HasMemory(name string) (bool, error)
	// Close releases what the stores hold open.
	Close()
}

// Stores opens the stores that say whether a name already holds memory.
type Stores interface {
	// Open consults the stores. It returns what answers, and a note for each store it could not consult: a
	// store that cannot be read must not stop the command (most never touch it), but it must not read as
	// "nothing is established" in silence either, so the notes are part of what the caller reports.
	Open() (established Established, notes []string)
}

// KnownName is a name a repository was known by before, as its ledger remembers it.
type KnownName struct {
	Name string
	// Adoptable is whether the name may be adopted on the ledger's word alone, once nothing derives it. It is
	// false for the loose spellings (a bare directory name can name somebody else's repository).
	Adoptable bool
}

// Ledger remembers the names a repository has been known by, in the repository's own git directory.
type Ledger interface {
	// Known returns the names the repository whose common directory is commonDir was known by, most recently
	// seen first. A ledger there is none of is empty, not broken.
	Known(commonDir string) ([]KnownName, error)
	// Record merges the names the repository derives into its ledger, stamping at as when they were seen.
	Record(commonDir string, derived []projectid.DerivedName, at time.Time) error
}

// Clock tells the time a name is seen at.
type Clock interface {
	Now() time.Time
}

// UseCase is the adoption, over the ports it owns.
type UseCase struct {
	Identity projectid.Resolver
	Stores   Stores
	Ledger   Ledger
	Clock    Clock
}

// Adopt resolves dir's identity and lets storage override which of the derivable names is used, also
// considering the names the ledger remembers. It returns the adoption and the notes the operator is owed: a
// store that could not be consulted, a ledger that could not be read or written. A note is never a failure: the
// command the operator actually asked for has nothing to do with a record that could not be kept, and refusing
// to run because of one would be its own kind of memory loss.
//
// The ledger is consulted BEFORE resolving, because it answers what this repository was called before. It is
// written after only when the policy says so.
func (u UseCase) Adopt(dir string, policy RecordPolicy) (projectid.Adoption, []string, error) {
	established, notes := u.Stores.Open()
	defer established.Close()

	remembered, commonDir, ledgerNotes := u.remembered(dir)
	notes = append(notes, ledgerNotes...)

	a, err := u.Identity.AdoptWith(dir, projectid.AdoptOptions{
		Established: established.HasMemory,
		Remembered:  remembered,
	})
	if err != nil {
		return projectid.Adoption{}, notes, err
	}

	if policy == RecordDerived {
		notes = append(notes, u.record(commonDir, a.Derived)...)
	}
	return a, notes, nil
}

// remembered reads the names this repository was known by from its ledger, and returns the common directory the
// ledger lives under so the caller can write back to the same place.
//
// Only the STRICT names are offered for adoption. A ledger's bare directory name can equally name somebody
// else's repository, and adopting it would bind this one to another project's memory -- see
// projectid.DerivedName's Strict field for why the visible failure is the better trade.
func (u UseCase) remembered(dir string) (names []string, commonDir string, notes []string) {
	commonDir, err := u.Identity.CommonDir(dir)
	if err != nil {
		// Not a repository, or unreadable metadata. AdoptWith is about to report that far more precisely than
		// this path could.
		return nil, "", nil
	}

	known, err := u.Ledger.Known(commonDir)
	if err != nil {
		return nil, commonDir, []string{fmt.Sprintf("this repository's identity ledger could not be read (%v)", err)}
	}
	for _, k := range known {
		if k.Adoptable {
			names = append(names, k.Name)
		}
	}
	return names, commonDir, nil
}

// record writes today's derived names into the repository's ledger, so a name stays findable after the
// repository stops deriving it.
func (u UseCase) record(commonDir string, derived []projectid.DerivedName) []string {
	if commonDir == "" || len(derived) == 0 {
		return nil
	}
	if err := u.Ledger.Record(commonDir, derived, u.Clock.Now().UTC()); err != nil {
		return []string{fmt.Sprintf("this repository's identity ledger could not be written (%v)", err)}
	}
	return nil
}
