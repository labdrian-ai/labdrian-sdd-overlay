package promote

import (
	"context"
	"errors"
	"fmt"
	"strconv"
)

// ErrMemoryNotFound is ExplicitPromoteCard's error when memorycard holds no
// such memory -- R-032's "rejected with a clear error rather than
// silently doing nothing", distinguishable via errors.Is so a caller (the
// CLI's exit 7, not_found) never has to string-match an error message.
var ErrMemoryNotFound = errors.New("memory not found")

// ExplicitPromoteCard promotes the memorycard memory named by id (Writer.Promote). It is explicit: nothing about the
// memory has to make it eligible. The promotion of a memory whose page is already at its revision answers
// ActionAlreadyPromoted and writes nothing (pageStyle.rewriteCurrent).
func ExplicitPromoteCard(ctx context.Context, w *Writer, src Source, id int64) (Result, error) {
	m, ok, err := src.ByID(strconv.FormatInt(id, 10))
	if err != nil {
		return Result{}, fmt.Errorf("promote: look up memorycard memory %d: %w", id, err)
	}
	if !ok {
		return Result{}, fmt.Errorf("%w: memorycard memory %d", ErrMemoryNotFound, id)
	}
	return w.Promote(ctx, m, true)
}
