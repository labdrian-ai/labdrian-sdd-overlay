package promote

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrPageNotFound is Reconcile's error when the named address has no
// promoted page at all -- a typo, or an address from a report the vault has
// since moved past. Distinguishable via errors.Is so the CLI can answer its
// not_found exit code without string-matching a message.
var ErrPageNotFound = errors.New("promote: no promoted page at that address")

// ErrLocalEditPreserved is Reconcile's refusal for a page that is not
// wedged at all: its entry records a positive revision and its content has
// diverged from that entry. That is an ordinary local edit, which the
// local-edit precedence rule (R-030) exists to preserve -- promotion
// refuses that page on every run, names it, and leaves it byte-identical,
// and the page is held rather than lost.
var ErrLocalEditPreserved = errors.New("promote: page has a local edit the precedence rule preserves")

// ReconcileOutcome reports what Reconcile did for one address.
type ReconcileOutcome struct {
	// Address and Path name the page reconcile looked at.
	Address string
	Path    string
	// Adopted reports whether the precedence store was actually written.
	// False means the entry already described the page and there was
	// nothing to reconcile.
	Adopted bool
	// PromotedRevision is the revision now recorded for the page.
	PromotedRevision int
}

// Reconcile adopts ONE named, already-promoted page into the precedence
// store: it records the page's current on-disk state as longterm-mem's own
// last write, so future promotions of that page proceed normally.
//
// It exists for the residue the automatic path deliberately refuses.
// Promotion cannot attribute a diverged page to one of its own interrupted
// writes unless the page's own engram_revision stands above the revision
// its entry records; an entry that records no revision, or no entry at all,
// carries no such evidence, so the page is refused -- and that refusal is a
// SKIP, which suppresses the very store write that would have repaired the
// entry. Every later run repeats it identically. `doctor`'s
// precedence-sidecar check names exactly those pages, and until now naming
// them was all anyone could do.
//
// A human naming one address is the unambiguous consent the automatic path
// lacks, and it is the whole design. Reconcile is per address on purpose:
// there is no bulk form, because a bulk form would reintroduce, behind a
// flag, the silent mass-adoption that ambiguity rules out. The refusal of a
// bulk invocation lives at the command surface (cmd_promote.go), which is
// where the addresses are named.
//
// Two states are NOT wedged and are answered differently, because they are
// different things:
//
//   - An entry that already fingerprints the page is a NO-OP, not an error.
//     Reconcile is what an operator runs off a doctor report, and doctor
//     reports and repairs are separated by however long it takes to read
//     one; a page repaired in between (by an ordinary promotion, which is
//     how the legacy population shrinks) must not make the command fail. An
//     idempotent command can be re-run and scripted; one that errors on
//     "already fine" cannot.
//
//   - An entry recording a POSITIVE revision whose page has diverged is an
//     ordinary local edit, and it is REFUSED (ErrLocalEditPreserved). That
//     page is not wedged: R-030 is holding a human's edit, promotion says
//     so on every run, and the edit is preserved rather than lost. Adopting
//     it here would overwrite that edit on the next promotion while
//     reporting an ordinary update -- which is precisely the silent
//     adoption this design removed, and being asked for it by name does not
//     change what would be destroyed. The remedy is the human's: keep the
//     edit (and leave the page refused), or fold it into Engram and let the
//     ordinary promotion path rewrite the page.
func Reconcile(vaultRoot, address string) (ReconcileOutcome, error) {
	if address == "" {
		return ReconcileOutcome{}, fmt.Errorf("promote: reconcile requires a page address")
	}

	rel := filepath.Join(pagePathPrefix, address+".md")
	full := filepath.Join(vaultRoot, rel)
	raw, err := os.ReadFile(full)
	if err != nil {
		if os.IsNotExist(err) {
			return ReconcileOutcome{}, fmt.Errorf("%w: %s", ErrPageNotFound, address)
		}
		return ReconcileOutcome{}, fmt.Errorf("promote: read %s: %w", full, err)
	}

	store, err := LoadPrecedenceStore(vaultRoot)
	if err != nil {
		return ReconcileOutcome{}, err
	}

	entry, tracked := store.Get(address)
	switch {
	case tracked && entry.MatchesPage(string(raw)):
		return ReconcileOutcome{Address: address, Path: rel, PromotedRevision: entry.PromotedRevision}, nil
	case tracked && entry.PromotedRevision > 0:
		return ReconcileOutcome{}, fmt.Errorf("%w: %s records revision %d for %s and the page no longer matches it; promotion refuses that page to keep the edit, and reconcile does not override it", ErrLocalEditPreserved, precedenceManifestRelPath, entry.PromotedRevision, address)
	}

	fmBlock, ok := frontmatterBlock(string(raw))
	if !ok {
		return ReconcileOutcome{}, fmt.Errorf("promote: %s has no parseable frontmatter block", full)
	}
	// The revision is read off the PAGE, never supplied by the caller: the
	// entry has to describe the bytes that are actually there. A page whose
	// engram_revision cannot be read as a positive integer is refused,
	// because adopting it would record another entry with no usable
	// revision -- the exact state this command exists to end, recreated
	// while reporting success.
	revision, ok := frontmatterRevision(fmBlock)
	if !ok || revision <= 0 {
		return ReconcileOutcome{}, fmt.Errorf("promote: %s carries no usable engram_revision, so reconciling it would record an entry every later promotion refuses just as it refuses this one", full)
	}

	store.Set(address, PrecedenceEntry{
		BodyHash:         hashText(string(raw)[len(fmBlock):]),
		FrontmatterHash:  hashText(fmBlock),
		PromotedRevision: revision,
	})
	if err := store.Save(vaultRoot); err != nil {
		return ReconcileOutcome{}, err
	}
	return ReconcileOutcome{Address: address, Path: rel, Adopted: true, PromotedRevision: revision}, nil
}
