package main

import (
	"fmt"
	"os"

	"github.com/labdrian-ai/labdrian-sdd-overlay/longterm-mem/internal/engram"
)

// declareDegradedEngram writes one line to stderr when store is being read
// through engram.Open's immutable=1 fallback, so a caller of `sync` or
// `promote` can tell a live corpus from a frozen one.
//
// internal/query already declares this on the READ path, as the
// engram_degraded_snapshot diagnostic in its own result. The same Store
// drives the two commands that WRITE from Engram, and they declared
// nothing at all -- so exactly the surface where a frozen corpus does the
// most damage was the silent one. The fallback serves a snapshot taken
// when the connection was opened and holds it for the life of the process:
// a stale query returns stale answers, while a stale sync promotes pages
// that assert the current state of a corpus it can no longer see, and then
// records the sync as complete against it.
//
// It goes to stderr, not stdout, for the same reason cmdPromote's refusal
// diagnostic does: stdout carries the command's result (the promoted
// address, the sync counts) and a caller parsing it must not find a
// diagnostic mixed in. And it is emitted from the store, not returned as
// an error: a degraded connection is a reported state, never a failure --
// cmdStatus has always treated it that way (exit 0 with the fact on the
// report), and the write commands agree.
func declareDegradedEngram(store *engram.Store, command string) {
	degraded, cause := store.Degraded()
	if !degraded {
		return
	}
	fmt.Fprintf(os.Stderr, "longterm-mem: %s: engram is being read through the immutable=1 fallback, so this run works from the snapshot taken when the connection was opened, not the live database: %s\n", command, cause)
}
