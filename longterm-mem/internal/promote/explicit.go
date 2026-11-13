package promote

import (
	"context"
	"errors"
	"fmt"
	"strconv"
)

// ErrObservationNotFound is ExplicitPromote's error when the source reports no
// such memory -- R-032's "rejected with a clear error rather than
// silently doing nothing", distinguishable via errors.Is so a caller (the
// CLI's exit 7, not_found) never has to string-match an error message.
var ErrObservationNotFound = errors.New("promote: observation not found")

// ExplicitPromote promotes the Engram observation named by id through
// Writer.Promote(m, explicit=true) (R-032): the same page-emission,
// addressing, and registration path Writer.Promote already uses for any
// other eligible observation, since explicit overrides Eligible's
// automatic criteria (R-007) rather than bypassing Promote entirely
// (design.md's explicit directive). It reads the observation through src, the
// port every store is an adapter of, so what makes a memory the same piece of
// memory as one of another store (its lineage) reaches the decision, and an
// observation whose lineage memory already has a page is refused with a
// BridgedError, as a sync leaves it alone. It is the one call both the CLI
// promote subcommand and the MCP promote tool make (task 8b.11), so neither
// surface can drift from the other.
func ExplicitPromote(ctx context.Context, w *Writer, src Source, id int64) (Result, error) {
	m, ok, err := src.ByID(strconv.FormatInt(id, 10))
	if err != nil {
		return Result{}, fmt.Errorf("promote: look up observation %d: %w", id, err)
	}
	if !ok {
		return Result{}, fmt.Errorf("%w: %d", ErrObservationNotFound, id)
	}
	return w.Promote(ctx, m, true)
}

// ExplicitPromoteCard promotes the memorycard memory named by id (Writer.Promote). Like ExplicitPromote it is
// explicit: nothing about the memory has to make it eligible. It differs in what it says: a memory, not an
// observation, is looked up, and the promotion of a memory whose page is already at its revision answers
// ActionAlreadyPromoted and writes nothing (pageStyle.rewriteCurrent).
func ExplicitPromoteCard(ctx context.Context, w *Writer, src Source, id int64) (Result, error) {
	m, ok, err := src.ByID(strconv.FormatInt(id, 10))
	if err != nil {
		return Result{}, fmt.Errorf("promote: look up memorycard memory %d: %w", id, err)
	}
	if !ok {
		return Result{}, fmt.Errorf("%w: memorycard memory %d", ErrObservationNotFound, id)
	}
	return w.Promote(ctx, m, true)
}
