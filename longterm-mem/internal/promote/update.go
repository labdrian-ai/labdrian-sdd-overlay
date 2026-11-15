package promote

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// ActionKind reports what UpdateInPlace (or, after 6.8, Writer.Promote)
// did to a promoted page.
//
// The integers are not a contract: nothing stores or sends them, and every
// caller outside this package compares a kind by name or renders it through
// String. Only ActionNone is promised to be the zero value.
type ActionKind int

const (
	// ActionNone means nothing was done: the zero value, so an Action nobody
	// filled in (the Action that comes back beside an error, or the Result of
	// an observation that was not eligible) can never read as a write.
	ActionNone ActionKind = iota
	// ActionCreated means a brand-new page was written (Writer.Promote
	// only; UpdateInPlace never returns this).
	ActionCreated
	// ActionUpdated means the on-disk page and the precedence store
	// entry for its address were refreshed with freshly rendered
	// content.
	ActionUpdated
	// ActionSkippedLocalEdit means the page's on-disk content diverged
	// from what longterm-mem itself last wrote there (R-030): the
	// content update was skipped and Diagnostic names the page.
	ActionSkippedLocalEdit
	// ActionSkippedUnreadablePages means no readable page belongs to the observation and a page could not be
	// read, so it cannot be told whether the observation already has a page: nothing was written for it, no
	// address was taken, and Diagnostic names the pages that could not be read. It is a skip like
	// ActionSkippedLocalEdit, and the Result carries no page.
	ActionSkippedUnreadablePages
	// ActionAlreadyPromoted means the memory of memorycard already has its page and a promote writes no
	// existing page: nothing was written, and the Result names the page it has (PromoteCard only).
	ActionAlreadyPromoted
)

// Action reports the outcome of one promotion write.
type Action struct {
	Kind ActionKind
	// Diagnostic is set only when Kind is one of the skips, ActionSkippedLocalEdit or
	// ActionSkippedUnreadablePages.
	Diagnostic *Diagnostic
}

// String renders k as the plain string both cmd_promote.go's CLI output
// and the MCP promote tool's PromoteOut.Action render over the wire
// (task 8b.11): one source of truth instead of two callers separately
// mapping ActionKind's int encoding to the same names.
func (k ActionKind) String() string {
	switch k {
	case ActionNone:
		return "none"
	case ActionCreated:
		return "created"
	case ActionUpdated:
		return "updated"
	case ActionSkippedLocalEdit:
		return "skipped_local_edit"
	case ActionSkippedUnreadablePages:
		return "skipped_unreadable_pages"
	case ActionAlreadyPromoted:
		return "already_promoted"
	default:
		return "unknown"
	}
}

