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
//     a field only our own writes advance -- corroborated by the body's
//     promotion footer and by the frontmatter around the handful of lines
//     two of our own renders may legitimately disagree on.
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
// Every one of these is required, and each fails closed:
//
//   - Both renders name a revision, and revisionsAllowAdoption accepts the
//     pair against the entry (see there: only a RECORDED entry can answer
//     this at all, so a legacy entry -- hashes with no revision -- is
//     refused unconditionally).
//   - The body ends in the promotion footer for the observation and the
//     exact revision the page claims. That catches an appended note, a
//     truncation or a replaced body.
//   - The frontmatter, normalized by corroboratingFrontmatter, is the block
//     this very promotion would render. That catches every key a human
//     added, changed or deleted outside the handful of lines that
//     legitimately differ between two of our own renders -- which is what
//     the footer check alone does NOT catch, and the reason a
//     frontmatter-only edit inside this window used to be adopted and
//     overwritten while the run reported action=updated.
//
// What remains uncorroborated is an edit buried MID-BODY (the footer
// survives it, and the body has no second witness) and an edit confined to
// the lines corroboratingFrontmatter has to blank: created, updated,
// engram_revision, status and related (the ones WE move), plus title,
// aliases, tags, engram_type, engram_sync_id and project (the ones the
// OBSERVATION moves). A human who hand-sets title: or inserts an alias
// inside this window therefore loses that edit to action=updated. The
// fingerprint that would have separated those from our own write is
// precisely what the interruption destroyed, and every one of those lines is
// a line one of our own renders legitimately rewrites -- so keeping any of
// them compared does not preserve the edit, it only converts the loss into a
// permanent refusal of a page nothing is wrong with. The residual is real,
// and it is bounded: type, address, sources and engram_id still have to
// match exactly, and so does every key a human added or deleted.
func isOwnUnrecordedUpdate(entry PrecedenceEntry, fmBlock, body string, page Page) bool {
	onDisk, ok := frontmatterRevision(fmBlock)
	if !ok {
		return false
	}
	incoming, ok := frontmatterRevision(page.Frontmatter)
	if !ok {
		return false
	}
	if !revisionsAllowAdoption(entry, onDisk, incoming) {
		return false
	}
	obsID, ok := frontmatterEngramID(page.Frontmatter)
	if !ok {
		return false
	}
	if !strings.HasSuffix(body, promotionFooter(obsID, onDisk)) {
		return false
	}
	return corroboratingFrontmatter(fmBlock) == corroboratingFrontmatter(page.Frontmatter)
}

// revisionsAllowAdoption reports whether the revision the page on disk
// claims can be attributed to one of OUR interrupted writes rather than to
// a human, given what the entry records.
//
// Only a RECORDED entry can answer that. It names the revision it
// fingerprinted, so the page has to stand strictly above it (only our own
// renders advance engram_revision, so a page level with the entry was
// changed by someone else) and no higher than the render coming in (a page
// claiming a revision Engram itself has not reached is not a render we
// could have produced).
//
// A LEGACY entry -- hashes only, no revision -- can answer nothing, and
// this function says so. The tempting inference is that a moved frontmatter
// hash proves our own unrecorded write, because every render of ours puts
// engram_revision in the frontmatter and therefore always moves that hash.
// It does not: a human editing ANY frontmatter line moves that same hash
// identically, so the predicate is satisfied by every legacy-tracked page a
// human has ever hand-edited -- no interruption required. Combined with the
// line kinds corroboratingFrontmatter has to blank (created, updated, status,
// related and every observation-derived field, which between them are most of
// the lines a human touches by hand), it
// would unlock adoption across the whole legacy population and overwrite
// mid-body edits while reporting action=updated. In the package whose
// guarantee (R-030) is preserving human edits, ambiguous evidence must
// refuse, so a diverged legacy entry is refused.
//
// The legacy fixed point that inference was meant to break is broken the
// unambiguous way instead: a legacy entry whose page still matches its
// recorded fingerprints is NOT diverged, so it never reaches this branch at
// all -- it takes the ordinary update path, and entryFor records the
// revision the older build never wrote. The legacy population therefore
// shrinks page by page with nothing adopted. What stays wedged is only a
// legacy entry whose page has ALSO diverged, and that residue is reported
// by doctor's precedence-sidecar check rather than left silent.
func revisionsAllowAdoption(entry PrecedenceEntry, onDisk, incoming int) bool {
	if entry.PromotedRevision <= 0 {
		return false
	}
	return onDisk > entry.PromotedRevision && onDisk <= incoming
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
	return blankFrontmatterValues(fmBlock, volatileFrontmatterFields[:]...)
}

