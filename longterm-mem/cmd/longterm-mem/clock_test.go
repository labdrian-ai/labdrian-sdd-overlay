package main

import (
	"testing"
	"time"
)

// The clock the commands hand to promotion tells the wall-clock time in UTC. promote converts what it is
// told as well, so this is not what keeps a page's date right; it is the adapter saying plainly what
// it is.
func TestUTCClockTellsTheWallClockTimeInUTC(t *testing.T) {
	before := time.Now()
	got := utcClock{}.Now()
	after := time.Now()

	if got.Location() != time.UTC {
		t.Errorf("Now() is in %v, want UTC", got.Location())
	}
	if got.Before(before) || got.After(after) {
		t.Errorf("Now() = %v, want a time between %v and %v: it is the wall clock, not a fixed instant", got, before, after)
	}
}

// fixtureInstant is the instant of a fixture page the tests render: a day that is not today, so no test
// depends on the date it runs.
var fixtureInstant = time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
