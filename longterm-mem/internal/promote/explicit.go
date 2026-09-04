package promote

import (
	"errors"
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
)

// ObservationLookup resolves one Engram observation by id, a function seam
// (matching this package's Deps convention elsewhere) so ExplicitPromote
// never depends on a concrete *engram.Store directly: production callers
// (cmd_promote.go, the MCP promote tool via cmd_mcp.go) wire it to
// (*engram.Store).ObservationByID, and tests wire it to a fake.
type ObservationLookup func(id int64) (engram.Observation, bool, error)

// ErrObservationNotFound is ExplicitPromote's error when lookup reports no
// such observation -- R-032's "rejected with a clear error rather than
// silently doing nothing", distinguishable via errors.Is so a caller (the
// CLI's exit 7, not_found) never has to string-match an error message.
var ErrObservationNotFound = errors.New("promote: observation not found")

// ExplicitPromote promotes the Engram observation named by id through
// Writer.Promote(obs, explicit=true) (R-032): the same page-emission,
// addressing, and registration path Writer.Promote already uses for any
// other eligible observation, since explicit overrides Eligible's
// automatic criteria (R-007) rather than bypassing Promote entirely
// (design.md's explicit directive). It is the one call both the CLI
// promote subcommand and the MCP promote tool make (task 8b.11), so
// neither surface can drift from the other.
func ExplicitPromote(w *Writer, lookup ObservationLookup, id int64) (Result, error) {
	obs, ok, err := lookup(id)
	if err != nil {
		return Result{}, fmt.Errorf("promote: look up observation %d: %w", id, err)
	}
	if !ok {
		return Result{}, fmt.Errorf("%w: %d", ErrObservationNotFound, id)
	}
	return w.Promote(obs, true)
}
