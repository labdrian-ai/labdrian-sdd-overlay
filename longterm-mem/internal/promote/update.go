package promote

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ActionKind reports what UpdateInPlace (or, after 6.8, Writer.Promote)
// did to a promoted page.
type ActionKind int

const (
	// ActionCreated means a brand-new page was written (Writer.Promote
	// only; UpdateInPlace never returns this).
	ActionCreated ActionKind = iota
	// ActionUpdated means the on-disk page and the precedence store
	// entry for its address were refreshed with freshly rendered
	// content.
	ActionUpdated
	// ActionSkippedLocalEdit means the page's on-disk content diverged
	// from what longterm-mem itself last wrote there (R-030): the
	// content update was skipped and Diagnostic names the page.
	ActionSkippedLocalEdit
)

// Action reports the outcome of one promotion write.
type Action struct {
	Kind ActionKind
	// Diagnostic is set only when Kind == ActionSkippedLocalEdit.
	Diagnostic *Diagnostic
}

// String renders k as the plain string both cmd_promote.go's CLI output
// and the MCP promote tool's PromoteOut.Action render over the wire
// (task 8b.11): one source of truth instead of two callers separately
// mapping ActionKind's int encoding to the same three names.
func (k ActionKind) String() string {
	switch k {
	case ActionCreated:
		return "created"
	case ActionUpdated:
		return "updated"
	case ActionSkippedLocalEdit:
		return "skipped_local_edit"
	default:
		return "unknown"
	}
}

// UpdateInPlace re-promotes an already-promoted page (R-008): existingPath
// is overwritten with page's freshly rendered content and store's entry
// for page.Address is updated to match. The write is refused, with a
// diagnostic naming existingPath, whenever those bytes are not provably
// longterm-mem's own last write (R-030): no store entry at all means
// unknown provenance (a lost or hand-deleted sidecar, a page promoted
// before tracking existed, a human's own file) and fails closed, and an
// entry disagreeing with the on-disk hashes is a local edit.
//
// Two divergences are neither, and both have the same cause: the page write
// and the store's persistence are separate durable steps, so an
// interruption between them leaves the page ahead of the store.
//
//   - An interrupted CREATE leaves a page with no entry at all, so the
//     tracked branch's comparisons are not even reachable.
//     isOwnUnrecordedWrite settles it from the bytes, normalizing away only
//     the two wall-clock stamps EmitPage takes from nowFunc -- without
//     which the comparison would succeed on a same-day retry and fail every
//     day after, which is worse than not reconciling at all.
//   - An interrupted UPDATE leaves new content fingerprinted by the
//     PREVIOUS entry. When the retry renders the same revision, the same
//     stamp-normalized comparison settles it. When Engram has moved on, no
//     comparison against the incoming bytes can: the retry renders a
//     different revision than the one on disk, so the two are supposed to
//     differ. isOwnUnrecordedUpdate settles that case from the page's own
//     engram_revision standing ahead of the revision the entry records --
//     a field only our own writes advance.
//
// Neither reconciliation ever adopts a page level with its entry's recorded
// revision and diverged in content: that is a human's edit, and preserving
// it is what R-030 is for.
//
// Writer's create path no longer opens the second window (it persists the
// fingerprint before publishing the page), but a vault already wedged by
// the old order stays wedged forever without this: the refusal is a skip,
// and a skip also suppresses the sidecar Save and the catalog/log repair
// that would have healed it.
//
// store is mutated in place; persisting it (PrecedenceStore.Save) is the
// caller's job, keeping this a narrow primitive alongside Allocate/
// EmitPage -- Writer (6.8) owns pairing the two writes per run.
func UpdateInPlace(store PrecedenceStore, page Page, existingPath string) (Action, error) {
	current, err := os.ReadFile(existingPath)
	if err != nil {
		return Action{}, fmt.Errorf("promote: read %s: %w", existingPath, err)
	}

	fmBlock, ok := frontmatterBlock(string(current))
	if !ok {
		return Action{}, fmt.Errorf("promote: %s has no parseable frontmatter block", existingPath)
	}
	body := string(current)[len(fmBlock):]
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
		if !isOwnUnrecordedWrite(fmBlock, body, page) && !isOwnUnrecordedUpdate(entry, fmBlock, body, page) {
			return skippedAction(existingPath, page.Address, "was edited locally since longterm-mem last wrote it"), nil
		}
		// Reconciled: the page is an own write the store never caught up
		// with, so fall through to the normal update below, which
		// republishes it and records the provenance the interrupted run
		// never got to.
	}

	if err := writeFileAtomic(existingPath, []byte(rendered)); err != nil {
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

// frontmatterRevision reads a frontmatter block's engram_revision as an
// integer. A block missing the field, or carrying something that is not an
// integer, reports false rather than a zero revision: every caller treats
// "no readable revision" as no evidence and fails closed, which a silent 0
// would quietly turn into "revision 0", the smallest possible revision and
// therefore the most permissive answer.
func frontmatterRevision(fmBlock string) (int, bool) {
	raw, ok := parseFrontmatterFields(fmBlock)[engramRevisionField]
	if !ok {
		return 0, false
	}
	revision, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, false
	}
	return revision, true
}

