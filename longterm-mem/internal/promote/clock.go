package promote

import (
	"errors"
	"time"
)

// Clock is the port through which promotion reads the time: the day a page is created and updated, the
// instant a promotion is logged and the instant a sync completes. The package owns it and never reads
// the wall clock itself; the composition root hands the Writer a real clock, and a test hands it one
// that says what the test needs.
//
// What a Clock answers is an instant. The zone is not promote's to leave open: every date and time
// promotion writes is in UTC (the vault's pages and log have always been), so promote converts what it
// is told and does not rely on the adapter having done so.
type Clock interface {
	Now() time.Time
}

// errNoClock is what a Writer without a Clock answers: a port the caller forgot to wire is named, rather
// than found by a nil dereference in the first call that needs the time.
var errNoClock = errors.New("promote: the writer has no clock")

// utc reads the clock in UTC, the zone every date and time promotion writes is in.
func utc(clock Clock) time.Time { return clock.Now().UTC() }