// UpdateInPlace re-promotes an already-promoted page (R-008): the page
// promoted under page.Address, which pages reads and writes, is overwritten
// with page's freshly rendered content and store's entry for page.Address is
// updated to match. The write is refused, with a diagnostic naming the page's
// location, whenever those bytes are not provably
// longterm-mem's own last write (R-030): no store entry at all means
// unknown provenance (a lost or hand-deleted sidecar, a page promoted
// before tracking existed, a human's own file) and fails closed, and an
// entry disagreeing with the on-disk hashes is a local edit.
//
// One divergence is neither: the page write and the store's persistence are separate durable steps, so an
// interruption between them leaves the page ahead of the store. An interrupted CREATE leaves a page with no entry
// at all, so the tracked branch's comparisons are not even reachable; an interrupted UPDATE leaves new content
// fingerprinted by the PREVIOUS entry. When the retry renders the same revision, isOwnUnrecordedWrite settles both
// from the bytes, normalizing away only the two stamps EmitPage takes from its time argument -- without which the
// comparison would succeed on a same-day retry and fail every day after, which is worse than not reconciling at
// all. A retry that renders a different revision than the one on disk is refused as a local edit: preserving it is
// what R-030 is for, and `promote reconcile` is the way to adopt such a page.
//
// Writer's create path no longer opens the second window (it persists the fingerprint before publishing the
// page), but a vault already wedged by the old order stays wedged forever without this: the refusal is a skip, and
// a skip also suppresses the sidecar Save and the catalog/log repair that would have healed it.
//
// store is mutated in place; persisting it (PrecedenceStore.Save) is the
// caller's job, keeping this a narrow primitive alongside allocateAddress/
// EmitPage -- Writer (6.8) owns pairing the two writes per run.
func UpdateInPlace(pages PageRepository, store PrecedenceStore, page Page) (Action, error) {
	if memory.IsMissing(pages) {
		return Action{}, errNoPageRepository
	}
	existingPath := pages.PageLocation(page.Address)
	current, err := pages.ReadPage(page.Address)
	if err != nil {
		return Action{}, fmt.Errorf("promote: read %s: %w", existingPath, err)
	}

	fmBlock, ok := frontmatterBlock(current)
	if !ok {
		return Action{}, fmt.Errorf("promote: %s has no parseable frontmatter block", existingPath)
	}
	body := current[len(fmBlock):]
	rendered := page.Frontmatter + page.Body

	entry, tracked := store.Get(page.Address)
	switch {
	case !tracked:
		if !isOwnUnrecordedWrite(fmBlock, body, page) {
			return skippedAction(existingPath, page.Address, "was not recorded as written by longterm-mem, so its provenance is unknown"), nil
		}
		// Adopted: the page is our own interrupted write, so fall through
		// to the normal update below, which republishes it and records the
		// provenance the interrupted run never got to.
	case entry.BodyHash != hashText(body) || entry.FrontmatterHash != hashText(fmBlock):
		if !isOwnUnrecordedWrite(fmBlock, body, page) {
			return skippedAction(existingPath, page.Address, "was edited locally since longterm-mem last wrote it"), nil
		}
		// Reconciled: the page is an own write the store never caught up
		// with, so fall through to the normal update below, which
		// republishes it and records the provenance the interrupted run
		// never got to.
	}

	if err := pages.WritePage(page.Address, rendered); err != nil {
		return Action{}, err
	}
	store.Set(page.Address, entryFor(page))
	return Action{Kind: ActionUpdated}, nil
}

// entryFor builds the precedence entry that records page as longterm-mem's
// own last write: the two content hashes R-030 compares against, plus the
// revision that render carried, read back out of the rendered frontmatter
// rather than passed in separately so the recorded revision is by
// construction the one the page on disk actually shows.
func entryFor(page Page) PrecedenceEntry {
	entry := PrecedenceEntry{
		BodyHash:        hashText(page.Body),
		FrontmatterHash: hashText(page.Frontmatter),
	}
	if revision, ok := frontmatterRevision(page.Frontmatter); ok {
		entry.PromotedRevision = revision
	}
	return entry
}

// frontmatterRevision reads the revision a frontmatter block records for
// Engram (engram_revision, or source_revision beside source: engram, through
// revisionOf; no id is needed) as an integer. A block missing the field, or
// carrying something that is not an integer, reports false rather than a zero
// revision: every caller treats "no readable revision" as no evidence and
// fails closed, which a silent 0 would quietly turn into "revision 0", the
// smallest possible revision and therefore the most permissive answer.
func frontmatterRevision(fmBlock string) (int, bool) {
	raw, _ := revisionOf(parseFrontmatterFields(fmBlock), engramSource)
	if raw == "" {
		return 0, false
	}
	revision, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	return revision, true
}

// pageRevision reads the revision a frontmatter block records for whatever identity it names: memorycard's
// (memorycard_revision, or source_revision beside source: memorycard), else Engram's as frontmatterRevision reads it.
// Only `promote reconcile` reads a page's revision this way; the promotion paths keep reading the Engram one, so
// what they record does not change.
//
// Precedence and strictness: the memorycard identity is read first, then the Engram one, and the first identity
// whose revision field is present decides. A present but non-numeric revision reports false and never falls back to
// the other identity, so a conflicting page is refused rather than adopted at a guess. A block with no revision in
// either identity reports false, as frontmatterRevision does.
func pageRevision(fmBlock string) (int, bool) {
	fields := parseFrontmatterFields(fmBlock)
	for _, source := range []string{memorycardSource, engramSource} {
		raw, _ := revisionOf(fields, source)
		if raw == "" {
			continue
		}
		revision, err := strconv.Atoi(raw)
		return revision, err == nil
	}
	return 0, false
}

// volatileFrontmatterFields are the only two frontmatter values EmitPage
// takes from the time it is given (page.go's at) rather than from the
// observation, so they are the only two that can differ between two renders
// of identical Engram content. isOwnUnrecordedWrite normalizes exactly
// these away and nothing else: every other field, and the whole body, still
// has to match byte for byte.
var volatileFrontmatterFields = [...]string{"created", "updated"}