// volatileFrontmatterFields are the only two frontmatter values EmitPage
// takes from the wall clock (page.go's nowFunc) rather than from the
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

// isOwnUnrecordedUpdate settles the case isOwnUnrecordedWrite cannot: a
// TRACKED page whose on-disk bytes no longer match its entry because an
// earlier update published a render the sidecar never caught up with, and
// Engram has since moved on. The retry then renders a DIFFERENT revision
// than the one on disk, so no comparison against the incoming bytes --
// byte-for-byte, or with the wall-clock stamps normalized away -- can ever
// settle it: the two renders are of different revisions and are supposed
// to differ. Left unsettled, that page is a permanent skipped_local_edit,
// because a skip suppresses the very Save that would repair the entry.
//
// The page's own engram_revision is the evidence the sidecar lost. Only
// longterm-mem's own writes advance that field; a human editing a page
// leaves it exactly where the last promotion put it. So a page standing
// AHEAD of the revision its entry records is a page our own write moved,
// and one standing level with it is a page someone else changed -- which
// stays a refusal, since preserving that edit is the entire purpose of the
// branch this sits in.
//
// Three conditions, all required, and each fails closed:
//
//   - The entry records a revision at all. A zero is an entry written
//     before the field existed (or a status-only patch that inherited
//     one), which is no evidence, not "revision 0".
//   - The page's revision is above the entry's and no higher than the
//     incoming render's. A page claiming a revision Engram itself has not
//     reached is not a render we could have produced.
//   - The body ends in the promotion footer for the observation and the
//     exact revision the page claims. Corroboration, not proof: it costs
//     nothing and catches an appended note, a truncation or a replaced
//     body, but an edit buried mid-body still leaves the footer intact.
//     The fingerprint that could have separated those two is precisely
//     what the interruption destroyed, so the residual ambiguity is
//     irreducible -- narrowing it is the most that is available.
func isOwnUnrecordedUpdate(entry PrecedenceEntry, fmBlock, body string, page Page) bool {
	if entry.PromotedRevision <= 0 {
		return false
	}
	onDisk, ok := frontmatterRevision(fmBlock)
	if !ok {
		return false
	}
	incoming, ok := frontmatterRevision(page.Frontmatter)
	if !ok {
		return false
	}
	if onDisk <= entry.PromotedRevision || onDisk > incoming {
		return false
	}
	obsID, ok := frontmatterEngramID(page.Frontmatter)
	if !ok {
		return false
	}
	return strings.HasSuffix(body, promotionFooter(obsID, onDisk))
}

// frontmatterEngramID reads a frontmatter block's engram_id, failing the
// same closed way frontmatterRevision does: an absent or unparseable value
// reports false rather than observation 0.
func frontmatterEngramID(fmBlock string) (int64, bool) {
	raw, ok := parseFrontmatterFields(fmBlock)[engramIDField]
	if !ok {
		return 0, false
	}
	obsID, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
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
	lines := strings.Split(fmBlock, "\n")
	for i, line := range lines {
		for _, key := range volatileFrontmatterFields {
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

// hashText returns text's sha256 hex digest (D6): the fingerprint
// PrecedenceEntry stores for a page's body and frontmatter separately.
func hashText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}
