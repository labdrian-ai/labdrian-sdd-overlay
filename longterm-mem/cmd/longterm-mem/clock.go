package main

import "time"

// utcClock is the promote.Clock the commands wire in: the wall clock, in UTC. It is the one place the
// promotion path reads the time from the machine.
type utcClock struct{}

func (utcClock) Now() time.Time { return time.Now().UTC() }
