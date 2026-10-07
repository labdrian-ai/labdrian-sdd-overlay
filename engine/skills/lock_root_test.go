package skills

// What the lock layer locks, and what it says about it: install fails closed when it
// cannot name the directory it would write into, a busy message names the directory
// that was really locked, a raw call never leaves a lock file behind for a registry
// that is not there, and the busy-error walk ends on a cyclic chain.

import (
	"fmt"
	"testing"
)

// ---- the walk along a chain of wrapped errors ends -------------------------------------

// loopErr is an error whose Unwrap returns itself: a hand-written wrapper can do
// that, and nothing in the language forbids it.
type loopErr struct{}

func (e *loopErr) Error() string { return "wraps itself" }
func (e *loopErr) Unwrap() error { return e }

func TestIsBusyEndsOnAnErrorThatWrapsItself(t *testing.T) {
	if isBusy(&loopErr{}) {
		t.Error("isBusy(self-wrapping error) = true, want false")
	}
	if !isBusy(fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", busyErr{"x"}))) {
		t.Error("isBusy did not see a busy error two wraps down")
	}
}
