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