// isOwnUnrecordedWrite reports whether the page currently on disk (its
// frontmatter block and body, already split) is what this very promotion
// would write, ignoring the two wall-clock stamps above.
//
// It settles the one case the store cannot: a page with no entry at all.
// The create path publishes the page and the sidecar as two separate
// durable steps, so an interruption between them leaves a page nothing
// records -- and refusing that page as "unknown provenance" is permanent,
// because the refusal also suppresses the Save and the registration that
// would have repaired it. The bytes are the evidence the store lost.
//
// This never weakens the unknown-provenance guard it sits in front of. A
// page whose content is byte-identical to our own render carries nothing a
// human wrote that our render does not already reproduce, so adopting it
// destroys no edit; the only thing an adoption can discard is a
// hand-authored created:/updated: value, which the ordinary update path
// (which rewrites the whole page from EmitPage's own render) already
// discards on every run. Any other divergence -- one body character, one
// changed status: -- is still a refusal.
func isOwnUnrecordedWrite(fmBlock, body string, page Page) bool {
	return body == page.Body &&
		withoutVolatileStamps(fmBlock) == withoutVolatileStamps(page.Frontmatter)
}

// frontmatterEngramID reads the id of the Engram identity a frontmatter block names (engram_id or source_id,
// through identityOf), failing the
// same closed way frontmatterRevision does: an absent or unparseable value
// reports false rather than observation 0.
func frontmatterEngramID(fmBlock string) (int64, bool) {
	identity, named := identityOf(parseFrontmatterFields(fmBlock), engramSource)
	if !named {
		return 0, false
	}
	obsID, err := strconv.ParseInt(strings.TrimSpace(identity.ID), 10, 64)
	if err != nil {
		return 0, false
	}
	return obsID, true
}

// withoutVolatileStamps blanks the value of each volatileFrontmatterFields
// line in a frontmatter block, leaving the key (so a page that dropped the
// field entirely still differs from one that carries it) and every other
// line untouched. It is only ever applied to a frontmatter block, never to
// a body, so body prose opening with "created: " cannot be normalized away.
func withoutVolatileStamps(fmBlock string) string {
	return blankFrontmatterValues(fmBlock, volatileFrontmatterFields[:]...)
}

// blankFrontmatterValues blanks the value of every `key: ...` line in a
// frontmatter block whose key is one of keys, leaving the key itself (so a
// page that dropped the field entirely still differs from one that carries
// it) and every other line untouched. It is only ever applied to a
// frontmatter block, never to a body, so body prose opening with
// "created: " cannot be normalized away.
func blankFrontmatterValues(fmBlock string, keys ...string) string {
	lines := strings.Split(fmBlock, "\n")
	for i, line := range lines {
		for _, key := range keys {
			if strings.HasPrefix(line, key+": ") {
				lines[i] = key + ":"
			}
		}
	}
	return strings.Join(lines, "\n")
}

// skippedAction builds the R-030 refusal: reason states why the page's
// bytes are not provably longterm-mem's own, and the diagnostic names
// both the path and the address so a caller batching many pages can
// report exactly which one was left alone.
func skippedAction(existingPath, address, reason string) Action {
	return Action{
		Kind: ActionSkippedLocalEdit,
		Diagnostic: &Diagnostic{
			Rule:   "local-edit-precedence",
			Detail: fmt.Sprintf("%s (address %s) %s; content update skipped", existingPath, address, reason),
		},
	}
}

// unreadablePagesAction is the skip of a memory that no readable page carries while a page could not be read: it
// names the memory (as a memory of its store is named, noun) and every page that could not be read, and says
// nothing was written.
func unreadablePagesAction(noun, id string, unreadable []UnreadablePage) Action {
	return Action{
		Kind: ActionSkippedUnreadablePages,
		Diagnostic: &Diagnostic{
			Rule: "unreadable-pages",
			Detail: fmt.Sprintf("%s %s was not promoted: no readable page belongs to it, but a page that could not be read may: %s; nothing was written for it",
				noun, id, describePages(unreadable)),
		},
	}
}

// hashText returns text's sha256 hex digest (D6): the fingerprint
// PrecedenceEntry stores for a page's body and frontmatter separately.
func hashText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}
