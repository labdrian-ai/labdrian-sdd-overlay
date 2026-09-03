package promote

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
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
//   - An interrupted UPDATE leaves new content fingerprinted by the
//     previous entry. When the on-disk bytes are byte-identical to what
//     this call would write, that prior write merely completed unrecorded
//     -- the entry is re-derived from disk instead of misreading our own
//     content as a local edit and skipping it forever.
//   - An interrupted CREATE leaves a page with no entry at all, so the
//     reconciliation above is not even reachable. isOwnUnrecordedWrite
//     settles that case from the bytes instead, normalizing away only the
//     two wall-clock stamps EmitPage takes from nowFunc -- without which
//     the comparison would succeed on a same-day retry and fail every day
//     after, which is worse than not reconciling at all.
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
		if string(current) != rendered {
			return skippedAction(existingPath, page.Address, "was edited locally since longterm-mem last wrote it"), nil
		}
		// Already holds exactly this content: an own write whose store
		// update never landed. Re-derive the entry instead of skipping.
		store.Set(page.Address, PrecedenceEntry{
			BodyHash:        hashText(page.Body),
			FrontmatterHash: hashText(page.Frontmatter),
		})
		return Action{Kind: ActionUpdated}, nil
	}

	if err := writeFileAtomic(existingPath, []byte(rendered)); err != nil {
		return Action{}, err
	}
	store.Set(page.Address, PrecedenceEntry{
		BodyHash:        hashText(page.Body),
		FrontmatterHash: hashText(page.Frontmatter),
	})
	return Action{Kind: ActionUpdated}, nil
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
