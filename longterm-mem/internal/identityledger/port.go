package identityledger

import (
	"time"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/adoption"
	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/projectid"
)

// Ledger is the identity ledger as the adoption use case sees it (adoption.Ledger): the file under a
// repository's git common directory, read and merged into by the functions of this package. It holds nothing.
type Ledger struct{}

var _ adoption.Ledger = Ledger{}

// Known returns every name the repository has been known by, most recently seen first, as the use case
// understands them. A ledger that is not there is emptiness; one that cannot be trusted is ErrCorrupt.
func (Ledger) Known(commonDir string) ([]adoption.KnownName, error) {
	entries, err := Names(commonDir)
	if err != nil {
		return nil, err
	}
	known := make([]adoption.KnownName, 0, len(entries))
	for _, e := range entries {
		known = append(known, adoption.KnownName{Name: e.Name, Adoptable: e.Adoptable})
	}
	return known, nil
}

// Record merges the names a repository derives into its ledger. A strict name may later be adopted on the
// ledger's word alone; a loose one may not.
func (Ledger) Record(commonDir string, derived []projectid.DerivedName, at time.Time) error {
	names := make([]Name, 0, len(derived))
	for _, d := range derived {
		names = append(names, Name{Name: d.Name, Rule: string(d.Rule), Adoptable: d.Strict})
	}
	return Record(commonDir, names, at)
}
