package promote

import (
	"context"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/memory"
)

// Outcome is what PromoteCardAndReindex did: the result of the promotion, and the failure of the index rebuild that
// followed a page being written, which sits beside a promotion that succeeded and is not its failure.
type Outcome struct {
	Result Result
	// IndexRebuildErr is the rebuild's failure, or nil -- which also covers "no rebuild was attempted", since a
	// promotion that wrote no page rebuilds nothing.
	IndexRebuildErr error
}

// PromoteCardAndReindex promotes the memory named by id explicitly (ExplicitPromoteCard) and, when that promotion wrote
// a page, rebuilds the vault's index before it returns: what the MCP promote tool does, so that a page promoted over MCP is
// found by MCP query without somebody running `sync` out of band. The error is the promotion's; the rebuild's failure
// is Outcome.IndexRebuildErr, beside the result, because the page is already written and durable by then.
// With no rebuilder wired, or one that holds a nil pointer, nothing is rebuilt.
func PromoteCardAndReindex(ctx context.Context, w *Writer, src Source, id int64, index IndexRebuilder) (Outcome, error) {
	result, err := ExplicitPromoteCard(ctx, w, src, id)
	if err != nil {
		return Outcome{}, err
	}
	return Outcome{Result: result, IndexRebuildErr: reindexAfterPromote(ctx, result, index)}, nil
}

// reindexAfterPromote rebuilds the vault index after a promotion, but only
// when that promotion actually WROTE a page.
//
// The rebuild is keyed off the promotion's OUTCOME, never off "promote was
// called". A rebuild walks the whole vault and shells out to the vault's
// own tooling; paying that for a promotion that wrote nothing is the entire
// cost for none of the benefit, on a surface a client can call in a loop.
// Two outcomes write nothing and both reach here as ordinary, non-error
// results: a refusal under local-edit precedence (ActionSkippedLocalEdit),
// and an ineligible observation, which Writer.Promote reports as a true
// no-op. "No error was returned" therefore does not mean a page was
// written, and cannot be the test.
//
// The returned error is the rebuild's own.
func reindexAfterPromote(ctx context.Context, result Result, rebuilder IndexRebuilder) error {
	if !promotionWrotePage(result) || memory.IsMissing(rebuilder) {
		return nil
	}
	return rebuilder.RebuildIndex(ctx)
}

// promotionWrotePage reports whether result describes a promotion that put
// bytes on disk.
//
// The kind alone answers it. Writer.Promote reports an INELIGIBLE observation
// as a zero Result, and the zero ActionKind is ActionNone, so a promotion that
// touched no file never reads as a write. (It used to be ActionCreated, the
// first iota, and a guard on the page address stood in front of the switch to
// catch that.)
func promotionWrotePage(result Result) bool {
	switch result.Action.Kind {
	case ActionCreated, ActionUpdated:
		return true
	default:
		return false
	}
}
