package promote

import (
	"context"
	"errors"
	"testing"
)

// countingRebuilder is an IndexRebuilder that counts its calls and answers err.
type countingRebuilder struct {
	calls int
	err   error
}

func (r *countingRebuilder) RebuildIndex(context.Context) error {
	r.calls++
	return r.err
}

// readyWriter is a Writer with every port a sync asks for, over fakes.
func readyWriter(t *testing.T) *Writer {
	t.Helper()
	w, _, _ := pagesWriter(t, newMemPages(nil))
	registration(w)
	return w
}

// A function is an IndexRebuilder as it is written.
func TestAFunctionIsAnIndexRebuilder(t *testing.T) {
	boom := errors.New("boom")
	var rebuilder IndexRebuilder = IndexRebuilderFunc(func(context.Context) error { return boom })

	if err := rebuilder.RebuildIndex(t.Context()); err != boom {
		t.Fatalf("RebuildIndex = %v, want the function's answer", err)
	}
}

// Sync asks the rebuilder once when it has one, and reports its failure as the sync's own, wrapped.
func TestSyncAsksTheRebuilderOnceAndWrapsItsFailure(t *testing.T) {
	boom := errors.New("scripts offline")
	rebuilder := &countingRebuilder{err: boom}
	w := readyWriter(t)

	_, err := Sync(t.Context(), Deps{Memory: &fakeMemory{}, Writer: w, Index: rebuilder}, pagesProject)

	if rebuilder.calls != 1 {
		t.Errorf("the rebuilder was asked %d times, want once", rebuilder.calls)
	}
	if !errors.Is(err, boom) {
		t.Errorf("Sync = %v, want the rebuilder's failure wrapped", err)
	}
}

// With no rebuilder wired, or one that holds a nil pointer, Sync promotes and records its completion without a
// rebuild and without panicking.
func TestSyncWithoutARebuilderSkipsTheRebuild(t *testing.T) {
	var typedNil *countingRebuilder
	for name, rebuilder := range map[string]IndexRebuilder{"none": nil, "a nil pointer": typedNil} {
		t.Run(name, func(t *testing.T) {
			w := readyWriter(t)

			if _, err := Sync(t.Context(), Deps{Memory: &fakeMemory{}, Writer: w, Index: rebuilder}, pagesProject); err != nil {
				t.Fatalf("Sync: %v", err)
			}
		})
	}
}
