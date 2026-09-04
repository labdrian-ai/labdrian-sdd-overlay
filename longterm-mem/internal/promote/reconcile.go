package promote

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrPageNotFound is Reconcile's error when the named address has no
// promoted page at all -- a typo, or an address from a report the vault has
// since moved past. Distinguishable via errors.Is so the CLI can answer its
// not_found exit code without string-matching a message.
var ErrPageNotFound = errors.New("promote: no promoted page at that address")

// ErrLocalEditPreserved is Reconcile's refusal for a page that is not
// wedged at all: its entry records a positive revision and its content has
// diverged from that entry. That is an ordinary local edit, which the
// local-edit precedence rule (R-030) exists to preserve -- promotion names
// that page and leaves it byte-identical, and the page is held rather than
// lost.
//
// The refusal is not unconditional, and saying it is would be untrue: when
// the page's OWN engram_revision stands strictly above the revision the
// entry records, promotion attributes it to one of its own interrupted
// updates and adopts it (update.go's isOwnUnrecordedUpdate) -- only our own
// renders advance that field. What reconcile declines to do is override the
// refusal in the case where that evidence is absent, which is where an
// edit would actually be destroyed.
var ErrLocalEditPreserved = errors.New("promote: page has a local edit the precedence rule preserves")

// ErrInvalidAddress is Reconcile's refusal for an address that is not a
// promoted-page address at all. The address is operator-supplied and names
// BOTH a file to read and the key an entry is written under, so an
// unvalidated one is two defects at once: it reads a file outside
// wiki/memory (`../../secret`), and it writes a sidecar key no promotion
// will ever look up, which nothing removes.
var ErrInvalidAddress = errors.New("promote: not a promoted-page address")

// ErrNotThatPage is Reconcile's refusal when wiki/memory/<address>.md
// exists but its own frontmatter says it is some other page: another
// address (a copy or a rename), or another project's page. Adopting it
// would fingerprint content under a key that does not describe it.
var ErrNotThatPage = errors.New("promote: the file at that address is not that promoted page")

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
//     page is not wedged: R-030 is holding a human's edit, and promotion
//     either adopts it (when the page's own engram_revision stands above
//     the entry's, which is our own interrupted update) or names it on
//     every run, so the edit is preserved rather than lost. Adopting
//     it here would overwrite that edit on the next promotion while
//     reporting an ordinary update -- which is precisely the silent
//     adoption this design removed, and being asked for it by name does not
//     change what would be destroyed. The remedy is the human's: keep the
//     edit (and leave the page refused), or fold it into Engram and let the
//     ordinary promotion path rewrite the page.
func Reconcile(vaultRoot, project, address string) (ReconcileOutcome, error) {
	// The address is checked against the allocator's own shape BEFORE it
	// touches the filesystem, because it is used twice: as a path segment
	// and as the sidecar key. addressPattern (lint.go) is the same
	// ^c-\d{6}$ every promoted page is linted against, and it admits no
	// separator, no dot and no `..`, so a resolved path can only land
	// directly inside wiki/memory.
	if !addressPattern.MatchString(address) {
		return ReconcileOutcome{}, fmt.Errorf("%w: %q does not match %s, so it names neither a page under %s nor a key any promotion looks up", ErrInvalidAddress, address, addressPattern, pagePathPrefix)
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

	fmBlock, ok := frontmatterBlock(string(raw))
	if !ok {
		return ReconcileOutcome{}, fmt.Errorf("promote: %s has no parseable frontmatter block", full)
	}
	// The filename says which page this is supposed to be; the page's own
	// frontmatter says which page it IS. They can disagree -- a copied or
	// renamed file, a hand-made one, or another project's page under a
	// colliding address -- and every one of those adopts content under a
	// key that does not describe it. The project is compared only when the
	// page carries one: `project:` is rendered by every current build, so a
	// page that lacks it predates the field, and refusing the legacy
	// population for a field it never had would decline the pages this
	// command exists for. An address is different: it is a required page
	// field, so its absence is corruption, not age.
	fields := parseFrontmatterFields(fmBlock)
	if got := strings.TrimSpace(fields["address"]); got != address {
		return ReconcileOutcome{}, fmt.Errorf("%w: %s carries address %q, not %q", ErrNotThatPage, full, got, address)
	}
	if got := strings.TrimSpace(fields[projectField]); got != "" && got != project {
		return ReconcileOutcome{}, fmt.Errorf("%w: %s belongs to project %q, not %q", ErrNotThatPage, full, got, project)
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
		return ReconcileOutcome{}, fmt.Errorf("%w: %s records revision %d for %s and the page no longer matches it; promotion adopts such a page only when the page's own engram_revision stands above %d (which only longterm-mem's own writes advance) and otherwise refuses it to keep the edit, and reconcile does not override that refusal", ErrLocalEditPreserved, precedenceManifestRelPath, entry.PromotedRevision, address, entry.PromotedRevision)
	}

	// The revision is read off the PAGE, never supplied by the caller: the
	// entry has to describe the bytes that are actually there.
	//
	// ZERO IS ADOPTED. Eligibility (eligible.go) promotes a pinned or
	// eligible-typed observation whose revision_count is 0, and EmitPage
	// copies that 0 into the page, so revision 0 is an ordinary promoted
	// state reached by the ordinary path -- and doctor's wedge predicate
	// (PromotedRevision <= 0 && !MatchesPage) names exactly those pages
	// once they diverge. Refusing them left the only advertised repair
	// declining the population it was written for. The refusal's premise
	// was also false: the wedge is an entry that no longer MATCHES its
	// page, and an adopted entry fingerprints the bytes on disk, so the
	// next promotion never reaches UpdateInPlace's divergence branch at
	// all -- it updates, exactly as reconcile_repair_test.go drives it.
	//
	// A MISSING OR NON-NUMERIC revision is still refused, and that is a
	// different thing from zero: it is not a value any render of ours
	// produces, so it is corruption or a hand-edit, and there is no
	// revision to record. So is a negative one.
	revision, ok := frontmatterRevision(fmBlock)
	if !ok || revision < 0 {
		return ReconcileOutcome{}, fmt.Errorf("promote: %s carries no readable engram_revision, so there is no revision to record for it; a page promoted at revision 0 is adopted, but a missing or non-numeric one is corruption", full)
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
