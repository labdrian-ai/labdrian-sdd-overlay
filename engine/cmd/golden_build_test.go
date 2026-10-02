package main

import (
	"errors"
	"strings"
	"testing"
)

// The engine binary the golden tests run is built once for the whole run. A build that fails
// once for a reason that will not repeat (the toolchain's cache busy, a disk hiccup) must not
// fail every case that needs the binary, so the build is tried again; and one that keeps
// failing is reported with what every attempt printed, so a person can tell a hiccup from a
// broken tree.
func TestBuildWithRetry(t *testing.T) {
	t.Run("a build that works is run once", func(t *testing.T) {
		calls := 0
		err := buildWithRetry(3, func(attempt int) error { calls++; return nil })
		if err != nil || calls != 1 {
			t.Errorf("buildWithRetry = %v after %d calls, want nil after 1", err, calls)
		}
	})
	t.Run("a transient failure is absorbed", func(t *testing.T) {
		calls := 0
		err := buildWithRetry(2, func(attempt int) error {
			calls++
			if attempt == 1 {
				return errors.New("go: cache busy")
			}
			return nil
		})
		if err != nil || calls != 2 {
			t.Errorf("buildWithRetry = %v after %d calls, want nil after 2", err, calls)
		}
	})
	t.Run("a failure that repeats names every attempt", func(t *testing.T) {
		calls := 0
		err := buildWithRetry(2, func(attempt int) error {
			calls++
			return errors.New("attempt output " + string(rune('0'+attempt)))
		})
		if calls != 2 {
			t.Errorf("made %d attempts, want 2", calls)
		}
		if err == nil {
			t.Fatal("buildWithRetry = nil, want the failure")
		}
		for _, want := range []string{"failed 2 times", "attempt 1: attempt output 1", "attempt 2: attempt output 2"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not contain %q", err, want)
			}
		}
	})
	t.Run("at least one attempt is made", func(t *testing.T) {
		calls := 0
		_ = buildWithRetry(0, func(int) error { calls++; return nil })
		if calls != 1 {
			t.Errorf("made %d attempts for a count of 0, want 1", calls)
		}
	})
}
