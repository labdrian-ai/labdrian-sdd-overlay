package promote

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
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

// UpdateInPlace re-promotes an already-promoted page (R-008): existingPath
// is overwritten with page's freshly rendered content and store's entry
// for page.Address is updated to match. The write is refused, with a
// diagnostic naming existingPath, whenever those bytes are not provably
// longterm-mem's own last write (R-030): no store entry at all means
// unknown provenance (a lost or hand-deleted sidecar, a page promoted
// before tracking existed, a human's own file) and fails closed, and an
// entry disagreeing with the on-disk hashes is a local edit.
//
// One divergence is neither: the page write and the store's persistence
// are separate durable steps, so an interruption between them leaves new
// content fingerprinted by the previous entry. When the on-disk bytes are
// byte-identical to what this call would write, that prior write merely
// completed unrecorded -- the entry is re-derived from disk instead of
// misreading our own content as a local edit and skipping it forever.
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
		return skippedAction(existingPath, page.Address, "was not recorded as written by longterm-mem, so its provenance is unknown"), nil
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
