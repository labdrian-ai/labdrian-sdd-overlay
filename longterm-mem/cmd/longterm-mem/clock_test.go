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

// The binary's commands are composed with the wall clock, in the composition root and nowhere else:
// injecting a clock into the commands must not change what the real program reads, and no command may
// reach for the wall clock behind the injected one.
func TestOnlyTheCompositionRootNamesTheWallClock(t *testing.T) {
	program, _ := sourceFiles(t)
	for _, name := range program {
		uses := usesOfIdent(t, name, "utcClock")
		switch name {
		case "commands.go":
			if len(uses) == 0 {
				t.Error("commands.go no longer composes the commands with utcClock: the binary would run without a clock")
			}
		case "clock.go":
		default:
			for _, at := range uses {
				t.Errorf("%s names utcClock; only the composition root (commands.go) picks the wall clock", at)
			}
		}
	}
}

// fixedClock is the promote.Clock of a test that must not depend on the day it runs: it always tells the
// instant it was given.
type fixedClock struct{ at time.Time }

func (c fixedClock) Now() time.Time { return c.at }

// fixtureInstant is the instant of a fixture page the tests render: a day that is not today, so no test
// depends on the date it runs.
var fixtureInstant = time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