// corroboratingFrontmatter reduces a frontmatter block to the part two of
// our OWN renders, of two DIFFERENT revisions of the same observation, must
// still agree on -- so that comparing the page on disk against the incoming
// render is a real test of "did this block come out of Render()" rather
// than a comparison guaranteed to fail.
//
// Two kinds of line are blanked, and nothing else is:
//
//   - the lines two of our own renders may legitimately differ on because
//     WE moved them: created and updated, the stamps EmitPage takes from
//     the wall clock (withoutVolatileStamps); engram_revision, which
//     differs by construction, since the whole premise of this
//     reconciliation is that the page holds an older revision than the one
//     now being rendered; and status and related, the two lines Propagate
//     (R-033) rewrites in place on a page it never re-bodies, recording
//     only the patched block's hash.
//   - the lines two of our own renders may legitimately differ on because
//     the OBSERVATION moved them: title, aliases, tags, engram_type,
//     engram_sync_id and project are ALL rendered from the observation
//     (page.go's EmitPage reads obs.Title, obs.Type, obs.SyncID and
//     obs.Project), so a retitle, a retype, an Engram project move or merge,
//     or a sync_id backfilled onto an observation that had none changes them
//     between the interrupted write and the retry with no human touching the
//     page. Comparing any of them refuses an ordinary Engram-side move --
//     and R-008 scenario 2 makes a retitle a first-class supported case --
//     while that refusal is a skip, so it suppresses the Save, the entry
//     never advances, and every later revision repeats it: a permanent
//     wedge, the exact class this reconciliation exists to end. The set
//     blanked here is therefore exactly "every field the observation
//     supplies", not a hand-picked subset of it: leaving one such field
//     compared to keep it as a human-edit witness buys that witness at the
//     price of a permanent wedge on an ordinary Engram operation, and this
//     package's whole reason for reconciling is to end wedges.
//
// Every other key keeps its value, and a key present in one block and
// absent from the other still differs -- which is what makes a human's
// added, changed or deleted frontmatter key visible here. What is left
// covered is what the observation does not supply and only Render() writes:
// type, address, sources and engram_id.
func corroboratingFrontmatter(fmBlock string) string {
	normalized := blankFrontmatterValues(withoutVolatileStamps(fmBlock),
		engramRevisionField, statusField, titleField, engramTypeField,
		engramSyncIDField, projectField)
	for _, key := range []string{relatedField, aliasesField, tagsField} {
		normalized = blankFrontmatterListSection(normalized, key)
	}
	return normalized
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

// blankFrontmatterListSection replaces a frontmatter block's key: list
// section -- either the inline `key: []` form or a `key:` header followed
// by its `  - ` item lines, the two shapes writeListField emits -- with the
// bare key, so both forms normalize to the same thing.
//
// A block with no such section is returned untouched rather than having one
// inserted: a page whose key a human deleted must keep differing from one
// that still carries it, exactly as blankFrontmatterValues leaves a dropped
// scalar visible. (This is the one behavioral difference from
// frontmatter.go's setListField, which inserts, because a patch that
// reports success having written nothing is wrong for a WRITER and right
// for a COMPARATOR.)
func blankFrontmatterListSection(fmBlock, key string) string {
	lines := strings.Split(fmBlock, "\n")
	start := -1
	for i, line := range lines {
		if strings.HasPrefix(line, key+":") {
			start = i
			break
		}
	}
	if start == -1 {
		return fmBlock
	}

	end := start + 1
	if lines[start] != key+": []" {
		for end < len(lines) && strings.HasPrefix(lines[end], "  - ") {
			end++
		}
	}

	blanked := make([]string, 0, len(lines)-(end-start)+1)
	blanked = append(blanked, lines[:start]...)
	blanked = append(blanked, key+":")
	blanked = append(blanked, lines[end:]...)
	return strings.Join(blanked, "\n")
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
